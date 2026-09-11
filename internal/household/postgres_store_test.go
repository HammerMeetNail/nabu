package household

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

var testTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func TestPostgresHouseholdStore_CreateHousehold(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WithArgs(lifecycleLockKey).WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO households (name, initials, invite_code) VALUES ($1, $2, $3) RETURNING id, name, initials, invite_code, created_at`)).
		WithArgs("My Home", "", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "initials", "invite_code", "created_at"}).
			AddRow(1, "My Home", "MH", "ABC123", testTime))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO user_households (user_id, household_id, role) VALUES ($1, $2, 'owner') ON CONFLICT (user_id, household_id) DO NOTHING`)).
		WithArgs(int64(1), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET active_household_id = $1, household_id = $1, role = 'owner' WHERE id = $2`)).
		WithArgs(int64(1), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()
	hh, err := store.CreateHousehold(context.Background(), "My Home", "", 1)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	if hh.ID != 1 {
		t.Fatalf("ID = %d, want 1", hh.ID)
	}
}

func TestPostgresHouseholdStore_GetHousehold(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, COALESCE(initials, ''), invite_code, created_at FROM households WHERE id = $1`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "initials", "invite_code", "created_at"}).
			AddRow(1, "My Home", "MH", "ABC", testTime))

	hh, err := store.GetHousehold(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetHousehold: %v", err)
	}
	if hh.Name != "My Home" {
		t.Fatalf("Name = %q, want My Home", hh.Name)
	}
}

func TestPostgresHouseholdStore_GetHouseholdNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectQuery(`SELECT`).WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)

	_, err = store.GetHousehold(context.Background(), 9)
	if err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPostgresHouseholdStore_GetMembers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT u.id, u.email, u.display_name, u.avatar_color, u.email_verified, uh.role FROM user_households uh JOIN users u ON u.id = uh.user_id WHERE uh.household_id = $1`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "display_name", "avatar_color", "email_verified", "role"}).
			AddRow(1, "a@b.com", "Alice", "#F00", true, "owner"))

	members, err := store.GetMembers(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("len = %d, want 1", len(members))
	}
}

func TestPostgresHouseholdStore_AddMember(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WithArgs(lifecycleLockKey).WillReturnResult(sqlmock.NewResult(0, 1))

	// AddMember(ctx, householdID=1, userID=2, role="member")
	// First exec: INSERT INTO user_households (user_id=$2, household_id=$1, role=$3)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO user_households (user_id, household_id, role) VALUES ($1, $2, $3) ON CONFLICT (user_id, household_id) DO NOTHING`)).
		WithArgs(int64(2), int64(1), "member").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Second exec: UPDATE users SET active_household_id=$1, household_id=$1, role=$2 WHERE id=$3
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET active_household_id = $1, household_id = $1, role = $2 WHERE id = $3`)).
		WithArgs(int64(1), "member", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()
	err = store.AddMember(context.Background(), 1, 2, "member")
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
}

func TestPostgresHouseholdStore_RemoveMember(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WithArgs(lifecycleLockKey).WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(int64(-1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id FROM chores").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}))

	// RemoveMember(ctx, householdID=1, userID=2)
	// First exec: DELETE FROM user_households WHERE user_id=$1 AND household_id=$2
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM user_households WHERE user_id = $1 AND household_id = $2`)).
		WithArgs(int64(2), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Second exec: UPDATE users SET active_household_id=..., household_id=..., role=... WHERE id=$1 AND (...)
	// The query uses $1 (userID) and $2 (householdID) — 2 args total
	mock.ExpectExec(`UPDATE users SET`).
		WithArgs(int64(2), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(`UPDATE chore_schedules SET assigned_to_user_id`).WithArgs(int64(1), int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM chore_reminder_prefs`).WithArgs(int64(2), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE households SET invite_code`).WithArgs(sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM invites WHERE household_id`).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err = store.RemoveMember(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
}

func TestPostgresHouseholdStore_UseInvite_ConsumesUnderCap(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WithArgs(lifecycleLockKey).WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE invites SET used_count = used_count + 1
		WHERE code = $1
		  AND (max_uses <= 0 OR used_count < max_uses)
		  AND (expires_at IS NULL OR expires_at > NOW())`)).
		WithArgs("CODE123456").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()
	err = store.UseInvite(context.Background(), "CODE123456")
	if err != nil {
		t.Fatalf("UseInvite: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostgresHouseholdStore_UseInvite_RejectsAtCap(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WithArgs(lifecycleLockKey).WillReturnResult(sqlmock.NewResult(0, 1))

	// 0 rows affected → code unknown, exhausted, or expired; same sentinel
	// either way so callers cannot distinguish.
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE invites SET used_count = used_count + 1
		WHERE code = $1
		  AND (max_uses <= 0 OR used_count < max_uses)
		  AND (expires_at IS NULL OR expires_at > NOW())`)).
		WithArgs("CODE123456").
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectRollback()
	err = store.UseInvite(context.Background(), "CODE123456")
	if err != ErrInviteNotFound {
		t.Fatalf("err = %v, want ErrInviteNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostgresHouseholdStore_UseInvite_RejectsExpired(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WithArgs(lifecycleLockKey).WillReturnResult(sqlmock.NewResult(0, 1))

	// Expired rows also match 0 rows affected → ErrInviteNotFound.
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE invites SET used_count = used_count + 1
		WHERE code = $1
		  AND (max_uses <= 0 OR used_count < max_uses)
		  AND (expires_at IS NULL OR expires_at > NOW())`)).
		WithArgs("CODE123456").
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectRollback()
	err = store.UseInvite(context.Background(), "CODE123456")
	if err != ErrInviteNotFound {
		t.Fatalf("err = %v, want ErrInviteNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostgresHouseholdStore_UpdateMemberRole(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WithArgs(lifecycleLockKey).WillReturnResult(sqlmock.NewResult(0, 1))

	// UpdateMemberRole(ctx, householdID=1, userID=2, role="admin")
	// First exec: UPDATE user_households SET role=$1 WHERE user_id=$2 AND household_id=$3
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(int64(-1)).WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE user_households SET role = $1 WHERE user_id = $2 AND household_id = $3`)).
		WithArgs("admin", int64(2), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Second exec: UPDATE users SET role=$1 WHERE id=$2 AND active_household_id=$3
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET role = $1 WHERE id = $2 AND active_household_id = $3`)).
		WithArgs("admin", int64(2), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()
	err = store.UpdateMemberRole(context.Background(), 1, 2, "admin")
	if err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
}

// TestJoinHousehold_ExhaustedInvite_Postgres verifies the service consumes the
// one-time invite BEFORE adding the member, and that an exhausted invite
// (0 rows affected by the atomic UPDATE) rejects the join without ever calling
// AddMember — sqlmock fails the test if any unexpected call happens.
func TestJoinHousehold_ExhaustedInvite_Postgres(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)
	svc := NewService(store, nil)

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))

	const code = "EXHAUSTED1"

	// Lookup succeeds but the code is already at max_uses.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, household_id, code, created_by, max_uses, used_count, expires_at, created_at
		FROM invites WHERE code = $1`)).
		WithArgs(code).
		WillReturnRows(sqlmock.NewRows([]string{"id", "household_id", "code", "created_by", "max_uses", "used_count", "expires_at", "created_at"}).
			AddRow(1, 1, code, 1, 1, 1, testTime, testTime))

	// Not already a member.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role FROM user_households WHERE user_id = $1 AND household_id = $2`)).
		WithArgs(int64(2), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}))

	// Capacity check: one existing member.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT u.id, u.email, u.display_name, u.avatar_color, u.email_verified, uh.role
		FROM user_households uh
		JOIN users u ON u.id = uh.user_id
		WHERE uh.household_id = $1`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "display_name", "avatar_color", "email_verified", "role"}).
			AddRow(9, "owner@example.com", "Owner", "#FF0000", true, "owner"))

	// The atomic consume: exhausted → 0 rows → ErrInviteNotFound.
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE invites SET used_count = used_count + 1
		WHERE code = $1
		  AND (max_uses <= 0 OR used_count < max_uses)
		  AND (expires_at IS NULL OR expires_at > NOW())`)).
		WithArgs(code).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectRollback()

	_, err = svc.JoinHousehold(context.Background(), 2, code)
	if err != ErrInviteNotFound {
		t.Fatalf("JoinHousehold: err = %v, want ErrInviteNotFound", err)
	}
	// No AddMember expectations were registered; any stray call fails here.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations (AddMember must never run): %v", err)
	}
}

