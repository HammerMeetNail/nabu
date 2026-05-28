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
	handler := NewStatsHandler(statsService)

	user, session, _ := authService.Register(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"alice@example.com", "password123",
	)
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
