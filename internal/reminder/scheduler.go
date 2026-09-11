package reminder

import (
	"context"
	"fmt"
	"log"

	"github.com/HammerMeetNail/nabu/internal/diagnostics"
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
	store                        Store
	schedStore                   schedule.Store
	schedSvc                     *schedule.Service
	notifStore                   notification.Store
	choreStore                   chore.Store
	hhStore                      household.Store
	userPrefs                    userprefs.Store
	pushSender                   notification.PushSender
	leader                       LeaderLock
	now                          func() time.Time
	cursor                       candidateCursor
	queryCount                   func() uint64
	observe                      func(TickReport)
	tickTimeout                  time.Duration
	maxCandidates, maxDeliveries int
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
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := s.leader.Release(cleanupCtx); err != nil {
				log.Printf("reminder: leader release failed")
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
				log.Printf("reminder: tick failed class=%s correlation_id=%s", diagnostics.ErrorClass(err), diagnostics.RequestID())
			}

			purgeCounter++
			if purgeCounter >= 20 { // purge roughly every 10 minutes
				purgeCtx, purgeCancel := context.WithTimeout(ctx, 5*time.Second)
				n, err := s.store.PurgeOldReminders(purgeCtx)
				purgeCancel()
				if err != nil {
					log.Printf("reminder: purge failed class=%s correlation_id=%s", diagnostics.ErrorClass(err), diagnostics.RequestID())
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
	acquireCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ok, err := s.leader.TryAcquire(acquireCtx)
	if err != nil {
		log.Printf("reminder: leader acquire failed class=%s correlation_id=%s", diagnostics.ErrorClass(err), diagnostics.RequestID())
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

func (s *Scheduler) eligibleUsers(ctx context.Context, sch schedule.ChoreSchedule, c chore.Chore) []int64 {
	isPrivate := c.Visibility == chore.VisibilityAdmins
	// For private chores, verify assigned user is still admin
	if sch.AssignedUserID != nil {
		uid := *sch.AssignedUserID
		role, err := s.hhStore.GetMembershipForHousehold(ctx, uid, sch.HouseholdID)
		if err != nil || (isPrivate && role != household.RoleOwner && role != household.RoleAdmin) {
			return nil
		}
		if s.userHasScheduleReminderEnabled(ctx, uid) {
			return []int64{uid}
		}
		return nil
	}

	members, err := s.hhStore.GetMembers(ctx, sch.HouseholdID)
	if err != nil {
		log.Printf("reminder: membership read failed class=%s correlation_id=%s", diagnostics.ErrorClass(err), diagnostics.RequestID())
		return nil
	}

	var users []int64
	for _, m := range members {
		if isPrivate && m.Role != household.RoleOwner && m.Role != household.RoleAdmin {
			continue
		}
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
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("invalid time")
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

// reminderPushData is the extra payload a schedule reminder carries so
// clients can attach actions: the PWA service worker reads choreId/type to
// offer "Log now"/"Snooze 30m" buttons, and the APNs sender maps "category"
// to aps.category so the iOS app's NABU_REMINDER action category attaches.
func reminderPushData(choreID int64) map[string]any {
	return map[string]any{
		"choreId":  choreID,
		"type":     "schedule_reminder",
		"category": "NABU_REMINDER",
	}
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

func (s *Scheduler) dueDate(sch schedule.ChoreSchedule, now time.Time, leadMinutes int) (time.Time, bool) {
	if _, _, err := parseHM(sch.SpecificTime); err != nil {
		return time.Time{}, false
	}
	// A lead window can begin the prior evening; the late window can end the
	// following morning. Test each candidate's own local recurrence date.
	for _, offset := range []int{-1, 0, 1} {
		date := now.AddDate(0, 0, offset)
		if !s.schedSvc.IsActiveForDay(sch, date) {
			continue
		}
		scheduled := computeScheduleTime(date, sch.SpecificTime)
		if !now.Before(scheduled.Add(-time.Duration(leadMinutes)*time.Minute)) && !now.After(scheduled.Add(time.Duration(leadMinutes+5)*time.Minute)) {
			return date, true
		}
	}
	return time.Time{}, false
}
