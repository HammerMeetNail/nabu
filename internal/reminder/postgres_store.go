package reminder

import (
	"context"
	"database/sql"
	"errors"

	"github.com/HammerMeetNail/nabu/internal/push"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) GetChoreReminderPrefs(ctx context.Context, userID int64) ([]ChoreReminderPref, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, chore_id, enabled, lead_minutes
		 FROM chore_reminder_prefs WHERE user_id = $1`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChoreReminderPref
	for rows.Next() {
		var p ChoreReminderPref
		if err := rows.Scan(&p.UserID, &p.ChoreID, &p.Enabled, &p.LeadMinutes); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetChoreReminderPref(ctx context.Context, userID, choreID int64) (ChoreReminderPref, error) {
	var p ChoreReminderPref
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, chore_id, enabled, lead_minutes
		 FROM chore_reminder_prefs WHERE user_id = $1 AND chore_id = $2`,
		userID, choreID,
	).Scan(&p.UserID, &p.ChoreID, &p.Enabled, &p.LeadMinutes)
	if err == sql.ErrNoRows {
		return ChoreReminderPref{UserID: userID, ChoreID: choreID, Enabled: false, LeadMinutes: 10}, nil
	}
	return p, err
}

func (s *PostgresStore) UpdateChoreReminderPref(ctx context.Context, pref ChoreReminderPref) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chore_reminder_prefs (user_id, chore_id, enabled, lead_minutes)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, chore_id) DO UPDATE SET
		   enabled = EXCLUDED.enabled,
		   lead_minutes = EXCLUDED.lead_minutes`,
		pref.UserID, pref.ChoreID, pref.Enabled, pref.LeadMinutes)
	return err
}

// ClaimReminder commits the reminder's record and its push job together, so a
// reminder is queued once and never lost between the two.
func (s *PostgresStore) ClaimReminder(ctx context.Context, scheduleID, userID int64, scheduledDate string, job push.Job) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var claimed bool
	err = tx.QueryRowContext(ctx,
		`INSERT INTO schedule_reminders (schedule_id, user_id, scheduled_date)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (schedule_id, user_id, scheduled_date) DO NOTHING
		 RETURNING true`,
		scheduleID, userID, scheduledDate).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := push.InsertJob(ctx, tx, job); err != nil {
		return false, err
	}
	return true, tx.Commit()
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
