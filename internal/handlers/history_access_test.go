package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
)

func TestVisibleHistoryIsFilteredBeforeLimitsAndOlderExistence(t *testing.T) {
	for _, query := range []string{"?q=needle", ""} {
		t.Run(query, func(t *testing.T) {
			h, session, authService := setupLogTest(t)
			ctx := context.Background()
			members := household.NewMemoryStore()
			hh, err := members.CreateHousehold(ctx, "Fixture", "", 2)
			if err != nil {
				t.Fatal(err)
			}
			if err := members.AddMember(ctx, hh.ID, 1, household.RoleMember); err != nil {
				t.Fatal(err)
			}
			chores := chore.NewMemoryStore()
			visible, err := chores.CreateChore(ctx, chore.Chore{HouseholdID: hh.ID, Name: "Visible"})
			if err != nil {
				t.Fatal(err)
			}
			hidden, err := chores.CreateChore(ctx, chore.Chore{HouseholdID: hh.ID, Name: "Hidden", Visibility: chore.VisibilityAdmins})
			if err != nil {
				t.Fatal(err)
			}
			logs := chorelog.NewMemoryStore()
			h.service = chorelog.NewService(logs)
			h.WithChoreStore(chores, members)
			old := today().AddDate(0, 0, -20)
			older, err := logs.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: hh.ID, ChoreID: visible.ID, UserID: 1, CompletedAt: old, Note: "needle visible"})
			if err != nil {
				t.Fatal(err)
			}
			for i := range 110 {
				_, err := logs.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: hh.ID, ChoreID: hidden.ID, UserID: 2, CompletedAt: today().Add(time.Duration(i) * time.Second), Note: "needle hidden"})
				if err != nil {
					t.Fatal(err)
				}
			}
			r := withUser(httptest.NewRequest(http.MethodGet, "/api/logs/history"+query, nil), authService, session)
			w := httptest.NewRecorder()
			h.History(w, r)
			if w.Code != 200 {
				t.Fatalf("history status=%d", w.Code)
			}
			var body struct {
				Logs    []chorelog.ChoreLog `json:"logs"`
				HasMore bool                `json:"hasMore"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if query != "" {
				if len(body.Logs) != 1 || body.Logs[0].ID != older.ID {
					t.Fatal("hidden matches consumed visible search result slots")
				}
			} else if len(body.Logs) != 0 || !body.HasMore {
				t.Fatal("an empty visible week truncated older authorized history")
			}
		})
	}
}
