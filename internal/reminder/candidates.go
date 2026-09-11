package reminder

import (
	"context"
	"encoding/json"
	"time"

	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/schedule"
)

type candidateCursor struct{ ScheduleID, UserID int64 }

// Candidate pages are authorization-filtered and carry the scalar preferences
// needed for the exact Go calendar calculation. No per-member preference reads
// are needed. Keeping recurrence in one implementation preserves DST semantics.
type candidate struct {
	Schedule     schedule.ChoreSchedule
	UserID       int64
	LeadMinutes  int
	Preferences  notification.ReminderPreference
	UserTimezone string
	SentDates    []string
}

type candidateReader interface {
	CandidatePage(context.Context, candidateCursor, int, time.Time) ([]candidate, error)
}

func (s *PostgresStore) CandidatePage(ctx context.Context, after candidateCursor, limit int, now time.Time) ([]candidate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
 s.id, s.household_id, s.chore_id, s.frequency_type, s.specific_time,
 COALESCE(s.days_of_week,'[]'::jsonb), COALESCE(s.interval_days,0),
 COALESCE(s.day_of_month,0), s.month_weekday, COALESCE(s.month_of_year,0),
 s.recurrence_end_date, s.start_date, s.created_at,
 m.user_id, CASE WHEN COALESCE(cp.enabled,false) THEN cp.lead_minutes
                ELSE COALESCE(np.default_reminder_lead_minutes,10) END,
 COALESCE(np.timezone,'UTC'), COALESCE(up.timezone,''),
 COALESCE(np.quiet_hours_start,''), COALESCE(np.quiet_hours_end,''),
 COALESCE((SELECT jsonb_agg(r.scheduled_date::text) FROM schedule_reminders r
           WHERE r.schedule_id=s.id AND r.user_id=m.user_id
           AND r.scheduled_date BETWEEN $4::date-2 AND $4::date+2), '[]'::jsonb)
 FROM chore_schedules s
 JOIN chores c ON c.id=s.chore_id AND c.household_id=s.household_id
 JOIN user_households m ON m.household_id=s.household_id
 LEFT JOIN reminder_preferences np ON np.user_id=m.user_id
 LEFT JOIN user_preferences up ON up.user_id=m.user_id
 LEFT JOIN chore_reminder_prefs cp ON cp.user_id=m.user_id AND cp.chore_id=s.chore_id
 WHERE s.is_active AND COALESCE(s.specific_time,'')<>''
 AND (s.id,m.user_id)>($1,$2)
 AND (s.assigned_to_user_id=m.user_id OR (s.assigned_to_user_id IS NULL AND cp.enabled))
 AND (c.visibility='household' OR m.role IN ('owner','admin'))
 AND COALESCE(np.push_enabled,true)
 AND (COALESCE(np.enabled_push_types,'[]'::jsonb)='[]'::jsonb
      OR (jsonb_typeof(np.enabled_push_types)='array' AND np.enabled_push_types ? 'schedule_reminder'))
 ORDER BY s.id,m.user_id LIMIT $3`, after.ScheduleID, after.UserID, min(max(limit, 1), 256), now.UTC().Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []candidate
	for rows.Next() {
		var c candidate
		var days, monthWeekday, sent []byte
		var start *time.Time
		sch := &c.Schedule
		if err := rows.Scan(&sch.ID, &sch.HouseholdID, &sch.ChoreID, &sch.FrequencyType, &sch.SpecificTime,
			&days, &sch.IntervalDays, &sch.DayOfMonth, &monthWeekday, &sch.MonthOfYear,
			&sch.RecurrenceEnd, &start, &sch.CreatedAt, &c.UserID, &c.LeadMinutes,
			&c.Preferences.Timezone, &c.UserTimezone, &c.Preferences.QuietHoursStart, &c.Preferences.QuietHoursEnd, &sent); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(days, &sch.DaysOfWeek); err != nil {
			return nil, err
		}
		if len(monthWeekday) > 0 {
			if err := json.Unmarshal(monthWeekday, &sch.MonthWeekday); err != nil {
				return nil, err
			}
		}
		if err := json.Unmarshal(sent, &c.SentDates); err != nil {
			return nil, err
		}
		if start != nil {
			sch.StartDate = &schedule.DateOnly{Time: *start}
		}
		sch.IsActive = true
		out = append(out, c)
	}
	return out, rows.Err()
}

func reminderLocation(notificationZone, userZone string) *time.Location {
	if notificationZone != "" && notificationZone != "UTC" {
		if loc, err := time.LoadLocation(notificationZone); err == nil {
			return loc
		}
	}
	if userZone != "" {
		if loc, err := time.LoadLocation(userZone); err == nil {
			return loc
		}
	}
	return time.UTC
}

func quietAt(p notification.ReminderPreference, now time.Time) bool {
	if p.QuietHoursStart == "" || p.QuietHoursEnd == "" {
		return false
	}
	loc := time.UTC
	if p.Timezone != "" {
		if found, err := time.LoadLocation(p.Timezone); err == nil {
			loc = found
		}
	}
	return isBetween(now.In(loc), p.QuietHoursStart, p.QuietHoursEnd)
}
