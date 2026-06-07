// internal/schedule/service.go

package schedule

import (
	"encoding/json"
	"time"
)

// TimePeriod represents when a chore is scheduled.
type TimePeriod string

const (
	PeriodAnytime TimePeriod = "anytime"
)

// DateOnly is a time.Time that marshals/unmarshals as a plain YYYY-MM-DD string.
// This keeps the JSON API surface clean (no RFC-3339 timezone noise for dates).
type DateOnly struct {
	time.Time
}

func (d DateOnly) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(d.Time.UTC().Format("2006-01-02"))
}

func (d *DateOnly) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}
	d.Time = t
	return nil
}

// MonthWeekday encodes "Nth weekday of the month", e.g. 3rd Monday.
type MonthWeekday struct {
	Week int `json:"week"` // 1-5
	Day  int `json:"day"`  // 0=Sunday … 6=Saturday
}

// ChoreSchedule is the canonical schedule record.
type ChoreSchedule struct {
	ID             int64         `json:"id"`
	HouseholdID    int64         `json:"householdId"`
	ChoreID        int64         `json:"choreId"`
	FrequencyType  string        `json:"frequencyType"`
	TimePeriod     TimePeriod    `json:"timePeriod"`
	SpecificTime   string        `json:"specificTime,omitempty"` // "HH:MM", optional
	TimesOfDay     []string      `json:"timesOfDay"`             // legacy; kept for compat
	DaysOfWeek     []int         `json:"daysOfWeek"`
	IntervalDays   int           `json:"intervalDays"`
	DayOfMonth     int           `json:"dayOfMonth,omitempty"`
	MonthWeekday   *MonthWeekday `json:"monthWeekday,omitempty"`
	MonthOfYear    int           `json:"monthOfYear,omitempty"`
	RecurrenceEnd  *time.Time    `json:"recurrenceEnd,omitempty"`
	StartDate      *DateOnly     `json:"startDate,omitempty"` // for "once" frequency
	TargetCount    int           `json:"targetCount"`
	IsActive       bool          `json:"isActive"`
	IsFollowUp     bool          `json:"isFollowUp"`
	AssignedUserID *int64        `json:"assignedUserId"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
}

// Service holds schedule business logic.
type Service struct{}

// NewService creates a new Service.
func NewService() *Service {
	return &Service{}
}

// IsActiveForDay returns true if the schedule should show a chore card on the
// given calendar date. It does NOT check the time of day — that is handled by
// the UI bucketing logic.
func (s *Service) IsActiveForDay(sch ChoreSchedule, date time.Time) bool {
	if !sch.IsActive {
		return false
	}
	if sch.RecurrenceEnd != nil && date.After(*sch.RecurrenceEnd) {
		return false
	}

	d := date.Truncate(24 * time.Hour)

	switch sch.FrequencyType {
	case "once":
		if sch.StartDate == nil {
			return false
		}
		sd := sch.StartDate.Time.UTC().Truncate(24 * time.Hour)
		return d.Equal(sd)

	case "daily":
		return true

	case "weekly":
		wd := int(d.Weekday())
		for _, allowed := range sch.DaysOfWeek {
			if wd == allowed {
				return true
			}
		}
		return false

	case "every_n_days":
		if sch.IntervalDays <= 0 {
			return false
		}
		origin := sch.CreatedAt.Truncate(24 * time.Hour)
		if sch.StartDate != nil {
			origin = sch.StartDate.Time.UTC().Truncate(24 * time.Hour)
		}
		diff := int(d.Sub(origin).Hours() / 24)
		return diff >= 0 && diff%sch.IntervalDays == 0

	case "monthly_by_date":
		return d.Day() == sch.DayOfMonth

	case "monthly_by_weekday":
		if sch.MonthWeekday == nil {
			return false
		}
		return isNthWeekdayOfMonth(d, sch.MonthWeekday.Week, sch.MonthWeekday.Day)

	case "yearly":
		return d.Day() == sch.DayOfMonth && int(d.Month()) == sch.MonthOfYear

	default:
		return false
	}
}

// isNthWeekdayOfMonth returns true if t is the Nth occurrence of the given
// weekday (0=Sun…6=Sat) in its month.
func isNthWeekdayOfMonth(t time.Time, week, weekday int) bool {
	count := 0
	for d := 1; d <= t.Day(); d++ {
		if int(time.Date(t.Year(), t.Month(), d, 0, 0, 0, 0, t.Location()).Weekday()) == weekday {
			count++
		}
	}
	return int(t.Weekday()) == weekday && count == week
}

// GetSchedulesForDate filters a slice of schedules to those active on date.
func (s *Service) GetSchedulesForDate(schedules []ChoreSchedule, date time.Time) []ChoreSchedule {
	var out []ChoreSchedule
	for _, sch := range schedules {
		if s.IsActiveForDay(sch, date) {
			out = append(out, sch)
		}
	}
	return out
}
