package household

import (
	"context"
	"database/sql"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateHousehold(ctx context.Context, name, initials string, ownerID int64) (Household, error) {
	code := GenerateInviteCode()
	var hh Household
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO households (name, initials, invite_code) VALUES ($1, $2, $3)
		RETURNING id, name, initials, invite_code, created_at
	`, name, initials, code).Scan(&hh.ID, &hh.Name, &hh.Initials, &hh.InviteCode, &hh.CreatedAt)
	if err != nil {
		return Household{}, err
	}
	// Insert into user_households
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_households (user_id, household_id, role) VALUES ($1, $2, 'owner')
		ON CONFLICT (user_id, household_id) DO NOTHING
	`, ownerID, hh.ID)
	if err != nil {
		return Household{}, err
	}
	// Set active_household_id and keep household_id in sync
	_, err = s.db.ExecContext(ctx, `
		UPDATE users SET active_household_id = $1, household_id = $1, role = 'owner' WHERE id = $2
	`, hh.ID, ownerID)
	return hh, err
}

func (s *PostgresStore) GetHousehold(ctx context.Context, id int64) (Household, error) {
	var hh Household
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, COALESCE(initials, ''), invite_code, created_at FROM households WHERE id = $1
	`, id).Scan(&hh.ID, &hh.Name, &hh.Initials, &hh.InviteCode, &hh.CreatedAt)
	if err == sql.ErrNoRows {
		return Household{}, ErrNotFound
	}
	return hh, err
}

func (s *PostgresStore) GetUserHousehold(ctx context.Context, userID int64) (Household, error) {
	var hh Household
	err := s.db.QueryRowContext(ctx, `
		SELECT h.id, h.name, COALESCE(h.initials, ''), h.invite_code, h.created_at
		FROM households h
		JOIN users u ON u.active_household_id = h.id
		WHERE u.id = $1
	`, userID).Scan(&hh.ID, &hh.Name, &hh.Initials, &hh.InviteCode, &hh.CreatedAt)
	if err == sql.ErrNoRows {
		// Fallback to household_id for backward compat during transition
		err = s.db.QueryRowContext(ctx, `
			SELECT h.id, h.name, COALESCE(h.initials, ''), h.invite_code, h.created_at
			FROM households h
			JOIN users u ON u.household_id = h.id
			WHERE u.id = $1
		`, userID).Scan(&hh.ID, &hh.Name, &hh.Initials, &hh.InviteCode, &hh.CreatedAt)
		if err == sql.ErrNoRows {
			return Household{}, ErrNotFound
		}
	}
	return hh, err
}

func (s *PostgresStore) UpdateHousehold(ctx context.Context, id int64, name, initials string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE households SET name = $1, initials = $2 WHERE id = $3`, name, initials, id)
	return err
}

func (s *PostgresStore) GetMembers(ctx context.Context, householdID int64) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.email, u.display_name, u.avatar_color, u.email_verified, uh.role
		FROM user_households uh
		JOIN users u ON u.id = uh.user_id
		WHERE uh.household_id = $1
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.DisplayName, &m.AvatarColor, &m.EmailVerified, &m.Role); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (s *PostgresStore) AddMember(ctx context.Context, householdID, userID int64, role string) error {
	// Insert into user_households
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_households (user_id, household_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, household_id) DO NOTHING
	`, userID, householdID, role)
	if err != nil {
		return err
	}
	// Set as active household and keep users.household_id in sync
	_, err = s.db.ExecContext(ctx, `
		UPDATE users SET active_household_id = $1, household_id = $1, role = $2 WHERE id = $3
	`, householdID, role, userID)
	return err
}

func (s *PostgresStore) RemoveMember(ctx context.Context, householdID, userID int64) error {
	// Remove from user_households
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM user_households WHERE user_id = $1 AND household_id = $2
	`, userID, householdID)
	if err != nil {
		return err
	}
	// If this was their active household, clear it or switch to another
	_, err = s.db.ExecContext(ctx, `
		UPDATE users SET
			active_household_id = (
				SELECT household_id FROM user_households WHERE user_id = $1 LIMIT 1
			),
			household_id = (
				SELECT household_id FROM user_households WHERE user_id = $1 LIMIT 1
			),
			role = COALESCE(
				(SELECT role FROM user_households WHERE user_id = $1 LIMIT 1),
				'member'
			)
		WHERE id = $1 AND (active_household_id = $2 OR household_id = $2)
	`, userID, householdID)
	return err
}

func (s *PostgresStore) UpdateMemberRole(ctx context.Context, householdID, userID int64, role string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_households SET role = $1 WHERE user_id = $2 AND household_id = $3
	`, role, userID, householdID)
	if err != nil {
		return err
	}
	// Keep users.role in sync if this is their active household
	_, err = s.db.ExecContext(ctx, `
		UPDATE users SET role = $1 WHERE id = $2 AND active_household_id = $3
	`, role, userID, householdID)
	return err
}

func (s *PostgresStore) GetMembership(ctx context.Context, userID int64) (int64, string, error) {
	// Get role in the user's currently active household
	var hhID sql.NullInt64
	var role sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT uh.household_id, uh.role
		FROM user_households uh
		JOIN users u ON u.id = $1 AND u.active_household_id = uh.household_id
		WHERE uh.user_id = $1
	`, userID).Scan(&hhID, &role)
	if err == sql.ErrNoRows || !hhID.Valid {
		// Fallback: try using users.household_id for backward compat
		var legacyHHID sql.NullInt64
		var legacyRole string
		ferr := s.db.QueryRowContext(ctx, `
			SELECT household_id, role FROM users WHERE id = $1
		`, userID).Scan(&legacyHHID, &legacyRole)
		if ferr != nil || !legacyHHID.Valid {
			return 0, "", ErrNotFound
		}
		return legacyHHID.Int64, legacyRole, nil
	}
	if err != nil {
		return 0, "", err
	}
	return hhID.Int64, role.String, nil
}

