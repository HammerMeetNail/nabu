package reminder

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/push"
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
	maxCandidates, maxReminders := s.maxCandidates, s.maxReminders
	if maxCandidates == 0 {
		maxCandidates = 8192
	}
	if maxReminders == 0 {
		maxReminders = 64
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
		// PostgreSQL pages carry dedup dates from the same bounded read, which
		// saves a claim; the claim itself is the dedup.
		if quietAt(c.Preferences, now) || slices.Contains(c.SentDates, date) {
			r.Skipped++
			return nil
		}
		lag := now.Sub(computeRemindTime(day, c.Schedule.SpecificTime, c.LeadMinutes)).Milliseconds()
		r.MaxLagMS = max(r.MaxLagMS, lag)
		queueStarted := time.Now()
		queued, err := s.queueReminder(ctx, c, day, date, now)
		r.DeliveryMS += time.Since(queueStarted).Milliseconds()
		if err != nil {
			r.Failed++
			return ctx.Err()
		}
		if !queued {
			r.Skipped++
			return nil
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
				if r.Candidates >= maxCandidates || r.Reminded+r.Failed >= maxReminders {
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
			if r.Candidates >= maxCandidates || r.Reminded+r.Failed >= maxReminders {
				r.BudgetReached = true
				return ctx.Err()
			}
		}
	}
	s.cursor = candidateCursor{}
	return ctx.Err()
}

// queueReminder claims a due reminder and queues its push job, which a push
// worker delivers once it has checked the recipient may still see the
// activity. It reports false when the reminder was already claimed. The job
// expires when the reminder stops being due, as the tick would no longer
// send it.
func (s *Scheduler) queueReminder(ctx context.Context, c candidate, day time.Time, date string, now time.Time) (bool, error) {
	sch := c.Schedule
	current, err := s.choreStore.GetChore(ctx, sch.ChoreID)
	if errors.Is(err, chore.ErrNotFound) || (err == nil && current.HouseholdID != sch.HouseholdID) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	data := reminderPushData(sch.ChoreID)
	data["householdId"] = sch.HouseholdID
	// A minute at least, so one due at the window's edge still goes out.
	dueUntil := computeScheduleTime(day, sch.SpecificTime).Add(time.Duration(c.LeadMinutes+5) * time.Minute)
	if earliest := now.Add(time.Minute); dueUntil.Before(earliest) {
		dueUntil = earliest
	}
	job := push.Job{
		UserID: c.UserID, HouseholdID: sch.HouseholdID, ChoreID: sch.ChoreID, Type: "schedule_reminder",
		Title:     fmt.Sprintf("%s %s", current.Icon, current.Name),
		Body:      fmt.Sprintf("Due at %s", formatTime(sch.SpecificTime)),
		Data:      data,
		DedupeKey: fmt.Sprintf("reminder:%d:%d:%s", sch.ID, c.UserID, date),
		ExpiresAt: dueUntil,
	}
	queued, err := s.store.ClaimReminder(ctx, sch.ID, c.UserID, date, job)
	if queued && s.wakePushes != nil {
		s.wakePushes()
	}
	return queued, err
}
