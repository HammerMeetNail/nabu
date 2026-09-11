package handlers

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/daynote"
	"github.com/HammerMeetNail/nabu/internal/household"
	logsvc "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"github.com/HammerMeetNail/nabu/internal/schedule"
)

// ExportHandler produces a normalized, multi-record CSV for an administrator's
// active household. The existing log-only export remains available to every
// household member; this endpoint is intentionally broader and role-gated.
type ExportHandler struct {
	householdService *household.Service
	householdStore   household.Store
	choreStore       chore.Store
	logService       *logsvc.Service
	scheduleStore    schedule.Store
	dayNoteService   *daynote.Service
}

func NewExportHandler(
	householdService *household.Service,
	householdStore household.Store,
	choreStore chore.Store,
	logService *logsvc.Service,
	scheduleStore schedule.Store,
	dayNoteService *daynote.Service,
) *ExportHandler {
	return &ExportHandler{
		householdService: householdService,
		householdStore:   householdStore,
		choreStore:       choreStore,
		logService:       logService,
		scheduleStore:    scheduleStore,
		dayNoteService:   dayNoteService,
	}
}

var householdExportColumns = []string{
	"record_type",
	"id",
	"household_id",
	"household_name",
	"household_initials",
	"household_created_at",
	"user_id",
	"email",
	"display_name",
	"avatar_color",
	"email_verified",
	"role",
	"chore_id",
	"chore_name",
	"chore_icon",
	"chore_color",
	"chore_category",
	"chore_sort_order",
	"chore_is_predefined",
	"chore_predefined_key",
	"chore_created_by",
	"chore_created_at",
	"chore_indicator_labels",
	"chore_indicator_defaults",
	"chore_has_volume_ml",
	"chore_follow_up_enabled",
	"chore_last_follow_up_minutes",
	"chore_has_rating",
	"chore_metric_type",
	"chore_metric_unit",
	"chore_subjects",
	"chore_visibility",
	"log_id",
	"log_user_id",
	"log_chore_id",
	"log_completed_at",
	"log_date",
	"log_slot_hour",
	"log_title",
	"log_note",
	"log_indicators",
	"log_indicator_volumes",
	"log_volume_ml", "log_metric_unit",
	"log_rating",
	"log_duration_seconds",
	"log_subject",
	"log_created_at",
	"schedule_id",
	"schedule_chore_id",
	"schedule_frequency_type",
	"schedule_time_period",
	"schedule_specific_time",
	"schedule_times_of_day",
	"schedule_days_of_week",
	"schedule_interval_days",
	"schedule_day_of_month",
	"schedule_month_weekday",
	"schedule_month_of_year",
	"schedule_recurrence_end",
	"schedule_start_date",
	"schedule_target_count",
	"schedule_is_active",
	"schedule_is_follow_up",
	"schedule_assigned_user_id",
	"schedule_created_at",
	"schedule_updated_at",
	"day_note_date",
	"day_note",
	"day_note_updated_by",
	"day_note_updated_at",
	"invite_id",
	"invite_created_by",
	"invite_max_uses",
	"invite_used_count",
	"invite_expires_at",
	"invite_created_at",
}

// The stores expose range queries rather than an unbounded ListAll method. The
// supported database date range gives the export an effectively unbounded
// window without inventing a product-specific cutoff such as 2000-01-01.
var householdExportStart = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
var householdExportEnd = time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)

