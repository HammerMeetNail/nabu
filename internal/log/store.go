package log

import (
	"context"
	"time"
)

type ChoreLog struct {
	ID          int64     `json:"id"`
	HouseholdID int64     `json:"householdId"`
	UserID      int64     `json:"userId"`
	ChoreID     int64     `json:"choreId"`
	CompletedAt time.Time `json:"completedAt"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"createdAt"`
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
	DeleteLog(ctx context.Context, id int64) error
	FindLog(ctx context.Context, householdID, choreID int64, date time.Time) (*ChoreLog, error)
	ListLogs(ctx context.Context, householdID int64, date time.Time) ([]ChoreLog, error)
	ListLogsRange(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, error)
}
