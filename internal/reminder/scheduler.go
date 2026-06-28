package reminder

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/schedule"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

const tickInterval = 30 * time.Second

type Scheduler struct {
	store      Store
	schedStore schedule.Store
	schedSvc   *schedule.Service
	notifStore notification.Store
	choreStore chore.Store
	hhStore    household.Store
	userPrefs  userprefs.Store
	pushSender notification.PushSender
	leader     LeaderLock
}

// SetLeaderLock configures an optional single-runner guard. When set, the
// scheduler only runs ticks while this instance holds leadership, so running
// multiple app instances does not produce duplicate reminders. When nil (the
// default, e.g. single-node or in-memory mode) the scheduler always ticks.
func (s *Scheduler) SetLeaderLock(l LeaderLock) {
	s.leader = l
}

func NewScheduler(
	store Store,
	schedStore schedule.Store,
	schedSvc *schedule.Service,
	notifStore notification.Store,
	choreStore chore.Store,
	hhStore household.Store,
	userPrefs userprefs.Store,
	pushSender notification.PushSender,
) *Scheduler {
	return &Scheduler{
		store:      store,
		schedStore: schedStore,
		schedSvc:   schedSvc,
		notifStore: notifStore,
		choreStore: choreStore,
		hhStore:    hhStore,
		userPrefs:  userPrefs,
		pushSender: pushSender,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	log.Printf("reminder: scheduler started (interval=%v)", tickInterval)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	if s.leader != nil {
		defer func() {
			if err := s.leader.Release(context.Background()); err != nil {
				log.Printf("reminder: leader release error: %v", err)
			}
		}()
	}

	var purgeCounter int
	wasLeader := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.acquireLeadership(ctx, &wasLeader) {
				continue
			}
			if err := s.tick(ctx); err != nil {
				log.Printf("reminder: tick error: %v", err)
			}

			purgeCounter++
			if purgeCounter >= 20 { // purge roughly every 10 minutes
				if n, err := s.store.PurgeOldReminders(ctx); err != nil {
					log.Printf("reminder: purge error: %v", err)
				} else if n > 0 {
					log.Printf("reminder: purged %d old reminders", n)
				}
				purgeCounter = 0
			}
		}
	}
}

// acquireLeadership reports whether this instance may run a tick. With no
// leader lock configured it is always true. Otherwise it attempts to acquire/
// hold leadership and logs leadership transitions (via *wasLeader) so the log
// shows exactly one instance taking over.
func (s *Scheduler) acquireLeadership(ctx context.Context, wasLeader *bool) bool {
	if s.leader == nil {
		return true
	}
	ok, err := s.leader.TryAcquire(ctx)
	if err != nil {
		log.Printf("reminder: leader acquire error: %v", err)
		ok = false
	}
	if ok && !*wasLeader {
		log.Printf("reminder: acquired leadership, running ticks")
	} else if !ok && *wasLeader {
		log.Printf("reminder: lost leadership, pausing ticks")
	}
	*wasLeader = ok
	return ok
}

func (s *Scheduler) tick(ctx context.Context) error {
	now := time.Now().UTC()

	schedules, err := s.schedStore.ListActiveWithTime(ctx)
	if err != nil {
		return fmt.Errorf("list active schedules: %w", err)
	}

	activeToday := 0
	sent := 0

	for _, sch := range schedules {
		if !s.schedSvc.IsActiveForDay(sch, now) {
			continue
		}

		activeToday++

		users := s.eligibleUsers(ctx, sch)
		if len(users) == 0 {
			continue
		}

		log.Printf("reminder: checking schedule=%d chore=%d time=%q users=%v",
			sch.ID, sch.ChoreID, sch.SpecificTime, users)

		ch, err := s.choreStore.GetChore(ctx, sch.ChoreID)
		if err != nil {
			log.Printf("reminder: get chore %d: %v", sch.ChoreID, err)
			continue
		}

		for _, userID := range users {
			leadMin := s.getLeadMinutes(ctx, userID, sch.ChoreID)
			loc := s.userLocation(ctx, userID)
			userNow := now.In(loc)
			userToday := userNow.Format("2006-01-02")

			remindAt := computeRemindTime(userNow, sch.SpecificTime, leadMin)

			if userNow.Before(remindAt) {
				log.Printf("reminder: skip schedule=%d chore=%d user=%d now=%s remindAt=%s tz=%s (not yet)",
					sch.ID, sch.ChoreID, userID,
					userNow.Format("15:04"), remindAt.Format("15:04"), loc.String())
				continue
			}

			schedTime := computeScheduleTime(userNow, sch.SpecificTime)
			maxLate := schedTime.Add(time.Duration(leadMin+5) * time.Minute)
			if userNow.After(maxLate) {
				log.Printf("reminder: skip schedule=%d chore=%d user=%d now=%s sched=%s (too late)",
					sch.ID, sch.ChoreID, userID,
					userNow.Format("15:04"), schedTime.Format("15:04"))
				continue
			}

			if inQuiet, _ := s.isInQuietHours(ctx, userID, userNow); inQuiet {
				continue
			}

			alreadySent, err := s.store.HasReminder(ctx, sch.ID, userID, userToday)
			if err != nil {
				log.Printf("reminder: check dedup: %v", err)
				continue
			}
			if alreadySent {
				continue
			}

			if _, err := s.notifStore.GetReminderPreferences(ctx, userID); err != nil {
				continue
			}

			title := fmt.Sprintf("%s %s", ch.Icon, ch.Name)
			body := fmt.Sprintf("Due at %s", formatTime(sch.SpecificTime))

			if err := s.pushSender.SendPushToUser(ctx, userID, title, body); err != nil {
				log.Printf("reminder: push to %d: %v", userID, err)
				continue
			}
			sent++

			log.Printf("reminder: sent to user %d title=%q", userID, title)

			if err := s.store.RecordReminder(ctx, sch.ID, userID, userToday); err != nil {
				log.Printf("reminder: record: %v", err)
			}
		}
	}

	log.Printf("reminder: tick done active=%d sent=%d (total schedules=%d)", activeToday, sent, len(schedules))

	return nil
}

