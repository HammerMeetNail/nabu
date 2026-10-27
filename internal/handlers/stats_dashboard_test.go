package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	logsvc "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/stats"
)

type dashboardFixture struct {
	handler     *StatsHandler
	authService *auth.Service
	logs        *logsvc.Service
	alice, bob  string // sessions
	aliceHH     int64
	aliceUser   int64
	aliceChore  int64
	bobChore    int64
}

// Alice and Bob each have a household with one chore; Alice has logged hers.
func setupDashboardTest(t *testing.T) dashboardFixture {
	t.Helper()
	ctx := context.Background()
	authService := auth.NewService(auth.NewMemoryStore())
	authService.SetMailer(mail.NewMemorySender(), "http://localhost:8080")
	authService.SetAuditLogger(nil)
	householdStore := household.NewMemoryStore()
	households := household.NewService(householdStore, authService)
	choreStore := chore.NewMemoryStore()
	logStore := logsvc.NewMemoryStore()
	logs := logsvc.NewService(logStore)
	handler := NewStatsHandler(stats.NewService(logStore, &testChoreStore{s: choreStore}).WithMemberships(householdStore), nil)

	alice, aliceSession := quickRegister(authService, "alice@example.com")
	bob, bobSession := quickRegister(authService, "bob@example.com")
	aliceHH, err := households.CreateHousehold(ctx, "Alice Home", "", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	bobHH, err := households.CreateHousehold(ctx, "Bob Home", "", bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	aliceChore, err := choreStore.CreateChore(ctx, chore.Chore{HouseholdID: aliceHH.ID, Name: "Water Plants", CreatedBy: &alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	bobChore, err := choreStore.CreateChore(ctx, chore.Chore{HouseholdID: bobHH.ID, Name: "bob-private-canary", CreatedBy: &bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logs.LogChore(ctx, aliceHH.ID, alice.ID, aliceChore.ID, nil, "", nil, nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := logs.LogChore(ctx, bobHH.ID, bob.ID, bobChore.ID, nil, "bob-note-canary", nil, nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	return dashboardFixture{handler: handler, authService: authService, logs: logs, alice: aliceSession.ID, bob: bobSession.ID,
		aliceHH: aliceHH.ID, aliceUser: alice.ID, aliceChore: aliceChore.ID, bobChore: bobChore.ID}
}

func dashboardRequest(f dashboardFixture, session string, reads []string, header http.Header) *httptest.ResponseRecorder {
	query := url.Values{"read": reads}
	r := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/dashboard?"+query.Encode(), nil), f.authService, session)
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	f.handler.Dashboard(w, r)
	return w
}

func dashboardResults(t *testing.T, w *httptest.ResponseRecorder) []dashboardResult {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Results []dashboardResult `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid body %q: %v", w.Body.String(), err)
	}
	return body.Results
}

func compactJSON(t *testing.T, data []byte) string {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, bytes.TrimSpace(data)); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	return out.String()
}

func TestStatsDashboardAnswersEachReadLikeItsEndpoint(t *testing.T) {
	f := setupDashboardTest(t)
	type direct struct {
		path    string
		handler http.HandlerFunc
	}
	reads := map[string]direct{
		"overview":                {"/api/stats/overview", f.handler.Overview},
		"chores?period=month":     {"/api/stats/chores?period=month", f.handler.ChoreStats},
		"leaderboard?period=week": {"/api/stats/leaderboard?period=week", f.handler.Leaderboard},
		fmt.Sprintf("chores/%d/time-series?period=daily", f.aliceChore): {
			fmt.Sprintf("/api/stats/chores/%d/time-series?period=daily", f.aliceChore), f.handler.ChoreTimeSeries},
		fmt.Sprintf("chores/%d/summary?period=week", f.aliceChore): {
			fmt.Sprintf("/api/stats/chores/%d/summary?period=week", f.aliceChore), f.handler.ChoreSummary},
	}
	var order []string
	for read := range reads {
		order = append(order, read)
	}
	results := dashboardResults(t, dashboardRequest(f, f.alice, order, nil))
	if len(results) != len(order) {
		t.Fatalf("got %d results for %d reads", len(results), len(order))
	}
	for i, read := range order {
		want := reads[read]
		r := withUser(httptest.NewRequest(http.MethodGet, want.path, nil), f.authService, f.alice)
		r.SetPathValue("id", fmt.Sprint(f.aliceChore))
		w := httptest.NewRecorder()
		want.handler(w, r)
		if results[i].Status != w.Code {
			t.Errorf("%s: status %d, endpoint %d", read, results[i].Status, w.Code)
		}
		if got, endpoint := compactJSON(t, results[i].Data), compactJSON(t, w.Body.Bytes()); got != endpoint {
			t.Errorf("%s: dashboard %s\nendpoint %s", read, got, endpoint)
		}
	}
}

// Each read is authorized on its own: another household's chore is not
// found, and anything outside the Stats reads is refused without failing
// the rest of the page.
func TestStatsDashboardRefusesReadsOutsideStatsAndHousehold(t *testing.T) {
	f := setupDashboardTest(t)
	reads := []string{
		"overview",
		fmt.Sprintf("chores/%d/time-series", f.bobChore),
		fmt.Sprintf("chores/%d/summary", f.bobChore),
		fmt.Sprintf("chores/%d", f.bobChore),
		"dashboard?read=overview",
		"../me",
		"chores/abc",
		"chores/1/../../me",
		"/api/stats/overview",
		"http://evil.example/api/stats/overview",
		"//evil.example/overview",
		"heatmap",
	}
	w := dashboardRequest(f, f.alice, reads, nil)
	results := dashboardResults(t, w)
	want := []int{200, 404, 404, 404, 404, 404, 404, 404, 400, 400, 400, 200}
	for i, status := range want {
		if results[i].Status != status {
			t.Errorf("%q: status %d, want %d (%s)", reads[i], results[i].Status, status, results[i].Data)
		}
	}
	for _, canary := range []string{"bob-private-canary", "bob-note-canary", "Bob Home"} {
		if strings.Contains(w.Body.String(), canary) {
			t.Fatalf("another household's data leaked (%s): %s", canary, w.Body.String())
		}
	}
}

func TestStatsDashboardRevalidatesWithETag(t *testing.T) {
	f := setupDashboardTest(t)
	reads := []string{"overview", fmt.Sprintf("chores/%d/summary?period=week", f.aliceChore)}
	first := dashboardRequest(f, f.alice, reads, nil)
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" {
		t.Fatalf("first: status %d etag %q", first.Code, etag)
	}
	if got := first.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control %q: household data must not be cached by a browser or proxy", got)
	}

	same := dashboardRequest(f, f.alice, reads, http.Header{"If-None-Match": {etag}})
	if same.Code != http.StatusNotModified || same.Body.Len() != 0 {
		t.Fatalf("unchanged: status %d body %q", same.Code, same.Body.String())
	}

	if _, err := f.logs.LogChore(context.Background(), f.aliceHH, f.aliceUser, f.aliceChore, nil, "", nil, nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	changed := dashboardRequest(f, f.alice, reads, http.Header{"If-None-Match": {etag}})
	if changed.Code != http.StatusOK || changed.Header().Get("ETag") == etag {
		t.Fatalf("after a new log: status %d etag %q (was %q)", changed.Code, changed.Header().Get("ETag"), etag)
	}

	// Bob's identical request describes Bob's data, so Alice's ETag never matches it.
	other := dashboardRequest(f, f.bob, []string{"overview"}, http.Header{"If-None-Match": {etag}})
	if other.Code != http.StatusOK {
		t.Fatalf("another account revalidated with Alice's ETag: %d", other.Code)
	}
}

func TestStatsDashboardRejectsMalformedRequests(t *testing.T) {
	f := setupDashboardTest(t)
	if w := dashboardRequest(f, f.alice, nil, nil); w.Code != http.StatusBadRequest {
		t.Errorf("no reads: %d", w.Code)
	}
	many := make([]string, maxDashboardReads+1)
	for i := range many {
		many[i] = "overview"
	}
	if w := dashboardRequest(f, f.alice, many, nil); w.Code != http.StatusBadRequest {
		t.Errorf("%d reads: %d", len(many), w.Code)
	}
	if w := dashboardRequest(f, f.alice, many[:maxDashboardReads], nil); w.Code != http.StatusOK {
		t.Errorf("%d reads: %d", maxDashboardReads, w.Code)
	}

	_, session := quickRegister(f.authService, "nohousehold@example.com")
	if w := dashboardRequest(f, session.ID, []string{"overview"}, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("no household: %d", w.Code)
	}
}

func TestDashboardETagMatching(t *testing.T) {
	etag := `"abc"`
	for header, want := range map[string]bool{
		`"abc"`:      true,
		`W/"abc"`:    true,
		`"x", "abc"`: true,
		`*`:          true,
		`"abd"`:      false,
		``:           false,
		`abc`:        false,
	} {
		if got := etagMatches(header, etag); got != want {
			t.Errorf("etagMatches(%q) = %v, want %v", header, got, want)
		}
	}
}