func (s *PostgresStore) GetMembershipForHousehold(ctx context.Context, userID, householdID int64) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx, `
		SELECT role FROM user_households WHERE user_id = $1 AND household_id = $2
	`, userID, householdID).Scan(&role)
	if err == sql.ErrNoRows {
		return "", ErrNotMember
	}
	return role, err
}

func (s *PostgresStore) ListUserHouseholds(ctx context.Context, userID int64) ([]HouseholdWithRole, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT h.id, h.name, COALESCE(h.initials, ''), uh.role
		FROM user_households uh
		JOIN households h ON h.id = uh.household_id
		WHERE uh.user_id = $1
		ORDER BY uh.joined_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []HouseholdWithRole
	for rows.Next() {
		var h HouseholdWithRole
		if err := rows.Scan(&h.ID, &h.Name, &h.Initials, &h.Role); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

func (s *PostgresStore) SetActiveHousehold(ctx context.Context, userID, householdID int64) error {
	// Verify user is a member of the target household
	var role string
	err := s.db.QueryRowContext(ctx, `
		SELECT role FROM user_households WHERE user_id = $1 AND household_id = $2
	`, userID, householdID).Scan(&role)
	if err == sql.ErrNoRows {
		return ErrNotMember
	}
	if err != nil {
		return err
	}
	// Update active household and keep household_id in sync
	_, err = s.db.ExecContext(ctx, `
		UPDATE users SET active_household_id = $1, household_id = $1, role = $2 WHERE id = $3
	`, householdID, role, userID)
	return err
}

func (s *PostgresStore) GetHouseholdByInviteCode(ctx context.Context, code string) (Household, error) {
	var hh Household
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, COALESCE(initials, ''), invite_code, created_at FROM households WHERE invite_code = $1
	`, code).Scan(&hh.ID, &hh.Name, &hh.Initials, &hh.InviteCode, &hh.CreatedAt)
	if err == sql.ErrNoRows {
		return Household{}, ErrInviteNotFound
	}
	return hh, err
}

func (s *PostgresStore) CreateInvite(ctx context.Context, householdID, createdBy int64, code string, maxUses int) (Invite, error) {
	var inv Invite
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO invites (household_id, code, created_by, max_uses, used_count)
		VALUES ($1, $2, $3, $4, 0)
		RETURNING id, household_id, code, created_by, max_uses, used_count, created_at
	`, householdID, code, createdBy, maxUses).Scan(&inv.ID, &inv.HouseholdID, &inv.Code, &inv.CreatedBy, &inv.MaxUses, &inv.UsedCount, &inv.CreatedAt)
	return inv, err
}

func (s *PostgresStore) GetInviteByCode(ctx context.Context, code string) (Invite, error) {
	var inv Invite
	err := s.db.QueryRowContext(ctx, `
		SELECT id, household_id, code, created_by, max_uses, used_count, COALESCE(expires_at, 'epoch'::timestamptz), created_at
		FROM invites WHERE code = $1
	`, code).Scan(&inv.ID, &inv.HouseholdID, &inv.Code, &inv.CreatedBy, &inv.MaxUses, &inv.UsedCount, &inv.ExpiresAt, &inv.CreatedAt)
	if err == sql.ErrNoRows {
		return Invite{}, ErrInviteNotFound
	}
	return inv, err
}

func (s *PostgresStore) GetInviteByID(ctx context.Context, id int64) (Invite, error) {
	var inv Invite
	err := s.db.QueryRowContext(ctx, `
		SELECT id, household_id, code, created_by, max_uses, used_count, COALESCE(expires_at, 'epoch'::timestamptz), created_at
		FROM invites WHERE id = $1
	`, id).Scan(&inv.ID, &inv.HouseholdID, &inv.Code, &inv.CreatedBy, &inv.MaxUses, &inv.UsedCount, &inv.ExpiresAt, &inv.CreatedAt)
	if err == sql.ErrNoRows {
		return Invite{}, ErrInviteNotFound
	}
	return inv, err
}

func (s *PostgresStore) GetInvites(ctx context.Context, householdID int64) ([]Invite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, household_id, code, created_by, max_uses, used_count, expires_at, created_at
		FROM invites WHERE household_id = $1
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invs []Invite
	for rows.Next() {
		var inv Invite
		if err := rows.Scan(&inv.ID, &inv.HouseholdID, &inv.Code, &inv.CreatedBy, &inv.MaxUses, &inv.UsedCount, &inv.ExpiresAt, &inv.CreatedAt); err != nil {
			return nil, err
		}
		invs = append(invs, inv)
	}
	return invs, rows.Err()
}

func (s *PostgresStore) UseInvite(ctx context.Context, code string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE invites SET used_count = used_count + 1 WHERE code = $1`, code)
	return err
}

func (s *PostgresStore) DeleteInvite(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM invites WHERE id = $1`, id)
	return err
}
