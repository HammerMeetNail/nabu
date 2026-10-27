package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
)

func TestUpdateMeSetsNameAndColor(t *testing.T) {
	svc := auth.NewService(auth.NewMemoryStore())
	svc.SetAuditLogger(nil)
	handler := NewAuthHandler(svc, "nabu_session", false, "http://localhost:8080")
	_, session := quickRegister(svc, "dana@example.com")
	patch := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		r := withUser(httptest.NewRequest(http.MethodPatch, "/api/me", strings.NewReader(body)), svc, session.ID)
		w := httptest.NewRecorder()
		handler.UpdateMe(w, r)
		return w
	}
	type me struct {
		User struct {
			Email       string `json:"email"`
			DisplayName string `json:"displayName"`
			AvatarColor string `json:"avatarColor"`
		} `json:"user"`
	}
	read := func(w *httptest.ResponseRecorder) me {
		t.Helper()
		var body me
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v", w.Body.String(), err)
		}
		return body
	}

	w := patch(`{"displayName":"  Dana O'Connor  ","avatarColor":"#2e86ab"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if got := read(w).User; got.DisplayName != "Dana O'Connor" || got.AvatarColor != "#2E86AB" || got.Email != "dana@example.com" {
		t.Fatalf("answer %+v", got)
	}
	r := withUser(httptest.NewRequest(http.MethodGet, "/api/me", nil), svc, session.ID)
	w = httptest.NewRecorder()
	handler.Me(w, r)
	if got := read(w).User; got.DisplayName != "Dana O'Connor" || got.AvatarColor != "#2E86AB" {
		t.Fatalf("GET /api/me after the change: %+v", got)
	}

	for body, want := range map[string]string{
		`{}`:                                 "nothing to update",
		`{"displayName":"   "}`:              auth.ErrInvalidDisplayName.Error(),
		`{"displayName":"Dana\u0007"}`:       auth.ErrInvalidDisplayName.Error(),
		`{"avatarColor":"blue"}`:             auth.ErrInvalidAvatarColor.Error(),
		`{"email":"other@example.com"}`:      "invalid request body",
		`{"displayName":"A","role":"owner"}`: "invalid request body",
		`not json`:                           "invalid request body",
	} {
		w := patch(body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), want) {
			t.Errorf("%s: %d %s, want 400 %q", body, w.Code, w.Body.String(), want)
		}
	}

	w = httptest.NewRecorder()
	handler.UpdateMe(w, httptest.NewRequest(http.MethodPatch, "/api/me", strings.NewReader(`{"displayName":"X"}`)))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d", w.Code)
	}
}
