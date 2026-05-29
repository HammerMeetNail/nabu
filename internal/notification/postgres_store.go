package notification

import (
	"context"
	"database/sql"
	"encoding/json"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateNotification(ctx context.Context, n Notification) (Notification, error) {
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO notifications (user_id, type, title, body)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		n.UserID, n.Type, n.Title, n.Body,
	).Scan(&n.ID, &n.CreatedAt)
	return n, err
}

func (s *PostgresStore) ListNotifications(ctx context.Context, userID int64, limit, offset int) ([]Notification, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, type, title, body, is_read, created_at
		 FROM notifications
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

func (s *PostgresStore) GetUnreadCount(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`,
		userID,
	).Scan(&count)
	return count, err
}

func (s *PostgresStore) MarkRead(ctx context.Context, id, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET is_read = true WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	return err
}

func (s *PostgresStore) MarkAllRead(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET is_read = true WHERE user_id = $1`,
		userID,
	)
	return err
}

func (s *PostgresStore) DeleteNotification(ctx context.Context, id, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM notifications WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	return err
}

func (s *PostgresStore) GetReminderPreferences(ctx context.Context, userID int64) (ReminderPreference, error) {
	var p ReminderPreference
	var rawTypes []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, push_enabled, email_enabled,
		        COALESCE(quiet_hours_start, ''), COALESCE(quiet_hours_end, ''), timezone,
		        COALESCE(enabled_push_types, '[]')
		 FROM reminder_preferences WHERE user_id = $1`,
		userID,
	).Scan(&p.UserID, &p.PushEnabled, &p.EmailEnabled, &p.QuietHoursStart, &p.QuietHoursEnd, &p.Timezone, &rawTypes)
	if err == sql.ErrNoRows {
		return ReminderPreference{UserID: userID, PushEnabled: true, Timezone: "UTC", EnabledPushTypes: []string{}}, nil
	}
	if err != nil {
		return p, err
	}
	if len(rawTypes) > 0 {
		if err := json.Unmarshal(rawTypes, &p.EnabledPushTypes); err != nil {
			p.EnabledPushTypes = []string{}
		}
	}
	if p.EnabledPushTypes == nil {
		p.EnabledPushTypes = []string{}
	}
	return p, nil
}

func (s *PostgresStore) UpdateReminderPreferences(ctx context.Context, prefs ReminderPreference) error {
	qhs := nullStr(prefs.QuietHoursStart)
	qhe := nullStr(prefs.QuietHoursEnd)
	types := prefs.EnabledPushTypes
	if types == nil {
		types = []string{}
	}
	rawTypes, err := json.Marshal(types)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO reminder_preferences (user_id, push_enabled, email_enabled, quiet_hours_start, quiet_hours_end, timezone, enabled_push_types)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (user_id) DO UPDATE SET
		   push_enabled = EXCLUDED.push_enabled,
		   email_enabled = EXCLUDED.email_enabled,
		   quiet_hours_start = EXCLUDED.quiet_hours_start,
		   quiet_hours_end = EXCLUDED.quiet_hours_end,
		   timezone = EXCLUDED.timezone,
		   enabled_push_types = EXCLUDED.enabled_push_types`,
		prefs.UserID, prefs.PushEnabled, prefs.EmailEnabled, qhs, qhe, prefs.Timezone, rawTypes,
	)
	return err
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
