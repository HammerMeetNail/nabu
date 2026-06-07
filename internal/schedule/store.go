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
}
