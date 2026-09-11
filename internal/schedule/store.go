// internal/schedule/store.go

package schedule

import "context"

// Store persists ChoreSchedule records.
type Store interface {
	Create(ctx context.Context, s ChoreSchedule) (ChoreSchedule, error)
	Get(ctx context.Context, id int64) (ChoreSchedule, error)
	ListByHousehold(ctx context.Context, householdID int64) ([]ChoreSchedule, error)
	ListActiveWithTime(ctx context.Context) ([]ChoreSchedule, error)
	Update(ctx context.Context, s ChoreSchedule) (ChoreSchedule, error)
	Delete(ctx context.Context, id int64) error
	DeleteFollowUpSchedulesByChore(ctx context.Context, choreID int64) error
	// ApplyLogEffects atomically applies a log's follow-up change once. A
	// failed attempt can be retried; applied=false means it already committed.
	ApplyLogEffects(ctx context.Context, effects LogEffects) (applied bool, err error)
}

type LogEffects struct {
	LogID, HouseholdID, ChoreID int64
	UpdateFollowUp              bool
	FollowUp                    *ChoreSchedule
	LastFollowUpMinutes         int
}