// TestJoinHousehold_ViaOneTimeInvite_Postgres proves the happy path ordering:
// UseInvite runs (and consumes) before AddMember on the Postgres store.
func TestJoinHousehold_ViaOneTimeInvite_Postgres(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)
	svc := NewService(store, nil)

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))

	const code = "CODE123456"

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, household_id, code, created_by, max_uses, used_count, expires_at, created_at
		FROM invites WHERE code = $1`)).
		WithArgs(code).
		WillReturnRows(sqlmock.NewRows([]string{"id", "household_id", "code", "created_by", "max_uses", "used_count", "expires_at", "created_at"}).
			AddRow(1, 1, code, 1, 0, 0, testTime, testTime))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role FROM user_households WHERE user_id = $1 AND household_id = $2`)).
		WithArgs(int64(2), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT u.id, u.email, u.display_name, u.avatar_color, u.email_verified, uh.role
		FROM user_households uh
		JOIN users u ON u.id = uh.user_id
		WHERE uh.household_id = $1`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "display_name", "avatar_color", "email_verified", "role"}).
			AddRow(9, "owner@example.com", "Owner", "#FF0000", true, "owner"))

	// Consume the invite (1 row) BEFORE the member insert.
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE invites SET used_count = used_count + 1
		WHERE code = $1
		  AND (max_uses <= 0 OR used_count < max_uses)
		  AND (expires_at IS NULL OR expires_at > NOW())`)).
		WithArgs(code).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO user_households (user_id, household_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, household_id) DO NOTHING`)).
		WithArgs(int64(2), int64(1), "member").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET active_household_id = $1, household_id = $1, role = $2 WHERE id = $3`)).
		WithArgs(int64(1), "member", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, COALESCE(initials, ''), invite_code, created_at FROM households WHERE id = $1`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "initials", "invite_code", "created_at"}).
			AddRow(1, "Test", "", "PERMANENT1", testTime))

	mock.ExpectCommit()

	hh, err := svc.JoinHousehold(context.Background(), 2, code)
	if err != nil {
		t.Fatalf("JoinHousehold: %v", err)
	}
	if hh.ID != 1 {
		t.Fatalf("joined household id = %d, want 1", hh.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
