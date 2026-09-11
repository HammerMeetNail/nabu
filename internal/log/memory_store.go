package log

import (
	"context"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryStore struct {
	deleted lifecycle.Tombstones
	mu      sync.RWMutex
	idSeq   int64
	logs    map[int64]ChoreLog
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
	if err := s.deleted.Check(log.UserID, log.HouseholdID, log.ChoreID, 0, 0); err != nil {
		return ChoreLog{}, err
	}

	if log.IdempotencyKey != "" {
		for _, existing := range s.logs {
			if existing.HouseholdID == log.HouseholdID && existing.IdempotencyKey == log.IdempotencyKey {
				return ChoreLog{}, ErrIdempotencyConflict
			}
		}
	}
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

func (s *MemoryStore) UpdateLog(_ context.Context, log ChoreLog, fields ...LogFields) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleted.Check(log.UserID, log.HouseholdID, log.ChoreID, 0, log.ID); err != nil {
		return err
	}

	existing, ok := s.logs[log.ID]
	if !ok {
		return ErrNotFound
	}
	if existing.HouseholdID != log.HouseholdID {
		return ErrNotFound
	}
	existing = mergeLog(existing, log, fieldsOrAll(fields))
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

func (s *MemoryStore) ListLogs(ctx context.Context, householdID int64, date time.Time) ([]ChoreLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ChoreLog
	for _, l := range s.logs {
		if readAllowed(ctx, householdID, l.ChoreID) && l.HouseholdID == householdID && logMatchesDate(l, date) {
			result = append(result, l)
		}
	}
	return result, nil
}

func (s *MemoryStore) ListLogsRange(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ChoreLog
	for _, l := range s.logs {
		if readAllowed(ctx, householdID, l.ChoreID) && l.HouseholdID == householdID && logInRange(l, start, end) {
			result = append(result, l)
			if err := readlimit.Check(ctx, len(result)); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (s *MemoryStore) ListLogUserIDs(_ context.Context, householdID int64) ([]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := map[int64]struct{}{}
	for _, l := range s.logs {
		if l.HouseholdID == householdID && l.UserID != 0 {
			ids[l.UserID] = struct{}{}
		}
	}
	result := make([]int64, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func (s *MemoryStore) HistoryLogs(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ChoreLog
	hasOlder := false
	for _, l := range s.logs {
		if readAllowed(ctx, householdID, l.ChoreID) && l.HouseholdID == householdID {
			if logInRange(l, start, end) {
				result = append(result, l)
			}
			if logBeforeRange(l, start) {
				hasOlder = true
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CompletedAt.Equal(result[j].CompletedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CompletedAt.After(result[j].CompletedAt)
	})
	if result == nil {
		result = []ChoreLog{}
	}
	return result, hasOlder, nil
}

func (s *MemoryStore) SearchHistoryLogs(ctx context.Context, householdID int64, query string, limit int) ([]ChoreLog, error) {
	if limit <= 0 {
		limit = 50
	}
	q := strings.ToLower(strings.TrimSpace(query))
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ChoreLog
	for _, l := range s.logs {
		if !readAllowed(ctx, householdID, l.ChoreID) || l.HouseholdID != householdID {
			continue
		}
		note := strings.ToLower(l.Note)
		title := ""
		if l.Title != nil {
			title = strings.ToLower(*l.Title)
		}
		if q == "" || strings.Contains(note, q) || strings.Contains(title, q) {
			result = append(result, l)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CompletedAt.Equal(result[j].CompletedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CompletedAt.After(result[j].CompletedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	if result == nil {
		result = []ChoreLog{}
	}
	return result, nil
}

func (s *MemoryStore) FindLogByIdempotencyKey(_ context.Context, householdID int64, key string) (*ChoreLog, error) {
	if key == "" {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.logs {
		if l.HouseholdID == householdID && l.IdempotencyKey == key {
			cp := l
			return &cp, nil
		}
	}
	return nil, nil
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

func (s *MemoryStore) LatestPerChore(ctx context.Context, householdID int64) (map[int64]ChoreLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := map[int64]ChoreLog{}
	for _, l := range s.logs {
		if !readAllowed(ctx, householdID, l.ChoreID) || l.HouseholdID != householdID {
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

func (s *MemoryStore) CleanupAccount(d *lifecycle.Deletion) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, l := range s.logs {
		if l.UserID == d.UserID || d.Households[l.HouseholdID] || d.Chores[l.ChoreID] {
			d.Logs[id] = true
			delete(s.logs, id)
		}
	}

	s.deleted.Mark(d)
}
