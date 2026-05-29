package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	displayName := emailToDisplay(email)
	var user User
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, display_name, avatar_color, role)
		VALUES ($1, $2, $3, '#19323C', 'owner')
		RETURNING id, email, password_hash, display_name, avatar_color, email_verified, role, created_at
	`, email, passwordHash, displayName).Scan(
		&user.ID, &user.Email, &passwordHash, &user.DisplayName,
		&user.AvatarColor, &user.EmailVerified, &user.Role, &user.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrDuplicateEmail
		}
		return User{}, err
	}
	return user, nil
}

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (User, string, error) {
	var user User
	var passwordHash string
	var householdID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, household_id, email, password_hash, display_name, avatar_color, email_verified, role, created_at
		FROM users WHERE email = $1
	`, email).Scan(
		&user.ID, &householdID, &user.Email, &passwordHash,
		&user.DisplayName, &user.AvatarColor, &user.EmailVerified, &user.Role, &user.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, "", ErrUserNotFound
		}
		return User{}, "", err
	}
	if householdID.Valid {
		user.HouseholdID = &householdID.Int64
	}
	return user, passwordHash, nil
}

func (s *PostgresStore) GetUserByID(ctx context.Context, id int64) (User, error) {
	var user User
	var householdID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, household_id, email, display_name, avatar_color, email_verified, role, created_at
		FROM users WHERE id = $1
	`, id).Scan(
		&user.ID, &householdID, &user.Email, &user.DisplayName,
		&user.AvatarColor, &user.EmailVerified, &user.Role, &user.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	if householdID.Valid {
		user.HouseholdID = &householdID.Int64
	}
	return user, nil
}

func (s *PostgresStore) GetUserByIDWithHash(ctx context.Context, id int64) (User, string, error) {
	var user User
	var passwordHash string
	var householdID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, household_id, email, password_hash, display_name, avatar_color, email_verified, role, created_at
		FROM users WHERE id = $1
	`, id).Scan(
		&user.ID, &householdID, &user.Email, &passwordHash,
		&user.DisplayName, &user.AvatarColor, &user.EmailVerified, &user.Role, &user.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, "", ErrUserNotFound
		}
		return User{}, "", err
	}
	if householdID.Valid {
		user.HouseholdID = &householdID.Int64
	}
	return user, passwordHash, nil
}

func (s *PostgresStore) FindUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	var householdID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, household_id, email, display_name, avatar_color, email_verified, role, created_at
		FROM users WHERE email = $1
	`, email).Scan(
		&user.ID, &householdID, &user.Email, &user.DisplayName,
		&user.AvatarColor, &user.EmailVerified, &user.Role, &user.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	if householdID.Valid {
		user.HouseholdID = &householdID.Int64
	}
	return user, nil
}

func (s *PostgresStore) VerifyEmail(ctx context.Context, userID int64) (User, error) {
	var user User
	var householdID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		UPDATE users SET email_verified = TRUE
		WHERE id = $1
		RETURNING id, household_id, email, display_name, avatar_color, email_verified, role, created_at
	`, userID).Scan(
		&user.ID, &householdID, &user.Email, &user.DisplayName,
		&user.AvatarColor, &user.EmailVerified, &user.Role, &user.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	if householdID.Valid {
		user.HouseholdID = &householdID.Int64
	}
	return user, nil
}

func (s *PostgresStore) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET password_hash = $1 WHERE id = $2
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
	result, err := s.db.ExecContext(ctx, `
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

func (s *PostgresStore) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (Session, error) {
	sessionID := randomToken(32)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, sessionID, userID, tokenHash, expiresAt, now)
	if err != nil {
		return Session{}, err
	}
	return Session{
		ID:        sessionID,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, nil
}

func (s *PostgresStore) GetSession(ctx context.Context, tokenHash string) (Session, error) {
	var session Session
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, expires_at, created_at
		FROM sessions WHERE token_hash = $1
	`, tokenHash).Scan(&session.ID, &session.UserID, &session.ExpiresAt, &session.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, err
	}
	return session, nil
}

func (s *PostgresStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *PostgresStore) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

func (s *PostgresStore) CreateAuthToken(ctx context.Context, userID *int64, email, tokenHash, kind string, expiresAt time.Time) (AuthToken, error) {
	var token AuthToken
	err := s.db.QueryRowContext(ctx, `
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

func (s *PostgresStore) ConsumeAuthToken(ctx context.Context, tokenHash, kind string) (AuthToken, error) {
	var token AuthToken
	var consumedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
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
