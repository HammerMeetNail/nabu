// internal/schedule/service_test.go

package schedule

import (
	"testing"
	"time"
)

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func TestIsActiveForDay_Inactive(t *testing.T) {
	svc := NewService()
	sch := ChoreSchedule{IsActive: false, FrequencyType: "daily"}
	if svc.IsActiveForDay(sch, date(2026, 4, 28)) {
		t.Fatal("inactive schedule should not be active")
	}
}

func TestIsActiveForDay_Daily(t *testing.T) {
	svc := NewService()
	sch := ChoreSchedule{IsActive: true, FrequencyType: "daily"}
	days := []time.Time{date(2026, 4, 28), date(2026, 5, 1), date(2026, 12, 31)}
	for _, d := range days {
		if !svc.IsActiveForDay(sch, d) {
			t.Fatalf("daily schedule should be active on %v", d)
		}
	}
}

func TestIsActiveForDay_Weekly(t *testing.T) {
	svc := NewService()
	// Mon=1, Wed=3, Fri=5
	sch := ChoreSchedule{IsActive: true, FrequencyType: "weekly", DaysOfWeek: []int{1, 3, 5}}
	tests := []struct {
		day  time.Time
		want bool
	}{
		{date(2026, 4, 27), true},  // Monday
		{date(2026, 4, 28), false}, // Tuesday
		{date(2026, 4, 29), true},  // Wednesday
		{date(2026, 4, 30), false}, // Thursday
		{date(2026, 5, 1), true},   // Friday
		{date(2026, 5, 2), false},  // Saturday
		{date(2026, 5, 3), false},  // Sunday
	}
	for _, tt := range tests {
		got := svc.IsActiveForDay(sch, tt.day)
		if got != tt.want {
			t.Errorf("IsActiveForDay(%v) = %v, want %v", tt.day, got, tt.want)
		}
	}
}

func TestIsActiveForDay_EveryNDays(t *testing.T) {
	svc := NewService()
	origin := date(2026, 4, 1) // Wednesday
	sch := ChoreSchedule{
		IsActive:      true,
		FrequencyType: "every_n_days",
		IntervalDays:  3,
		CreatedAt:     origin,
	}
	// Day 0: Apr 1 ✓, Day 3: Apr 4 ✓, Day 6: Apr 7 ✓, Day 2: Apr 3 ✗
	tests := []struct {
		day  time.Time
		want bool
	}{
		{date(2026, 4, 1), true},
		{date(2026, 4, 4), true},
		{date(2026, 4, 7), true},
		{date(2026, 4, 3), false},
		{date(2026, 4, 5), false},
	}
	for _, tt := range tests {
		if got := svc.IsActiveForDay(sch, tt.day); got != tt.want {
			t.Errorf("IsActiveForDay(%v) = %v, want %v", tt.day, got, tt.want)
		}
	}
}

func TestIsActiveForDay_MonthlyByDate(t *testing.T) {
	svc := NewService()
	sch := ChoreSchedule{IsActive: true, FrequencyType: "monthly_by_date", DayOfMonth: 15}
	tests := []struct {
		day  time.Time
		want bool
	}{
		{date(2026, 4, 15), true},
		{date(2026, 5, 15), true},
		{date(2026, 4, 14), false},
		{date(2026, 4, 16), false},
	}
	for _, tt := range tests {
		if got := svc.IsActiveForDay(sch, tt.day); got != tt.want {
			t.Errorf("date %v: got %v want %v", tt.day, got, tt.want)
		}
	}
}

func TestIsActiveForDay_MonthlyByWeekday(t *testing.T) {
	svc := NewService()
	// 3rd Monday of each month
	sch := ChoreSchedule{
		IsActive:      true,
		FrequencyType: "monthly_by_weekday",
		MonthWeekday:  &MonthWeekday{Week: 3, Day: 1}, // Monday=1
	}
	// April 2026: 1st Mon=6th, 2nd=13th, 3rd=20th
	// May 2026:   1st Mon=4th, 2nd=11th, 3rd=18th
	tests := []struct {
		day  time.Time
		want bool
	}{
		{date(2026, 4, 20), true},
		{date(2026, 4, 13), false}, // 2nd Monday
		{date(2026, 5, 18), true},
		{date(2026, 5, 11), false},
	}
	for _, tt := range tests {
		if got := svc.IsActiveForDay(sch, tt.day); got != tt.want {
			t.Errorf("date %v: got %v want %v", tt.day, got, tt.want)
		}
	}
}

func TestIsActiveForDay_Yearly(t *testing.T) {
	svc := NewService()
	sch := ChoreSchedule{
		IsActive:      true,
		FrequencyType: "yearly",
		DayOfMonth:    14,
		MonthOfYear:   2, // February 14
	}
	tests := []struct {
		day  time.Time
		want bool
	}{
		{date(2026, 2, 14), true},
		{date(2027, 2, 14), true},
		{date(2026, 2, 13), false},
		{date(2026, 3, 14), false},
	}
	for _, tt := range tests {
		if got := svc.IsActiveForDay(sch, tt.day); got != tt.want {
			t.Errorf("date %v: got %v want %v", tt.day, got, tt.want)
		}
	}
}

func TestIsActiveForDay_RecurrenceEnd(t *testing.T) {
	svc := NewService()
	endDate := date(2026, 5, 1)
	sch := ChoreSchedule{
		IsActive:      true,
		FrequencyType: "daily",
		RecurrenceEnd: &endDate,
	}
	if !svc.IsActiveForDay(sch, date(2026, 4, 30)) {
		t.Fatal("should be active before end date")
	}
	if svc.IsActiveForDay(sch, date(2026, 5, 2)) {
		t.Fatal("should NOT be active after end date")
	}
}

func TestGetSchedulesForDate(t *testing.T) {
	svc := NewService()
	schedules := []ChoreSchedule{
		{ID: 1, IsActive: true, FrequencyType: "daily", ChoreID: 10},
		{ID: 2, IsActive: true, FrequencyType: "weekly", ChoreID: 20, DaysOfWeek: []int{1}}, // Mon
		{ID: 3, IsActive: true, FrequencyType: "weekly", ChoreID: 30, DaysOfWeek: []int{2}}, // Tue
	}
	monday := date(2026, 4, 27)
	result := svc.GetSchedulesForDate(schedules, monday)
	// Should return IDs 1 (daily) and 2 (weekly on Monday)
	if len(result) != 2 {
		t.Fatalf("got %d schedules, want 2", len(result))
	}
	ids := map[int64]bool{}
	for _, r := range result {
		ids[r.ID] = true
	}
	if !ids[1] || !ids[2] {
		t.Errorf("got IDs %v, want {1,2}", ids)
	}
}
