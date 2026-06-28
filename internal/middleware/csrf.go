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
			if isStateChanging(r.Method) && strings.HasPrefix(r.URL.Path, "/api/") {
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
