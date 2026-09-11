package userprefs

import (
	"context"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"sync"
)

type memoryStore struct {
	deleted lifecycle.Tombstones
	mu      sync.RWMutex
	data    map[int64]Preferences
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
			ChoreOrder:            []int64{},
			HiddenHomeChoreIDs:    []int64{},
			StatsSectionOrder:     []string{},
			StatsSectionHidden:    []string{},
			VolumeUnit:            "ml",
			StatsWidgets:          []StatsWidget{},
			HideNotificationBadge: false,
		}, nil
	}
	// Return a copy so callers can't mutate internal state.
	out := Preferences{
		ChoreOrder:            make([]int64, len(p.ChoreOrder)),
		HiddenHomeChoreIDs:    make([]int64, len(p.HiddenHomeChoreIDs)),
		Timezone:              p.Timezone,
		StatsSectionOrder:     make([]string, len(p.StatsSectionOrder)),
		StatsSectionHidden:    make([]string, len(p.StatsSectionHidden)),
		VolumeUnit:            p.VolumeUnit,
		StatsWidgets:          make([]StatsWidget, len(p.StatsWidgets)),
		HideNotificationBadge: p.HideNotificationBadge,
	}
	if out.VolumeUnit == "" {
		out.VolumeUnit = "ml"
	}
	copy(out.ChoreOrder, p.ChoreOrder)
	copy(out.HiddenHomeChoreIDs, p.HiddenHomeChoreIDs)
	copy(out.StatsSectionOrder, p.StatsSectionOrder)
	copy(out.StatsSectionHidden, p.StatsSectionHidden)
	copy(out.StatsWidgets, p.StatsWidgets)
	return out, nil
}

func (s *memoryStore) Upsert(ctx context.Context, userID int64, p Preferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleted.Check(userID, 0, 0, 0, 0); err != nil {
		return err
	}

	cp := Preferences{
		ChoreOrder:            make([]int64, len(p.ChoreOrder)),
		HiddenHomeChoreIDs:    make([]int64, len(p.HiddenHomeChoreIDs)),
		Timezone:              p.Timezone,
		StatsSectionOrder:     make([]string, len(p.StatsSectionOrder)),
		StatsSectionHidden:    make([]string, len(p.StatsSectionHidden)),
		VolumeUnit:            p.VolumeUnit,
		StatsWidgets:          make([]StatsWidget, len(p.StatsWidgets)),
		HideNotificationBadge: p.HideNotificationBadge,
	}
	if cp.VolumeUnit != "oz" {
		cp.VolumeUnit = "ml"
	}
	copy(cp.ChoreOrder, p.ChoreOrder)
	copy(cp.HiddenHomeChoreIDs, p.HiddenHomeChoreIDs)
	copy(cp.StatsSectionOrder, p.StatsSectionOrder)
	copy(cp.StatsSectionHidden, p.StatsSectionHidden)
	copy(cp.StatsWidgets, p.StatsWidgets)
	s.data[userID] = cp
	return nil
}

func (s *memoryStore) CleanupAccount(d *lifecycle.Deletion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, d.UserID)
	s.deleted.Mark(d)
}
