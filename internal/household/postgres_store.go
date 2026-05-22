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

func (s *PostgresStore) CreateHousehold(ctx context.Context, name string, ownerID int64) (Household, error) {
	code := GenerateInviteCode()
	var hh Household
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO households (name, invite_code) VALUES ($1, $2)
		RETURNING id, name, invite_code, created_at
	`, name, code).Scan(&hh.ID, &hh.Name, &hh.InviteCode, &hh.CreatedAt)
	if err != nil {
		return Household{}, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET household_id = $1, role = 'owner' WHERE id = $2`, hh.ID, ownerID)
	return hh, err
}

func (s *PostgresStore) GetHousehold(ctx context.Context, id int64) (Household, error) {
	var hh Household
	err := s.db.QueryRowContext(ctx, `SELECT id, name, invite_code, created_at FROM households WHERE id = $1`, id).Scan(&hh.ID, &hh.Name, &hh.InviteCode, &hh.CreatedAt)
	if err == sql.ErrNoRows {
		return Household{}, ErrNotFound
	}
	return hh, err
}

func (s *PostgresStore) GetUserHousehold(ctx context.Context, userID int64) (Household, error) {
	var hh Household
	err := s.db.QueryRowContext(ctx, `
		SELECT h.id, h.name, h.invite_code, h.created_at
		FROM households h
		JOIN users u ON u.household_id = h.id
		WHERE u.id = $1
	`, userID).Scan(&hh.ID, &hh.Name, &hh.InviteCode, &hh.CreatedAt)
	if err == sql.ErrNoRows {
		return Household{}, ErrNotFound
	}
	return hh, err
}

func (s *PostgresStore) UpdateHousehold(ctx context.Context, id int64, name string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE households SET name = $1 WHERE id = $2`, name, id)
	return err
}

func (s *PostgresStore) GetMembers(ctx context.Context, householdID int64) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, email, display_name, avatar_color, role FROM users WHERE household_id = $1`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.DisplayName, &m.AvatarColor, &m.Role); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (s *PostgresStore) AddMember(ctx context.Context, householdID, userID int64, role string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET household_id = $1, role = $2 WHERE id = $3`, householdID, role, userID)
	return err
}

func (s *PostgresStore) RemoveMember(ctx context.Context, householdID, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET household_id = NULL, role = 'member' WHERE id = $1 AND household_id = $2`, userID, householdID)
	return err
}

func (s *PostgresStore) UpdateMemberRole(ctx context.Context, householdID, userID int64, role string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET role = $1 WHERE id = $2 AND household_id = $3`, role, userID, householdID)
	return err
}

func (s *PostgresStore) GetMembership(ctx context.Context, userID int64) (int64, string, error) {
	var hhID sql.NullInt64
	var role string
	err := s.db.QueryRowContext(ctx, `SELECT household_id, role FROM users WHERE id = $1`, userID).Scan(&hhID, &role)
	if err == sql.ErrNoRows || !hhID.Valid {
		return 0, "", ErrNotFound
	}
	return hhID.Int64, role, err
}

func (s *PostgresStore) GetHouseholdByInviteCode(ctx context.Context, code string) (Household, error) {
	var hh Household
	err := s.db.QueryRowContext(ctx, `SELECT id, name, invite_code, created_at FROM households WHERE invite_code = $1`, code).
		Scan(&hh.ID, &hh.Name, &hh.InviteCode, &hh.CreatedAt)
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
	err := s.db.QueryRowContext(ctx, `SELECT id, household_id, code, created_by, max_uses, used_count, COALESCE(expires_at, 'epoch'::timestamptz), created_at FROM invites WHERE code = $1`, code).
		Scan(&inv.ID, &inv.HouseholdID, &inv.Code, &inv.CreatedBy, &inv.MaxUses, &inv.UsedCount, &inv.ExpiresAt, &inv.CreatedAt)
	if err == sql.ErrNoRows {
		return Invite{}, ErrInviteNotFound
	}
	return inv, err
}

func (s *PostgresStore) GetInvites(ctx context.Context, householdID int64) ([]Invite, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, household_id, code, created_by, max_uses, used_count, expires_at, created_at FROM invites WHERE household_id = $1`, householdID)
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
