package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/auth"
)

type contextKey string

const userContextKey contextKey = "user"

func Session(authService *auth.Service, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authService == nil {
				next.ServeHTTP(w, r)
				return
			}
			if shouldSkip(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie(cookieName)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
		user, err := authService.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		// Stash the authenticated user as an audit.Actor so downstream
		// service-layer audit calls can attribute events to a principal
		// without each service depending on the HTTP middleware package.
		var hhID int64
		if user.HouseholdID != nil {
			hhID = *user.HouseholdID
		}
		ctx = audit.WithActor(ctx, audit.Actor{
			UserID:      user.ID,
			HouseholdID: hhID,
			Role:        user.Role,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func shouldSkip(path string) bool {
	return strings.HasPrefix(path, "/static/") ||
		strings.HasPrefix(path, "/service-worker.js") ||
		path == "/health" ||
		path == "/ready"
}

func CurrentUser(ctx context.Context) (auth.User, bool) {
	user, ok := ctx.Value(userContextKey).(auth.User)
	return user, ok
}

func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := CurrentUser(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func RequireHousehold(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := CurrentUser(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if user.HouseholdID == nil {
			http.Error(w, "no household", http.StatusBadRequest)
			return
		}
		next(w, r)
	}
}

func WithUser(ctx context.Context, user auth.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}
