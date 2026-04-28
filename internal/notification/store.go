package notification

import (
	"context"
	"time"
)

type Notification struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	IsRead    bool      `json:"isRead"`
	CreatedAt time.Time `json:"createdAt"`
}

type ReminderPreference struct {
	UserID          int64  `json:"userId"`
	PushEnabled     bool   `json:"pushEnabled"`
	EmailEnabled    bool   `json:"emailEnabled"`
	QuietHoursStart string `json:"quietHoursStart"`
	QuietHoursEnd   string `json:"quietHoursEnd"`
	Timezone        string `json:"timezone"`
}

type Store interface {
	CreateNotification(ctx context.Context, n Notification) (Notification, error)
	ListNotifications(ctx context.Context, userID int64, limit, offset int) ([]Notification, error)
	GetUnreadCount(ctx context.Context, userID int64) (int, error)
	MarkRead(ctx context.Context, id, userID int64) error
	MarkAllRead(ctx context.Context, userID int64) error
	DeleteNotification(ctx context.Context, id, userID int64) error
	GetReminderPreferences(ctx context.Context, userID int64) (ReminderPreference, error)
	UpdateReminderPreferences(ctx context.Context, prefs ReminderPreference) error
}

type MemoryStore struct {
	notifs []Notification
	idSeq  int64
	prefs  map[int64]ReminderPreference
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		prefs: map[int64]ReminderPreference{},
	}
}

func (s *MemoryStore) CreateNotification(_ context.Context, n Notification) (Notification, error) {
	s.idSeq++
	n.ID = s.idSeq
	n.CreatedAt = time.Now().UTC()
	s.notifs = append(s.notifs, n)
	return n, nil
}

func (s *MemoryStore) ListNotifications(_ context.Context, userID int64, limit, offset int) ([]Notification, error) {
	var result []Notification
	for _, n := range s.notifs {
		if n.UserID == userID {
			result = append(result, n)
		}
	}
	start := offset
	if start > len(result) {
		start = len(result)
	}
	end := start + limit
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], nil
}

func (s *MemoryStore) GetUnreadCount(_ context.Context, userID int64) (int, error) {
	count := 0
	for _, n := range s.notifs {
		if n.UserID == userID && !n.IsRead {
			count++
		}
	}
	return count, nil
}

func (s *MemoryStore) MarkRead(_ context.Context, id, userID int64) error {
	for i, n := range s.notifs {
		if n.ID == id && n.UserID == userID {
			s.notifs[i].IsRead = true
			return nil
		}
	}
	return nil
}

func (s *MemoryStore) MarkAllRead(_ context.Context, userID int64) error {
	for i, n := range s.notifs {
		if n.UserID == userID {
			s.notifs[i].IsRead = true
		}
	}
	return nil
}

func (s *MemoryStore) DeleteNotification(_ context.Context, id, userID int64) error {
	for i, n := range s.notifs {
		if n.ID == id && n.UserID == userID {
			s.notifs = append(s.notifs[:i], s.notifs[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *MemoryStore) GetReminderPreferences(_ context.Context, userID int64) (ReminderPreference, error) {
	p, ok := s.prefs[userID]
	if !ok {
		return ReminderPreference{UserID: userID, Timezone: "UTC"}, nil
	}
	return p, nil
}

func (s *MemoryStore) UpdateReminderPreferences(_ context.Context, prefs ReminderPreference) error {
	s.prefs[prefs.UserID] = prefs
	return nil
}
