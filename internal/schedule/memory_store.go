// internal/schedule/memory_store.go

package schedule

import (
	"context"
	"errors"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"sync"
	"time"
)

// MemoryStore is an in-memory implementation of Store.
type MemoryStore struct {
	deleted        lifecycle.Tombstones
	removedMembers map[[2]int64]bool
	mu             sync.RWMutex
	records        map[int64]ChoreSchedule
	nextID         int64
	logEffects     map[int64]struct{}
}

// NewMemoryStore creates a new MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[int64]ChoreSchedule), logEffects: make(map[int64]struct{}), nextID: 1}
}

func (s *MemoryStore) ApplyLogEffects(ctx context.Context, effects LogEffects) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if effects.FollowUp != nil {
		uid := lifecycle.UserID(effects.FollowUp.AssignedUserID)
		if err := s.deleted.Check(uid, 0, 0, 0, 0); err != nil {
			return false, err
		}
		if s.removedMembers[[2]int64{uid, effects.HouseholdID}] {
			return false, lifecycle.ErrDeleted
		}
	}

	if err := s.deleted.Check(0, effects.HouseholdID, effects.ChoreID, 0, effects.LogID); err != nil {
		return false, err
	}

	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, done := s.logEffects[effects.LogID]; done {
		return false, nil
	}
	if effects.UpdateFollowUp {
		for id, sch := range s.records {
			if sch.HouseholdID == effects.HouseholdID && sch.ChoreID == effects.ChoreID && sch.IsFollowUp {
				delete(s.records, id)
			}
		}
		if effects.FollowUp != nil {
			sch := *effects.FollowUp
			sch.ID = s.nextID
			s.nextID++
			sch.CreatedAt = time.Now().UTC()
			sch.UpdatedAt = sch.CreatedAt
			s.records[sch.ID] = sch
		}
	}
	s.logEffects[effects.LogID] = struct{}{}
	return true, nil
}

func (s *MemoryStore) Create(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.removedMembers[[2]int64{lifecycle.UserID(sch.AssignedUserID), sch.HouseholdID}] {
		return ChoreSchedule{}, lifecycle.ErrDeleted
	}

	if err := s.deleted.Check(lifecycle.UserID(sch.AssignedUserID), sch.HouseholdID, sch.ChoreID, 0, 0); err != nil {
		return ChoreSchedule{}, err
	}

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
			if err := readlimit.Check(ctx, len(out)); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func (s *MemoryStore) Update(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.removedMembers[[2]int64{lifecycle.UserID(sch.AssignedUserID), sch.HouseholdID}] {
		return ChoreSchedule{}, lifecycle.ErrDeleted
	}

	if err := s.deleted.Check(lifecycle.UserID(sch.AssignedUserID), sch.HouseholdID, sch.ChoreID, sch.ID, 0); err != nil {
		return ChoreSchedule{}, err
	}

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

func (s *MemoryStore) ListActiveWithTime(ctx context.Context) ([]ChoreSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ChoreSchedule
	for _, sch := range s.records {
		if sch.IsActive && sch.SpecificTime != "" {
			out = append(out, sch)
		}
	}
	return out, nil
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

func (s *MemoryStore) CleanupAccount(d *lifecycle.Deletion) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, sch := range s.records {
		if d.Households[sch.HouseholdID] || d.Chores[sch.ChoreID] {
			d.Schedules[id] = true
			delete(s.records, id)
		} else if sch.AssignedUserID != nil && *sch.AssignedUserID == d.UserID {
			sch.AssignedUserID = nil
			s.records[id] = sch
		}
	}
	for id := range s.logEffects {
		if d.Logs[id] {
			delete(s.logEffects, id)
		}
	}

	s.deleted.Mark(d)
}

func (s *MemoryStore) MembershipChanged(userID, householdID int64, member bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.removedMembers == nil {
		s.removedMembers = map[[2]int64]bool{}
	}
	key := [2]int64{userID, householdID}
	if member {
		delete(s.removedMembers, key)
		return
	}
	s.removedMembers[key] = true
	for id, sch := range s.records {
		if sch.HouseholdID == householdID && sch.AssignedUserID != nil && *sch.AssignedUserID == userID {
			sch.AssignedUserID = nil
			s.records[id] = sch
		}
	}
}
