package auth

import (
	"context"
	"database/sql"
	"time"

	"github.com/HammerMeetNail/nabu/internal/mail"
)

func (s *MemoryStore) QueueAuthMail(_ context.Context, userID int64, message mail.Message, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID()
	s.mailByID[id] = AuthMail{ID: id, UserID: userID, Message: message, ExpiresAt: expiresAt}
	return nil
}

func (s *MemoryStore) ClaimAuthMail(_ context.Context, now, leaseUntil time.Time, leaseID string) (AuthMail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var found AuthMail
	for id, job := range s.mailByID {
		if !now.Before(job.ExpiresAt) {
			delete(s.mailByID, id)
			continue
		}
		if now.Before(job.NextAttempt) || now.Before(job.LeaseUntil) {
			continue
		}
		if found.ID == 0 || job.ID < found.ID {
			found = job
		}
	}
	if found.ID == 0 {
		return AuthMail{}, sql.ErrNoRows
	}
	found.LeaseUntil, found.LeaseID = leaseUntil, leaseID
	found.Attempts++
	s.mailByID[found.ID] = found
	return found, nil
}

func (s *MemoryStore) FinishAuthMail(_ context.Context, job AuthMail, delivered bool, nextAttempt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.mailByID[job.ID]
	if !ok || current.LeaseID != job.LeaseID {
		return nil
	}
	if delivered {
		delete(s.mailByID, job.ID)
		return nil
	}
	current.LeaseID, current.LeaseUntil, current.NextAttempt = "", time.Time{}, nextAttempt
	s.mailByID[job.ID] = current
	return nil
}
