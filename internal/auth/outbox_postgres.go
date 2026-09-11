package auth

import (
	"context"
	"time"

	"github.com/HammerMeetNail/nabu/internal/mail"
)

func (s *PostgresStore) QueueAuthMail(ctx context.Context, userID int64, message mail.Message, expiresAt time.Time) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO auth_mail_outbox (user_id, recipient, subject, body, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, userID, message.To, message.Subject, message.Body, expiresAt)
	return err
}

func (s *PostgresStore) ClaimAuthMail(ctx context.Context, now, leaseUntil time.Time, leaseID string) (AuthMail, error) {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM auth_mail_outbox WHERE expires_at <= $1`, now); err != nil {
		return AuthMail{}, err
	}
	var job AuthMail
	err := s.q.QueryRowContext(ctx, `UPDATE auth_mail_outbox SET lease_until = $2, lease_id = $3, attempts = attempts + 1
		WHERE id = (SELECT id FROM auth_mail_outbox WHERE next_attempt <= $1 AND expires_at > $1
		AND (lease_until IS NULL OR lease_until <= $1) ORDER BY next_attempt, id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, user_id, recipient, subject, body, expires_at, next_attempt, lease_until, lease_id, attempts`,
		now, leaseUntil, leaseID).Scan(&job.ID, &job.UserID, &job.Message.To, &job.Message.Subject, &job.Message.Body,
		&job.ExpiresAt, &job.NextAttempt, &job.LeaseUntil, &job.LeaseID, &job.Attempts)
	return job, err
}

func (s *PostgresStore) FinishAuthMail(ctx context.Context, job AuthMail, delivered bool, nextAttempt time.Time) error {
	if delivered {
		_, err := s.q.ExecContext(ctx, `DELETE FROM auth_mail_outbox WHERE id = $1 AND lease_id = $2`, job.ID, job.LeaseID)
		return err
	}
	_, err := s.q.ExecContext(ctx, `UPDATE auth_mail_outbox SET lease_until = NULL, lease_id = '', next_attempt = $3
		WHERE id = $1 AND lease_id = $2`, job.ID, job.LeaseID, nextAttempt)
	return err
}
