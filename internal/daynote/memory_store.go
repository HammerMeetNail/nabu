package daynote

import (
	"context"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"sort"
	"sync"
	"time"
)

type memoryStore struct {
	deleted lifecycle.Tombstones
	mu      sync.RWMutex
	data    map[int64]map[string]DayNote // householdID -> date -> note
}

// NewMemoryStore returns an in-memory Store for tests and the no-database mode.
func NewMemoryStore() Store {
	return &memoryStore{data: map[int64]map[string]DayNote{}}
}

func (s *memoryStore) ListRange(ctx context.Context, householdID int64, start, end time.Time) ([]DayNote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	var out []DayNote
	for date, n := range s.data[householdID] {
		parsed, err := time.Parse(time.DateOnly, date)
		if err == nil && !parsed.Before(start) && parsed.Before(end) {
			out = append(out, n)
			if err := readlimit.Check(ctx, len(out)); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}

func (s *memoryStore) Upsert(_ context.Context, householdID int64, date, note string, updatedBy int64) (DayNote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleted.Check(updatedBy, householdID, 0, 0, 0); err != nil {
		return DayNote{}, err
	}

	if s.data[householdID] == nil {
		s.data[householdID] = map[string]DayNote{}
	}
	if note == "" {
		delete(s.data[householdID], date)
		return DayNote{Date: date, Note: ""}, nil
	}
	uid := updatedBy
	n := DayNote{Date: date, Note: note, UpdatedBy: &uid, UpdatedAt: time.Now().UTC()}
	s.data[householdID][date] = n
	return n, nil
}

func (s *memoryStore) CleanupAccount(d *lifecycle.Deletion) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for hhID, notes := range s.data {
		if d.Households[hhID] {
			delete(s.data, hhID)
			continue
		}
		for date, n := range notes {
			if n.UpdatedBy != nil && *n.UpdatedBy == d.UserID {
				n.UpdatedBy = nil
				notes[date] = n
			}
		}
	}

	s.deleted.Mark(d)
}
