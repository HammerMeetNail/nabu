package notification

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/diagnostics"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"github.com/HammerMeetNail/nabu/internal/push"
	"log"
	"time"
)

type Service struct {
	store      Store
	households household.Store
	chores     chore.Store
	now        func() time.Time

	// events holds logs and joins waiting to fan out; nil turns fan-out off.
	events     EventStore
	wakeEvents func()
	wake       chan struct{}

	// pushes takes each recipient's push job; nil sends no pushes.
	pushes     push.JobStore
	wakePushes func()
}

type Scope struct{ HouseholdID, ChoreID int64 }

// errAccessChanged reports an activity its recipient can no longer see.
var errAccessChanged = errors.New("notification access changed")

func (s *Service) WithAuthorization(hh household.Store, chores chore.Store) *Service {
	s.households, s.chores = hh, chores
	return s
}

// deliver runs fn while userID is a member of the scope's household who can
// see its activity, and keeps that membership fixed until fn returns.
func (s *Service) deliver(ctx context.Context, userID int64, scopes []Scope, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.households == nil {
		return fn()
	} // explicitly unwired internal/test service
	if len(scopes) == 0 || scopes[0].HouseholdID == 0 {
		return errors.New("notification household required")
	}
	scope := scopes[0]
	return household.WithMember(ctx, s.households, userID, scope.HouseholdID, func(role string) error {
		if scope.ChoreID != 0 {
			c, err := s.chores.GetChore(ctx, scope.ChoreID)
			if err != nil {
				return err
			}
			if c.HouseholdID != scope.HouseholdID || (c.Visibility == chore.VisibilityAdmins && role != household.RoleOwner && role != household.RoleAdmin) {
				return errAccessChanged
			}
		}
		return fn()
	})
}

// accessLost reports a recipient who may no longer see a notification, as
// opposed to a failure worth retrying.
func accessLost(err error) bool {
	return errors.Is(err, household.ErrNotMember) || errors.Is(err, chore.ErrNotFound) ||
		errors.Is(err, errAccessChanged) || errors.Is(err, lifecycle.ErrDeleted)
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now, wake: make(chan struct{}, 1)}
}

// WithEvents turns on fan-out: logs and joins are written to events, and wake
// tells the fan-out worker (which may be another Service, on the delivery
// pool) that one is waiting.
func (s *Service) WithEvents(events EventStore, wake func()) *Service {
	s.events, s.wakeEvents = events, wake
	return s
}

// WithPushQueue makes a fan-out queue a push job for each recipient who
// allows it, and call wake after.
func (s *Service) WithPushQueue(jobs push.JobStore, wake func()) *Service {
	s.pushes, s.wakePushes = jobs, wake
	return s
}

// WakeEvents asks RunEvents to look for waiting events now.
func (s *Service) WakeEvents() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// List returns up to 50 most-recent notifications for a user, plus the
// current unread count.
func (s *Service) List(ctx context.Context, userID int64) ([]Notification, int, error) {
	page, err := s.ListPage(ctx, userID, "")
	return page.Notifications, page.UnreadCount, err
}

// GetNotificationPreferences returns the current user's reminder/notification
// preferences, defaulting to all types enabled if no row exists yet.
func (s *Service) GetNotificationPreferences(ctx context.Context, userID int64) (ReminderPreference, error) {
	return s.store.GetReminderPreferences(ctx, userID)
}

// UpdateNotificationPreferences patches the notification preferences for a
// user.  Only fields present in the request struct (non-nil pointers) are
// applied; others are left unchanged.
func (s *Service) UpdateNotificationPreferences(ctx context.Context, prefs ReminderPreference) error {
	return s.store.UpdateReminderPreferences(ctx, prefs)
}

// AvailableNotificationTypes returns the list of notification types that can
// be toggled on/off by the user.
func AvailableNotificationTypes() []NotificationTypeInfo {
	return []NotificationTypeInfo{
		{
			Type:        "chore_logged",
			Label:       "Chore Logged",
			Description: "When someone else in your household logs a chore.",
		},
		{
			Type:        "household_joined",
			Label:       "Household Joined",
			Description: "When someone joins your household.",
		},
		{
			Type:        "schedule_reminder",
			Label:       "Schedule Reminder",
			Description: "When a scheduled chore's time is approaching.",
		},
	}
}

