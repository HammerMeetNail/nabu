package schedule

import (
	"testing"
	"time"
)

func TestRecurrenceUsesCalendarComponentsAcrossDSTAndYear(t *testing.T) {
	svc := NewService()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := DateOnly{Time: time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)}
	sch := ChoreSchedule{IsActive: true, FrequencyType: "every_n_days", IntervalDays: 2, StartDate: &start}
	for _, tc := range []struct {
		day  int
		want bool
	}{{8, false}, {9, true}, {10, false}, {11, true}} {
		d := time.Date(2026, 3, tc.day, 20, 30, 0, 0, loc)
		if got := svc.IsActiveForDay(sch, d); got != tc.want {
			t.Fatalf("DST day=%d got=%v", tc.day, got)
		}
	}
	sch.FrequencyType = "once"
	if !svc.IsActiveForDay(sch, time.Date(2026, 3, 7, 23, 30, 0, 0, loc)) {
		t.Fatal("once skipped local evening")
	}
	if svc.IsActiveForDay(sch, time.Date(2026, 3, 8, 0, 30, 0, 0, loc)) {
		t.Fatal("once repeated following local day")
	}
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	sch = ChoreSchedule{IsActive: true, FrequencyType: "yearly", DayOfMonth: 31, MonthOfYear: 12, RecurrenceEnd: &end}
	if !svc.IsActiveForDay(sch, time.Date(2026, 12, 31, 23, 30, 0, 0, loc)) {
		t.Fatal("inclusive last local day skipped")
	}
	if svc.IsActiveForDay(sch, time.Date(2027, 1, 1, 0, 30, 0, 0, loc)) {
		t.Fatal("last local day leaked into next year")
	}
}
