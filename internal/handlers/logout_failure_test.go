package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
)

type failedLogoutStore struct{ auth.Store }

func (s failedLogoutStore) DeleteSession(context.Context, string) error {
	return errors.New("injected session deletion failure")
}

func TestLogoutFailureKeepsSessionForRetry(t *testing.T) {
	store := auth.NewMemoryStore()
	service := auth.NewService(failedLogoutStore{store})
	_, session := quickRegister(service, "logout-failure@example.com")
	h := NewAuthHandler(service, "nabu_session", false, "http://localhost")
	r := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: "nabu_session", Value: session.ID})
	w := httptest.NewRecorder()
	h.Logout(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("logout reported success: %d %s", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("failed logout discarded the cookie needed to retry")
	}
	if _, err := service.Authenticate(r.Context(), session.ID); err != nil {
		t.Fatal("fixture session not retained", err)
	}
}
