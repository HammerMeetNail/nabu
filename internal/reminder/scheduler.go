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
)

const tickInterval = 30 * time.Second

type Scheduler struct {
	store      Store
	schedStore schedule.Store
	schedSvc   *schedule.Service
	notifStore notification.Store
	choreStore chore.Store
	hhStore    household.Store
	pushSender notification.PushSender
}

func NewScheduler(
	store Store,
	schedStore schedule.Store,
	schedSvc *schedule.Service,
	notifStore notification.Store,
	choreStore chore.Store,
	hhStore household.Store,
	pushSender notification.PushSender,
) *Scheduler {
	return &Scheduler{
		store:      store,
		schedStore: schedStore,
		schedSvc:   schedSvc,
		notifStore: notifStore,
		choreStore: choreStore,
		hhStore:    hhStore,
		pushSender: pushSender,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	var purgeCounter int

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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

func (s *Scheduler) tick(ctx context.Context) error {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	schedules, err := s.schedStore.ListActiveWithTime(ctx)
	if err != nil {
		return fmt.Errorf("list active schedules: %w", err)
	}

	for _, sch := range schedules {
		if !s.schedSvc.IsActiveForDay(sch, now) {
			continue
		}

		if sch.SpecificTime == "" {
			continue
		}

		users := s.eligibleUsers(ctx, sch)
		if len(users) == 0 {
			continue
		}

		ch, err := s.choreStore.GetChore(ctx, sch.ChoreID)
		if err != nil {
			log.Printf("reminder: get chore %d: %v", sch.ChoreID, err)
			continue
		}

		for _, userID := range users {
			leadMin := s.getLeadMinutes(ctx, userID, sch.ChoreID)
			remindAt := computeRemindTime(now, sch.SpecificTime, leadMin)

			if now.Before(remindAt) {
				continue
			}

			if inQuiet, _ := s.isInQuietHours(ctx, userID, now); inQuiet {
				continue
			}

			alreadySent, err := s.store.HasReminder(ctx, sch.ID, userID, today)
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

			if err := s.store.RecordReminder(ctx, sch.ID, userID, today); err != nil {
				log.Printf("reminder: record: %v", err)
			}
		}
	}

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

	remind := time.Date(now.Year(), now.Month(), now.Day(), sh, sm, 0, 0, time.UTC)
	return remind.Add(-time.Duration(leadMinutes) * time.Minute)
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
