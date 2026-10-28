package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// An invite link's preview needs no session, keeps the code out of the logs,
// and spends the join's per-IP budget, so it can't be used to sweep codes.
func TestInvitePreviewRoute(t *testing.T) {
	t.Setenv("RATE_LIMIT_JOIN_MAX", "3")
	srv := newTestSrv(t)
	if closer, ok := srv.(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	ann := registerAs(t, srv, "ann@example.com")
	if rec := srvRequest(srv, http.MethodPost, "/api/household", `{"name":"Ann's home"}`, "tok", ann); rec.Code >= 300 {
		t.Fatalf("create household: %d %s", rec.Code, rec.Body.String())
	}
	var hh struct {
		Household struct {
			InviteCode string `json:"inviteCode"`
		} `json:"household"`
	}
	_ = json.Unmarshal(srvRequest(srv, http.MethodGet, "/api/household", "", "", ann).Body.Bytes(), &hh)
	code := hh.Household.InviteCode
	if code == "" {
		t.Fatal("no invite code")
	}

	var logs bytes.Buffer
	restore := captureJSONLogs(t, &logs)
	rec := srvRequest(srv, http.MethodGet, "/api/household/join/preview?code="+code, "", "", "")
	restore()
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Ann's home"`) {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if !strings.Contains(logs.String(), `"path":"/api/household/join/preview"`) || strings.Contains(logs.String(), code) {
		t.Fatalf("request log must record the path without the code:\n%s", logs.String())
	}

	if rec := srvRequest(srv, http.MethodGet, "/api/household/join/preview?code=NOPE234567", "", "", ""); rec.Code != http.StatusNotFound ||
		!strings.Contains(rec.Body.String(), "isn't valid any more") {
		t.Fatalf("unknown code: %d %s", rec.Code, rec.Body.String())
	}
	if rec := srvRequest(srv, http.MethodPost, "/api/household/join/preview?code="+code, "", "tok", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d, want 405", rec.Code)
	}
	// Three requests spent the budget of three; the join shares it.
	rec = srvRequest(srv, http.MethodPost, "/api/household/join", `{"inviteCode":"`+code+`"}`, "tok", registerAs(t, srv, "ben@example.com"))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("join after three previews: %d (Retry-After %q), want 429", rec.Code, rec.Header().Get("Retry-After"))
	}
}

// captureJSONLogs sends the server's log lines to w as JSON, the way
// cmd/server writes them, until the returned function (or cleanup) restores
// slog's default and the log package's writer and flags.
func captureJSONLogs(t *testing.T, w io.Writer) func() {
	t.Helper()
	previous, writer, flags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewJSONHandler(w, nil)))
	restore := func() {
		slog.SetDefault(previous)
		log.SetOutput(writer)
		log.SetFlags(flags)
	}
	t.Cleanup(restore)
	return restore
}
