package reminder

import (
	"context"
	"database/sql"
	"time"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) GetChoreReminderPrefs(ctx context.Context, userID int64) ([]ChoreReminderPref, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, chore_id, enabled, lead_minutes,
		        COALESCE(quiet_hours_start,''), COALESCE(quiet_hours_end,'')
		 FROM chore_reminder_prefs WHERE user_id = $1`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChoreReminderPref
	for rows.Next() {
		var p ChoreReminderPref
		if err := rows.Scan(&p.UserID, &p.ChoreID, &p.Enabled, &p.LeadMinutes, &p.QuietHoursStart, &p.QuietHoursEnd); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetChoreReminderPref(ctx context.Context, userID, choreID int64) (ChoreReminderPref, error) {
	var p ChoreReminderPref
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, chore_id, enabled, lead_minutes,
		        COALESCE(quiet_hours_start,''), COALESCE(quiet_hours_end,'')
		 FROM chore_reminder_prefs WHERE user_id = $1 AND chore_id = $2`,
		userID, choreID,
	).Scan(&p.UserID, &p.ChoreID, &p.Enabled, &p.LeadMinutes, &p.QuietHoursStart, &p.QuietHoursEnd)
	if err == sql.ErrNoRows {
		return ChoreReminderPref{UserID: userID, ChoreID: choreID, Enabled: false, LeadMinutes: 10}, nil
	}
	return p, err
}

func (s *PostgresStore) UpdateChoreReminderPref(ctx context.Context, pref ChoreReminderPref) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chore_reminder_prefs (user_id, chore_id, enabled, lead_minutes, quiet_hours_start, quiet_hours_end)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id, chore_id) DO UPDATE SET
		   enabled = EXCLUDED.enabled,
		   lead_minutes = EXCLUDED.lead_minutes,
		   quiet_hours_start = EXCLUDED.quiet_hours_start,
		   quiet_hours_end = EXCLUDED.quiet_hours_end`,
		pref.UserID, pref.ChoreID, pref.Enabled, pref.LeadMinutes,
		nullQuietHours(pref.QuietHoursStart), nullQuietHours(pref.QuietHoursEnd))
	return err
}

// nullQuietHours stores an unset quiet-hours bound as NULL (empty = no bound).
func nullQuietHours(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *PostgresStore) HasReminder(ctx context.Context, scheduleID, userID int64, scheduledDate string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM schedule_reminders
		 WHERE schedule_id = $1 AND user_id = $2 AND scheduled_date = $3)`,
		scheduleID, userID, scheduledDate,
	).Scan(&exists)
	return exists, err
}

func (s *PostgresStore) RecordReminder(ctx context.Context, scheduleID, userID int64, scheduledDate string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO schedule_reminders (schedule_id, user_id, scheduled_date)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (schedule_id, user_id, scheduled_date) DO NOTHING`,
		scheduleID, userID, scheduledDate)
	return err
}

func (s *PostgresStore) PurgeOldReminders(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM schedule_reminders WHERE scheduled_date < CURRENT_DATE - INTERVAL '7 days'`)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}

func (s *PostgresStore) ListRecentReminders(ctx context.Context, userID int64, limit int) ([]ReminderRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.schedule_id, s.chore_id, c.name, c.icon, r.scheduled_date::text, r.reminded_at
		 FROM schedule_reminders r
		 JOIN chore_schedules s ON s.id = r.schedule_id
		 LEFT JOIN chores c ON c.id = s.chore_id
		 WHERE r.user_id = $1
		 ORDER BY r.reminded_at DESC, r.schedule_id DESC
		 LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReminderRecord
	for rows.Next() {
		var rec ReminderRecord
		var name, icon sql.NullString
		var remindedAt time.Time
		if err := rows.Scan(&rec.ScheduleID, &rec.ChoreID, &name, &icon, &rec.ScheduledDate, &remindedAt); err != nil {
			return nil, err
		}
		rec.ChoreName = name.String
		rec.ChoreIcon = icon.String
		rec.RemindedAt = remindedAt.UTC()
		out = append(out, rec)
	}
	if out == nil {
		out = []ReminderRecord{}
	}
	return out, rows.Err()
}
