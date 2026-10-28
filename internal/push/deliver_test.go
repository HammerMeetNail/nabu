package push

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/testsync"
)

type fakeTarget string

func (t fakeTarget) Key() string { return string(t) }

// fakeChannel delivers to scripted devices. outcomes lists each device's
// answers in turn; the last one repeats, and none means Delivered.
type fakeChannel struct {
	mu         sync.Mutex
	devices    map[int64][]string
	outcomes   map[string][]Outcome
	targetsErr error
	sent       []string
	messages   []Message
	block      chan struct{}
	entered    chan struct{}
}

func (c *fakeChannel) Targets(_ context.Context, userID int64) ([]Target, error) {
	if c.targetsErr != nil {
		return nil, c.targetsErr
	}
	var targets []Target
	for _, key := range c.devices[userID] {
		targets = append(targets, fakeTarget(key))
	}
	return targets, nil
}

func (c *fakeChannel) Deliver(ctx context.Context, target Target, m Message) Outcome {
	if c.block != nil {
		c.entered <- struct{}{}
		select {
		case <-c.block:
		case <-ctx.Done():
			return Retry
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := target.Key()
	c.sent = append(c.sent, key)
	c.messages = append(c.messages, m)
	answers := c.outcomes[key]
	if len(answers) == 0 {
		return Delivered
	}
	outcome := answers[0]
	if len(answers) > 1 {
		c.outcomes[key] = answers[1:]
	}
	return outcome
}

func (c *fakeChannel) sends() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sent...)
}

type deliveryFixture struct {
	jobs    *MemoryJobStore
	channel *fakeChannel
	d       *Deliverer
	guarded []int64
	guard   error
	now     time.Time
}

func newDelivery(t *testing.T, devices map[int64][]string) *deliveryFixture {
	t.Helper()
	f := &deliveryFixture{jobs: NewMemoryJobStore(), channel: &fakeChannel{devices: devices, outcomes: map[string][]Outcome{}},
		now: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)}
	guard := func(_ context.Context, job Job, deliver func() error) error {
		f.guarded = append(f.guarded, job.UserID)
		if f.guard != nil {
			return f.guard
		}
		return deliver()
	}
	f.d = NewDeliverer(f.jobs, guard, f.channel)
	f.d.now = func() time.Time { return f.now }
	return f
}

func (f *deliveryFixture) enqueue(t *testing.T, job Job) {
	t.Helper()
	if job.ExpiresAt.IsZero() {
		job.ExpiresAt = f.now.Add(JobTTL)
	}
	if err := f.jobs.EnqueuePush(context.Background(), job); err != nil {
		t.Fatal(err)
	}
}

func TestDelivererSendsToEveryDeviceOnce(t *testing.T) {
	f := newDelivery(t, map[int64][]string{1: {"phone", "laptop"}, 2: {"tablet"}})
	f.enqueue(t, Job{UserID: 1, HouseholdID: 9, Title: "🍽️ Dishes", Body: "Alice logged this", Data: map[string]any{"type": "chore_logged"}})
	f.d.DeliverPending(context.Background())
	if got := f.channel.sends(); !slices.Equal(got, []string{"phone", "laptop"}) {
		t.Fatalf("sent to %v", got)
	}
	m := f.channel.messages[0]
	if m.UserID != 1 || m.Title != "🍽️ Dishes" || m.Body != "Alice logged this" || m.Data["type"] != "chore_logged" || m.CollapseID != "nabu-1" {
		t.Fatalf("message = %+v", m)
	}
	if !slices.Equal(f.guarded, []int64{1, 1}) {
		t.Fatalf("each send is guarded on its own: %v", f.guarded)
	}
	if len(f.jobs.Pending()) != 0 {
		t.Fatal("delivered job left pending")
	}
	f.now = f.now.Add(time.Hour)
	f.d.DeliverPending(context.Background())
	if len(f.channel.sends()) != 2 {
		t.Fatal("finished job delivered again")
	}
}

func TestDelivererRetriesOnlyTheDevicesThatFailed(t *testing.T) {
	f := newDelivery(t, map[int64][]string{1: {"phone", "laptop", "old"}})
	f.channel.outcomes["laptop"] = []Outcome{Retry, Retry, Delivered}
	f.channel.outcomes["old"] = []Outcome{GiveUp}
	f.enqueue(t, Job{UserID: 1, HouseholdID: 9, Title: "t"})
	ctx := context.Background()
	f.d.DeliverPending(ctx)
	pending := f.jobs.Pending()
	if len(pending) != 1 || !slices.Equal(pending[0].Done, []string{"phone", "old"}) {
		t.Fatalf("progress after a failure = %+v", pending)
	}
	// Not due until its back-off passes: 10s, then 20s.
	f.now = f.now.Add(9 * time.Second)
	f.d.DeliverPending(ctx)
	if got := len(f.channel.sends()); got != 3 {
		t.Fatalf("retried before its delay: %d sends", got)
	}
	f.now = f.now.Add(time.Second)
	f.d.DeliverPending(ctx)
	f.now = f.now.Add(20 * time.Second)
	f.d.DeliverPending(ctx)
	if got := f.channel.sends(); !slices.Equal(got, []string{"phone", "laptop", "old", "laptop", "laptop"}) {
		t.Fatalf("sends = %v", got)
	}
	if len(f.jobs.Pending()) != 0 {
		t.Fatal("job kept after every device was done")
	}
}

