package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
)

func CSRF(cookieName string) func(http.Handler) http.Handler {
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
					Secure:   requestIsHTTPS(r),
				})
			}
			if isStateChanging(r.Method) && strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("X-CSRF-Token") != token {
				http.Error(w, "csrf token invalid", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
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
