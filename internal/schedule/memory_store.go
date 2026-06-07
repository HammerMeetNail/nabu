// internal/schedule/memory_store.go

package schedule

import (
	"context"
	"errors"
	"sync"
	"time"
)

// MemoryStore is an in-memory implementation of Store.
type MemoryStore struct {
	mu      sync.RWMutex
	records map[int64]ChoreSchedule
	nextID  int64
}

// NewMemoryStore creates a new MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[int64]ChoreSchedule), nextID: 1}
}

func (s *MemoryStore) Create(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sch.ID = s.nextID
	sch.CreatedAt = time.Now().UTC()
	sch.UpdatedAt = sch.CreatedAt
	s.nextID++
	s.records[sch.ID] = sch
	return sch, nil
}

func (s *MemoryStore) Get(ctx context.Context, id int64) (ChoreSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sch, ok := s.records[id]
	if !ok {
		return ChoreSchedule{}, errors.New("schedule not found")
	}
	return sch, nil
}

func (s *MemoryStore) ListByHousehold(ctx context.Context, householdID int64) ([]ChoreSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ChoreSchedule
	for _, sch := range s.records {
		if sch.HouseholdID == householdID {
			out = append(out, sch)
		}
	}
	return out, nil
}

func (s *MemoryStore) Update(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[sch.ID]; !ok {
		return ChoreSchedule{}, errors.New("schedule not found")
	}
	sch.UpdatedAt = time.Now().UTC()
	s.records[sch.ID] = sch
	return sch, nil
}

func (s *MemoryStore) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[id]; !ok {
		return errors.New("schedule not found")
	}
	delete(s.records, id)
	return nil
}

func (s *MemoryStore) DeleteFollowUpSchedulesByChore(ctx context.Context, choreID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sch := range s.records {
		if sch.ChoreID == choreID && sch.IsFollowUp {
			delete(s.records, id)
		}
	}
	return nil
}
