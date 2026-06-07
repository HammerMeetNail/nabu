package reminder

import "context"

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
	HasReminder(ctx context.Context, scheduleID, userID int64, scheduledDate string) (bool, error)
	RecordReminder(ctx context.Context, scheduleID, userID int64, scheduledDate string) error
	PurgeOldReminders(ctx context.Context) (int64, error)
}
