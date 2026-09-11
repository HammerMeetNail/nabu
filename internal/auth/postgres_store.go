package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type authQuerier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type PostgresStore struct {
	db            *sql.DB
	q             authQuerier
	inTransaction bool
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db, q: db}
}

// InSQLTransaction shares an already-open transaction with household lifecycle
// operations. The caller owns commit/rollback and must use this store's DB.
func (s *PostgresStore) InSQLTransaction(db *sql.DB, tx *sql.Tx) (Store, error) {
	if db != s.db || tx == nil {
		return nil, errors.New("auth transaction database mismatch")
	}
	return &PostgresStore{db: db, q: tx, inTransaction: true}, nil
}

func (s *PostgresStore) InTransaction(ctx context.Context, fn func(Store) error) error {
	if s.inTransaction {
		return fn(s)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(&PostgresStore{db: s.db, q: tx, inTransaction: true}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	displayName := emailToDisplay(email)
	var user User
	err := s.q.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, display_name, avatar_color, role)
		VALUES ($1, $2, $3, '#19323C', 'owner')
		RETURNING id, email, password_hash, display_name, avatar_color, email_verified, role, created_at, auth_version
	`, email, passwordHash, displayName).Scan(
		&user.ID, &user.Email, &passwordHash, &user.DisplayName,
		&user.AvatarColor, &user.EmailVerified, &user.Role, &user.CreatedAt, &user.AuthVersion,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrDuplicateEmail
		}
		return User{}, err
	}
	user.HasPassword = passwordHash != ""
	return user, nil
}

// Legacy household/role fields are denormalized compatibility data, never authority.
// A stale active pointer grants nothing after the corresponding membership is removed.
func (s *PostgresStore) userWithHash(ctx context.Context, predicate string, value any) (User, string, error) {
	var user User
	var hash string
	var householdID sql.NullInt64
	query := `SELECT u.id, uh.household_id, u.email, u.password_hash, u.display_name,
  u.avatar_color, u.email_verified, COALESCE(uh.role, ''), u.created_at, u.auth_version
  FROM users u LEFT JOIN user_households uh ON uh.user_id = u.id AND uh.household_id = u.active_household_id
  WHERE ` + predicate + ` = $1`
	if s.inTransaction {
		query += ` FOR UPDATE OF u`
	}
	err := s.q.QueryRowContext(ctx, query, value).Scan(&user.ID, &householdID, &user.Email,
		&hash, &user.DisplayName, &user.AvatarColor, &user.EmailVerified, &user.Role, &user.CreatedAt, &user.AuthVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrUserNotFound
	}
	if err != nil {
		return User{}, "", err
	}
	if householdID.Valid {
		user.HouseholdID = &householdID.Int64
	}
	user.HasPassword = hash != ""
	return user, hash, nil
}

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (User, string, error) {
	return s.userWithHash(ctx, "u.email", email)
}
func (s *PostgresStore) GetUserByIDWithHash(ctx context.Context, id int64) (User, string, error) {
	return s.userWithHash(ctx, "u.id", id)
}
func (s *PostgresStore) GetUserByID(ctx context.Context, id int64) (User, error) {
	user, _, err := s.GetUserByIDWithHash(ctx, id)
	return user, err
}
func (s *PostgresStore) FindUserByEmail(ctx context.Context, email string) (User, error) {
	user, _, err := s.GetUserByEmail(ctx, email)
	return user, err
}
func (s *PostgresStore) VerifyEmail(ctx context.Context, userID int64) (User, error) {
	if _, err := s.q.ExecContext(ctx, `UPDATE users SET email_verified = TRUE WHERE id = $1`, userID); err != nil {
		return User{}, err
	}
	return s.GetUserByID(ctx, userID)
}

func (s *PostgresStore) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	result, err := s.q.ExecContext(ctx, `
		UPDATE users SET password_hash = $1, auth_version = auth_version + 1 WHERE id = $2
	`, passwordHash, userID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *PostgresStore) SetUserHousehold(ctx context.Context, userID, householdID int64, role string) error {
	result, err := s.q.ExecContext(ctx, `
		UPDATE users SET household_id = $1, role = $2 WHERE id = $3
	`, householdID, role, userID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, userID, authVersion int64, tokenHash string, expiresAt time.Time) (Session, error) {
	var session Session
	err := s.InTransaction(ctx, func(store Store) error {
		tx := store.(*PostgresStore)
		user, err := tx.GetUserByID(ctx, userID)
		if err != nil {
			return err
		}
		if user.AuthVersion != authVersion {
			return ErrInvalidCredentials
		}
		session = Session{ID: randomToken(32), UserID: userID, AuthVersion: authVersion,
			ExpiresAt: expiresAt, LastSeenAt: time.Now().UTC(), CreatedAt: time.Now().UTC()}
		_, err = tx.q.ExecContext(ctx, `INSERT INTO sessions
   (id, user_id, auth_version, token_hash, expires_at, last_seen_at, created_at)
   VALUES ($1, $2, $3, $4, $5, $6, $7)`, session.ID, userID, authVersion, tokenHash,
			expiresAt, session.LastSeenAt, session.CreatedAt)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *PostgresStore) GetSession(ctx context.Context, tokenHash string) (Session, error) {
	var session Session
	err := s.q.QueryRowContext(ctx, `
		SELECT id, user_id, expires_at, last_seen_at, created_at, auth_version
		FROM sessions WHERE token_hash = $1
	`, tokenHash).Scan(&session.ID, &session.UserID, &session.ExpiresAt, &session.LastSeenAt, &session.CreatedAt, &session.AuthVersion)
	if err != nil {
		if err == sql.ErrNoRows {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, err
	}
	return session, nil
}

func (s *PostgresStore) TouchSession(ctx context.Context, tokenHash string, lastSeenAt time.Time) error {
	_, err := s.q.ExecContext(ctx, `
		UPDATE sessions SET last_seen_at = $1 WHERE token_hash = $2
	`, lastSeenAt, tokenHash)
	return err
}

func (s *PostgresStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *PostgresStore) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

func (s *PostgresStore) DeleteUser(ctx context.Context, userID int64) error {
	return s.InTransaction(ctx, func(store Store) error {
		tx := store.(*PostgresStore)
		if _, err := tx.GetUserByID(ctx, userID); errors.Is(err, ErrUserNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		for _, query := range []string{
			`DELETE FROM invites WHERE created_by = $1`,
			`UPDATE day_notes SET updated_by = NULL WHERE updated_by = $1`,
			`UPDATE chores SET created_by = NULL WHERE created_by = $1`,
			`UPDATE chore_schedules SET assigned_to_user_id = NULL WHERE assigned_to_user_id = $1`,
			`DELETE FROM auth_tokens WHERE email = (SELECT email FROM users WHERE id = $1)`,
			`DELETE FROM users WHERE id = $1`,
		} {
			if _, err := tx.q.ExecContext(ctx, query, userID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *PostgresStore) CreateAuthToken(ctx context.Context, userID *int64, email, tokenHash, kind string, expiresAt time.Time) (AuthToken, error) {
	var token AuthToken
	err := s.q.QueryRowContext(ctx, `
		INSERT INTO auth_tokens (user_id, email, token_hash, kind, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id, user_id, email, token_hash, kind, expires_at, created_at
	`, userID, email, tokenHash, kind, expiresAt).Scan(
		&token.ID, &token.UserID, &token.Email, &token.TokenHash, &token.Kind,
		&token.ExpiresAt, &token.CreatedAt,
	)
	if err != nil {
		return AuthToken{}, err
	}
	return token, nil
}

func (s *PostgresStore) GetAuthToken(ctx context.Context, tokenHash, kind string) (AuthToken, error) {
	var token AuthToken
	err := s.q.QueryRowContext(ctx, `SELECT id, user_id, email, token_hash, kind, expires_at, created_at
  FROM auth_tokens WHERE token_hash = $1 AND kind = $2 AND consumed_at IS NULL AND expires_at > NOW()`,
		tokenHash, kind).Scan(&token.ID, &token.UserID, &token.Email, &token.TokenHash, &token.Kind, &token.ExpiresAt, &token.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthToken{}, ErrInvalidToken
	}
	return token, err
}

func (s *PostgresStore) DeleteUserAuthTokens(ctx context.Context, userID int64, email string) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM auth_mail_outbox WHERE user_id = $1`, userID); err != nil {
		return err
	}
	_, err := s.q.ExecContext(ctx, `DELETE FROM auth_tokens WHERE user_id = $1 OR email = $2`, userID, email)
	return err
}

func (s *PostgresStore) ConsumeAuthToken(ctx context.Context, tokenHash, kind string) (AuthToken, error) {
	var token AuthToken
	var consumedAt sql.NullTime
	err := s.q.QueryRowContext(ctx, `
		UPDATE auth_tokens
		SET consumed_at = NOW()
		WHERE token_hash = $1 AND kind = $2 AND consumed_at IS NULL AND expires_at > NOW()
		RETURNING id, user_id, email, token_hash, kind, expires_at, consumed_at, created_at
	`, tokenHash, kind).Scan(
		&token.ID, &token.UserID, &token.Email, &token.TokenHash, &token.Kind,
		&token.ExpiresAt, &consumedAt, &token.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return AuthToken{}, ErrInvalidToken
		}
		return AuthToken{}, err
	}
	if consumedAt.Valid {
		token.ConsumedAt = &consumedAt.Time
	}
	return token, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
