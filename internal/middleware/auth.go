package middleware

import (
	"context"
	"net/http"
)

type contextKey string

const (
	userContextKey contextKey = "user"
)

func Session(sessionService SessionService, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			user, err := sessionService.Authenticate(r.Context(), cookie.Value)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type SessionService interface {
	Authenticate(ctx context.Context, sessionToken string) (UserInfo, error)
}

type UserInfo struct {
	ID    int64
	Email string
}

func CurrentUser(ctx context.Context) (UserInfo, bool) {
	user, ok := ctx.Value(userContextKey).(UserInfo)
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
