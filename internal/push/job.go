package push

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/HammerMeetNail/nabu/internal/lifecycle"
)

// JobTTL bounds how long a notification may wait for delivery: one that
// arrives later than this is noise. A reminder's job expires sooner, with the
// window in which it is still due.
const JobTTL = 15 * time.Minute

// Job is one notification for one user, delivered to each of their devices.
// It is durable: any instance claims a due job under a lease, so a process that
// stops part-way leaves the rest to be retried.
type Job struct {
	ID          int64
	UserID      int64
	HouseholdID int64
	// ChoreID is zero when the notification is about the household rather
	// than one activity.
	ChoreID   int64
	Type      string
	Title     string
	Body      string
	Data      map[string]any
	DedupeKey string
	// Done lists the devices (Target.Key) delivered to or given up on.
	Done      []string
	Attempts  int
	LeaseID   string
	ExpiresAt time.Time
}

type JobStore interface {
	// EnqueuePush adds a job. A job whose dedupe key is already queued is
	// dropped.
	EnqueuePush(ctx context.Context, job Job) error
	// ClaimPush leases the next due job, or returns sql.ErrNoRows.
	ClaimPush(ctx context.Context, now, leaseUntil time.Time, leaseID string) (Job, error)
	// FinishPush records the job's progress under its lease. A zero retryAt
	// finishes it.
	FinishPush(ctx context.Context, job Job, retryAt time.Time) error
}

// Execer is the part of *sql.DB and *sql.Tx that InsertJob needs.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// InsertJob adds a job through q, which may be the caller's transaction: a
// reminder's record and its push commit together.
func InsertJob(ctx context.Context, q Execer, job Job) error {
	data := job.Data
	if data == nil {
		data = map[string]any{}
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO push_jobs (user_id, household_id, chore_id, type, title, body, data, dedupe_key, expires_at)
		VALUES ($1, $2, NULLIF($3, 0), $4, $5, $6, $7, NULLIF($8, ''), $9) ON CONFLICT (dedupe_key) DO NOTHING`,
		job.UserID, job.HouseholdID, job.ChoreID, job.Type, job.Title, job.Body, encoded, job.DedupeKey, job.ExpiresAt)
	return err
}

type PostgresJobStore struct {
	db *sql.DB
}

func NewPostgresJobStore(db *sql.DB) *PostgresJobStore {
	return &PostgresJobStore{db: db}
}

func (s *PostgresJobStore) EnqueuePush(ctx context.Context, job Job) error {
	return InsertJob(ctx, s.db, job)
}

func (s *PostgresJobStore) ClaimPush(ctx context.Context, now, leaseUntil time.Time, leaseID string) (Job, error) {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM push_jobs WHERE expires_at <= $1`, now); err != nil {
		return Job{}, err
	}
	var job Job
	var data, done []byte
	err := s.db.QueryRowContext(ctx, `UPDATE push_jobs SET lease_until = $2, lease_id = $3, attempts = attempts + 1
		WHERE id = (SELECT id FROM push_jobs WHERE next_attempt <= $1 AND expires_at > $1
		AND (lease_until IS NULL OR lease_until <= $1) ORDER BY next_attempt, id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, user_id, household_id, COALESCE(chore_id, 0), type, title, body, data, COALESCE(dedupe_key, ''),
		done, attempts, lease_id, expires_at`, now, leaseUntil, leaseID).Scan(&job.ID, &job.UserID, &job.HouseholdID,
		&job.ChoreID, &job.Type, &job.Title, &job.Body, &data, &job.DedupeKey, &done, &job.Attempts, &job.LeaseID, &job.ExpiresAt)
	if err != nil {
		return Job{}, err
	}
	if err := json.Unmarshal(data, &job.Data); err != nil {
		return Job{}, err
	}
	if err := json.Unmarshal(done, &job.Done); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *PostgresJobStore) FinishPush(ctx context.Context, job Job, retryAt time.Time) error {
	done := job.Done
	if done == nil {
		done = []string{}
	}
	encoded, err := json.Marshal(done)
	if err != nil {
		return err
	}
	next := any(retryAt)
	if retryAt.IsZero() {
		next = "infinity"
	}
	_, err = s.db.ExecContext(ctx, `UPDATE push_jobs SET lease_until = NULL, lease_id = '', done = $3, next_attempt = $4
		WHERE id = $1 AND lease_id = $2`, job.ID, job.LeaseID, encoded, next)
	return err
}

// MemoryJobStore backs tests and the zero-DB development mode. Delivery
// workers claim jobs while HTTP handlers and the scheduler add them, so every
// method takes mu.
type MemoryJobStore struct {
	mu      sync.Mutex
	deleted lifecycle.Tombstones
	seq     int64
	jobs    []memoryJob
}

type memoryJob struct {
	job         Job
	nextAttempt time.Time
	leaseUntil  time.Time
	finished    bool
}

func NewMemoryJobStore() *MemoryJobStore {
	return &MemoryJobStore{}
}

func (s *MemoryJobStore) EnqueuePush(_ context.Context, job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleted.Check(job.UserID, job.HouseholdID, job.ChoreID, 0, 0); err != nil {
		return err
	}
	if job.DedupeKey != "" {
		for _, queued := range s.jobs {
			if queued.job.DedupeKey == job.DedupeKey {
				return nil
			}
		}
	}
	s.seq++
	job.ID, job.Done, job.Attempts, job.LeaseID = s.seq, nil, 0, ""
	job.Data = copyData(job.Data)
	s.jobs = append(s.jobs, memoryJob{job: job})
	return nil
}

func (s *MemoryJobStore) ClaimPush(_ context.Context, now, leaseUntil time.Time, leaseID string) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.jobs[:0]
	for _, queued := range s.jobs {
		if queued.job.ExpiresAt.After(now) {
			kept = append(kept, queued)
		}
	}
	s.jobs = kept
	for i := range s.jobs {
		queued := &s.jobs[i]
		if queued.finished || queued.nextAttempt.After(now) || queued.leaseUntil.After(now) {
			continue
		}
		queued.leaseUntil = leaseUntil
		queued.job.LeaseID = leaseID
		queued.job.Attempts++
		job := queued.job
		job.Data = copyData(job.Data)
		job.Done = append([]string(nil), job.Done...)
		return job, nil
	}
	return Job{}, sql.ErrNoRows
}

func (s *MemoryJobStore) FinishPush(_ context.Context, job Job, retryAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		queued := &s.jobs[i]
		if queued.job.ID != job.ID || queued.job.LeaseID != job.LeaseID {
			continue
		}
		queued.job.Done = append([]string(nil), job.Done...)
		queued.job.LeaseID = ""
		queued.leaseUntil = time.Time{}
		queued.nextAttempt = retryAt
		queued.finished = retryAt.IsZero()
		return nil
	}
	// The lease expired and another worker took the job, as in PostgreSQL.
	return nil
}

// Pending returns the jobs not yet finished, for tests.
func (s *MemoryJobStore) Pending() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Job
	for _, queued := range s.jobs {
		if !queued.finished {
			job := queued.job
			job.Data = copyData(job.Data)
			out = append(out, job)
		}
	}
	return out
}

func (s *MemoryJobStore) CleanupAccount(d *lifecycle.Deletion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.jobs[:0]
	for _, queued := range s.jobs {
		if queued.job.UserID != d.UserID && !d.Households[queued.job.HouseholdID] && !d.Chores[queued.job.ChoreID] {
			kept = append(kept, queued)
		}
	}
	s.jobs = kept
	s.deleted.Mark(d)
}

func copyData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		out[k] = v
	}
	return out
}
