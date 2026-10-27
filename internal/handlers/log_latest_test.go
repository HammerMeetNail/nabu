package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type latestEntry struct {
	ID          int64     `json:"id"`
	ChoreID     int64     `json:"choreId"`
	CompletedAt time.Time `json:"completedAt"`
}

// A log write answers with its chore's latest log, so Home updates that one
// tile instead of reloading every chore's latest log.
func TestLogWritesReportTheirChoresLatestLog(t *testing.T) {
	handler, session, authService := setupLogTest(t)
	now := time.Now().UTC().Truncate(time.Second)

	call := func(method, id, body string) map[string]json.RawMessage {
		t.Helper()
		path := "/api/logs"
		if id != "" {
			path += "/" + id
		}
		req := withUser(httptest.NewRequest(method, path, strings.NewReader(body)), authService, session)
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		switch method {
		case http.MethodPost:
			handler.Create(rec, req)
		case http.MethodPatch:
			handler.Update(rec, req)
		default:
			handler.Delete(rec, req)
		}
		if rec.Code >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		if _, ok := payload["latest"]; !ok {
			t.Fatalf("%s %s: no latest in %s", method, path, rec.Body.String())
		}
		return payload
	}
	latest := func(payload map[string]json.RawMessage) *latestEntry {
		t.Helper()
		var entry *latestEntry
		if err := json.Unmarshal(payload["latest"], &entry); err != nil {
			t.Fatal(err)
		}
		return entry
	}
	create := func(choreID int64, at time.Time) (int64, *latestEntry) {
		t.Helper()
		payload := call(http.MethodPost, "", fmt.Sprintf(`{"choreId":%d,"completedAt":%q}`, choreID, at.Format(time.RFC3339)))
		var created latestEntry
		if err := json.Unmarshal(payload["log"], &created); err != nil {
			t.Fatal(err)
		}
		return created.ID, latest(payload)
	}

	newest, got := create(7, now)
	if got == nil || got.ID != newest {
		t.Fatalf("first log: latest %+v, want log %d", got, newest)
	}
	backdated, got := create(7, now.Add(-2*time.Hour))
	if got == nil || got.ID != newest {
		t.Fatalf("a backdated log is not the latest: got %+v, want log %d", got, newest)
	}
	other, got := create(8, now.Add(time.Hour))
	if got == nil || got.ID != other || got.ChoreID != 8 {
		t.Fatalf("another chore's log reports that chore: %+v", got)
	}

	// Moving the newest log before the backdated one makes that one the latest.
	got = latest(call(http.MethodPatch, fmt.Sprint(newest), fmt.Sprintf(`{"completedAt":%q}`, now.Add(-3*time.Hour).Format(time.RFC3339))))
	if got == nil || got.ID != backdated {
		t.Fatalf("after moving the newest log back: latest %+v, want log %d", got, backdated)
	}

	got = latest(call(http.MethodDelete, fmt.Sprint(backdated), ""))
	if got == nil || got.ID != newest {
		t.Fatalf("after deleting the latest: %+v, want log %d", got, newest)
	}
	if got := latest(call(http.MethodDelete, fmt.Sprint(newest), "")); got != nil {
		t.Fatalf("a chore with no logs left reports null, got %+v", got)
	}
}
