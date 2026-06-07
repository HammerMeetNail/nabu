package reminder

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type MemoryStore struct {
	mu    sync.RWMutex
	prefs map[string]ChoreReminderPref
	sent  map[string]bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		prefs: map[string]ChoreReminderPref{},
		sent:  map[string]bool{},
	}
}

func remKey(userID, choreID int64) string {
	return fmt.Sprintf("%d:%d", userID, choreID)
}

func sentKey(scheduleID, userID int64, scheduledDate string) string {
	return fmt.Sprintf("%d:%d:%s", scheduleID, userID, scheduledDate)
}

func (s *MemoryStore) GetChoreReminderPrefs(_ context.Context, userID int64) ([]ChoreReminderPref, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ChoreReminderPref
	for _, p := range s.prefs {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *MemoryStore) GetChoreReminderPref(_ context.Context, userID, choreID int64) (ChoreReminderPref, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.prefs[remKey(userID, choreID)]
	if !ok {
		return ChoreReminderPref{UserID: userID, ChoreID: choreID, Enabled: false, LeadMinutes: 10}, nil
	}
	return p, nil
}

func (s *MemoryStore) UpdateChoreReminderPref(_ context.Context, prefs ChoreReminderPref) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefs[remKey(prefs.UserID, prefs.ChoreID)] = prefs
	return nil
}

func (s *MemoryStore) HasReminder(_ context.Context, scheduleID, userID int64, scheduledDate string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sent[sentKey(scheduleID, userID, scheduledDate)], nil
}

func (s *MemoryStore) RecordReminder(_ context.Context, scheduleID, userID int64, scheduledDate string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent[sentKey(scheduleID, userID, scheduledDate)] = true
	return nil
}

func (s *MemoryStore) PurgeOldReminders(_ context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var purged int64
	cutoff := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	for key := range s.sent {
		parts := splitReminderKey(key)
		if len(parts) == 3 && parts[2] < cutoff {
			delete(s.sent, key)
			purged++
		}
	}
	return purged, nil
}

func splitReminderKey(key string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	parts = append(parts, key[start:])
	return parts
}
