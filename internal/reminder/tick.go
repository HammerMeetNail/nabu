package reminder

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/schedule"
)

type TickReport struct {
	Event            string `json:"event"`
	DurationMS       int64  `json:"duration_ms"`
	Candidates       int    `json:"candidates"`
	CandidateQueries int    `json:"candidate_queries"`
	PoolQueries      uint64 `json:"pool_queries"`
	Due              int    `json:"due"`
	Reminded         int    `json:"reminded"`
	Skipped          int    `json:"skipped"`
	Failed           int    `json:"failed"`
	DeliveryMS       int64  `json:"delivery_ms"`
	MaxLagMS         int64  `json:"max_lag_ms"`
	BudgetReached    bool   `json:"budget_reached"`
}

// Query count covers this pool, including concurrent notification delivery.
func (s *Scheduler) SetQueryCounter(count func() uint64) { s.queryCount = count }

func (s *Scheduler) tick(parent context.Context) (err error) {
	started := time.Now()
	timeout := s.tickTimeout
	if timeout == 0 {
		timeout = 25 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	r := TickReport{Event: "reminder.tick"}
	var initialQueries uint64
	if s.queryCount != nil {
		initialQueries = s.queryCount()
	}
	defer func() {
		r.DurationMS = time.Since(started).Milliseconds()
		if s.queryCount != nil {
			r.PoolQueries = s.queryCount() - initialQueries
		}
		if ctx.Err() != nil {
			r.BudgetReached = true
		}
		if s.observe != nil {
			s.observe(r)
		}
		encoded, _ := json.Marshal(r)
		log.Print(string(encoded))
	}()
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now()
	}
	maxCandidates, maxDeliveries := s.maxCandidates, s.maxDeliveries
	if maxCandidates == 0 {
		maxCandidates = 8192
	}
	if maxDeliveries == 0 {
		maxDeliveries = 64
	}
	process := func(c candidate) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.cursor = candidateCursor{c.Schedule.ID, c.UserID}
		r.Candidates++
		day, due := s.dueDate(c.Schedule, now.In(reminderLocation(c.Preferences.Timezone, c.UserTimezone)), c.LeadMinutes)
		if !due {
			r.Skipped++
			return nil
		}
		r.Due++
		date := day.Format("2006-01-02")
		if quietAt(c.Preferences, now) || slices.Contains(c.SentDates, date) {
			r.Skipped++
			return nil
		}
		// Memory/test stores retain their explicit dedup path. PostgreSQL pages
		// already carry dedup dates from the same bounded read.
		if _, bulk := s.store.(candidateReader); !bulk {
			already, err := s.store.HasReminder(ctx, c.Schedule.ID, c.UserID, date)
			if err != nil {
				r.Failed++
				return nil
			}
			if already {
				r.Skipped++
				return nil
			}
		}
		lag := now.Sub(computeRemindTime(day, c.Schedule.SpecificTime, c.LeadMinutes)).Milliseconds()
		r.MaxLagMS = max(r.MaxLagMS, lag)
		deliveryStarted := time.Now()
		err := s.deliverCandidate(ctx, c)
		r.DeliveryMS += time.Since(deliveryStarted).Milliseconds()
		if err != nil {
			r.Failed++
			return ctx.Err()
		}
		if err = s.store.RecordReminder(ctx, c.Schedule.ID, c.UserID, date); err != nil {
			r.Failed++
			return err
		}
		r.Reminded++
		return nil
	}
	if reader, ok := s.store.(candidateReader); ok {
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			pageSize := min(256, maxCandidates-r.Candidates)
			page, err := reader.CandidatePage(ctx, s.cursor, pageSize, now)
			r.CandidateQueries++
			if err != nil {
				return err
			}
			for _, c := range page {
				if err := process(c); err != nil {
					return err
				}
				if r.Candidates >= maxCandidates || r.Reminded+r.Failed >= maxDeliveries {
					r.BudgetReached = true
					return ctx.Err()
				}
			}
			if len(page) < pageSize {
				s.cursor = candidateCursor{}
				return ctx.Err()
			}
		}
	}
	// The no-database mode uses its goroutine-safe stores and the same due and
	// delivery logic.
	schedules, err := s.schedStore.ListActiveWithTime(ctx)
	if err != nil {
		return err
	}
	slices.SortFunc(schedules, func(a, b schedule.ChoreSchedule) int { return cmp.Compare(a.ID, b.ID) })
	for _, sch := range schedules {
		if err := ctx.Err(); err != nil {
			return err
		}
		ch, err := s.choreStore.GetChore(ctx, sch.ChoreID)
		if err != nil {
			continue
		}
		users := s.eligibleUsers(ctx, sch, ch)
		slices.Sort(users)
		for _, uid := range users {
			if sch.ID < s.cursor.ScheduleID || (sch.ID == s.cursor.ScheduleID && uid <= s.cursor.UserID) {
				continue
			}
			pref, err := s.notifStore.GetReminderPreferences(ctx, uid)
			if err != nil {
				continue
			}
			up, _ := s.userPrefs.Get(ctx, uid)
			if err := process(candidate{Schedule: sch, UserID: uid, LeadMinutes: s.getLeadMinutes(ctx, uid, sch.ChoreID), Preferences: pref, UserTimezone: up.Timezone}); err != nil {
				return err
			}
			if r.Candidates >= maxCandidates || r.Reminded+r.Failed >= maxDeliveries {
				r.BudgetReached = true
				return ctx.Err()
			}
		}
	}
	s.cursor = candidateCursor{}
	return ctx.Err()
}

func (s *Scheduler) deliverCandidate(ctx context.Context, c candidate) error {
	deliveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	sch := c.Schedule
	return household.WithMember(deliveryCtx, s.hhStore, c.UserID, sch.HouseholdID, func(role string) error {
		current, err := s.choreStore.GetChore(deliveryCtx, sch.ChoreID)
		if err != nil || current.HouseholdID != sch.HouseholdID {
			return chore.ErrNotFound
		}
		if current.Visibility == chore.VisibilityAdmins && role != household.RoleOwner && role != household.RoleAdmin {
			return household.ErrNotAuthorized
		}
		title := fmt.Sprintf("%s %s", current.Icon, current.Name)
		body := fmt.Sprintf("Due at %s", formatTime(sch.SpecificTime))
		if ds, ok := s.pushSender.(interface {
			SendPushToUserWithData(context.Context, int64, string, string, map[string]any) error
		}); ok {
			data := reminderPushData(sch.ChoreID)
			data["householdId"] = sch.HouseholdID
			return ds.SendPushToUserWithData(deliveryCtx, c.UserID, title, body, data)
		}
		return s.pushSender.SendPushToUser(deliveryCtx, c.UserID, title, body)
	})
}
