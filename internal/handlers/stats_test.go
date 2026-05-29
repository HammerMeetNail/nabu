package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dave/choresy/internal/auth"
	"github.com/dave/choresy/internal/chore"
	"github.com/dave/choresy/internal/household"
	logsvc "github.com/dave/choresy/internal/log"
	"github.com/dave/choresy/internal/mail"
	"github.com/dave/choresy/internal/stats"
	"github.com/dave/choresy/internal/userprefs"
)

func setupStatsTest(t *testing.T) (*StatsHandler, string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	logStore := logsvc.NewMemoryStore()
	choreStore := chore.NewMemoryStore()
	statsService := stats.NewService(logStore, &testChoreStore{s: choreStore})
	handler := NewStatsHandler(statsService, nil)

	user, session := quickRegister(authService, "alice@example.com")
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", user.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	return handler, session.ID, authService
}

func TestStatsLeaderboard(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/leaderboard?period=week", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Leaderboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"leaderboard"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestStatsStreaks(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/streaks", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Streaks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"streaks"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestStatsOverview(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/overview", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Overview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"overview"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestStatsHeatmap(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/heatmap?start=2026-01-01&end=2026-04-01", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Heatmap(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"heatmap"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestStatsBreakdown(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/breakdown", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Breakdown(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"breakdown"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestStatsRecap(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/recap", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Recap(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"recap"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestStatsBusyHours(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/busy-hours", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.BusyHours(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"busyHours"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestStatsChoreStats(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/chores", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.ChoreStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"choreStats"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestStatsChoreStatsByID_NotFound(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/chores/9999", nil), authService, sessionID)
	req.SetPathValue("id", "9999")
	rec := httptest.NewRecorder()

	handler.ChoreStatsByID(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestStatsLeaderboardMonthly(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/leaderboard?period=month", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Leaderboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsNoHousehold(t *testing.T) {
	// Build a handler with an unauthenticated user (no household)
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	logStore := logsvc.NewMemoryStore()
	choreStore := chore.NewMemoryStore()
	statsService := stats.NewService(logStore, &testChoreStore{s: choreStore})
	handler := NewStatsHandler(statsService, nil)

	// Register without creating household
	_, session := quickRegister(authService, "nohousehold@example.com")

	for _, name := range []string{"leaderboard", "streaks", "heatmap", "breakdown", "recap", "busyhours", "chorestats"} {
		var rec *httptest.ResponseRecorder
		var req *http.Request
		switch name {
		case "leaderboard":
			req = withUser(httptest.NewRequest(http.MethodGet, "/api/stats/leaderboard", nil), authService, session.ID)
			rec = httptest.NewRecorder()
			handler.Leaderboard(rec, req)
		case "streaks":
			req = withUser(httptest.NewRequest(http.MethodGet, "/api/stats/streaks", nil), authService, session.ID)
			rec = httptest.NewRecorder()
			handler.Streaks(rec, req)
		case "heatmap":
			req = withUser(httptest.NewRequest(http.MethodGet, "/api/stats/heatmap", nil), authService, session.ID)
			rec = httptest.NewRecorder()
			handler.Heatmap(rec, req)
		case "breakdown":
			req = withUser(httptest.NewRequest(http.MethodGet, "/api/stats/breakdown", nil), authService, session.ID)
			rec = httptest.NewRecorder()
			handler.Breakdown(rec, req)
		case "recap":
			req = withUser(httptest.NewRequest(http.MethodGet, "/api/stats/recap", nil), authService, session.ID)
			rec = httptest.NewRecorder()
			handler.Recap(rec, req)
		case "busyhours":
			req = withUser(httptest.NewRequest(http.MethodGet, "/api/stats/busy-hours", nil), authService, session.ID)
			rec = httptest.NewRecorder()
			handler.BusyHours(rec, req)
		case "chorestats":
			req = withUser(httptest.NewRequest(http.MethodGet, "/api/stats/chores", nil), authService, session.ID)
			rec = httptest.NewRecorder()
			handler.ChoreStats(rec, req)
		}
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without household: status = %d, want 401", name, rec.Code)
		}
	}
}

type testChoreStore struct {
	s chore.Store
}

func (a *testChoreStore) GetChore(ctx context.Context, id int64) (stats.ChoreInfo, error) {
	c, err := a.s.GetChore(ctx, id)
	if err != nil {
		return stats.ChoreInfo{}, err
	}
	return stats.ChoreInfo{ID: c.ID, Name: c.Name, Icon: c.Icon, Color: c.Color, Category: c.Category, HasVolumeML: c.HasVolumeML, IndicatorLabels: c.IndicatorLabels}, nil
}

func (a *testChoreStore) ListChores(ctx context.Context, householdID int64) ([]stats.ChoreInfo, error) {
	chores, err := a.s.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}
	result := make([]stats.ChoreInfo, len(chores))
	for i, c := range chores {
		result[i] = stats.ChoreInfo{ID: c.ID, Name: c.Name, Icon: c.Icon, Color: c.Color, Category: c.Category, HasVolumeML: c.HasVolumeML, IndicatorLabels: c.IndicatorLabels}
	}
	return result, nil
}

// setupStatsTestWithPrefs sets up a stats handler with a real userprefs store so
// userLocation() exercises the non-nil branch.
func setupStatsTestWithPrefs(t *testing.T) (*StatsHandler, string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	logStore := logsvc.NewMemoryStore()
	choreStore := chore.NewMemoryStore()
	statsService := stats.NewService(logStore, &testChoreStore{s: choreStore})

	prefsStore := userprefs.NewMemoryStore()
	handler := NewStatsHandler(statsService, prefsStore)

	user, session := quickRegister(authService, "prefuser@example.com")
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"Pref Home", user.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	return handler, session.ID, authService
}

func TestStatsLeaderboardDefaultPeriod(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	// No ?period param → should default to "week"
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/leaderboard", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.Leaderboard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsUserLocationWithPrefsStore(t *testing.T) {
	handler, sessionID, authService := setupStatsTestWithPrefs(t)
	// userLocation() will be called; prefs have no timezone set → returns UTC
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/leaderboard?period=month", nil),
		authService, sessionID)
	rec := httptest.NewRecorder()
	handler.Leaderboard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsBreakdownWithDates(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet,
		"/api/stats/breakdown?start=2026-01-01&end=2026-04-01", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.Breakdown(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsBusyHoursWithDates(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet,
		"/api/stats/busy-hours?start=2026-01-01&end=2026-03-01", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.BusyHours(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsOverviewNoHousehold(t *testing.T) {
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	logStore := logsvc.NewMemoryStore()
	choreStore := chore.NewMemoryStore()
	statsService := stats.NewService(logStore, &testChoreStore{s: choreStore})
	handler := NewStatsHandler(statsService, nil)

	_, session := quickRegister(authService, "nohh2@example.com")
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/overview", nil),
		authService, session.ID)
	rec := httptest.NewRecorder()
	handler.Overview(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsChoreStatsByIDNoHousehold(t *testing.T) {
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	logStore := logsvc.NewMemoryStore()
	choreStore := chore.NewMemoryStore()
	statsService := stats.NewService(logStore, &testChoreStore{s: choreStore})
	handler := NewStatsHandler(statsService, nil)

	_, session := quickRegister(authService, "nohh3@example.com")
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/chores/1", nil),
		authService, session.ID)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.ChoreStatsByID(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsChoreStatsByIDInvalidID(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/chores/abc", nil),
		authService, sessionID)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()
	handler.ChoreStatsByID(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsChoreStatsByIDNotFound(t *testing.T) {
	handler, sessionID, authService := setupStatsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/chores/9999", nil),
		authService, sessionID)
	req.SetPathValue("id", "9999")
	rec := httptest.NewRecorder()
	handler.ChoreStatsByID(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

// TestStatsUserLocationValidTimezone exercises the full userLocation path:
// prefsStore != nil, user authenticated, timezone set to valid IANA name.
func TestStatsUserLocationValidTimezone(t *testing.T) {
	handler, sessionID, authService := setupStatsTestWithPrefs(t)

	// The session user was created in setupStatsTestWithPrefs; find the user ID
	// to call Upsert.  We do it via the preferences handler which shares the same store.
	prefsStore := userprefs.NewMemoryStore()
	// Re-create a handler with the same stores but with a prefsStore we control.
	authStore2 := auth.NewMemoryStore()
	authService2 := auth.NewService(authStore2)
	mailer2 := mail.NewMemorySender()
	authService2.SetMailer(mailer2, "http://localhost:8080")
	authService2.SetAuditLogger(nil)
	householdStore2 := household.NewMemoryStore()
	householdService2 := household.NewService(householdStore2, authService2)
	logStore2 := logsvc.NewMemoryStore()
	choreStore2 := chore.NewMemoryStore()
	statsService2 := stats.NewService(logStore2, &testChoreStore{s: choreStore2})
	handler2 := NewStatsHandler(statsService2, prefsStore)
	user2, session2 := quickRegister(authService2, "tz@example.com")
	if _, err := householdService2.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"TZ Home", user2.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	// Seed a valid timezone so userLocation returns a real *time.Location.
	if err := prefsStore.Upsert(
		context.Background(), user2.ID,
		userprefs.Preferences{Timezone: "America/New_York"},
	); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	_ = handler // original handler only used for reference; use handler2 below
	_ = sessionID
	_ = authService
	req := withUser(
		httptest.NewRequest(http.MethodGet, "/api/stats/leaderboard?period=month", nil),
		authService2, session2.ID,
	)
	rec := httptest.NewRecorder()
	handler2.Leaderboard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

// TestStatsUserLocationInvalidTimezone covers the LoadLocation error branch.
func TestStatsUserLocationInvalidTimezone(t *testing.T) {
	prefsStore := userprefs.NewMemoryStore()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)
	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)
	logStore := logsvc.NewMemoryStore()
	choreStore := chore.NewMemoryStore()
	statsService := stats.NewService(logStore, &testChoreStore{s: choreStore})
	handler := NewStatsHandler(statsService, prefsStore)
	user, session := quickRegister(authService, "badtz@example.com")
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"Bad TZ Home", user.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	// Set an invalid timezone; LoadLocation should fail → returns UTC.
	if err := prefsStore.Upsert(
		context.Background(), user.ID,
		userprefs.Preferences{Timezone: "Not/A/Real/Timezone"},
	); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	req := withUser(
		httptest.NewRequest(http.MethodGet, "/api/stats/leaderboard?period=month", nil),
		authService, session.ID,
	)
	rec := httptest.NewRecorder()
	handler.Leaderboard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
