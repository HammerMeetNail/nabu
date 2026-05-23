package log

import (
	"context"
	"sync"
	"time"
)

type MemoryStore struct {
	mu    sync.RWMutex
	idSeq int64
	logs  map[int64]ChoreLog
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{logs: map[int64]ChoreLog{}}
}

func (s *MemoryStore) nextID() int64 {
	s.idSeq++
	return s.idSeq
}

func (s *MemoryStore) CreateLog(_ context.Context, log ChoreLog) (ChoreLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log.ID = s.nextID()
	log.CreatedAt = time.Now().UTC()
	s.logs[log.ID] = log
	return log, nil
}

func (s *MemoryStore) GetLog(_ context.Context, id int64) (ChoreLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.logs[id]
	if !ok {
		return ChoreLog{}, ErrNotFound
	}
	if l.Indicators == nil {
		l.Indicators = []string{}
	}
	return l, nil
}

func (s *MemoryStore) UpdateLog(_ context.Context, log ChoreLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.logs[log.ID]
	if !ok {
		return ErrNotFound
	}
	existing.Note = log.Note
	if log.Indicators == nil {
		existing.Indicators = []string{}
	} else {
		existing.Indicators = log.Indicators
	}
	s.logs[log.ID] = existing
	return nil
}

func (s *MemoryStore) DeleteLog(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.logs, id)
	return nil
}

func (s *MemoryStore) FindLog(_ context.Context, householdID, choreID int64, date time.Time) (*ChoreLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.logs {
		if l.HouseholdID == householdID && l.ChoreID == choreID {
			y1, m1, d1 := l.CompletedAt.UTC().Date()
			y2, m2, d2 := date.UTC().Date()
			if y1 == y2 && m1 == m2 && d1 == d2 {
				return &l, nil
			}
		}
	}
	return nil, ErrNotFound
}

func (s *MemoryStore) ListLogs(_ context.Context, householdID int64, date time.Time) ([]ChoreLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ChoreLog
	for _, l := range s.logs {
		if l.HouseholdID == householdID {
			y1, m1, d1 := l.CompletedAt.UTC().Date()
			y2, m2, d2 := date.UTC().Date()
			if y1 == y2 && m1 == m2 && d1 == d2 {
				result = append(result, l)
			}
		}
	}
	return result, nil
}

func (s *MemoryStore) ListLogsRange(_ context.Context, householdID int64, start, end time.Time) ([]ChoreLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ChoreLog
	for _, l := range s.logs {
		if l.HouseholdID == householdID && !l.CompletedAt.Before(start) && l.CompletedAt.Before(end) {
			result = append(result, l)
		}
	}
	return result, nil
}

func (s *MemoryStore) LatestPerChore(_ context.Context, householdID int64) (map[int64]ChoreLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := map[int64]ChoreLog{}
	for _, l := range s.logs {
		if l.HouseholdID != householdID {
			continue
		}
		if existing, ok := result[l.ChoreID]; !ok || l.CompletedAt.After(existing.CompletedAt) {
			if l.Indicators == nil {
				l.Indicators = []string{}
			}
			result[l.ChoreID] = l
		}
	}
	return result, nil
}
