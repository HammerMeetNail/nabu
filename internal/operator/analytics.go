package operator

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

type Summary struct {
	AsOf                   time.Time `json:"asOf"`
	RegisteredUsers        int64     `json:"registeredUsers"`
	VerifiedUsers          int64     `json:"verifiedUsers"`
	Households             int64     `json:"households"`
	Authors7Days           int64     `json:"authors7Days"`
	Authors30Days          int64     `json:"authors30Days"`
	ActiveHouseholds7Days  int64     `json:"activeHouseholds7Days"`
	ActiveHouseholds30Days int64     `json:"activeHouseholds30Days"`
	Logs30Days             int64     `json:"logs30Days"`
	LogsWithActor30Days    int64     `json:"logsWithActor30Days"`
}

func (s *Service) Summary(ctx context.Context, access Access) (Summary, error) {
	if err := s.requireAccess(access, ScopeSummary); err != nil {
		return Summary{}, err
	}
	now := s.now()
	result := Summary{AsOf: now}
	err := s.db.QueryRowContext(ctx, `SELECT
	 (SELECT COUNT(*) FROM users),
	 (SELECT COUNT(*) FROM users WHERE email_verified),
	 (SELECT COUNT(*) FROM households),
	 (SELECT COUNT(DISTINCT l.idempotency_actor_id) FROM chore_logs l JOIN users u ON u.id = l.idempotency_actor_id WHERE l.created_at >= $1),
	 (SELECT COUNT(DISTINCT l.idempotency_actor_id) FROM chore_logs l JOIN users u ON u.id = l.idempotency_actor_id WHERE l.created_at >= $2),
	 (SELECT COUNT(DISTINCT household_id) FROM chore_logs WHERE created_at >= $1),
	 (SELECT COUNT(DISTINCT household_id) FROM chore_logs WHERE created_at >= $2),
	 (SELECT COUNT(*) FROM chore_logs WHERE created_at >= $2),
	 (SELECT COUNT(*) FROM chore_logs WHERE created_at >= $2 AND idempotency_actor_id IS NOT NULL)`,
		now.AddDate(0, 0, -7), now.AddDate(0, 0, -30)).Scan(
		&result.RegisteredUsers, &result.VerifiedUsers, &result.Households,
		&result.Authors7Days, &result.Authors30Days, &result.ActiveHouseholds7Days,
		&result.ActiveHouseholds30Days, &result.Logs30Days, &result.LogsWithActor30Days)
	return result, err
}

type UserRow struct {
	ID                  int64      `json:"id"`
	Email               string     `json:"email"`
	RegisteredAt        time.Time  `json:"registeredAt"`
	EmailVerified       bool       `json:"emailVerified"`
	HouseholdCount      int64      `json:"householdCount"`
	Authored7Days       int64      `json:"authored7Days"`
	Authored30Days      int64      `json:"authored30Days"`
	HouseholdLogs30Days int64      `json:"householdLogs30Days"`
	LastAuthoredAt      *time.Time `json:"lastAuthoredAt"`
	LastHouseholdLogAt  *time.Time `json:"lastHouseholdLogAt"`
	LastSessionSeenAt   *time.Time `json:"lastSessionSeenAt"`
}

