package stats

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

type aggregateChores struct{ store chore.Store }

func (a aggregateChores) GetChore(ctx context.Context, id int64) (ChoreInfo, error) {
	c, err := a.store.GetChore(ctx, id)
	return ChoreInfo{ID: c.ID, HouseholdID: c.HouseholdID, Name: c.Name, Icon: c.Icon, Category: c.Category, MetricType: c.MetricType, MetricUnit: c.MetricUnit, Visibility: c.Visibility}, err
}
func (a aggregateChores) ListChores(ctx context.Context, id int64) ([]ChoreInfo, error) {
	cs, err := a.store.ListChores(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []ChoreInfo{}
	for _, c := range cs {
		info, err := a.GetChore(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, nil
}

func TestPostgresAggregatesMatchAuthorizedMemorySemantics(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO households(id,name,invite_code) VALUES(1,'Fixture','fixture'),(2,'Other','other')`,
		`INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'viewer@example.invalid','','Viewer'),(2,'author@example.invalid','','Author'),(3,'other@example.invalid','','Other')`,
		`INSERT INTO user_households(user_id,household_id,role) VALUES(1,1,'member'),(2,1,'owner'),(3,2,'owner')`,
		`INSERT INTO chores(id,household_id,name,category,visibility,metric_type,metric_unit) VALUES(10,1,'Visible','care','household','amount','mL'),(11,1,'Custom','','household','duration','seconds'),(12,1,'Private','secret','admins','check',''),(13,2,'Other','other','household','check','')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	pg, mem := chorelog.NewPostgresStore(db), chorelog.NewMemoryStore()
	memberships := household.NewPostgresStore(db)
	chores := aggregateChores{chore.NewPostgresStore(db)}
	sqlSvc := NewService(pg, chores).WithMemberships(memberships)
	memorySvc := NewService(mem, chores).WithMemberships(memberships)
	viewer := WithViewer(ctx, 1)
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	intp := func(n int) *int { return &n }
	strp := func(s string) *string { return &s }
	add := func(l chorelog.ChoreLog) {
		t.Helper()
		if _, err := pg.CreateLog(ctx, l); err != nil {
			t.Fatal(err)
		}
		if _, err := mem.CreateLog(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	// Both DST boundaries, exact completion bounds, canonical dates outside the
	// widened window, per-label volume replacement (including all nonpositive),
	// and distinct author/viewer identities. No JSON expansion may multiply counts.
	for _, day := range []time.Time{time.Date(2026, 3, 8, 0, 0, 0, 0, ny), time.Date(2026, 11, 1, 0, 0, 0, 0, ny)} {
		for i, at := range []time.Time{day.Add(-time.Second), day, day.Add(3 * time.Hour), day.AddDate(0, 0, 1).Add(-time.Second), day.AddDate(0, 0, 1)} {
			entry := chorelog.ChoreLog{HouseholdID: 1, UserID: 2, ChoreID: 10, CompletedAt: at, VolumeML: intp(999), DurationSeconds: intp(31), IndicatorVolumes: map[string]int{"a": 60, "b": 80, "bad": -1}}
			switch i {
			case 0:
				entry.IndicatorVolumes = nil
			case 1:
				entry.IndicatorVolumes = map[string]int{}
			case 2:
				entry.IndicatorVolumes = map[string]int{"a": 0, "b": -5}
			case 3:
				entry.LogDate = strp(day.AddDate(0, -1, 0).Format(time.DateOnly))
			}
			add(entry)
		}
	}
	now := time.Now().In(ny)
	midday := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, ny)
	for i := range 8 {
		user, id := int64(2), int64(10)
		if i < 2 {
			user = 1
			id = 11
		}
		add(chorelog.ChoreLog{HouseholdID: 1, UserID: user, ChoreID: id, CompletedAt: midday, VolumeML: intp(100), DurationSeconds: intp(60)})
	}
	add(chorelog.ChoreLog{HouseholdID: 1, UserID: 2, ChoreID: 12, CompletedAt: midday, VolumeML: intp(99999)})
	add(chorelog.ChoreLog{HouseholdID: 2, UserID: 3, ChoreID: 13, CompletedAt: midday, VolumeML: intp(99999)})
	// Overview's original year-candidate bound excludes this later canonical date
	// even though the completion time belongs to the current week.
	add(chorelog.ChoreLog{HouseholdID: 1, UserID: 1, ChoreID: 11, CompletedAt: midday, LogDate: strp(now.AddDate(0, 0, 10).Format(time.DateOnly))})
	for _, loc := range []*time.Location{time.UTC, ny, paris} {
		t.Run(loc.String(), func(t *testing.T) {
			for _, start := range []time.Time{time.Date(2026, 3, 8, 0, 0, 0, 0, loc), time.Date(2026, 11, 1, 0, 0, 0, 0, loc)} {
				end := start.AddDate(0, 0, 1)
				got, err := sqlSvc.getLeaderboard(viewer, 1, start, end, loc)
				if err != nil {
					t.Fatal(err)
				}
				want, err := memorySvc.getLeaderboard(viewer, 1, start, end, loc)
				if err != nil {
					t.Fatal(err)
				}
				sortLeaderboard(got)
				sortLeaderboard(want)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("leaderboard %s: got=%+v want=%+v", start, got, want)
				}
				gc, err := sqlSvc.GetCategoryBreakdown(viewer, 1, start, end, loc)
				if err != nil {
					t.Fatal(err)
				}
				wc, err := memorySvc.GetCategoryBreakdown(viewer, 1, start, end, loc)
				if err != nil {
					t.Fatal(err)
				}
				sortCategories(gc)
				sortCategories(wc)
				if !reflect.DeepEqual(gc, wc) {
					t.Fatalf("categories: got=%+v want=%+v", gc, wc)
				}
			}
			for _, period := range []string{"day", "week", "month", "all"} {
				for _, author := range []int64{0, 1, 2} {
					got, err := sqlSvc.GetTopChores(viewer, 1, author, 100, period, loc)
					if err != nil {
						t.Fatal(err)
					}
					want, err := memorySvc.GetTopChores(viewer, 1, author, 100, period, loc)
					if err != nil {
						t.Fatal(err)
					}
					sort.Slice(got, func(i, j int) bool { return got[i].ChoreID < got[j].ChoreID })
					sort.Slice(want, func(i, j int) bool { return want[i].ChoreID < want[j].ChoreID })
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("top %s author=%d: got=%+v want=%+v", period, author, got, want)
					}
				}
				for _, id := range []int64{10, 11} {
					got, err := sqlSvc.GetChoreSummary(viewer, 1, id, period, loc)
					if err != nil {
						t.Fatal(err)
					}
					want, err := memorySvc.GetChoreSummary(viewer, 1, id, period, loc)
					if err != nil {
						t.Fatal(err)
					}
					sortLeaderboard(got.ByMember)
					sortLeaderboard(want.ByMember)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("summary %s chore=%d: got=%+v want=%+v", period, id, got, want)
					}
				}
			}
			got, err := sqlSvc.GetAllTimeLeaderboard(viewer, 1, loc)
			if err != nil {
				t.Fatal(err)
			}
			want, err := memorySvc.GetAllTimeLeaderboard(viewer, 1, loc)
			if err != nil {
				t.Fatal(err)
			}
			sortLeaderboard(got)
			sortLeaderboard(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("all time: got=%+v want=%+v", got, want)
			}
			goa, err := sqlSvc.GetWeeklyOverview(viewer, 1, 2, loc)
			if err != nil {
				t.Fatal(err)
			}
			woa, err := memorySvc.GetWeeklyOverview(viewer, 1, 2, loc)
			if err != nil {
				t.Fatal(err)
			}
			sortLeaderboard(goa.Leaderboard)
			sortLeaderboard(woa.Leaderboard)
			sortCategories(goa.Breakdown)
			sortCategories(woa.Breakdown)
			sortCategories(goa.Recap.ByCategory)
			sortCategories(woa.Recap.ByCategory)
			if !reflect.DeepEqual(goa, woa) {
				t.Fatalf("overview: got=%+v want=%+v", goa, woa)
			}
		})
	}
	// Fast paths must retain current permission checks and cross-household denial.
	for _, id := range []int64{12, 13} {
		if _, err := sqlSvc.GetChoreSummary(viewer, 1, id, "all", ny); err == nil {
			t.Fatalf("hidden summary accepted: %d", id)
		}
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM user_households WHERE user_id=1`); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"leaderboard": func() error { _, err := sqlSvc.GetAllTimeLeaderboard(viewer, 1, ny); return err },
		"top":         func() error { _, err := sqlSvc.GetTopChores(viewer, 1, 2, 5, "all", ny); return err },
		"overview":    func() error { _, err := sqlSvc.GetWeeklyOverview(viewer, 1, 2, ny); return err },
		"summary":     func() error { _, err := sqlSvc.GetChoreSummary(viewer, 1, 10, "all", ny); return err },
	} {
		if err := call(); err == nil {
			t.Fatalf("removed viewer accepted by %s", name)
		}
	}
}
