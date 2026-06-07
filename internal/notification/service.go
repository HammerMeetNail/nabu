package notification

import (
	"context"
	"fmt"
	"log"
)

// MemberInfo is a minimal member representation used to fan out notifications
// without creating an import cycle with the household package.
type MemberInfo struct {
	UserID      int64
	DisplayName string
}

// PushSender is an optional interface for sending Web Push messages.
// If nil, push is skipped.
type PushSender interface {
	SendPushToUser(ctx context.Context, userID int64, title, body string) error
}

type Service struct {
	store      Store
	pushSender PushSender // may be nil
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// WithPushSender attaches a push sender that will be called whenever a
// notification is created for a user.
func (s *Service) WithPushSender(ps PushSender) *Service {
	s.pushSender = ps
	return s
}

// List returns up to 50 most-recent notifications for a user, plus the
// current unread count.
func (s *Service) List(ctx context.Context, userID int64) ([]Notification, int, error) {
	notifs, err := s.store.ListNotifications(ctx, userID, 50, 0)
	if err != nil {
		return nil, 0, err
	}
	unread, err := s.store.GetUnreadCount(ctx, userID)
	return notifs, unread, err
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

// NotifyChoreLogged creates a "chore_logged" notification for every household
// member except the one attributed on the log and the one who performed the
// action.  It also sends a Web Push to each recipient if a PushSender is
// configured.
//
// This is intentionally fire-and-forget: callers should invoke it in a
// goroutine so individual push failures do not block the HTTP response.
func (s *Service) NotifyChoreLogged(ctx context.Context, members []MemberInfo, loggerID, actorID int64, choreName, choreIcon string) {
	loggerName := "Someone"
	for _, m := range members {
		if m.UserID == loggerID {
			if m.DisplayName != "" {
				loggerName = m.DisplayName
			}
			break
		}
	}

	title := fmt.Sprintf("%s %s", choreIcon, choreName)
	body := fmt.Sprintf("%s logged this", loggerName)

	for _, m := range members {
		if m.UserID == loggerID || m.UserID == actorID {
			continue
		}
		n, err := s.store.CreateNotification(ctx, Notification{
			UserID: m.UserID,
			Type:   "chore_logged",
			Title:  title,
			Body:   body,
		})
		if err != nil {
			continue
		}
		if s.pushSender != nil && s.shouldSendPush(ctx, m.UserID, "chore_logged") {
			log.Printf("notif: sending push to user %d title=%q", n.UserID, n.Title)
			_ = s.pushSender.SendPushToUser(ctx, n.UserID, n.Title, n.Body)
		}
	}
}

// NotifyHouseholdJoined creates a "household_joined" notification for every
// household member except the one who just joined.  It also sends a Web Push
// to each recipient if a PushSender is configured.
//
// This is intentionally fire-and-forget: callers should invoke it in a
// goroutine so individual push failures do not block the HTTP response.
func (s *Service) NotifyHouseholdJoined(ctx context.Context, members []MemberInfo, joinerID int64, joinerName string, householdName string) {
	title := "New Member"
	body := fmt.Sprintf("%s joined %s", joinerName, householdName)

	for _, m := range members {
		if m.UserID == joinerID {
			continue
		}
		n, err := s.store.CreateNotification(ctx, Notification{
			UserID: m.UserID,
			Type:   "household_joined",
			Title:  title,
			Body:   body,
		})
		if err != nil {
			continue
		}
		if s.pushSender != nil && s.shouldSendPush(ctx, m.UserID, "household_joined") {
			log.Printf("notif: sending household_joined push to user %d", n.UserID)
			_ = s.pushSender.SendPushToUser(ctx, n.UserID, n.Title, n.Body)
		}
	}
}

// shouldSendPush checks whether a push notification of the given type should be
// sent to the user, respecting their notification preferences.
func (s *Service) shouldSendPush(ctx context.Context, userID int64, notifType string) bool {
	prefs, err := s.store.GetReminderPreferences(ctx, userID)
	if err != nil {
		return true
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
