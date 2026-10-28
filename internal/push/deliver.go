package push

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/HammerMeetNail/nabu/internal/diagnostics"
)

// Target is one registered device: a browser subscription or an iOS device.
type Target interface {
	// Key identifies the registration in a job's Done list.
	Key() string
}

// Outcome is what one delivery to one device came to.
type Outcome int

const (
	// Delivered: the provider accepted the notification.
	Delivered Outcome = iota
	// GiveUp: the device can't have it (the provider refused it, the device
	// is gone, or its registration or session no longer authorizes it).
	GiveUp
	// Retry: a transient failure, such as a network error, 429 or 5xx.
	Retry
)

// Message is what a job delivers to each device.
type Message struct {
	UserID int64
	Title  string
	Body   string
	Data   map[string]any
	// CollapseID lets a provider show a retried delivery once.
	CollapseID string
}

// Channel delivers to one kind of device.
type Channel interface {
	// Targets lists the user's registrations whose sessions are current.
	Targets(ctx context.Context, userID int64) ([]Target, error)
	// Deliver sends to one device while holding its registration and session,
	// so a sign-out waits for the send and none starts after it. It removes a
	// device the provider reports gone.
	Deliver(ctx context.Context, target Target, m Message) Outcome
}

// ErrRevoked reports a registration or session that no longer authorizes
// delivery.
var ErrRevoked = errors.New("push registration revoked")

// ErrNoAccess reports a recipient who may no longer see the notification.
var ErrNoAccess = errors.New("push recipient no longer has access")

// Guard runs deliver while the job's recipient may still see it: a current
// member of the household who can see its activity. The membership stays
// fixed until deliver returns, so a removal waits for one send. It returns
// ErrNoAccess when the recipient may not see it.
type Guard func(ctx context.Context, job Job, deliver func() error) error

const (
	// A job sends to one user's devices, each send bounded to 10s.
	jobLease = 3 * time.Minute
	// Stop starting sends with time left to record progress.
	jobBudget = jobLease - 30*time.Second
)

// Deliverer delivers push jobs. Every instance runs it; claims skip the jobs
// another worker holds.
type Deliverer struct {
	jobs     JobStore
	guard    Guard
	channels []Channel
	now      func() time.Time
	wake     chan struct{}
}

func NewDeliverer(jobs JobStore, guard Guard, channels ...Channel) *Deliverer {
	return &Deliverer{jobs: jobs, guard: guard, channels: channels, now: time.Now, wake: make(chan struct{}, 1)}
}

// Wake asks a running worker to look for due jobs now.
func (d *Deliverer) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Run delivers due jobs until ctx ends. Several workers may share one
// Deliverer.
func (d *Deliverer) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		d.DeliverPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-d.wake:
		}
	}
}

// DeliverPending delivers up to 20 due jobs.
func (d *Deliverer) DeliverPending(ctx context.Context) {
	for i := 0; i < 20 && ctx.Err() == nil; i++ {
		now := d.now()
		job, err := d.jobs.ClaimPush(ctx, now, now.Add(jobLease), diagnostics.RequestID())
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		if err != nil {
			log.Printf("push: claim failed class=%s", diagnostics.ErrorClass(err))
			return
		}
		// Another worker may take the next job while this one sends.
		d.Wake()
		var retryAt time.Time
		if d.deliver(ctx, &job) {
			retryAt = d.now().Add(retryDelay(job.Attempts))
		}
		// Record progress even at shutdown, so another instance can take the
		// rest at once instead of waiting out the lease.
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		if err := d.jobs.FinishPush(finishCtx, job, retryAt); err != nil {
			log.Printf("push: finish failed job=%d class=%s", job.ID, diagnostics.ErrorClass(err))
		}
		cancel()
	}
}

// deliver sends the job to each of its recipient's devices not yet done, and
// reports whether any should be tried again.
func (d *Deliverer) deliver(ctx context.Context, job *Job) (retry bool) {
	ctx, cancel := context.WithTimeout(ctx, jobBudget)
	defer cancel()
	m := Message{UserID: job.UserID, Title: job.Title, Body: job.Body, Data: job.Data,
		CollapseID: "nabu-" + strconv.FormatInt(job.ID, 10)}
	done := make(map[string]bool, len(job.Done))
	for _, key := range job.Done {
		done[key] = true
	}
	for _, channel := range d.channels {
		targets, err := channel.Targets(ctx, job.UserID)
		if err != nil {
			retry = true
			continue
		}
		for _, target := range targets {
			key := target.Key()
			if done[key] {
				continue
			}
			if ctx.Err() != nil {
				return true
			}
			outcome := Retry
			err := d.guard(ctx, *job, func() error {
				outcome = channel.Deliver(ctx, target, m)
				return nil
			})
			if errors.Is(err, ErrNoAccess) {
				log.Printf("push: recipient lost access job=%d", job.ID)
				return false
			}
			if err != nil || outcome == Retry {
				retry = true
				continue
			}
			done[key] = true
			job.Done = append(job.Done, key)
		}
	}
	return retry
}

// retryDelay backs off from 10s to 2 minutes, well inside a job's lifetime.
func retryDelay(attempts int) time.Duration {
	return min(10*time.Second<<min(max(attempts-1, 0), 4), 2*time.Minute)
}
