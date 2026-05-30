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

	// AddMember(ctx, householdID=1, userID=2, role="member")
	// First exec: INSERT INTO user_households (user_id=$2, household_id=$1, role=$3)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO user_households (user_id, household_id, role) VALUES ($1, $2, $3) ON CONFLICT (user_id, household_id) DO NOTHING`)).
		WithArgs(int64(2), int64(1), "member").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Second exec: UPDATE users SET active_household_id=$1, household_id=$1, role=$2 WHERE id=$3
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET active_household_id = $1, household_id = $1, role = $2 WHERE id = $3`)).
		WithArgs(int64(1), "member", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

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

	err = store.RemoveMember(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
}

func TestPostgresHouseholdStore_UpdateMemberRole(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	// UpdateMemberRole(ctx, householdID=1, userID=2, role="admin")
	// First exec: UPDATE user_households SET role=$1 WHERE user_id=$2 AND household_id=$3
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE user_households SET role = $1 WHERE user_id = $2 AND household_id = $3`)).
		WithArgs("admin", int64(2), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Second exec: UPDATE users SET role=$1 WHERE id=$2 AND active_household_id=$3
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET role = $1 WHERE id = $2 AND active_household_id = $3`)).
		WithArgs("admin", int64(2), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.UpdateMemberRole(context.Background(), 1, 2, "admin")
	if err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
}