func TestDelivererStopsWhenTheRecipientLosesAccess(t *testing.T) {
	f := newDelivery(t, map[int64][]string{1: {"phone", "laptop"}})
	f.guard = errors.Join(ErrNoAccess, errors.New("removed from the household"))
	f.enqueue(t, Job{UserID: 1, HouseholdID: 9, Title: "t"})
	f.d.DeliverPending(context.Background())
	if len(f.channel.sends()) != 0 || len(f.guarded) != 1 {
		t.Fatalf("sends=%v guarded=%v", f.channel.sends(), f.guarded)
	}
	if len(f.jobs.Pending()) != 0 {
		t.Fatal("job kept for a recipient who lost access")
	}
}

func TestDelivererRetriesTransientFailures(t *testing.T) {
	for name, setup := range map[string]func(*deliveryFixture){
		"guard":   func(f *deliveryFixture) { f.guard = errors.New("database unavailable") },
		"targets": func(f *deliveryFixture) { f.channel.targetsErr = errors.New("database unavailable") },
	} {
		t.Run(name, func(t *testing.T) {
			f := newDelivery(t, map[int64][]string{1: {"phone"}})
			setup(f)
			f.enqueue(t, Job{UserID: 1, HouseholdID: 9, Title: "t"})
			f.d.DeliverPending(context.Background())
			pending := f.jobs.Pending()
			if len(pending) != 1 || len(pending[0].Done) != 0 || len(f.channel.sends()) != 0 {
				t.Fatalf("pending=%+v sends=%v", pending, f.channel.sends())
			}
		})
	}
}

func TestDelivererDropsExpiredJobs(t *testing.T) {
	f := newDelivery(t, map[int64][]string{1: {"phone"}})
	f.enqueue(t, Job{UserID: 1, HouseholdID: 9, Title: "t", ExpiresAt: f.now.Add(time.Minute)})
	f.now = f.now.Add(time.Minute)
	f.d.DeliverPending(context.Background())
	if len(f.channel.sends()) != 0 || len(f.jobs.Pending()) != 0 {
		t.Fatal("expired job delivered or kept")
	}
}

// A shutdown during a send gives up the lease, so another instance can retry
// the job at once.
func TestDelivererReleasesItsJobAtShutdown(t *testing.T) {
	f := newDelivery(t, map[int64][]string{1: {"phone"}})
	f.channel.block, f.channel.entered = make(chan struct{}), make(chan struct{}, 1)
	f.enqueue(t, Job{UserID: 1, HouseholdID: 9, Title: "t"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.d.DeliverPending(ctx); close(done) }()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	testsync.Receive(t, waitCtx, f.channel.entered)
	cancel()
	testsync.Receive(t, waitCtx, done)
	f.now = f.now.Add(10 * time.Second)
	f.channel.block = nil
	f.d.DeliverPending(context.Background())
	if got := f.channel.sends(); !slices.Equal(got, []string{"phone"}) {
		t.Fatalf("sends after restart = %v", got)
	}
}

func TestMemoryJobStoreLeasesAndDedupes(t *testing.T) {
	ctx := context.Background()
	jobs := NewMemoryJobStore()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	job := Job{UserID: 1, HouseholdID: 9, Title: "t", DedupeKey: "event:1:1", ExpiresAt: now.Add(JobTTL)}
	for range 2 {
		if err := jobs.EnqueuePush(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	claimed, err := jobs.ClaimPush(ctx, now, now.Add(jobLease), "first")
	if err != nil || claimed.Attempts != 1 {
		t.Fatalf("claim = %+v %v", claimed, err)
	}
	if _, err := jobs.ClaimPush(ctx, now, now.Add(jobLease), "second"); err == nil {
		t.Fatal("a leased job, or its duplicate, was claimed again")
	}
	// A lease that runs out lets another worker take the job.
	retaken, err := jobs.ClaimPush(ctx, now.Add(jobLease), now.Add(2*jobLease), "second")
	if err != nil || retaken.ID != claimed.ID || retaken.Attempts != 2 {
		t.Fatalf("retake = %+v %v", retaken, err)
	}
	// The first worker's late finish no longer counts.
	if err := jobs.FinishPush(ctx, claimed, time.Time{}); err != nil || len(jobs.Pending()) != 1 {
		t.Fatalf("stale finish err=%v pending=%d", err, len(jobs.Pending()))
	}
	if err := jobs.FinishPush(ctx, retaken, time.Time{}); err != nil || len(jobs.Pending()) != 0 {
		t.Fatalf("finish err=%v pending=%d", err, len(jobs.Pending()))
	}
	// A finished job keeps its key until it expires.
	if err := jobs.EnqueuePush(ctx, job); err != nil || len(jobs.Pending()) != 0 {
		t.Fatal("finished job's duplicate was queued")
	}
}

func TestRetryDelayBacksOff(t *testing.T) {
	var got []time.Duration
	for attempts := 1; attempts <= 6; attempts++ {
		got = append(got, retryDelay(attempts))
	}
	want := []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 2 * time.Minute, 2 * time.Minute}
	if !slices.Equal(got, want) {
		t.Fatalf("delays = %v", got)
	}
}
