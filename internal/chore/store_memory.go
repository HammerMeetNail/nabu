package chore

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrNotFound      = errors.New("chore not found")
	ErrDuplicateName = errors.New("a chore with this name already exists")
)

type MemoryStore struct {
	mu     sync.RWMutex
	idSeq  int64
	chores map[int64]Chore
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		chores: map[int64]Chore{},
	}
}

func (s *MemoryStore) nextID() int64 {
	s.idSeq++
	return s.idSeq
}

func (s *MemoryStore) CreateChore(_ context.Context, chore Chore) (Chore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.chores {
		if existing.HouseholdID == chore.HouseholdID && existing.Name == chore.Name {
			return Chore{}, ErrDuplicateName
		}
	}

	chore.ID = s.nextID()
	chore.CreatedAt = time.Now().UTC()
	s.chores[chore.ID] = chore
	return chore, nil
}

func (s *MemoryStore) GetChore(_ context.Context, id int64) (Chore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chore, ok := s.chores[id]
	if !ok {
		return Chore{}, ErrNotFound
	}
	return chore, nil
}

func (s *MemoryStore) ListChores(_ context.Context, householdID int64) ([]Chore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Chore
	for _, c := range s.chores {
		if c.HouseholdID == householdID {
			result = append(result, c)
		}
	}
	sortChores(result)
	return result, nil
}

func (s *MemoryStore) UpdateChore(_ context.Context, chore Chore) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.chores[chore.ID]
	if !ok {
		return ErrNotFound
	}
	for _, c := range s.chores {
		if c.HouseholdID == existing.HouseholdID && c.ID != chore.ID && c.Name == chore.Name {
			return ErrDuplicateName
		}
	}
	chore.HouseholdID = existing.HouseholdID
	chore.IsPredefined = existing.IsPredefined
	chore.CreatedAt = existing.CreatedAt
	chore.CreatedBy = existing.CreatedBy
	s.chores[chore.ID] = chore
	return nil
}

func (s *MemoryStore) DeleteChore(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.chores[id]; !ok {
		return ErrNotFound
	}
	delete(s.chores, id)
	return nil
}

func (s *MemoryStore) ReorderChores(_ context.Context, householdID int64, choreIDs []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	order := 0
	for _, id := range choreIDs {
		if c, ok := s.chores[id]; ok && c.HouseholdID == householdID {
			c.SortOrder = order
			s.chores[id] = c
			order++
		}
	}
	return nil
}

func (s *MemoryStore) SeedPredefinedChores(_ context.Context, householdID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, pc := range PredefinedChores {
		exists := false
		for _, existing := range s.chores {
			if existing.HouseholdID == householdID && existing.Name == pc.Name {
				exists = true
				break
			}
		}
		if !exists {
			chore := Chore{
				ID:              s.nextID(),
				HouseholdID:     householdID,
				Name:            pc.Name,
				Icon:            pc.Icon,
				Color:           pc.Color,
				SortOrder:       pc.SortOrder,
				Category:        pc.Category,
				IsPredefined:    true,
				CreatedAt:       time.Now().UTC(),
				IndicatorLabels: pc.IndicatorLabels,
			}
			s.chores[chore.ID] = chore
		}
	}
	return nil
}

func sortChores(chores []Chore) {
	sort.Slice(chores, func(i, j int) bool {
		if chores[i].SortOrder != chores[j].SortOrder {
			return chores[i].SortOrder < chores[j].SortOrder
		}
		return chores[i].Name < chores[j].Name
	})
}
