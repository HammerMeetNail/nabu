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

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO households (name, invite_code) VALUES ($1, $2)`)).
		WithArgs("My Home", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "invite_code", "created_at"}).
			AddRow(1, "My Home", "ABC123", testTime))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET household_id = $1, role = 'owner' WHERE id = $2`)).
		WithArgs(int64(1), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	hh, err := store.CreateHousehold(context.Background(), "My Home", 1)
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

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, invite_code, created_at FROM households WHERE id = $1`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "invite_code", "created_at"}).
			AddRow(1, "My Home", "ABC", testTime))

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

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, email, display_name, avatar_color, email_verified, role FROM users WHERE household_id = $1`)).
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

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET household_id = $1, role = $2 WHERE id = $3`)).
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

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET household_id = NULL, role = 'member' WHERE id = $1 AND household_id = $2`)).
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

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET role = $1 WHERE id = $2 AND household_id = $3`)).
		WithArgs("admin", int64(2), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.UpdateMemberRole(context.Background(), 1, 2, "admin")
	if err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
}
