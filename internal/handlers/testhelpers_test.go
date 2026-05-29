package handlers

import (
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/dave/choresy/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

// testPasswordHash is the bcrypt hash of "password123", computed once at init
// to avoid expensive bcrypt hashing in every handler test.
var testPasswordHash string

func init() {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		log.Fatalf("failed to pre-compute test bcrypt hash: %v", err)
	}
	testPasswordHash = string(hash)
}

// quickRegister creates a user and session using a pre-computed bcrypt hash,
// bypassing the expensive password hashing step.
func quickRegister(authService *auth.Service, email string) (auth.User, auth.Session) {
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	user, session, err := authService.RegisterWithHash(ctx, email, testPasswordHash)
	if err != nil {
		panic("quickRegister: " + err.Error())
	}
	return user, session
}
