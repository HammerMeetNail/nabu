package household

import (
	"context"
	"database/sql"
	"errors"
	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"github.com/jackc/pgx/v5/pgconn"
)

type householdQuerier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type PostgresStore struct {
	db *sql.DB
	q  householdQuerier
	tx *sql.Tx
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db, q: db}
}

// Lifecycle writes are infrequent and span multiple households on deletion.
// One transaction-scoped lock gives them a consistent order across replicas.
// Ordinary reads and logging do not take it. Never acquire it after user locks.
const lifecycleLockKey int64 = 0x6e6162755f686873

func (s *PostgresStore) InTransaction(ctx context.Context, fn func(Store) error) error {
	if s.tx != nil {
		return fn(s)
	}
	// FK inserts can race with multi-domain deletion despite existing-row lock
	// ordering. PostgreSQL rolls back a deadlock/serialization victim entirely;
	// retry only those aborted transactions, never an ambiguous commit failure.
	for attempt := 0; ; attempt++ {
		err := s.transactionOnce(ctx, fn)
		var pgerr *pgconn.PgError
		if attempt >= 2 || ctx.Err() != nil || !errors.As(err, &pgerr) || (pgerr.Code != "40P01" && pgerr.Code != "40001") {
			return err
		}
	}
}

func (s *PostgresStore) transactionOnce(ctx context.Context, fn func(Store) error) error {
	if s.tx != nil {
		return fn(s)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, lifecycleLockKey); err != nil {
		return err
	}
	if err := fn(&PostgresStore{db: s.db, q: tx, tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) AuthInTransaction(users auth.Store) (auth.Store, error) {
	pg, ok := users.(*auth.PostgresStore)
	if !ok || s.tx == nil {
		return nil, errors.New("account transaction unavailable")
	}
	return pg.InSQLTransaction(s.db, s.tx)
}

func (s *PostgresStore) RevokeInvites(ctx context.Context, householdID int64) error {
	return s.InTransaction(ctx, func(store Store) error {
		tx := store.(*PostgresStore)
		if _, err := tx.q.ExecContext(ctx, `UPDATE households SET invite_code = $1 WHERE id = $2`, GenerateInviteCode(), householdID); err != nil {
			return err
		}
		_, err := tx.q.ExecContext(ctx, `DELETE FROM invites WHERE household_id = $1`, householdID)
		return err
	})
}

func (s *PostgresStore) CreateHousehold(ctx context.Context, name, initials string, ownerID int64) (Household, error) {
	if s.tx == nil {
		var result Household
		err := s.InTransaction(ctx, func(store Store) error {
			var err error
			result, err = store.CreateHousehold(ctx, name, initials, ownerID)
			return err
		})
		if err != nil {
			return Household{}, err
		}
		return result, nil
	}
	code := GenerateInviteCode()
	var hh Household
	err := s.q.QueryRowContext(ctx, `
		INSERT INTO households (name, initials, invite_code) VALUES ($1, $2, $3)
		RETURNING id, name, initials, invite_code, created_at
	`, name, initials, code).Scan(&hh.ID, &hh.Name, &hh.Initials, &hh.InviteCode, &hh.CreatedAt)
	if err != nil {
		return Household{}, err
	}
	// Insert into user_households
	_, err = s.q.ExecContext(ctx, `
		INSERT INTO user_households (user_id, household_id, role) VALUES ($1, $2, 'owner')
		ON CONFLICT (user_id, household_id) DO NOTHING
	`, ownerID, hh.ID)
	if err != nil {
		return Household{}, err
	}
	// Set active_household_id and keep household_id in sync
	_, err = s.q.ExecContext(ctx, `
		UPDATE users SET active_household_id = $1, household_id = $1, role = 'owner' WHERE id = $2
	`, hh.ID, ownerID)
	return hh, err
}

func (s *PostgresStore) GetHousehold(ctx context.Context, id int64) (Household, error) {
	var hh Household
	err := s.q.QueryRowContext(ctx, `
		SELECT id, name, COALESCE(initials, ''), invite_code, created_at FROM households WHERE id = $1
	`, id).Scan(&hh.ID, &hh.Name, &hh.Initials, &hh.InviteCode, &hh.CreatedAt)
	if err == sql.ErrNoRows {
		return Household{}, ErrNotFound
	}
	return hh, err
}

func (s *PostgresStore) GetUserHousehold(ctx context.Context, userID int64) (Household, error) {
	var hh Household
	err := s.q.QueryRowContext(ctx, `SELECT h.id, h.name, COALESCE(h.initials, ''), h.invite_code, h.created_at
  FROM households h JOIN users u ON u.active_household_id = h.id
  JOIN user_households uh ON uh.user_id = u.id AND uh.household_id = h.id
  WHERE u.id = $1`, userID).Scan(&hh.ID, &hh.Name, &hh.Initials, &hh.InviteCode, &hh.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Household{}, ErrNotFound
	}
	return hh, err
}

func (s *PostgresStore) UpdateHousehold(ctx context.Context, id int64, name, initials string) error {
	if s.tx == nil {
		return s.InTransaction(ctx, func(store Store) error { return store.UpdateHousehold(ctx, id, name, initials) })
	}
	_, err := s.q.ExecContext(ctx, `UPDATE households SET name = $1, initials = $2 WHERE id = $3`, name, initials, id)
	return err
}

func (s *PostgresStore) DeleteHousehold(ctx context.Context, id int64) error {
	if s.tx == nil {
		return s.InTransaction(ctx, func(store Store) error { return store.DeleteHousehold(ctx, id) })
	}
	if err := s.lockHousehold(ctx, id); err != nil {
		return err
	}
	// Membership policy is checked by the account transaction. User pointers
	// use SET NULL FKs; deleting a household can never delete another account.
	_, err := s.q.ExecContext(ctx, `DELETE FROM households WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) GetMembers(ctx context.Context, householdID int64) ([]Member, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT u.id, u.email, u.display_name, u.avatar_color, u.email_verified, uh.role
		FROM user_households uh
		JOIN users u ON u.id = uh.user_id
		WHERE uh.household_id = $1
	`+readlimit.SQL(ctx), householdID)
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
	if err := readlimit.Check(ctx, len(members)); err != nil {
		return nil, err
	}
	return members, rows.Err()
}

func (s *PostgresStore) AddMember(ctx context.Context, householdID, userID int64, role string) error {
	if s.tx == nil {
		return s.InTransaction(ctx, func(store Store) error { return store.AddMember(ctx, householdID, userID, role) })
	}
	// Insert into user_households
	result, err := s.q.ExecContext(ctx, `
		INSERT INTO user_households (user_id, household_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, household_id) DO NOTHING
	`, userID, householdID, role)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrAlreadyMember
	}
	// Set as active household and keep users.household_id in sync
	_, err = s.q.ExecContext(ctx, `
		UPDATE users SET active_household_id = $1, household_id = $1, role = $2 WHERE id = $3
	`, householdID, role, userID)
	return err
}

func (s *PostgresStore) RemoveMember(ctx context.Context, householdID, userID int64) error {
	if s.tx == nil {
		return s.InTransaction(ctx, func(store Store) error { return store.RemoveMember(ctx, householdID, userID) })
	}
	if err := s.lockHousehold(ctx, householdID); err != nil {
		return err
	}
	// Logging and lifecycle cleanup acquire chore locks before schedule/log
	// rows. Keep the same order when clearing assignments.
	if err := s.lockChores(ctx, `SELECT id FROM chores WHERE household_id=$1 ORDER BY id FOR NO KEY UPDATE`, householdID); err != nil {
		return err
	}
	// Remove from user_households
	result, err := s.q.ExecContext(ctx, `
		DELETE FROM user_households WHERE user_id = $1 AND household_id = $2
	`, userID, householdID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotMember
	}
	// If this was their active household, clear it or switch to another
	_, err = s.q.ExecContext(ctx, `
		UPDATE users SET
			active_household_id = (
				SELECT household_id FROM user_households WHERE user_id = $1 ORDER BY household_id LIMIT 1
			),
			household_id = (
				SELECT household_id FROM user_households WHERE user_id = $1 ORDER BY household_id LIMIT 1
			),
			role = COALESCE(
				(SELECT role FROM user_households WHERE user_id = $1 ORDER BY household_id LIMIT 1),
				'member'
			)
		WHERE id = $1 AND (active_household_id = $2 OR household_id = $2)
	`, userID, householdID)
	if err != nil {
		return err
	}
	if _, err := s.q.ExecContext(ctx, `UPDATE chore_schedules SET assigned_to_user_id = NULL WHERE household_id = $1 AND assigned_to_user_id = $2`, householdID, userID); err != nil {
		return err
	}
	if _, err := s.q.ExecContext(ctx, `DELETE FROM chore_reminder_prefs WHERE user_id = $1 AND chore_id IN (SELECT id FROM chores WHERE household_id = $2)`, userID, householdID); err != nil {
		return err
	}
	return s.RevokeInvites(ctx, householdID)
}

func (s *PostgresStore) UpdateMemberRole(ctx context.Context, householdID, userID int64, role string) error {
	if s.tx == nil {
		return s.InTransaction(ctx, func(store Store) error { return store.UpdateMemberRole(ctx, householdID, userID, role) })
	}
	if err := s.lockHousehold(ctx, householdID); err != nil {
		return err
	}
	_, err := s.q.ExecContext(ctx, `
		UPDATE user_households SET role = $1 WHERE user_id = $2 AND household_id = $3
	`, role, userID, householdID)
	if err != nil {
		return err
	}
	// Keep users.role in sync if this is their active household
	_, err = s.q.ExecContext(ctx, `
		UPDATE users SET role = $1 WHERE id = $2 AND active_household_id = $3
	`, role, userID, householdID)
	return err
}

func (s *PostgresStore) GetMembership(ctx context.Context, userID int64) (int64, string, error) {
	var householdID int64
	var role string
	err := s.q.QueryRowContext(ctx, `SELECT uh.household_id, uh.role FROM user_households uh
  JOIN users u ON u.id = uh.user_id AND u.active_household_id = uh.household_id WHERE u.id = $1`, userID).Scan(&householdID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return householdID, role, err
}

func (s *PostgresStore) GetMembershipForHousehold(ctx context.Context, userID, householdID int64) (string, error) {
	var role string
	err := s.q.QueryRowContext(ctx, `
		SELECT role FROM user_households WHERE user_id = $1 AND household_id = $2
	`, userID, householdID).Scan(&role)
	if err == sql.ErrNoRows {
		return "", ErrNotMember
	}
	return role, err
}

func (s *PostgresStore) ListUserHouseholds(ctx context.Context, userID int64) ([]HouseholdWithRole, error) {
	rows, err := s.q.QueryContext(ctx, `
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
	if s.tx == nil {
		return s.InTransaction(ctx, func(store Store) error { return store.SetActiveHousehold(ctx, userID, householdID) })
	}
	// Verify user is a member of the target household
	var role string
	err := s.q.QueryRowContext(ctx, `
		SELECT role FROM user_households WHERE user_id = $1 AND household_id = $2
	`, userID, householdID).Scan(&role)
	if err == sql.ErrNoRows {
		return ErrNotMember
	}
	if err != nil {
		return err
	}
	// Update active household and keep household_id in sync
	_, err = s.q.ExecContext(ctx, `
		UPDATE users SET active_household_id = $1, household_id = $1, role = $2 WHERE id = $3
	`, householdID, role, userID)
	return err
}

func (s *PostgresStore) GetHouseholdByInviteCode(ctx context.Context, code string) (Household, error) {
	var hh Household
	err := s.q.QueryRowContext(ctx, `
		SELECT id, name, COALESCE(initials, ''), invite_code, created_at FROM households WHERE invite_code = $1
	`, code).Scan(&hh.ID, &hh.Name, &hh.Initials, &hh.InviteCode, &hh.CreatedAt)
	if err == sql.ErrNoRows {
		return Household{}, ErrInviteNotFound
	}
	return hh, err
}

func (s *PostgresStore) CreateInvite(ctx context.Context, householdID, createdBy int64, code string, maxUses int) (Invite, error) {
	if s.tx == nil {
		var result Invite
		err := s.InTransaction(ctx, func(store Store) error {
			var err error
			result, err = store.CreateInvite(ctx, householdID, createdBy, code, maxUses)
			return err
		})
		if err != nil {
			return Invite{}, err
		}
		return result, nil
	}
	var inv Invite
	err := s.q.QueryRowContext(ctx, `
		INSERT INTO invites (household_id, code, created_by, max_uses, used_count, expires_at)
 VALUES ($1, $2, $3, $4, 0, NOW() + INTERVAL '7 days')
 RETURNING id, household_id, code, created_by, max_uses, used_count, created_at, expires_at
	`, householdID, code, createdBy, maxUses).Scan(&inv.ID, &inv.HouseholdID, &inv.Code, &inv.CreatedBy, &inv.MaxUses, &inv.UsedCount, &inv.CreatedAt, &inv.ExpiresAt)
	return inv, err
}

func (s *PostgresStore) GetInviteByCode(ctx context.Context, code string) (Invite, error) {
	var inv Invite
	err := s.q.QueryRowContext(ctx, `
		SELECT id, household_id, code, created_by, max_uses, used_count, expires_at, created_at
		FROM invites WHERE code = $1
	`, code).Scan(&inv.ID, &inv.HouseholdID, &inv.Code, &inv.CreatedBy, &inv.MaxUses, &inv.UsedCount, &inv.ExpiresAt, &inv.CreatedAt)
	if err == sql.ErrNoRows {
		return Invite{}, ErrInviteNotFound
	}
	return inv, err
}

func (s *PostgresStore) GetInviteByID(ctx context.Context, id int64) (Invite, error) {
	var inv Invite
	err := s.q.QueryRowContext(ctx, `
		SELECT id, household_id, code, created_by, max_uses, used_count, expires_at, created_at
		FROM invites WHERE id = $1
	`, id).Scan(&inv.ID, &inv.HouseholdID, &inv.Code, &inv.CreatedBy, &inv.MaxUses, &inv.UsedCount, &inv.ExpiresAt, &inv.CreatedAt)
	if err == sql.ErrNoRows {
		return Invite{}, ErrInviteNotFound
	}
	return inv, err
}

func (s *PostgresStore) GetInvites(ctx context.Context, householdID int64) ([]Invite, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT id, household_id, code, created_by, max_uses, used_count, expires_at, created_at
		FROM invites WHERE household_id = $1
	`+readlimit.SQL(ctx), householdID)
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
	if err := readlimit.Check(ctx, len(invs)); err != nil {
		return nil, err
	}
	return invs, rows.Err()
}

// UseInvite atomically consumes one use of a one-time invite, honoring
// max_uses and expires_at in the same statement so concurrent joins can never
// overshoot the cap. Returns ErrInviteNotFound when the code is unknown,
// exhausted, or expired — the same sentinel the service surfaces for all
// three, so callers cannot distinguish them.
func (s *PostgresStore) UseInvite(ctx context.Context, code string) error {
	if s.tx == nil {
		return s.InTransaction(ctx, func(store Store) error { return store.UseInvite(ctx, code) })
	}
	result, err := s.q.ExecContext(ctx, `
		UPDATE invites SET used_count = used_count + 1
		WHERE code = $1
		  AND (max_uses <= 0 OR used_count < max_uses)
		  AND (expires_at IS NULL OR expires_at > NOW())`, code)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrInviteNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteInvite(ctx context.Context, id int64) error {
	if s.tx == nil {
		return s.InTransaction(ctx, func(store Store) error { return store.DeleteInvite(ctx, id) })
	}
	_, err := s.q.ExecContext(ctx, `DELETE FROM invites WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) lockChores(ctx context.Context, query string, args ...any) error {
	rows, err := s.q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
	}
	return rows.Err()
}

// LockAccountData locks every affected household's chores before any
// membership, user, schedule or log mutation, including multi-household users.
func (s *PostgresStore) LockAccountData(ctx context.Context, userID int64) error {
	if s.tx == nil {
		return errors.New("account transaction required")
	}
	rows, err := s.q.QueryContext(ctx, `SELECT household_id FROM user_households WHERE user_id=$1 ORDER BY household_id`, userID)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.lockHousehold(ctx, id); err != nil {
			return err
		}
	}

	return s.lockChores(ctx, `SELECT id FROM chores WHERE created_by=$1 OR household_id IN
 (SELECT household_id FROM user_households WHERE user_id=$1) ORDER BY id FOR NO KEY UPDATE`, userID)
}