// Data returns the active household's shared data as one CSV representation.
// GET /api/household/data
func (h *ExportHandler) Data(w http.ResponseWriter, r *http.Request) {
	r, cancel := exportContext(r)
	defer cancel()
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	hid, err := h.householdService.GetAdminHouseholdID(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, household.ErrNotAuthorized) {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		if errors.Is(err, household.ErrNotMember) || errors.Is(err, household.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "no household")
		} else {
			exportError(w, err)
		}
		return
	}

	// The shared gate reserved the household from the authenticated request.
	// An intervening switch must not move this job to another gate key.
	if user.HouseholdID == nil || hid != *user.HouseholdID {
		writeError(w, http.StatusForbidden, "household access changed; try again")
		return
	}
	start, end, err := exportRange(r, householdExportStart, householdExportEnd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	r = r.WithContext(readlimit.Remaining(r.Context(), 1))
	// Load every dataset before sending response headers. A failed lookup must
	// not produce a partial file that looks like a successful export.
	hh, err := h.householdStore.GetHousehold(r.Context(), hid)
	if err != nil {
		exportError(w, err)
		return
	}
	members, err := h.householdStore.GetMembers(r.Context(), hid)
	if err != nil {
		exportError(w, err)
		return
	}
	r = r.WithContext(readlimit.Remaining(r.Context(), len(members)))
	invites, err := h.householdStore.GetInvites(r.Context(), hid)
	if err != nil {
		exportError(w, err)
		return
	}
	r = r.WithContext(readlimit.Remaining(r.Context(), len(invites)))
	chores, err := h.choreStore.ListChores(r.Context(), hid)
	if err != nil {
		exportError(w, err)
		return
	}
	r = r.WithContext(readlimit.Remaining(r.Context(), len(chores)))
	visible := make(map[int64]struct{}, len(chores))
	for _, c := range chores {
		visible[c.ID] = struct{}{}
	}
	readCtx := logsvc.WithReadAccess(r.Context(), hid, user.ID, visible)
	logs, err := h.logService.GetLogsInRange(readCtx, hid, start, end)
	if err != nil {
		exportError(w, err)
		return
	}
	r = r.WithContext(readlimit.Remaining(r.Context(), len(logs)))
	schedules, err := h.scheduleStore.ListByHousehold(r.Context(), hid)
	if err != nil {
		exportError(w, err)
		return
	}
	r = r.WithContext(readlimit.Remaining(r.Context(), len(schedules)))
	var dayNotes []daynote.DayNote
	if h.dayNoteService != nil {
		dayNotes, err = h.dayNoteService.ListRange(r.Context(), hid, start, end)
		if err != nil {
			exportError(w, err)
			return
		}
	}

	sort.Slice(members, func(i, j int) bool { return members[i].UserID < members[j].UserID })
	sort.Slice(chores, func(i, j int) bool { return chores[i].ID < chores[j].ID })
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].CompletedAt.Equal(logs[j].CompletedAt) {
			return logs[i].ID < logs[j].ID
		}
		return logs[i].CompletedAt.Before(logs[j].CompletedAt)
	})
	sort.Slice(schedules, func(i, j int) bool { return schedules[i].ID < schedules[j].ID })
	sort.Slice(dayNotes, func(i, j int) bool { return dayNotes[i].Date < dayNotes[j].Date })
	sort.Slice(invites, func(i, j int) bool { return invites[i].ID < invites[j].ID })

	buffer := &exportBuffer{ctx: r.Context()}
	cw := csv.NewWriter(buffer)
	if err := cw.Write(householdExportColumns); err != nil {
		exportError(w, err)
		return
	}

	write := func(row map[string]string) {
		values := make([]string, len(householdExportColumns))
		for i, column := range householdExportColumns {
			values[i] = csvSafe(row[column])
		}
		_ = cw.Write(values)
	}
	base := func(recordType, id string) map[string]string {
		return map[string]string{
			"record_type":  recordType,
			"id":           id,
			"household_id": strconv.FormatInt(hid, 10),
		}
	}

	row := base("household", formatInt64(hh.ID))
	row["household_name"] = hh.Name
	row["household_initials"] = hh.Initials
	row["household_created_at"] = formatTime(hh.CreatedAt)
	write(row)

	for _, m := range members {
		row := base("member", formatInt64(m.UserID))
		row["user_id"] = formatInt64(m.UserID)
		row["email"] = m.Email
		row["display_name"] = m.DisplayName
		row["avatar_color"] = m.AvatarColor
		row["email_verified"] = strconv.FormatBool(m.EmailVerified)
		row["role"] = m.Role
		write(row)
	}

	for _, c := range chores {
		row := base("chore", formatInt64(c.ID))
		row["chore_id"] = formatInt64(c.ID)
		row["chore_name"] = c.Name
		row["chore_icon"] = c.Icon
		row["chore_color"] = c.Color
		row["chore_category"] = c.Category
		row["chore_sort_order"] = strconv.Itoa(c.SortOrder)
		row["chore_is_predefined"] = strconv.FormatBool(c.IsPredefined)
		row["chore_predefined_key"] = c.PredefinedKey
		row["chore_created_by"] = formatInt64Ptr(c.CreatedBy)
		row["chore_created_at"] = formatTime(c.CreatedAt)
		row["chore_indicator_labels"] = jsonString(c.IndicatorLabels)
		row["chore_indicator_defaults"] = jsonString(c.IndicatorDefaults)
		row["chore_has_volume_ml"] = strconv.FormatBool(c.HasVolumeML)
		row["chore_follow_up_enabled"] = strconv.FormatBool(c.FollowUpEnabled)
		row["chore_last_follow_up_minutes"] = strconv.Itoa(c.LastFollowUpMinutes)
		row["chore_has_rating"] = strconv.FormatBool(c.HasRating)
		row["chore_metric_type"] = c.MetricType
		row["chore_metric_unit"] = c.MetricUnit
		row["chore_subjects"] = jsonString(c.Subjects)
		row["chore_visibility"] = c.Visibility
		write(row)
	}

	for _, l := range logs {
		row := base("log", formatInt64(l.ID))
		row["log_id"] = formatInt64(l.ID)
		row["log_user_id"] = formatInt64(l.UserID)
		row["log_chore_id"] = formatInt64(l.ChoreID)
		row["log_completed_at"] = formatTime(l.CompletedAt)
		if l.LogDate != nil {
			row["log_date"] = *l.LogDate
		}
		row["log_slot_hour"] = formatIntPtr(l.SlotHour)
		row["log_title"] = formatStringPtr(l.Title)
		row["log_note"] = l.Note
		row["log_indicators"] = jsonString(l.Indicators)
		row["log_indicator_volumes"] = jsonString(l.IndicatorVolumes)
		row["log_volume_ml"] = formatIntPtr(l.VolumeML)
		row["log_metric_unit"] = l.MetricUnit
		if l.Rating != nil {
			row["log_rating"] = strconv.FormatFloat(float64(*l.Rating)/10, 'f', -1, 64)
		}
		row["log_duration_seconds"] = formatIntPtr(l.DurationSeconds)
		row["log_subject"] = formatStringPtr(l.Subject)
		row["log_created_at"] = formatTime(l.CreatedAt)
		write(row)
	}

	for _, s := range schedules {
		row := base("schedule", formatInt64(s.ID))
		row["schedule_id"] = formatInt64(s.ID)
		row["schedule_chore_id"] = formatInt64(s.ChoreID)
		row["schedule_frequency_type"] = s.FrequencyType
		row["schedule_time_period"] = string(s.TimePeriod)
		row["schedule_specific_time"] = s.SpecificTime
		row["schedule_times_of_day"] = jsonString(s.TimesOfDay)
		row["schedule_days_of_week"] = jsonString(s.DaysOfWeek)
		row["schedule_interval_days"] = strconv.Itoa(s.IntervalDays)
		row["schedule_day_of_month"] = strconv.Itoa(s.DayOfMonth)
		row["schedule_month_weekday"] = jsonString(s.MonthWeekday)
		row["schedule_month_of_year"] = strconv.Itoa(s.MonthOfYear)
		row["schedule_recurrence_end"] = formatTimePtr(s.RecurrenceEnd)
		row["schedule_start_date"] = formatDateOnlyPtr(s.StartDate)
		row["schedule_target_count"] = strconv.Itoa(s.TargetCount)
		row["schedule_is_active"] = strconv.FormatBool(s.IsActive)
		row["schedule_is_follow_up"] = strconv.FormatBool(s.IsFollowUp)
		row["schedule_assigned_user_id"] = formatInt64Ptr(s.AssignedUserID)
		row["schedule_created_at"] = formatTime(s.CreatedAt)
		row["schedule_updated_at"] = formatTime(s.UpdatedAt)
		write(row)
	}

	for _, n := range dayNotes {
		row := base("day_note", n.Date)
		row["day_note_date"] = n.Date
		row["day_note"] = n.Note
		row["day_note_updated_by"] = formatInt64Ptr(n.UpdatedBy)
		row["day_note_updated_at"] = formatTime(n.UpdatedAt)
		write(row)
	}

	for _, inv := range invites {
		row := base("invite", formatInt64(inv.ID))
		row["invite_id"] = formatInt64(inv.ID)
		row["invite_created_by"] = formatInt64(inv.CreatedBy)
		row["invite_max_uses"] = strconv.Itoa(inv.MaxUses)
		row["invite_used_count"] = strconv.Itoa(inv.UsedCount)
		row["invite_expires_at"] = formatTimePtr(inv.ExpiresAt)
		row["invite_created_at"] = formatTime(inv.CreatedAt)
		// Invite codes are bearer capabilities and are deliberately not exported.
		write(row)
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		exportError(w, err)
		return
	}
	current, err := h.householdService.GetAdminHouseholdID(r.Context(), user.ID)
	if err != nil && !errors.Is(err, household.ErrNotMember) && !errors.Is(err, household.ErrNotFound) && !errors.Is(err, household.ErrNotAuthorized) {
		exportError(w, err)
		return
	}
	if err != nil || current != hid {
		writeError(w, http.StatusForbidden, "household access changed; try again")
		return
	}
	sendExport(w, r, buffer, "nabu-household-data.csv")
}

func formatInt64(id int64) string {
	return strconv.FormatInt(id, 10)
}

func formatInt64Ptr(value *int64) string {
	if value == nil {
		return ""
	}
	return formatInt64(*value)
}

func formatIntPtr(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func formatStringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatTimePtr(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func formatDateOnlyPtr(value *schedule.DateOnly) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.Format("2006-01-02")
}

func jsonString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}
