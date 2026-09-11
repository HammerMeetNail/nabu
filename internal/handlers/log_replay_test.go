package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/schedule"
)

type failFirstLogEffects struct {
	schedule.Store
	failed atomic.Bool
}

func (s *failFirstLogEffects) ApplyLogEffects(ctx context.Context, e schedule.LogEffects) (bool, error) {
	if s.failed.CompareAndSwap(false, true) {
		return false, errors.New("synthetic effect failure")
	}
	return s.Store.ApplyLogEffects(ctx, e)
}

func TestLogReplayFinishesEffectsOnce(t *testing.T) {
	for _, failFirst := range []bool{false, true} {
		h, session, authService, schedules, _, hid := setupLogTestWithFollowUp(t)
		// Execute the real notification fanout inline so completion is observable
		// without sleeps or assertions racing detached goroutines.
		h.dispatch = func(f func()) { f() }
		notifs := notification.NewMemoryStore()
		h.notifService = notification.NewService(notifs)
		recipient, _ := quickRegister(authService, "recipient@example.invalid")
		if err := h.householdStore.AddMember(context.Background(), hid, recipient.ID, household.RoleMember); err != nil {
			t.Fatal(err)
		}
		if failFirst {
			h.WithScheduleStore(&failFirstLogEffects{Store: schedules})
		}
		now := time.Now().UTC()
		body := `{"choreId":1,"idempotencyKey":"logical-log","note":"kept","completedAt":"` + now.Format(time.RFC3339) + `","followUpMinutes":60,"followUpTime":"` + now.Add(time.Hour).Format("2006-01-02T15:04") + `"}`
		post := func() *httptest.ResponseRecorder {
			r := withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body)), authService, session)
			w := httptest.NewRecorder()
			h.Create(w, r)
			return w
		}
		if failFirst {
			if w := post(); w.Code != http.StatusInternalServerError {
				t.Fatalf("first failure: %d %s", w.Code, w.Body.String())
			}
			rows, err := schedules.ListByHousehold(context.Background(), hid)
			if err != nil || len(rows) != 0 {
				t.Fatalf("partial follow-up: %v %v", rows, err)
			}
		}
		// Race two identical retries (or initial submits), then retry once more.
		start := make(chan struct{})
		results := make(chan *httptest.ResponseRecorder, 2)
		for i := 0; i < 2; i++ {
			go func() { <-start; results <- post() }()
		}
		close(start)
		for i := 0; i < 2; i++ {
			select {
			case w := <-results:
				if w.Code != http.StatusCreated {
					t.Fatalf("retry: %d %s", w.Code, w.Body.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("retry did not finish")
			}
		}
		before, err := schedules.ListByHousehold(context.Background(), hid)
		if err != nil || len(before) != 1 {
			t.Fatalf("follow-ups: %v %v", before, err)
		}
		if w := post(); w.Code != http.StatusCreated {
			t.Fatalf("replay: %d", w.Code)
		}
		after, err := schedules.ListByHousehold(context.Background(), hid)
		if err != nil || len(after) != 1 || after[0].ID != before[0].ID {
			t.Fatalf("follow-up repeated: %v %v", after, err)
		}
		notifications, err := notifs.ListNotifications(context.Background(), recipient.ID, 50, 0)
		if err != nil || len(notifications) != 1 {
			t.Fatalf("notifications=%d err=%v", len(notifications), err)
		}
		logs, err := h.service.GetTodayLogs(context.Background(), hid)
		if err != nil || len(logs) != 1 {
			t.Fatalf("logs=%d err=%v", len(logs), err)
		}
	}
}
