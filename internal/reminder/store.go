package reminder

import (
	"context"

	"github.com/HammerMeetNail/nabu/internal/push"
)

type ChoreReminderPref struct {
	UserID      int64 `json:"userId"`
	ChoreID     int64 `json:"choreId"`
	Enabled     bool  `json:"enabled"`
	LeadMinutes int   `json:"leadMinutes"`
}

type Store interface {
	GetChoreReminderPrefs(ctx context.Context, userID int64) ([]ChoreReminderPref, error)
	GetChoreReminderPref(ctx context.Context, userID, choreID int64) (ChoreReminderPref, error)
	UpdateChoreReminderPref(ctx context.Context, prefs ChoreReminderPref) error
	// ClaimReminder records that a schedule reminds userID on scheduledDate
	// and queues its push, both or neither. It reports false when that
	// reminder was already claimed.
	ClaimReminder(ctx context.Context, scheduleID, userID int64, scheduledDate string, job push.Job) (bool, error)
	PurgeOldReminders(ctx context.Context) (int64, error)
}