type UserPage struct {
	AsOf       time.Time `json:"asOf"`
	Users      []UserRow `json:"users"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

func (s *Service) Users(ctx context.Context, access Access, afterID int64, limit int) (UserPage, error) {
	if err := s.requireAccess(access, ScopeFull); err != nil {
		return UserPage{}, err
	}
	if afterID < 0 || limit < 1 || limit > 100 {
		return UserPage{}, ErrInvalidInput
	}
	now := s.now()
	page := UserPage{AsOf: now, Users: make([]UserRow, 0, limit)}
	rows, err := s.db.QueryContext(ctx, `WITH page AS (
	 SELECT id, email, created_at, email_verified FROM users WHERE id > $1 ORDER BY id LIMIT $2
	)
	SELECT u.id, u.email, u.created_at, u.email_verified,
	 (SELECT COUNT(*) FROM user_households uh WHERE uh.user_id = u.id),
	 a.count_7d, a.count_30d, h.count_30d,
	 a.last_log, h.last_log,
	 (SELECT MAX(last_seen_at) FROM sessions WHERE user_id = u.id)
	FROM page u
	CROSS JOIN LATERAL (
	 SELECT COUNT(*) FILTER (WHERE created_at >= $3) AS count_7d,
	        COUNT(*) FILTER (WHERE created_at >= $4) AS count_30d,
	        MAX(created_at) AS last_log
	 FROM chore_logs WHERE idempotency_actor_id = u.id
	) a
	CROSS JOIN LATERAL (
	 SELECT COUNT(l.id) FILTER (WHERE l.created_at >= $4) AS count_30d,
	        MAX(l.created_at) AS last_log
	 FROM user_households uh JOIN chore_logs l ON l.household_id = uh.household_id
	 WHERE uh.user_id = u.id
	) h
	ORDER BY u.id`, afterID, limit+1, now.AddDate(0, 0, -7), now.AddDate(0, 0, -30))
	if err != nil {
		return UserPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var row UserRow
		var own, household, session sql.NullTime
		if err := rows.Scan(&row.ID, &row.Email, &row.RegisteredAt, &row.EmailVerified,
			&row.HouseholdCount, &row.Authored7Days, &row.Authored30Days, &row.HouseholdLogs30Days,
			&own, &household, &session); err != nil {
			return UserPage{}, err
		}
		if own.Valid {
			row.LastAuthoredAt = &own.Time
		}
		if household.Valid {
			row.LastHouseholdLogAt = &household.Time
		}
		if session.Valid {
			row.LastSessionSeenAt = &session.Time
		}
		page.Users = append(page.Users, row)
	}
	if err := rows.Err(); err != nil {
		return UserPage{}, err
	}
	if len(page.Users) > limit {
		page.Users = page.Users[:limit]
		page.NextCursor = strconv.FormatInt(page.Users[len(page.Users)-1].ID, 10)
	}
	return page, nil
}

type Day struct {
	Date          string `json:"date"`
	Registrations int64  `json:"registrations"`
	Authors       int64  `json:"authors"`
}

type Activity struct {
	AsOf time.Time `json:"asOf"`
	Days []Day     `json:"days"`
}

func (s *Service) Activity(ctx context.Context, access Access, days int) (Activity, error) {
	if err := s.requireAccess(access, ScopeSummary); err != nil {
		return Activity{}, err
	}
	if days != 30 && days != 90 {
		return Activity{}, ErrInvalidInput
	}
	now := s.now()
	result := Activity{AsOf: now, Days: make([]Day, 0, 90)}
	start := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1-days)
	rows, err := s.db.QueryContext(ctx, `WITH dates AS (
	 SELECT generate_series(($1 AT TIME ZONE 'UTC')::date, ($2 AT TIME ZONE 'UTC')::date, INTERVAL '1 day')::date AS date
	), registrations AS (
	 SELECT (created_at AT TIME ZONE 'UTC')::date AS date, COUNT(*) AS n
	 FROM users WHERE created_at >= $1 AND created_at < $3 GROUP BY 1
	), authors AS (
	 SELECT (l.created_at AT TIME ZONE 'UTC')::date AS date, COUNT(DISTINCT l.idempotency_actor_id) AS n
	 FROM chore_logs l JOIN users u ON u.id = l.idempotency_actor_id
	 WHERE l.created_at >= $1 AND l.created_at < $3 GROUP BY 1
	)
	SELECT to_char(d.date, 'YYYY-MM-DD'), COALESCE(r.n,0), COALESCE(a.n,0)
	FROM dates d LEFT JOIN registrations r ON r.date = d.date LEFT JOIN authors a ON a.date = d.date
	ORDER BY d.date`, start, now.UTC().Truncate(24*time.Hour), now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1))
	if err != nil {
		return Activity{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var day Day
		if err := rows.Scan(&day.Date, &day.Registrations, &day.Authors); err != nil {
			return Activity{}, err
		}
		result.Days = append(result.Days, day)
	}
	return result, rows.Err()
}

type HouseholdRow struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	Members    int64      `json:"members"`
	Logs30Days int64      `json:"logs30Days"`
	LastLogAt  *time.Time `json:"lastLogAt"`
}

type HouseholdPage struct {
	AsOf       time.Time      `json:"asOf"`
	Households []HouseholdRow `json:"households"`
	NextCursor string         `json:"nextCursor,omitempty"`
}

func (s *Service) Households(ctx context.Context, access Access, afterID int64, limit int) (HouseholdPage, error) {
	if err := s.requireAccess(access, ScopeFull); err != nil {
		return HouseholdPage{}, err
	}
	if afterID < 0 || limit < 1 || limit > 100 {
		return HouseholdPage{}, ErrInvalidInput
	}
	now := s.now()
	page := HouseholdPage{AsOf: now, Households: make([]HouseholdRow, 0, limit)}
	rows, err := s.db.QueryContext(ctx, `WITH page AS (
	 SELECT id, name, created_at FROM households WHERE id > $1 ORDER BY id LIMIT $2
	)
	SELECT h.id, h.name, h.created_at,
	 (SELECT COUNT(*) FROM user_households uh WHERE uh.household_id = h.id),
	 (SELECT COUNT(*) FROM chore_logs l WHERE l.household_id = h.id AND l.created_at >= $3),
	 (SELECT MAX(created_at) FROM chore_logs l WHERE l.household_id = h.id)
	FROM page h ORDER BY h.id`, afterID, limit+1, now.AddDate(0, 0, -30))
	if err != nil {
		return HouseholdPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var row HouseholdRow
		var last sql.NullTime
		if err := rows.Scan(&row.ID, &row.Name, &row.CreatedAt, &row.Members, &row.Logs30Days, &last); err != nil {
			return HouseholdPage{}, err
		}
		if last.Valid {
			row.LastLogAt = &last.Time
		}
		page.Households = append(page.Households, row)
	}
	if err := rows.Err(); err != nil {
		return HouseholdPage{}, err
	}
	if len(page.Households) > limit {
		page.Households = page.Households[:limit]
		page.NextCursor = fmt.Sprint(page.Households[len(page.Households)-1].ID)
	}
	return page, nil
}
