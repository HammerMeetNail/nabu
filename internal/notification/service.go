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

// UnreadCount returns the number of unread notifications for a user.
func (s *Service) UnreadCount(ctx context.Context, userID int64) (int, error) {
	return s.store.GetUnreadCount(ctx, userID)
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
// member except the one who did the logging.  It also sends a Web Push to
// each recipient if a PushSender is configured.
//
// This is intentionally fire-and-forget: callers should invoke it in a
// goroutine so individual push failures do not block the HTTP response.
func (s *Service) NotifyChoreLogged(ctx context.Context, members []MemberInfo, loggerID int64, choreName, choreIcon string) {
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
		if m.UserID == loggerID {
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
		if s.pushSender != nil {
			log.Printf("notif: sending push to user %d title=%q", n.UserID, n.Title)
			// Best-effort — ignore push errors so a failed push device
			// doesn't stop other recipients from being notified.
			_ = s.pushSender.SendPushToUser(ctx, n.UserID, n.Title, n.Body)
		}
	}
}
