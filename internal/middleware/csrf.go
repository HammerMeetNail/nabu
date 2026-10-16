package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

// CSRF implements double-submit-cookie protection. The secure argument should
// reflect the SERVER_SECURE config (the deployment is served over HTTPS), which
// drives the cookie's Secure flag — exactly as the session cookie does. The
// flag is intentionally NOT derived from the client-supplied X-Forwarded-Proto
// header, which any client can spoof.
func CSRF(cookieName string, secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A request already authenticated by a personal API key carries no
			// ambient credentials (APIKey drops the Cookie header), so there is
			// nothing for a cross-site request to forge.
			if APIKeyID(r.Context()) != "" {
				next.ServeHTTP(w, r)
				return
			}
			token := ""
			if cookie, err := r.Cookie(cookieName); err == nil {
				token = cookie.Value
			}
			if token == "" {
				token = randomCSRFToken()
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    token,
					Path:     "/",
					SameSite: http.SameSiteLaxMode,
					Secure:   secure || r.TLS != nil,
				})
			}
			if isStateChanging(r.Method) && strings.HasPrefix(r.URL.Path, "/api/") && !isCSRFExempt(r.URL.Path) {
				headerToken := r.Header.Get("X-CSRF-Token")
				if subtle.ConstantTimeCompare([]byte(headerToken), []byte(token)) != 1 {
					http.Error(w, "csrf token invalid", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isCSRFExempt lists endpoints intentionally exempt from double-submit CSRF.
//
//   - /api/reminders/snooze is invoked by the service worker (which has no
//     access to the CSRF cookie/header) and is safe: it requires a valid
//     session cookie — which SameSite=Lax already prevents cross-site POSTs
//     from carrying — and only mutates the caller's own data behind an
//     ownership check.
//   - /api/auth/apple/web/callback receives Sign in with Apple's form_post
//     response, a cross-site POST from appleid.apple.com that cannot carry
//     our header token. The handler's OAuth state cookie is the
//     double-submit protection there, and the posted identity token is
//     signature- and nonce-verified before any session is created.
func isCSRFExempt(path string) bool {
	return path == "/api/reminders/snooze" || path == "/api/auth/apple/web/callback"
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func randomCSRFToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
