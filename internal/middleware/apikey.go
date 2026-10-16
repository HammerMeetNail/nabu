package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/diagnostics"
)

const apiKeyContextKey contextKey = "api_key_id"

// ErrInvalidAPIKey is what an APIKeyAuthenticator returns for any token that
// should get a 401 (unknown, revoked, malformed, or owner credentials changed).
var ErrInvalidAPIKey = errors.New("invalid API key")

// APIKeyAuthenticator resolves a personal API key to its owner.
type APIKeyAuthenticator func(ctx context.Context, token string) (user auth.User, keyID string, err error)

var logIDPath = regexp.MustCompile(`^/api/logs/[0-9]+$`)

// APIKeyAllowed is the complete surface a personal API key can reach: CRUD on
// activity logs, plus the reads needed to fill one in (chore IDs, member IDs
// for "Done by", and the caller's own profile). Everything else — account,
// household management, exports, keys themselves — needs a browser session.
func APIKeyAllowed(method, path string) bool {
	switch {
	case path == "/api/logs":
		return method == http.MethodPost
	case path == "/api/logs/today", path == "/api/logs/week", path == "/api/logs/month", path == "/api/logs/history":
		return method == http.MethodGet
	case logIDPath.MatchString(path):
		return method == http.MethodPatch || method == http.MethodDelete
	case path == "/api/chores", path == "/api/household", path == "/api/me":
		return method == http.MethodGet
	}
	return false
}

func apiKeyError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Bearer realm="nabu"`)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// APIKey authenticates `Authorization: Bearer nabu_pat_…` requests to /api/.
// It must wrap CSRF and Session: a validated key replaces cookie auth for the
// request (the Cookie header is dropped so a browser session can never mix
// in) and is exempt from double-submit CSRF, since browsers cannot attach an
// Authorization header cross-site. Operator routes keep their own bearer auth.
func APIKey(authenticate APIKeyAuthenticator, logger audit.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" || authenticate == nil || !strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/api/operator/") {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				apiKeyError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			user, keyID, err := authenticate(r.Context(), token)
			if err != nil {
				if errors.Is(err, ErrInvalidAPIKey) {
					apiKeyError(w, http.StatusUnauthorized, "unauthorized")
				} else {
					log.Printf("operation=%q error_class=%s", "api key authentication failed", diagnostics.ErrorClass(err))
					apiKeyError(w, http.StatusInternalServerError, "api key authentication failed")
				}
				return
			}
			if !APIKeyAllowed(r.Method, r.URL.Path) {
				apiKeyError(w, http.StatusForbidden, "API keys can only manage activity logs")
				return
			}
			var householdID int64
			if user.HouseholdID != nil {
				householdID = *user.HouseholdID
			}
			ctx := WithUser(r.Context(), user)
			ctx = context.WithValue(ctx, apiKeyContextKey, keyID)
			ctx = audit.WithActor(ctx, audit.Actor{UserID: user.ID, AuthVersion: user.AuthVersion, HouseholdID: householdID, Role: user.Role})
			if r.Method != http.MethodGet {
				audit.Emit(ctx, logger, "api_key.write", map[string]string{"key_id": keyID, "method": r.Method, "resource": r.URL.Path})
			}
			r = r.WithContext(ctx)
			r.Header = r.Header.Clone()
			r.Header.Del("Cookie")
			next.ServeHTTP(w, r)
		})
	}
}

// APIKeyID returns the personal API key that authenticated this request, or
// "" for browser-session requests.
func APIKeyID(ctx context.Context) string {
	id, _ := ctx.Value(apiKeyContextKey).(string)
	return id
}
