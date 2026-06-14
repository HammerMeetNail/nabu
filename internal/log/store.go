package log

import (
	"context"
	"time"
)

type ChoreLog struct {
	ID               int64          `json:"id"`
	HouseholdID      int64          `json:"householdId"`
	UserID           int64          `json:"userId"`
	ChoreID          int64          `json:"choreId"`
	CompletedAt      time.Time      `json:"completedAt"`
	Title            *string        `json:"title,omitempty"`
	Note             string         `json:"note"`
	Indicators       []string       `json:"indicators"`
	IndicatorVolumes map[string]int `json:"indicatorVolumes,omitempty"`
	SlotHour         *int           `json:"slotHour,omitempty"` // calendar hour (0-23) the log was made from; nil = anytime
	CreatedAt        time.Time      `json:"createdAt"`
	LogDate          *string        `json:"-"`
	VolumeML         *int           `json:"volumeML,omitempty"`
	Rating           *int           `json:"rating,omitempty"`
}

type DailySummary struct {
	Date        string         `json:"date"`
	TotalChores int            `json:"totalChores"`
	ChoresDone  int            `json:"choresDone"`
	ByUser      map[int64]int  `json:"byUser"`
	ByCategory  map[string]int `json:"byCategory"`
}

type Store interface {
	CreateLog(ctx context.Context, log ChoreLog) (ChoreLog, error)
	GetLog(ctx context.Context, id int64) (ChoreLog, error)
	UpdateLog(ctx context.Context, log ChoreLog) error
	DeleteLog(ctx context.Context, id int64) error
	FindLog(ctx context.Context, householdID, choreID int64, date time.Time) (*ChoreLog, error)
	ListLogs(ctx context.Context, householdID int64, date time.Time) ([]ChoreLog, error)
	ListLogsRange(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, error)
	// LatestPerChore returns the most recent log for each chore in the household.
	// Keys are chore IDs; chores with no logs are absent from the map.
	LatestPerChore(ctx context.Context, householdID int64) (map[int64]ChoreLog, error)
	// HistoryLogs returns logs between start and end (exclusive), ordered
	// newest-first, and whether older logs exist before start.
	HistoryLogs(ctx context.Context, householdID int64, start, end time.Time) (logs []ChoreLog, hasMore bool, err error)
}