func (s *Scheduler) eligibleUsers(ctx context.Context, sch schedule.ChoreSchedule) []int64 {
	if sch.AssignedUserID != nil {
		if s.userHasScheduleReminderEnabled(ctx, *sch.AssignedUserID) {
			return []int64{*sch.AssignedUserID}
		}
		return nil
	}

	members, err := s.hhStore.GetMembers(ctx, sch.HouseholdID)
	if err != nil {
		log.Printf("reminder: get members for household %d: %v", sch.HouseholdID, err)
		return nil
	}

	var users []int64
	for _, m := range members {
		if !s.userHasScheduleReminderEnabled(ctx, m.UserID) {
			continue
		}
		pref, err := s.store.GetChoreReminderPref(ctx, m.UserID, sch.ChoreID)
		if err != nil {
			continue
		}
		if pref.Enabled {
			users = append(users, m.UserID)
		}
	}
	return users
}

func (s *Scheduler) userHasScheduleReminderEnabled(ctx context.Context, userID int64) bool {
	prefs, err := s.notifStore.GetReminderPreferences(ctx, userID)
	if err != nil {
		return false
	}
	if !prefs.PushEnabled {
		return false
	}
	if len(prefs.EnabledPushTypes) == 0 {
		return true
	}
	for _, t := range prefs.EnabledPushTypes {
		if t == "schedule_reminder" {
			return true
		}
	}
	return false
}

func (s *Scheduler) userLocation(ctx context.Context, userID int64) *time.Location {
	prefs, err := s.notifStore.GetReminderPreferences(ctx, userID)
	if err == nil && prefs.Timezone != "" && prefs.Timezone != "UTC" {
		loc, err := time.LoadLocation(prefs.Timezone)
		if err == nil {
			return loc
		}
	}

	up, err := s.userPrefs.Get(ctx, userID)
	if err == nil && up.Timezone != "" {
		loc, err := time.LoadLocation(up.Timezone)
		if err == nil {
			return loc
		}
	}

	return time.UTC
}

func (s *Scheduler) getLeadMinutes(ctx context.Context, userID, choreID int64) int {
	pref, err := s.store.GetChoreReminderPref(ctx, userID, choreID)
	if err == nil && pref.Enabled {
		return pref.LeadMinutes
	}
	notifPrefs, err := s.notifStore.GetReminderPreferences(ctx, userID)
	if err == nil {
		return notifPrefs.DefaultReminderLeadMinutes
	}
	return 10
}

func (s *Scheduler) isInQuietHours(ctx context.Context, userID int64, now time.Time) (bool, error) {
	prefs, err := s.notifStore.GetReminderPreferences(ctx, userID)
	if err != nil {
		return false, err
	}
	if prefs.QuietHoursStart == "" || prefs.QuietHoursEnd == "" {
		return false, nil
	}

	loc := time.UTC
	if prefs.Timezone != "" {
		if tz, err := time.LoadLocation(prefs.Timezone); err == nil {
			loc = tz
		}
	}

	localNow := now.In(loc)
	return isBetween(localNow, prefs.QuietHoursStart, prefs.QuietHoursEnd), nil
}

func isBetween(t time.Time, start, end string) bool {
	sh, sm, err := parseHM(start)
	if err != nil {
		return false
	}
	eh, em, err := parseHM(end)
	if err != nil {
		return false
	}

	startMin := sh*60 + sm
	endMin := eh*60 + em
	nowMin := t.Hour()*60 + t.Minute()

	if startMin <= endMin {
		return nowMin >= startMin && nowMin < endMin
	}
	return nowMin >= startMin || nowMin < endMin
}

func parseHM(s string) (int, int, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time: %s", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return h, m, nil
}

func computeRemindTime(now time.Time, specificTime string, leadMinutes int) time.Time {
	sh, sm, err := parseHM(specificTime)
	if err != nil {
		return now.Add(-time.Minute)
	}

	remind := time.Date(now.Year(), now.Month(), now.Day(), sh, sm, 0, 0, now.Location())
	return remind.Add(-time.Duration(leadMinutes) * time.Minute)
}

func computeScheduleTime(now time.Time, specificTime string) time.Time {
	sh, sm, err := parseHM(specificTime)
	if err != nil {
		return now
	}
	return time.Date(now.Year(), now.Month(), now.Day(), sh, sm, 0, 0, now.Location())
}

func formatTime(specificTime string) string {
	h, m, err := parseHM(specificTime)
	if err != nil {
		return specificTime
	}

	ampm := "AM"
	if h >= 12 {
		ampm = "PM"
	}
	hour := h % 12
	if hour == 0 {
		hour = 12
	}

	return fmt.Sprintf("%d:%02d %s", hour, m, ampm)
}
