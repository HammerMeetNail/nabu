package reminder

import (
	"context"
	"fmt"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"github.com/HammerMeetNail/nabu/internal/push"
	"sync"
	"time"
)

type MemoryStore struct {
	deleted      lifecycle.Tombstones
	removedPrefs map[string]bool
	mu           sync.RWMutex
	prefs        map[string]ChoreReminderPref
	sent         map[string]bool
	jobs         push.JobStore
}

// WithJobs makes a claimed reminder queue its push in jobs.
func (s *MemoryStore) WithJobs(jobs push.JobStore) *MemoryStore {
	s.jobs = jobs
	return s
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
	if s.removedPrefs[remKey(prefs.UserID, prefs.ChoreID)] {
		return lifecycle.ErrDeleted
	}

	if err := s.deleted.Check(prefs.UserID, 0, prefs.ChoreID, 0, 0); err != nil {
		return err
	}

	s.prefs[remKey(prefs.UserID, prefs.ChoreID)] = prefs
	return nil
}

func (s *MemoryStore) ClaimReminder(ctx context.Context, scheduleID, userID int64, scheduledDate string, job push.Job) (bool, error) {
	key := sentKey(scheduleID, userID, scheduledDate)
	s.mu.Lock()
	if err := s.deleted.Check(userID, 0, 0, scheduleID, 0); err != nil {
		s.mu.Unlock()
		return false, err
	}
	if s.sent[key] {
		s.mu.Unlock()
		return false, nil
	}
	s.sent[key] = true
	jobs := s.jobs
	s.mu.Unlock()
	if jobs == nil {
		return true, nil
	}
	if err := jobs.EnqueuePush(ctx, job); err != nil {
		// Leave it for the next tick, as a rolled-back transaction would.
		s.mu.Lock()
		delete(s.sent, key)
		s.mu.Unlock()
		return false, err
	}
	return true, nil
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

func (s *MemoryStore) CleanupAccount(d *lifecycle.Deletion) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, p := range s.prefs {
		if p.UserID == d.UserID || d.Chores[p.ChoreID] {
			delete(s.prefs, key)
		}
	}
	for key := range s.sent {
		parts := splitReminderKey(key)
		if len(parts) != 3 {
			continue
		}
		var scheduleID, userID int64
		_, _ = fmt.Sscan(parts[0], &scheduleID)
		_, _ = fmt.Sscan(parts[1], &userID)
		if userID == d.UserID || d.Schedules[scheduleID] {
			delete(s.sent, key)
		}
	}

	s.deleted.Mark(d)
}

func (s *MemoryStore) MembershipChanged(userID int64, choreIDs []int64, member bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.removedPrefs == nil {
		s.removedPrefs = map[string]bool{}
	}
	for _, choreID := range choreIDs {
		key := remKey(userID, choreID)
		if member {
			delete(s.removedPrefs, key)
		} else {
			delete(s.prefs, key)
			s.removedPrefs[key] = true
		}
	}
}
