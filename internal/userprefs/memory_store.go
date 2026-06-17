package userprefs

import (
	"context"
	"sync"
)

type memoryStore struct {
	mu   sync.RWMutex
	data map[int64]Preferences
}

// NewMemoryStore returns an in-memory Store suitable for tests and the
// no-database dev mode.
func NewMemoryStore() Store {
	return &memoryStore{data: make(map[int64]Preferences)}
}

func (s *memoryStore) Get(ctx context.Context, userID int64) (Preferences, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.data[userID]
	if !ok {
		return Preferences{
			ChoreOrder:          []int64{},
			HiddenHomeChoreIDs:  []int64{},
			StatsSectionOrder:   []string{},
			StatsSectionHidden:  []string{},
		}, nil
	}
	// Return a copy so callers can't mutate internal state.
	out := Preferences{
		ChoreOrder:         make([]int64, len(p.ChoreOrder)),
		HiddenHomeChoreIDs: make([]int64, len(p.HiddenHomeChoreIDs)),
		Timezone:           p.Timezone,
		StatsSectionOrder:  make([]string, len(p.StatsSectionOrder)),
		StatsSectionHidden: make([]string, len(p.StatsSectionHidden)),
	}
	copy(out.ChoreOrder, p.ChoreOrder)
	copy(out.HiddenHomeChoreIDs, p.HiddenHomeChoreIDs)
	copy(out.StatsSectionOrder, p.StatsSectionOrder)
	copy(out.StatsSectionHidden, p.StatsSectionHidden)
	return out, nil
}

func (s *memoryStore) Upsert(ctx context.Context, userID int64, p Preferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := Preferences{
		ChoreOrder:         make([]int64, len(p.ChoreOrder)),
		HiddenHomeChoreIDs: make([]int64, len(p.HiddenHomeChoreIDs)),
		Timezone:           p.Timezone,
		StatsSectionOrder:  make([]string, len(p.StatsSectionOrder)),
		StatsSectionHidden: make([]string, len(p.StatsSectionHidden)),
	}
	copy(cp.ChoreOrder, p.ChoreOrder)
	copy(cp.HiddenHomeChoreIDs, p.HiddenHomeChoreIDs)
	copy(cp.StatsSectionOrder, p.StatsSectionOrder)
	copy(cp.StatsSectionHidden, p.StatsSectionHidden)
	s.data[userID] = cp
	return nil
}
