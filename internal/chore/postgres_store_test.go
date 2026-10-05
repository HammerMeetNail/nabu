package chore

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

var testTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func TestPostgresChoreStore_GetChore(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, household_id, name, icon, color, sort_order, category, is_predefined, COALESCE(predefined_key,''), created_by, created_at, indicator_labels, has_volume_ml, COALESCE(indicator_defaults,'[]'), follow_up_enabled, last_follow_up_minutes, has_rating, COALESCE(metric_type,'none'), COALESCE(metric_unit,''), COALESCE(subjects,'[]'), COALESCE(visibility,'household'), COALESCE(quiet_hours_start,''), COALESCE(quiet_hours_end,'') FROM chores WHERE id = $1`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "household_id", "name", "icon", "color", "sort_order", "category", "is_predefined", "predefined_key", "created_by", "created_at", "indicator_labels", "has_volume_ml", "indicator_defaults", "follow_up_enabled", "last_follow_up_minutes", "has_rating", "metric_type", "metric_unit", "subjects", "visibility", "quiet_hours_start", "quiet_hours_end"}).
			AddRow(1, 1, "Test", "🧹", "#FF0000", 0, "cleaning", false, "", nil, testTime, "[]", false, "[]", false, 0, false, "none", "", "[]", "household", "22:00", "07:00"))

	c, err := store.GetChore(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetChore: %v", err)
	}
	if c.Name != "Test" {
		t.Fatalf("Name = %q, want Test", c.Name)
	}
	if c.QuietHoursStart != "22:00" || c.QuietHoursEnd != "07:00" {
		t.Fatalf("quiet hours = %q..%q, want 22:00..07:00", c.QuietHoursStart, c.QuietHoursEnd)
	}
}

func TestPostgresChoreStore_GetChoreNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectQuery(`SELECT`).WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)

	_, err = store.GetChore(context.Background(), 9)
	if err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPostgresChoreStore_ListChores(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, household_id, name, icon, color, sort_order, category, is_predefined, COALESCE(predefined_key,''), created_by, created_at, indicator_labels, has_volume_ml, COALESCE(indicator_defaults,'[]'), follow_up_enabled, last_follow_up_minutes, has_rating, COALESCE(metric_type,'none'), COALESCE(metric_unit,''), COALESCE(subjects,'[]'), COALESCE(visibility,'household'), COALESCE(quiet_hours_start,''), COALESCE(quiet_hours_end,'') FROM chores WHERE household_id = $1 ORDER BY sort_order`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "household_id", "name", "icon", "color", "sort_order", "category", "is_predefined", "predefined_key", "created_by", "created_at", "indicator_labels", "has_volume_ml", "indicator_defaults", "follow_up_enabled", "last_follow_up_minutes", "has_rating", "metric_type", "metric_unit", "subjects", "visibility", "quiet_hours_start", "quiet_hours_end"}).
			AddRow(1, 1, "A", "🧹", "#F00", 0, "cleaning", false, "", nil, testTime, "[]", false, "[]", false, 0, false, "none", "", "[]", "household", "", "").
			AddRow(2, 1, "B", "🧹", "#0F0", 1, "care", false, "", nil, testTime, "[]", false, "[]", false, 0, false, "none", "", "[]", "household", "22:00", "07:00"))

	chores, err := store.ListChores(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListChores: %v", err)
	}
	if len(chores) != 2 {
		t.Fatalf("len = %d, want 2", len(chores))
	}
}

func TestPostgresChoreStore_UpdateChore(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE chores SET name=$1, icon=$2, color=$3, category=$4, indicator_labels=$5, indicator_defaults=$6, follow_up_enabled=$7, last_follow_up_minutes=$8, has_volume_ml=$9, has_rating=$10, metric_type=$11, metric_unit=$12, subjects=$13, visibility=$14, quiet_hours_start=$15, quiet_hours_end=$16 WHERE id=$17`)).
		WithArgs("Updated", "🧹", "#F00", "cleaning", "[]", "[]", false, 0, false, false, "none", "", "[]", "household", "21:00", "06:00", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.UpdateChore(context.Background(), Chore{ID: 1, Name: "Updated", Icon: "🧹", Color: "#F00", Category: "cleaning", Visibility: "household", QuietHoursStart: "21:00", QuietHoursEnd: "06:00"})
	if err != nil {
		t.Fatalf("UpdateChore: %v", err)
	}
}

func TestPostgresChoreStore_DeleteChore(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM chores WHERE id = $1`)).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.DeleteChore(context.Background(), 1)
	if err != nil {
		t.Fatalf("DeleteChore: %v", err)
	}
}

func TestPostgresChoreStore_ReorderChores(t *testing.T) {
	// ReorderChores uses UNNEST($1::bigint[]) which requires pgx array
	// support. sqlmock's driver cannot convert []int64 to a SQL value.
	t.Skip("sqlmock does not support pgx array types")
}

func TestPostgresChoreStore_CreateChore(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO chores (household_id, name, icon, color, sort_order, category, is_predefined, predefined_key, created_by, indicator_labels, has_volume_ml, indicator_defaults, follow_up_enabled, last_follow_up_minutes, has_rating, metric_type, metric_unit, subjects, visibility, quiet_hours_start, quiet_hours_end) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`)).
		WithArgs(int64(1), "New", "🧹", "#F00", 0, "cleaning", false, (*string)(nil), (*int64)(nil), "[]", false, "[]", false, 0, false, "none", "", "[]", "household", (*string)(nil), (*string)(nil)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(3, testTime))

	c, err := store.CreateChore(context.Background(), Chore{HouseholdID: 1, Name: "New", Icon: "🧹", Color: "#F00", Category: "cleaning", Visibility: "household"})
	if err != nil {
		t.Fatalf("CreateChore: %v", err)
	}
	if c.ID != 3 {
		t.Fatalf("ID = %d, want 3", c.ID)
	}
}
