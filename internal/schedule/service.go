package schedule

import (
	"time"
)

type ChoreSchedule struct {
	ID              int64     `json:"id"`
	HouseholdID     int64     `json:"householdId"`
	ChoreID         int64     `json:"choreId"`
	FrequencyType   string    `json:"frequencyType"`
	TimesOfDay      []string  `json:"timesOfDay"`
	DaysOfWeek      []int     `json:"daysOfWeek"`
	IntervalDays    int       `json:"intervalDays"`
	TargetCount     int       `json:"targetCount"`
	IsActive        bool      `json:"isActive"`
	AssignedUserID  *int64    `json:"assignedUserId"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Service struct {
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) IsSlotActive(schedule ChoreSchedule, now time.Time) bool {
	if !schedule.IsActive {
		return false
	}
	weekday := now.Weekday()
	switch schedule.FrequencyType {
	case "weekly":
		for _, d := range schedule.DaysOfWeek {
			if d == int(weekday) {
				for _, t := range schedule.TimesOfDay {
					if matchesTime(t, now) {
						return true
					}
				}
			}
		}
		return false
	case "daily":
		for _, t := range schedule.TimesOfDay {
			if matchesTime(t, now) {
				return true
			}
		}
		return false
	default:
		for _, t := range schedule.TimesOfDay {
			if matchesTime(t, now) {
				return true
			}
		}
		return false
	}
}

func (s *Service) GetTodaysSlots(schedules []ChoreSchedule) []int64 {
	var choreIDs []int64
	now := time.Now()
	for _, sch := range schedules {
		if s.IsSlotActive(sch, now) {
			choreIDs = append(choreIDs, sch.ChoreID)
		}
	}
	return choreIDs
}

func matchesTime(timeStr string, now time.Time) bool {
	h, m := 0, 0
	if _, err := fmtSscanf(timeStr, "%d:%d", &h, &m); err != nil {
		return false
	}
	return now.Hour() == h && now.Minute() >= m-1 && now.Minute() <= m+1
}

func fmtSscanf(s, format string, a ...any) (int, error) {
	var i, j int
	for k, arg := range a {
		if j >= len(format) {
			break
		}
		if format[j] != '%' {
			j++
			k--
			continue
		}
		_ = k
		if p, ok := arg.(*int); ok {
			n := 0
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				n = n*10 + int(s[i]-'0')
				i++
			}
			*p = n
		}
		if i < len(s) && j+1 < len(format) && s[i] == format[j+1] {
			i++
			j += 2
		}
	}
	return len(a), nil
}
