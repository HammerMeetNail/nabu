package log

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"strings"
	"time"
)

// nullIntToPtr converts a sql.NullInt64 to *int (nil when not valid).
func nullIntToPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// ptrToNullInt64 converts a *int to sql.NullInt64.
func ptrToNullInt64(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

// nullStr converts a string to sql.NullString (NULL when empty).
func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullStrPtr converts a *string to sql.NullString (NULL when nil or empty).
func nullStrPtr(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateLog(ctx context.Context, log ChoreLog) (ChoreLog, error) {
	indJSON, _ := json.Marshal(nilToEmptyLog(log.Indicators))
	var indVolJSON string
	if len(log.IndicatorVolumes) > 0 {
		b, _ := json.Marshal(log.IndicatorVolumes)
		indVolJSON = string(b)
	}
	var logDate sql.NullString
	if log.LogDate != nil {
		logDate = sql.NullString{String: *log.LogDate, Valid: true}
	}
	var title sql.NullString
	if log.Title != nil {
		title = sql.NullString{String: *log.Title, Valid: true}
	}
	var idemKey sql.NullString
	if log.IdempotencyKey != "" {
		idemKey = sql.NullString{String: log.IdempotencyKey, Valid: true}
	}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO chore_logs (household_id, user_id, chore_id, completed_at, note, indicators, slot_hour, log_date, volume_ml, indicator_volumes, rating, title, idempotency_key, duration_seconds, subject, idempotency_actor_id, idempotency_hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING id, created_at
	`, log.HouseholdID, log.UserID, log.ChoreID, log.CompletedAt, log.Note, string(indJSON), ptrToNullInt64(log.SlotHour), logDate, ptrToNullInt64(log.VolumeML), nullStr(indVolJSON), ptrToNullInt64(log.Rating), title, idemKey, ptrToNullInt64(log.DurationSeconds), nullStrPtr(log.Subject), log.IdempotencyActorID, log.IdempotencyHash).Scan(&log.ID, &log.CreatedAt)
	return log, err
}

func (s *PostgresStore) ListLogUserIDs(ctx context.Context, householdID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT user_id FROM chore_logs
		WHERE household_id = $1 AND user_id IS NOT NULL
		ORDER BY user_id
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *PostgresStore) FindLogByIdempotencyKey(ctx context.Context, householdID int64, key string) (*ChoreLog, error) {
	if key == "" {
		return nil, nil
	}
	var l ChoreLog
	var indJSON string
	var indVolJSON sql.NullString
	var slotHour sql.NullInt64
	var logDate sql.NullString
	var volumeML sql.NullInt64
	var rating sql.NullInt64
	var title sql.NullString
	var durationSec sql.NullInt64
	var subjectSQL sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), COALESCE(indicators,'[]'), slot_hour, created_at, log_date, volume_ml, indicator_volumes::text, rating, COALESCE(title,''), duration_seconds, subject, COALESCE(idempotency_actor_id,0), COALESCE(idempotency_hash,'')
		FROM chore_logs WHERE household_id = $1 AND idempotency_key = $2
		LIMIT 1
	`, householdID, key).Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &indJSON, &slotHour, &l.CreatedAt, &logDate, &volumeML, &indVolJSON, &rating, &title, &durationSec, &subjectSQL, &l.IdempotencyActorID, &l.IdempotencyHash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(indJSON), &l.Indicators)
	if l.Indicators == nil {
		l.Indicators = []string{}
	}
	l.SlotHour = nullIntToPtr(slotHour)
	if logDate.Valid {
		l.LogDate = &logDate.String
	}
	l.VolumeML = nullIntToPtr(volumeML)
	l.Rating = nullIntToPtr(rating)
	l.DurationSeconds = nullIntToPtr(durationSec)
	if subjectSQL.Valid {
		l.Subject = &subjectSQL.String
	}
	if title.Valid {
		l.Title = &title.String
	}
	if indVolJSON.Valid && indVolJSON.String != "" {
		_ = json.Unmarshal([]byte(indVolJSON.String), &l.IndicatorVolumes)
	}
	l.IdempotencyKey = key
	return &l, nil
}

func (s *PostgresStore) GetLog(ctx context.Context, id int64) (ChoreLog, error) {
	var l ChoreLog
	var indJSON string
	var indVolJSON sql.NullString
	var slotHour sql.NullInt64
	var logDate sql.NullString
	var volumeML sql.NullInt64
	var rating sql.NullInt64
	var title sql.NullString
	var durationSec sql.NullInt64
	var subjectSQL sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), COALESCE(indicators,'[]'), slot_hour, created_at, log_date, volume_ml, indicator_volumes::text, rating, COALESCE(title,''), duration_seconds, subject FROM chore_logs WHERE id = $1`, id).Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &indJSON, &slotHour, &l.CreatedAt, &logDate, &volumeML, &indVolJSON, &rating, &title, &durationSec, &subjectSQL)
	if err == sql.ErrNoRows {
		return ChoreLog{}, ErrNotFound
	}
	if err == nil {
		_ = json.Unmarshal([]byte(indJSON), &l.Indicators)
		if l.Indicators == nil {
			l.Indicators = []string{}
		}
		l.SlotHour = nullIntToPtr(slotHour)
		if logDate.Valid {
			l.LogDate = &logDate.String
		}
		l.VolumeML = nullIntToPtr(volumeML)
		l.Rating = nullIntToPtr(rating)
		l.DurationSeconds = nullIntToPtr(durationSec)
		if subjectSQL.Valid {
			l.Subject = &subjectSQL.String
		}
		if title.Valid {
			l.Title = &title.String
		}
		if indVolJSON.Valid && indVolJSON.String != "" {
			_ = json.Unmarshal([]byte(indVolJSON.String), &l.IndicatorVolumes)
		}
	}
	return l, err
}

func (s *PostgresStore) UpdateLog(ctx context.Context, entry ChoreLog, masks ...LogFields) error {
	fields := fieldsOrAll(masks)
	indicators, _ := json.Marshal(nilToEmptyLog(entry.Indicators))
	var indicatorVolumes any
	if len(entry.IndicatorVolumes) > 0 {
		b, _ := json.Marshal(entry.IndicatorVolumes)
		indicatorVolumes = string(b)
	}
	columns := []struct {
		field, column string
		value         any
	}{
		{"note", "note", entry.Note}, {"title", "title", entry.Title}, {"indicators", "indicators", string(indicators)},
		{"indicatorVolumes", "indicator_volumes", indicatorVolumes}, {"volumeML", "volume_ml", entry.VolumeML},
		{"rating", "rating", entry.Rating}, {"durationSeconds", "duration_seconds", entry.DurationSeconds},
		{"subject", "subject", entry.Subject}, {"userId", "user_id", entry.UserID}, {"completedAt", "completed_at", entry.CompletedAt.UTC()},
		{"hour", "slot_hour", entry.SlotHour}, {"date", "log_date", entry.LogDate},
	}
	var sets []string
	args := []any{entry.ID, entry.HouseholdID}
	for _, column := range columns {
		if fields.includes(column.field) {
			args = append(args, column.value)
			sets = append(sets, fmt.Sprintf("%s=$%d", column.column, len(args)))
		}
	}
	if len(sets) == 0 {
		return nil
	}
	// Only constants above form identifiers. Independent-field edits merge in
	// PostgreSQL itself, rather than replacing a stale read of the whole row.
	result, err := s.db.ExecContext(ctx, "UPDATE chore_logs SET "+strings.Join(sets, ",")+" WHERE id=$1 AND household_id=$2", args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func (s *PostgresStore) DeleteLog(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chore_logs WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) FindLog(ctx context.Context, householdID, choreID int64, date time.Time) (*ChoreLog, error) {
	var l ChoreLog
	var indJSON string
	var indVolJSON sql.NullString
	var slotHour sql.NullInt64
	var logDate sql.NullString
	var volumeML sql.NullInt64
	var rating sql.NullInt64
	var title sql.NullString
	var durationSec sql.NullInt64
	var subjectSQL sql.NullString
	dateStr := date.Format("2006-01-02")
	err := s.db.QueryRowContext(ctx, `SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), COALESCE(indicators,'[]'), slot_hour, created_at, log_date, volume_ml, indicator_volumes::text, rating, COALESCE(title,''), duration_seconds, subject FROM chore_logs WHERE household_id = $1 AND chore_id = $2 AND COALESCE(log_date, (completed_at AT TIME ZONE 'UTC')::date) = $3::date LIMIT 1`, householdID, choreID, dateStr).Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &indJSON, &slotHour, &l.CreatedAt, &logDate, &volumeML, &indVolJSON, &rating, &title, &durationSec, &subjectSQL)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err == nil {
		_ = json.Unmarshal([]byte(indJSON), &l.Indicators)
		if l.Indicators == nil {
			l.Indicators = []string{}
		}
		l.SlotHour = nullIntToPtr(slotHour)
		if logDate.Valid {
			l.LogDate = &logDate.String
		}
		l.VolumeML = nullIntToPtr(volumeML)
		l.Rating = nullIntToPtr(rating)
		l.DurationSeconds = nullIntToPtr(durationSec)
		if subjectSQL.Valid {
			l.Subject = &subjectSQL.String
		}
		if title.Valid {
			l.Title = &title.String
		}
		if indVolJSON.Valid && indVolJSON.String != "" {
			_ = json.Unmarshal([]byte(indVolJSON.String), &l.IndicatorVolumes)
		}
	}
	return &l, err
}

func (s *PostgresStore) ListLogs(ctx context.Context, householdID int64, date time.Time) ([]ChoreLog, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	return s.queryLogs(ctx, householdID, start.Format("2006-01-02"), end.Format("2006-01-02"))
}

func (s *PostgresStore) ListLogsRange(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, error) {
	return s.queryLogs(ctx, householdID, start.Format("2006-01-02"), end.Format("2006-01-02"))
}

const logColumns = `id, household_id, user_id, chore_id, completed_at,
 COALESCE(note,''), COALESCE(indicators,'[]'), slot_hour, created_at,
 log_date, volume_ml, indicator_volumes::text, rating, COALESCE(title,''), duration_seconds, subject`

func scanLogRows(rows *sql.Rows) ([]ChoreLog, error) {
	defer rows.Close()
	logs := []ChoreLog{}
	for rows.Next() {
		var l ChoreLog
		var indJSON string
		var indVolJSON, logDate, title, subjectSQL sql.NullString
		var slotHour, volumeML, rating, durationSec sql.NullInt64
		if err := rows.Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &indJSON, &slotHour, &l.CreatedAt, &logDate, &volumeML, &indVolJSON, &rating, &title, &durationSec, &subjectSQL); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(indJSON), &l.Indicators)
		if l.Indicators == nil {
			l.Indicators = []string{}
		}
		l.SlotHour = nullIntToPtr(slotHour)
		if logDate.Valid {
			l.LogDate = &logDate.String
		}
		l.VolumeML = nullIntToPtr(volumeML)
		l.Rating = nullIntToPtr(rating)
		l.DurationSeconds = nullIntToPtr(durationSec)
		if subjectSQL.Valid {
			l.Subject = &subjectSQL.String
		}
		if title.Valid {
			l.Title = &title.String
		}
		if indVolJSON.Valid && indVolJSON.String != "" {
			_ = json.Unmarshal([]byte(indVolJSON.String), &l.IndicatorVolumes)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (s *PostgresStore) LatestPerChore(ctx context.Context, householdID int64) (map[int64]ChoreLog, error) {
	access, accessArgs := readAccessSQL(ctx, householdID, "c.id", "c.household_id", 2)
	// One index lookup per authorized chore instead of sorting every historical
	// log. The higher ID wins when completion times are equal.
	rows, err := s.db.QueryContext(ctx, `SELECT latest.* FROM chores c
 CROSS JOIN LATERAL (SELECT `+logColumns+` FROM chore_logs
 WHERE household_id = $1 AND chore_id = c.id
 ORDER BY completed_at DESC, id DESC LIMIT 1) latest
 WHERE c.household_id = $1`+access, append([]any{householdID}, accessArgs...)...)
	if err != nil {
		return nil, err
	}
	logs, err := scanLogRows(rows)
	if err != nil {
		return nil, err
	}
	result := map[int64]ChoreLog{}
	for _, l := range logs {
		result[l.ChoreID] = l
	}
	return result, nil
}

func (s *PostgresStore) queryLogs(ctx context.Context, householdID int64, dateStart, dateEnd string) ([]ChoreLog, error) {
	access, accessArgs := readAccessSQL(ctx, householdID, "chore_logs.chore_id", "chore_logs.household_id", 4)
	rows, err := s.db.QueryContext(ctx, `SELECT `+logColumns+` FROM chore_logs
 WHERE household_id = $1
 AND COALESCE(log_date, (completed_at AT TIME ZONE 'UTC')::date) >= $2::date
 AND COALESCE(log_date, (completed_at AT TIME ZONE 'UTC')::date) < $3::date`+access+`
 ORDER BY completed_at, id`+readlimit.SQL(ctx), append([]any{householdID, dateStart, dateEnd}, accessArgs...)...)
	if err != nil {
		return nil, err
	}
	logs, err := scanLogRows(rows)
	if err != nil {
		return nil, err
	}
	if err := readlimit.Check(ctx, len(logs)); err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *PostgresStore) HistoryLogs(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, bool, error) {
	logs, err := s.queryLogs(ctx, householdID, start.Format(time.DateOnly), end.Format(time.DateOnly))
	if err != nil {
		return nil, false, err
	}
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
	access, accessArgs := readAccessSQL(ctx, householdID, "chore_logs.chore_id", "chore_logs.household_id", 3)
	var hasMore bool
	err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM chore_logs WHERE household_id = $1
 AND COALESCE(log_date, (completed_at AT TIME ZONE 'UTC')::date) < $2::date`+access+`)`, append([]any{householdID, start.Format(time.DateOnly)}, accessArgs...)...).Scan(&hasMore)
	if err != nil {
		return nil, false, err
	}
	return logs, hasMore, nil
}

func (s *PostgresStore) SearchHistoryLogs(ctx context.Context, householdID int64, query string, limit int) ([]ChoreLog, error) {
	if limit <= 0 {
		limit = 50
	}
	// Escape LIKE wildcards so user input is matched literally.
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	access, accessArgs := readAccessSQL(ctx, householdID, "chore_logs.chore_id", "chore_logs.household_id", 4)
	rows, err := s.db.QueryContext(ctx, `SELECT `+logColumns+` FROM chore_logs
 WHERE household_id = $1 AND (note ILIKE $2 ESCAPE '\' OR title ILIKE $2 ESCAPE '\')`+access+`
 ORDER BY completed_at DESC, id DESC LIMIT $3`, append([]any{householdID, "%" + esc + "%", limit}, accessArgs...)...)
	if err != nil {
		return nil, err
	}
	return scanLogRows(rows)
}

func nilToEmptyLog(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