// NotificationTypeInfo describes a notification type for the settings UI.
type NotificationTypeInfo struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// UnreadCount returns the number of unread notifications for a user.
func (s *Service) UnreadCount(ctx context.Context, userID int64) (int, error) {
	return s.store.GetUnreadCount(ctx, userID)
}

// MarkRead marks a single notification as read.
func (s *Service) MarkRead(ctx context.Context, id, userID int64) error {
	return s.store.MarkRead(ctx, id, userID)
}

// MarkAllRead marks all notifications for a user as read.
func (s *Service) MarkAllRead(ctx context.Context, userID int64) error {
	return s.store.MarkAllRead(ctx, userID)
}

// Delete removes a single notification.
func (s *Service) Delete(ctx context.Context, id, userID int64) error {
	return s.store.DeleteNotification(ctx, id, userID)
}

// ClearAll removes the recipient's entire history, including read and older rows.
func (s *Service) ClearAll(ctx context.Context, userID int64) error {
	return s.store.ClearAllNotifications(ctx, userID)
}

// EnqueueChoreLogged records that a log should fan out to the household:
// every other member except subjectID (whom the log is for) and actorID (who
// logged it). It writes one row, so a caller can wait for it.
func (s *Service) EnqueueChoreLogged(ctx context.Context, householdID, choreID, subjectID, actorID int64) error {
	return s.enqueue(ctx, Event{Kind: EventChoreLogged, HouseholdID: householdID, ChoreID: choreID,
		SubjectID: subjectID, ActorID: actorID})
}

// EnqueueHouseholdJoined records that joinerID's join should fan out to the
// household's other members.
func (s *Service) EnqueueHouseholdJoined(ctx context.Context, householdID, joinerID int64, joinerName, householdName string) error {
	return s.enqueue(ctx, Event{Kind: EventHouseholdJoined, HouseholdID: householdID, SubjectID: joinerID,
		ActorID: joinerID, Title: "New Member", Body: fmt.Sprintf("%s joined %s", joinerName, householdName)})
}

func (s *Service) enqueue(ctx context.Context, ev Event) error {
	if s.events == nil {
		return nil
	}
	ev.ExpiresAt = s.now().Add(eventTTL)
	if err := s.events.EnqueueEvent(ctx, ev); err != nil {
		return err
	}
	if s.wakeEvents != nil {
		s.wakeEvents()
	}
	return nil
}

