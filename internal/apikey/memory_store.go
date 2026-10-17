package apikey

import (
	"context"
	"sort"
	"sync"
	"time"
)

type memoryStore struct {
	mu   sync.Mutex
	keys map[string]Record
}

// NewMemoryStore returns an in-memory Store for tests and the no-database mode.
func NewMemoryStore() Store {
	return &memoryStore{keys: map[string]Record{}}
}

func clone(rec Record) Record {
	rec.TokenHash = append([]byte(nil), rec.TokenHash...)
	if rec.LastUsedAt != nil {
		t := *rec.LastUsedAt
		rec.LastUsedAt = &t
	}
	if rec.RevokedAt != nil {
		t := *rec.RevokedAt
		rec.RevokedAt = &t
	}
	return rec
}

func (s *memoryStore) Create(_ context.Context, rec Record, maxActive int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	active := 0
	for _, k := range s.keys {
		if k.UserID == rec.UserID && k.RevokedAt == nil && k.AuthVersion == rec.AuthVersion {
			active++
		}
	}
	if active >= maxActive {
		return ErrTooManyKeys
	}
	s.keys[rec.ID] = clone(rec)
	return nil
}

func (s *memoryStore) List(_ context.Context, userID int64) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, 0)
	for _, k := range s.keys {
		if k.UserID == userID {
			out = append(out, clone(k))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *memoryStore) Get(_ context.Context, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return clone(k), nil
}

func (s *memoryStore) Revoke(_ context.Context, userID int64, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[id]
	if !ok || k.UserID != userID {
		return ErrNotFound
	}
	if k.RevokedAt == nil {
		k.RevokedAt = &now
		s.keys[id] = k
	}
	return nil
}

func (s *memoryStore) Touch(_ context.Context, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[id]
	if !ok {
		return nil
	}
	if k.LastUsedAt == nil || now.Sub(*k.LastUsedAt) >= time.Hour {
		k.LastUsedAt = &now
		s.keys[id] = k
	}
	return nil
}
