package reminder

import (
	"context"
	"time"
)

type ChoreReminderPref struct {
	UserID      int64 `json:"userId"`
	ChoreID     int64 `json:"choreId"`
	Enabled     bool  `json:"enabled"`
	LeadMinutes int   `json:"leadMinutes"`
	// QuietHoursStart/QuietHoursEnd ("HH:MM", empty = unset) are this user's
	// quiet window for this chore's reminders, on top of the household-level
	// window stored on the chore and the user's global quiet hours.
	QuietHoursStart string `json:"quietHoursStart,omitempty"`
	QuietHoursEnd   string `json:"quietHoursEnd,omitempty"`
}

// ReminderRecord is one delivered schedule reminder, as surfaced by
// ListRecentReminders (the reminder delivery log).
type ReminderRecord struct {
	ScheduleID    int64     `json:"scheduleId"`
	ChoreID       int64     `json:"choreId"`
	ChoreName     string    `json:"choreName"`
	ChoreIcon     string    `json:"choreIcon"`
	ScheduledDate string    `json:"scheduledDate"` // YYYY-MM-DD in the schedule's calendar
	RemindedAt    time.Time `json:"remindedAt"`
}

type Store interface {
	GetChoreReminderPrefs(ctx context.Context, userID int64) ([]ChoreReminderPref, error)
	GetChoreReminderPref(ctx context.Context, userID, choreID int64) (ChoreReminderPref, error)
	UpdateChoreReminderPref(ctx context.Context, prefs ChoreReminderPref) error
	HasReminder(ctx context.Context, scheduleID, userID int64, scheduledDate string) (bool, error)
	RecordReminder(ctx context.Context, scheduleID, userID int64, scheduledDate string) error
	PurgeOldReminders(ctx context.Context) (int64, error)
	// ListRecentReminders returns the caller's most recently delivered
	// schedule reminders, newest first.
	ListRecentReminders(ctx context.Context, userID int64, limit int) ([]ReminderRecord, error)
}
