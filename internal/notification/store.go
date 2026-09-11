package notification

import (
	"context"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"sort"
	"sync"
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
	UserID                     int64    `json:"userId"`
	PushEnabled                bool     `json:"pushEnabled"`
	EmailEnabled               bool     `json:"emailEnabled"`
	QuietHoursStart            string   `json:"quietHoursStart"`
	QuietHoursEnd              string   `json:"quietHoursEnd"`
	Timezone                   string   `json:"timezone"`
	EnabledPushTypes           []string `json:"enabledPushTypes"`
	DefaultReminderLeadMinutes int      `json:"defaultReminderLeadMinutes"`
}

type Store interface {
	CreateNotification(ctx context.Context, n Notification) (Notification, error)
	ListNotifications(ctx context.Context, userID int64, limit, offset int) ([]Notification, error)
	ListNotificationsBefore(ctx context.Context, userID int64, before *Cursor, limit int) ([]Notification, error)
	GetUnreadCount(ctx context.Context, userID int64) (int, error)
	MarkRead(ctx context.Context, id, userID int64) error
	MarkAllRead(ctx context.Context, userID int64) error
	DeleteNotification(ctx context.Context, id, userID int64) error
	GetReminderPreferences(ctx context.Context, userID int64) (ReminderPreference, error)
	UpdateReminderPreferences(ctx context.Context, prefs ReminderPreference) error
}

// MemoryStore is an in-memory implementation for tests and the zero-DB fallback.
// The reminder scheduler goroutine reads preferences concurrently with HTTP
// handlers creating notifications and updating preferences, so all access is
// guarded by mu to avoid a data race (a concurrent map write is a fatal panic
// in Go).
type MemoryStore struct {
	deleted lifecycle.Tombstones
	mu      sync.Mutex
	notifs  []Notification
	idSeq   int64
	prefs   map[int64]ReminderPreference
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		prefs: map[int64]ReminderPreference{},
	}
}

func (s *MemoryStore) CreateNotification(_ context.Context, n Notification) (Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleted.Check(n.UserID, 0, 0, 0, 0); err != nil {
		return Notification{}, err
	}

	s.idSeq++
	n.ID = s.idSeq
	n.CreatedAt = time.Now().UTC()
	s.notifs = append(s.notifs, n)
	return n, nil
}

func (s *MemoryStore) ListNotifications(_ context.Context, userID int64, limit, offset int) ([]Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []Notification
	for _, n := range s.notifs {
		if n.UserID == userID {
			result = append(result, n)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
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
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, n := range s.notifs {
		if n.UserID == userID && !n.IsRead {
			count++
		}
	}
	return count, nil
}

func (s *MemoryStore) MarkRead(_ context.Context, id, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, n := range s.notifs {
		if n.ID == id && n.UserID == userID {
			s.notifs[i].IsRead = true
			return nil
		}
	}
	return nil
}

func (s *MemoryStore) MarkAllRead(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, n := range s.notifs {
		if n.UserID == userID {
			s.notifs[i].IsRead = true
		}
	}
	return nil
}

func (s *MemoryStore) DeleteNotification(_ context.Context, id, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, n := range s.notifs {
		if n.ID == id && n.UserID == userID {
			s.notifs = append(s.notifs[:i], s.notifs[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *MemoryStore) GetReminderPreferences(_ context.Context, userID int64) (ReminderPreference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.prefs[userID]
	if !ok {
		return ReminderPreference{UserID: userID, PushEnabled: true, Timezone: "UTC", EnabledPushTypes: []string{}, DefaultReminderLeadMinutes: 10}, nil
	}
	if p.EnabledPushTypes == nil {
		p.EnabledPushTypes = []string{}
	}
	return p, nil
}

func (s *MemoryStore) UpdateReminderPreferences(_ context.Context, prefs ReminderPreference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleted.Check(prefs.UserID, 0, 0, 0, 0); err != nil {
		return err
	}

	s.prefs[prefs.UserID] = prefs
	return nil
}

func (s *MemoryStore) CleanupAccount(d *lifecycle.Deletion) {
	s.mu.Lock()
	defer s.mu.Unlock()

	kept := s.notifs[:0]
	for _, n := range s.notifs {
		if n.UserID != d.UserID {
			kept = append(kept, n)
		}
	}
	s.notifs = kept
	delete(s.prefs, d.UserID)

	s.deleted.Mark(d)
}

func (s *MemoryStore) ListNotificationsBefore(ctx context.Context, userID int64, before *Cursor, limit int) ([]Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := []Notification{}
	for _, n := range s.notifs {
		if n.UserID != userID {
			continue
		}
		if before != nil && (n.CreatedAt.After(before.At) || (n.CreatedAt.Equal(before.At) && n.ID >= before.ID)) {
			continue
		}
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	limit = max(0, min(limit, PageSize+1))
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
