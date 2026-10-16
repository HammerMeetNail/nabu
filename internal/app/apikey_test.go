package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func bearerRequest(srv http.Handler, method, path, body, token string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

// apiKeyFixture registers a user with a household and seeded chores, then
// issues a personal API key through the Settings endpoint.
func apiKeyFixture(t *testing.T) (srv http.Handler, session string, choreID int64, keyID, token string) {
	t.Helper()
	srv = newTestSrv(t)
	if closer, ok := srv.(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	session = srvRegister(t, srv)
	if rec := srvRequest(srv, http.MethodPost, "/api/household", `{"name":"Home","initials":"H"}`, "tok", session); rec.Code >= 300 {
		t.Fatalf("create household: %d %s", rec.Code, rec.Body.String())
	}
	if rec := srvRequest(srv, http.MethodPost, "/api/chores/seed-defaults", "", "tok", session); rec.Code >= 300 {
		t.Fatalf("seed chores: %d %s", rec.Code, rec.Body.String())
	}
	rec := srvRequest(srv, http.MethodGet, "/api/chores", "", "", session)
	var chores struct {
		Chores []struct {
			ID int64 `json:"id"`
		} `json:"chores"`
	}
	decodeBody(t, rec, &chores)
	if len(chores.Chores) == 0 {
		t.Fatalf("no chores seeded: %s", rec.Body.String())
	}
	rec = srvRequest(srv, http.MethodPost, "/api/api-keys", `{"name":"backfill script"}`, "tok", session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
		Token string `json:"token"`
	}
	decodeBody(t, rec, &created)
	if !strings.HasPrefix(created.Token, "nabu_pat_") {
		t.Fatalf("unexpected token format %q", created.Token)
	}
	return srv, session, chores.Chores[0].ID, created.Key.ID, created.Token
}

func TestAPIKeyLogCRUDWithoutCSRF(t *testing.T) {
	srv, _, choreID, _, token := apiKeyFixture(t)

	if rec := bearerRequest(srv, http.MethodGet, "/api/me", "", token); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/me: %d %s", rec.Code, rec.Body.String())
	}
	if rec := bearerRequest(srv, http.MethodGet, "/api/chores", "", token); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/chores: %d %s", rec.Code, rec.Body.String())
	}

	rec := bearerRequest(srv, http.MethodPost, "/api/logs",
		fmt.Sprintf(`{"choreId":%d,"date":"2026-10-15","hour":3,"completedAt":"2026-10-15T03:10:00Z","note":"from api"}`, choreID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/logs: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("API key responses must not set cookies, got %q", rec.Header().Get("Set-Cookie"))
	}
	var created struct {
		Log struct {
			ID int64 `json:"id"`
		} `json:"log"`
	}
	decodeBody(t, rec, &created)

	rec = bearerRequest(srv, http.MethodGet, "/api/logs/history?before=2026-10-16", "", token)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "from api") {
		t.Fatalf("GET history: %d %s", rec.Code, rec.Body.String())
	}

	path := fmt.Sprintf("/api/logs/%d", created.Log.ID)
	if rec := bearerRequest(srv, http.MethodPatch, path, `{"note":"edited"}`, token); rec.Code != http.StatusOK {
		t.Fatalf("PATCH log: %d %s", rec.Code, rec.Body.String())
	}
	if rec := bearerRequest(srv, http.MethodDelete, path, "", token); rec.Code >= 300 {
		t.Fatalf("DELETE log: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAPIKeyIsLimitedToLogRoutes(t *testing.T) {
	srv, _, _, _, token := apiKeyFixture(t)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/api-keys", ""},
		{http.MethodPost, "/api/api-keys", `{"name":"x"}`},
		{http.MethodPost, "/api/chores", `{"name":"x"}`},
		{http.MethodPatch, "/api/household", `{"name":"x"}`},
		{http.MethodGet, "/api/household/invites", ""},
		{http.MethodGet, "/api/logs/export", ""},
		{http.MethodDelete, "/api/me", `{"confirm":"DELETE"}`},
		{http.MethodPost, "/api/auth/password", `{"newPassword":"something-new"}`},
		{http.MethodGet, "/api/stats/overview", ""},
	} {
		if rec := bearerRequest(srv, tc.method, tc.path, tc.body, token); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: want 403, got %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestAPIKeyHouseholdReadOmitsInvites(t *testing.T) {
	srv, session, _, _, token := apiKeyFixture(t)
	if rec := srvRequest(srv, http.MethodPost, "/api/household/invites", `{}`, "tok", session); rec.Code >= 300 {
		t.Fatalf("create invite: %d %s", rec.Code, rec.Body.String())
	}
	rec := bearerRequest(srv, http.MethodGet, "/api/household", "", token)
	var body struct {
		Members []any `json:"members"`
		Invites []any `json:"invites"`
	}
	decodeBody(t, rec, &body)
	if len(body.Members) == 0 || len(body.Invites) != 0 {
		t.Fatalf("want members and no invites, got %s", rec.Body.String())
	}
}

func TestAPIKeyRejectsBadTokensWithoutSessionFallback(t *testing.T) {
	srv, session, _, _, token := apiKeyFixture(t)
	sessionCookie := &http.Cookie{Name: "nabu_session", Value: session}
	for _, bad := range []string{"", "nabu_pat_short", token + "x", token[:len(token)-1] + "A", "nabu_op_aaaaaaaaaaaaaaaa." + strings.Repeat("a", 43)} {
		rec := bearerRequest(srv, http.MethodGet, "/api/me", "", bad, sessionCookie)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q: want 401, got %d", bad, rec.Code)
		}
	}
}

func TestAPIKeyRevocation(t *testing.T) {
	srv, session, _, keyID, token := apiKeyFixture(t)
	// Revoking requires a browser session with CSRF.
	if rec := srvRequest(srv, http.MethodDelete, "/api/api-keys/"+keyID, "", "", session); rec.Code != http.StatusForbidden {
		t.Fatalf("revoke without CSRF: want 403, got %d", rec.Code)
	}
	if rec := srvRequest(srv, http.MethodDelete, "/api/api-keys/"+keyID, "", "tok", session); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	if rec := bearerRequest(srv, http.MethodGet, "/api/me", "", token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key: want 401, got %d", rec.Code)
	}
	rec := srvRequest(srv, http.MethodGet, "/api/api-keys", "", "", session)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"revokedAt":"`) {
		t.Fatalf("list after revoke: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "nabu_pat_") {
		t.Fatal("key list must never include the token")
	}
}

func TestAPIKeyInvalidatedByPasswordChange(t *testing.T) {
	srv, session, _, _, token := apiKeyFixture(t)
	rec := srvRequest(srv, http.MethodPost, "/api/auth/password",
		`{"current_password":"password123","new_password":"password456"}`, "tok", session)
	if rec.Code >= 300 {
		t.Fatalf("change password: %d %s", rec.Code, rec.Body.String())
	}
	if rec := bearerRequest(srv, http.MethodGet, "/api/me", "", token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("key after password change: want 401, got %d", rec.Code)
	}
}

func TestAPIKeyCannotTouchAnotherHouseholdsLogs(t *testing.T) {
	srv, _, choreID, _, tokenA := apiKeyFixture(t)
	rec := bearerRequest(srv, http.MethodPost, "/api/logs", fmt.Sprintf(`{"choreId":%d}`, choreID), tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create log: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Log struct {
			ID int64 `json:"id"`
		} `json:"log"`
	}
	decodeBody(t, rec, &created)

	rec = srvRequest(srv, http.MethodPost, "/api/auth/register", `{"email":"api_key_other@example.com","password":"password123"}`, "tok", "")
	var sessionB string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "nabu_session" {
			sessionB = c.Value
		}
	}
	if sessionB == "" {
		t.Fatalf("register B: %d %s", rec.Code, rec.Body.String())
	}
	if rec := srvRequest(srv, http.MethodPost, "/api/household", `{"name":"Other","initials":"O"}`, "tok", sessionB); rec.Code >= 300 {
		t.Fatalf("create household B: %d %s", rec.Code, rec.Body.String())
	}
	rec = srvRequest(srv, http.MethodPost, "/api/api-keys", `{"name":"b"}`, "tok", sessionB)
	var keyB struct {
		Token string `json:"token"`
	}
	decodeBody(t, rec, &keyB)

	path := fmt.Sprintf("/api/logs/%d", created.Log.ID)
	if rec := bearerRequest(srv, http.MethodPatch, path, `{"note":"hijack"}`, keyB.Token); rec.Code < 400 {
		t.Errorf("cross-household PATCH: want 4xx, got %d", rec.Code)
	}
	if rec := bearerRequest(srv, http.MethodDelete, path, "", keyB.Token); rec.Code < 400 {
		t.Errorf("cross-household DELETE: want 4xx, got %d", rec.Code)
	}
	if rec := bearerRequest(srv, http.MethodPost, "/api/logs", fmt.Sprintf(`{"choreId":%d}`, choreID), keyB.Token); rec.Code < 400 {
		t.Errorf("cross-household create: want 4xx, got %d", rec.Code)
	}
}
