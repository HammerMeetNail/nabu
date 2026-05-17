package userprefs_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/dave/choresy/internal/userprefs"
)

// ─── Memory store tests ───────────────────────────────────────────────────────

func TestMemoryStore_GetMissing(t *testing.T) {
	s := userprefs.NewMemoryStore()
	p, err := s.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(p.ChoreOrder) != 0 {
		t.Fatalf("expected empty ChoreOrder, got %v", p.ChoreOrder)
	}
}

func TestMemoryStore_UpsertAndGet(t *testing.T) {
	s := userprefs.NewMemoryStore()
	want := userprefs.Preferences{ChoreOrder: []int64{3, 1, 2}}
	if err := s.Upsert(context.Background(), 42, want); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.Get(context.Background(), 42)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.ChoreOrder) != 3 || got.ChoreOrder[0] != 3 {
		t.Fatalf("ChoreOrder = %v, want [3 1 2]", got.ChoreOrder)
	}
}

func TestMemoryStore_UpsertOverwrites(t *testing.T) {
	s := userprefs.NewMemoryStore()
	_ = s.Upsert(context.Background(), 1, userprefs.Preferences{ChoreOrder: []int64{10, 20}})
	_ = s.Upsert(context.Background(), 1, userprefs.Preferences{ChoreOrder: []int64{20, 10}})
	p, _ := s.Get(context.Background(), 1)
	if len(p.ChoreOrder) != 2 || p.ChoreOrder[0] != 20 {
		t.Fatalf("expected [20 10], got %v", p.ChoreOrder)
	}
}

func TestMemoryStore_IsolateUsers(t *testing.T) {
	s := userprefs.NewMemoryStore()
	_ = s.Upsert(context.Background(), 1, userprefs.Preferences{ChoreOrder: []int64{1}})
	_ = s.Upsert(context.Background(), 2, userprefs.Preferences{ChoreOrder: []int64{2}})
	p1, _ := s.Get(context.Background(), 1)
	p2, _ := s.Get(context.Background(), 2)
	if p1.ChoreOrder[0] != 1 || p2.ChoreOrder[0] != 2 {
		t.Fatalf("user isolation broken: user1=%v user2=%v", p1.ChoreOrder, p2.ChoreOrder)
	}
}

// ─── Service tests ────────────────────────────────────────────────────────────

func TestService_GetAndUpdate(t *testing.T) {
	svc := userprefs.NewService(userprefs.NewMemoryStore())
	ctx := context.Background()

	p, err := svc.GetPreferences(ctx, 7)
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if len(p.ChoreOrder) != 0 {
		t.Fatalf("expected empty, got %v", p.ChoreOrder)
	}

	if err := svc.UpdateChoreOrder(ctx, 7, []int64{5, 3, 1}); err != nil {
		t.Fatalf("UpdateChoreOrder: %v", err)
	}
	p, err = svc.GetPreferences(ctx, 7)
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if len(p.ChoreOrder) != 3 || p.ChoreOrder[0] != 5 {
		t.Fatalf("ChoreOrder = %v, want [5 3 1]", p.ChoreOrder)
	}
}

func TestService_NilOrderBecomesEmpty(t *testing.T) {
	svc := userprefs.NewService(userprefs.NewMemoryStore())
	if err := svc.UpdateChoreOrder(context.Background(), 1, nil); err != nil {
		t.Fatalf("UpdateChoreOrder: %v", err)
	}
	p, _ := svc.GetPreferences(context.Background(), 1)
	if p.ChoreOrder == nil {
		t.Fatal("ChoreOrder must not be nil after UpdateChoreOrder(nil)")
	}
}

// ─── Postgres store tests ─────────────────────────────────────────────────────

func TestPostgresStore_GetMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := userprefs.NewPostgresStore(db)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT chore_order FROM user_preferences WHERE user_id = $1`)).
		WithArgs(int64(1)).
		WillReturnError(sql.ErrNoRows)

	p, err := store.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(p.ChoreOrder) != 0 {
		t.Fatalf("expected empty ChoreOrder, got %v", p.ChoreOrder)
	}
}

func TestPostgresStore_GetExisting(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := userprefs.NewPostgresStore(db)
	raw, _ := json.Marshal([]int64{3, 1, 2})
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT chore_order FROM user_preferences WHERE user_id = $1`)).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"chore_order"}).AddRow(raw))

	p, err := store.Get(context.Background(), 5)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(p.ChoreOrder) != 3 || p.ChoreOrder[0] != 3 {
		t.Fatalf("ChoreOrder = %v, want [3 1 2]", p.ChoreOrder)
	}
}

func TestPostgresStore_Upsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := userprefs.NewPostgresStore(db)
	raw, _ := json.Marshal([]int64{10, 20})
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO user_preferences`)).
		WithArgs(int64(9), raw).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.Upsert(context.Background(), 9, userprefs.Preferences{ChoreOrder: []int64{10, 20}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
}
