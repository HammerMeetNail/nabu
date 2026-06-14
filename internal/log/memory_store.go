package log

import (
	"context"
	"sort"
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
	existing.VolumeML = log.VolumeML
	existing.IndicatorVolumes = log.IndicatorVolumes
	existing.UserID = log.UserID
	existing.CompletedAt = log.CompletedAt
	existing.SlotHour = log.SlotHour
	existing.LogDate = log.LogDate
	existing.Rating = log.Rating
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
			if logMatchesDate(l, date) {
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
		if l.HouseholdID == householdID && logMatchesDate(l, date) {
			result = append(result, l)
		}
	}
	return result, nil
}

func (s *MemoryStore) ListLogsRange(_ context.Context, householdID int64, start, end time.Time) ([]ChoreLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ChoreLog
	for _, l := range s.logs {
		if l.HouseholdID == householdID && logInRange(l, start, end) {
			result = append(result, l)
		}
	}
	return result, nil
}

func (s *MemoryStore) HistoryLogs(_ context.Context, householdID int64, start, end time.Time) ([]ChoreLog, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ChoreLog
	hasOlder := false
	for _, l := range s.logs {
		if l.HouseholdID == householdID {
			if logInRange(l, start, end) {
				result = append(result, l)
			}
			if logBeforeRange(l, start) {
				hasOlder = true
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CompletedAt.After(result[j].CompletedAt)
	})
	if result == nil {
		result = []ChoreLog{}
	}
	return result, hasOlder, nil
}

func logMatchesDate(l ChoreLog, date time.Time) bool {
	y1, m1, d1 := logDateParts(l)
	y2, m2, d2 := date.UTC().Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func logDateParts(l ChoreLog) (int, time.Month, int) {
	if l.LogDate != nil {
		d, err := time.Parse("2006-01-02", *l.LogDate)
		if err == nil {
			return d.Date()
		}
	}
	return l.CompletedAt.UTC().Date()
}

func logInRange(l ChoreLog, start, end time.Time) bool {
	if l.LogDate != nil {
		d, err := time.Parse("2006-01-02", *l.LogDate)
		if err == nil {
			return !d.Before(start) && d.Before(end)
		}
	}
	return !l.CompletedAt.Before(start) && l.CompletedAt.Before(end)
}

func logBeforeRange(l ChoreLog, start time.Time) bool {
	if l.LogDate != nil {
		d, err := time.Parse("2006-01-02", *l.LogDate)
		if err == nil {
			return d.Before(start)
		}
	}
	return l.CompletedAt.Before(start)
}

func (s *MemoryStore) LatestPerChore(_ context.Context, householdID int64) (map[int64]ChoreLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := map[int64]ChoreLog{}
	for _, l := range s.logs {
		if l.HouseholdID != householdID {
			continue
		}
		if existing, ok := result[l.ChoreID]; !ok || l.CompletedAt.After(existing.CompletedAt) || (l.CompletedAt.Equal(existing.CompletedAt) && l.ID > existing.ID) {
			if l.Indicators == nil {
				l.Indicators = []string{}
			}
			result[l.ChoreID] = l
		}
	}
	return result, nil
}