// RunEvents fans out waiting events until ctx ends. Every instance runs it;
// claims skip the events another worker holds.
func (s *Service) RunEvents(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		s.DeliverPendingEvents(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

// DeliverPendingEvents fans out up to 20 due events. A failed fan-out is
// retried; the notifications and push jobs it already wrote are kept.
func (s *Service) DeliverPendingEvents(ctx context.Context) {
	if s.events == nil {
		return
	}
	for i := 0; i < 20 && ctx.Err() == nil; i++ {
		now := s.now()
		ev, err := s.events.ClaimEvent(ctx, now, now.Add(eventLease), diagnostics.RequestID())
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		if err != nil {
			log.Printf("notif: claim failed class=%s", diagnostics.ErrorClass(err))
			return
		}
		var retryAt time.Time
		if err := s.fanOut(ctx, ev); err != nil {
			log.Printf("notif: fan-out failed event=%d class=%s", ev.ID, diagnostics.ErrorClass(err))
			retryAt = s.now().Add(min(10*time.Second<<min(max(ev.Attempts-1, 0), 4), 2*time.Minute))
		}
		// Release it even at shutdown, so another instance can retry at once.
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		if err := s.events.FinishEvent(finishCtx, ev, retryAt); err != nil {
			log.Printf("notif: finish failed event=%d class=%s", ev.ID, diagnostics.ErrorClass(err))
		}
		cancel()
	}
}

func (s *Service) fanOut(ctx context.Context, ev Event) error {
	if s.households == nil || s.chores == nil {
		return nil
	}
	members, err := s.households.GetMembers(ctx, ev.HouseholdID)
	if err != nil {
		return err
	}
	scope := Scope{HouseholdID: ev.HouseholdID, ChoreID: ev.ChoreID}
	title, body := ev.Title, ev.Body
	var c chore.Chore
	if ev.Kind == EventChoreLogged {
		c, err = s.chores.GetChore(ctx, ev.ChoreID)
		if errors.Is(err, chore.ErrNotFound) {
			return nil // deleted since: nothing to tell anyone
		}
		if err != nil {
			return err
		}
		if c.HouseholdID != ev.HouseholdID {
			return nil
		}
		loggerName := "Someone"
		for _, m := range members {
			if m.UserID == ev.SubjectID && m.DisplayName != "" {
				loggerName = m.DisplayName
			}
		}
		title, body = fmt.Sprintf("%s %s", c.Icon, c.Name), fmt.Sprintf("%s logged this", loggerName)
	}
	var firstErr error
	for _, m := range members {
		// Disabled members receive no activity notifications.
		if m.Suspended || m.UserID == ev.SubjectID || m.UserID == ev.ActorID {
			continue
		}
		// For Admins-only tasks, only notify other admins/owners.
		if ev.Kind == EventChoreLogged && c.Visibility == chore.VisibilityAdmins &&
			m.Role != household.RoleOwner && m.Role != household.RoleAdmin {
			continue
		}
		if err := s.notify(ctx, ev, m.UserID, title, body, scope); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// notify writes one recipient's in-app notification and, if they allow
// pushes of its kind, their push job. Both are keyed to the event and the
// recipient, so a retried fan-out writes neither twice.
func (s *Service) notify(ctx context.Context, ev Event, userID int64, title, body string, scope Scope) error {
	key := fmt.Sprintf("event:%d:%d", ev.ID, userID)
	queued := false
	err := s.deliver(ctx, userID, []Scope{scope}, func() error {
		n, err := s.store.CreateNotification(ctx, Notification{UserID: userID, Type: ev.Kind, Title: title, Body: body, DedupeKey: key})
		if err != nil {
			return err
		}
		if s.pushes == nil || !s.shouldSendPush(ctx, userID, ev.Kind) {
			return nil
		}
		data := map[string]any{"type": n.Type, "householdId": scope.HouseholdID, "choreId": scope.ChoreID}
		if err := s.pushes.EnqueuePush(ctx, push.Job{UserID: userID, HouseholdID: scope.HouseholdID, ChoreID: scope.ChoreID,
			Type: n.Type, Title: n.Title, Body: n.Body, Data: data, DedupeKey: key, ExpiresAt: s.now().Add(push.JobTTL)}); err != nil {
			return err
		}
		log.Printf("notif: queued push to user %d type=%s", userID, n.Type)
		queued = true
		return nil
	})
	if queued && s.wakePushes != nil {
		s.wakePushes()
	}
	if accessLost(err) {
		return nil
	}
	return err
}

// PushGuard keeps a push job's recipient a member who can see it while one
// send runs: a removal waits for that send, and no later send starts.
func (s *Service) PushGuard() push.Guard {
	return func(ctx context.Context, job push.Job, deliver func() error) error {
		err := s.deliver(ctx, job.UserID, []Scope{{HouseholdID: job.HouseholdID, ChoreID: job.ChoreID}}, deliver)
		if accessLost(err) {
			return fmt.Errorf("%w: %w", push.ErrNoAccess, err)
		}
		return err
	}
}

// shouldSendPush checks whether a push notification of the given type should be
// sent to the user, respecting their notification preferences.
func (s *Service) shouldSendPush(ctx context.Context, userID int64, notifType string) bool {
	prefs, err := s.store.GetReminderPreferences(ctx, userID)
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
		if t == notifType {
			return true
		}
	}
	return false
}
