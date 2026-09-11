package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/reminder"
	"github.com/HammerMeetNail/nabu/internal/stats"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

type visibilityFailureStore struct {
	chore.Store
}

func (s visibilityFailureStore) ListChores(context.Context, int64) ([]chore.Chore, error) {
	return nil, errors.New("visibility lookup unavailable")
}

type membershipFailureStore struct {
	household.Store
}

func (s membershipFailureStore) GetMembershipForHousehold(context.Context, int64, int64) (string, error) {
	return "", errors.New("membership lookup unavailable")
}

type visibilityStatsAdapter struct{ store chore.Store }

func (s visibilityStatsAdapter) GetChore(ctx context.Context, id int64) (stats.ChoreInfo, error) {
	c, err := s.store.GetChore(ctx, id)
	return stats.ChoreInfo{ID: c.ID, HouseholdID: c.HouseholdID, Visibility: c.Visibility}, err
}

func (s visibilityStatsAdapter) ListChores(ctx context.Context, hid int64) ([]stats.ChoreInfo, error) {
	cs, err := s.store.ListChores(ctx, hid)
	if err != nil {
		return nil, err
	}
	out := make([]stats.ChoreInfo, len(cs))
	for i, c := range cs {
		out[i] = stats.ChoreInfo{ID: c.ID, HouseholdID: c.HouseholdID, Visibility: c.Visibility}
	}
	return out, nil
}

// SEC-5: the data read succeeds; a separate authorization read fails. Every
// collection must reject the response instead of returning content or counts.
func TestVisibilityLookupFailureClosesEveryCollection(t *testing.T) {
	for _, fail := range []string{"chores", "membership"} {
		t.Run(fail, func(t *testing.T) {
			lh, session, authService := setupLogTest(t)
			logs := chorelog.NewMemoryStore()
			lh.service = chorelog.NewService(logs)
			ctx := context.Background()
			cs := chore.NewMemoryStore()
			c, err := cs.CreateChore(ctx, chore.Chore{HouseholdID: 1, Name: "Private", Visibility: chore.VisibilityAdmins})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := lh.service.LogChore(ctx, 1, 1, c.ID, nil, "private-canary", nil, nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
				t.Fatal(err)
			}
			var chores chore.Store = cs
			var memberships household.Store = household.NewMemoryStore()
			if fail == "chores" {
				chores = visibilityFailureStore{Store: cs}
			} else {
				memberships = membershipFailureStore{Store: memberships}
			}
			lh.WithChoreStore(chores, memberships)
			ss := stats.NewService(logs, visibilityStatsAdapter{chores}).WithMemberships(memberships)
			sh := NewStatsHandler(ss, nil)
			prefs := userprefs.NewService(userprefs.NewMemoryStore())
			if _, err := prefs.UpdateStatsWidgets(ctx, 1, []userprefs.StatsWidget{{ID: "private", Type: "total", Title: "private-canary", ChoreIDs: []int64{c.ID}, Metric: "count", Period: "week"}}); err != nil {
				t.Fatal(err)
			}
			ph := NewPreferencesHandler(prefs).WithChoreStore(chores).WithHouseholdStore(memberships)
			rp := reminder.NewMemoryStore()
			if err := rp.UpdateChoreReminderPref(ctx, reminder.ChoreReminderPref{UserID: 1, ChoreID: c.ID, Enabled: true, LeadMinutes: 777}); err != nil {
				t.Fatal(err)
			}
			rh := NewChoreReminderPrefsHandler(rp).WithChoreStore(chores).WithHouseholdStore(memberships)
			cases := []struct {
				name, path, body string
				handler          http.HandlerFunc
			}{
				{"reminder-preferences", "/api/chore-reminder-prefs", "", rh.List},
				{"today", "/api/logs/today", "", lh.Today},
				{"week", "/api/logs/week", "", lh.Week},
				{"month", "/api/logs/month", "", lh.Month},
				{"history", "/api/logs/history", "", lh.History},
				{"search", "/api/logs/history?q=private", "", lh.History},
				{"latest", "/api/logs/latest", "", lh.LatestPerChore},
				{"export", "/api/logs/export", "", lh.Export},
				{"preferences", "/api/preferences", "", ph.Get},
				{"preferences-update", "/api/preferences", `{}`, ph.Update},
				{"leaderboard", "/api/stats/leaderboard", "", sh.Leaderboard},
				{"leaderboard-all", "/api/stats/leaderboard?period=all", "", sh.Leaderboard},
				{"streaks", "/api/stats/streaks", "", sh.Streaks},
				{"heatmap", "/api/stats/heatmap", "", sh.Heatmap},
				{"breakdown", "/api/stats/breakdown", "", sh.Breakdown},
				{"recap", "/api/stats/recap", "", sh.Recap},
				{"overview", "/api/stats/overview", "", sh.Overview},
				{"busy-hours", "/api/stats/busy-hours", "", sh.BusyHours},
				{"top-chores", "/api/stats/top-chores", "", sh.TopChores},
				{"top-chores-all", "/api/stats/top-chores?period=all", "", sh.TopChores},
				{"chore-stats", "/api/stats/chore?choreId=1", "", sh.ChoreStats},
				{"time-series", "/api/stats/chores/1/timeseries", "", sh.ChoreTimeSeries},
				{"summary-all", "/api/stats/chores/1/summary?period=all", "", sh.ChoreSummary},
				{"feeding-gaps", "/api/stats/feeding-gaps?start=" + time.Now().Format("2006-01-02"), "", sh.FeedingGaps},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					method := http.MethodGet
					if tc.body != "" {
						method = http.MethodPatch
					}
					r := withUser(httptest.NewRequest(method, tc.path, strings.NewReader(tc.body)), authService, session)
					r.SetPathValue("id", "1")
					w := httptest.NewRecorder()
					tc.handler(w, r)
					if w.Code != http.StatusInternalServerError {
						t.Fatalf("status %d: %s", w.Code, w.Body.String())
					}
					if strings.Contains(w.Body.String(), "private-canary") || strings.Contains(w.Body.String(), "unavailable") {
						t.Fatalf("unsanitized response: %s", w.Body.String())
					}
				})
			}
		})
	}
}
