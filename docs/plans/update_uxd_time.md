# Plan: Calendar Grid with Time-of-Day Scheduling

**Date:** 2026-04-28  
**Scope:** Add time-of-day scheduling to chore assignments using a Google Calendar-style
grid. Day view is the default; week view is a new tab. Fully responsive (mobile + desktop).
TDD throughout — every piece of logic gets a test written before the implementation.

---

## Table of Contents

1. [Goals & Non-Goals](#1-goals--non-goals)
2. [Design Decisions (Q&A summary)](#2-design-decisions-qa-summary)
3. [Architecture Overview](#3-architecture-overview)
4. [Database Migration](#4-database-migration)
5. [Backend: Schedule Service & Store](#5-backend-schedule-service--store)
6. [Backend: HTTP Handlers & Routes](#6-backend-http-handlers--routes)
7. [Frontend: State Changes](#7-frontend-state-changes)
8. [Frontend: New Files](#8-frontend-new-files)
9. [Frontend: Modified Files](#9-frontend-modified-files)
10. [UI/UX Specification](#10-uiux-specification)
11. [CSS Changes](#11-css-changes)
12. [Implementation Phases](#12-implementation-phases)
13. [Full Code Examples (TDD-first)](#13-full-code-examples-tdd-first)

---

## 1. Goals & Non-Goals

### Goals
- Replace the flat chore-card list with a time-bucketed day view (Morning / Afternoon /
  Evening / Night + Anytime for unscheduled chores).
- Add a week view tab: 7-column grid with hour rows (Google Calendar style), named period
  section dividers, chore cards inside the correct cell.
- Let users place chores in time slots by tapping an empty cell (bottom sheet picker) or
  dragging an existing card to a new slot.
- Support Google Calendar-style recurrence: daily, weekly (specific days), every N days,
  monthly by date, monthly by Nth weekday, yearly.
- Optional assignment: a specific household member can be named on each schedule entry,
  or it can be left open for anyone.
- All existing chores default to the "Anytime" bucket until the user manually moves them.
- Chore settings screen is also updated to configure schedule from there, staying in sync
  with the grid.
- Grandmother-friendly: big tap targets, clear labels, self-evident icons, no jargon.
- Fully responsive: touch-first on mobile, mouse/keyboard works on desktop.

### Non-Goals (out of scope for this iteration)
- Push/email notifications (infra exists but not wired — leave alone).
- Conflict detection (two chores overlapping in same cell is fine).
- Undo/redo of schedule changes.
- Sharing specific schedule slots across households.

---

## 2. Design Decisions (Q&A Summary)

| Decision | Choice |
|---|---|
| Primary view | Day view as default; week view added as a tab |
| Time granularity | Named periods (Morning/Afternoon/Evening/Night) + optional specific clock time |
| Scheduling entry point | Both: tap empty grid cell, or from chore settings — both sync |
| Assignment | Optional — a person can be assigned per schedule entry or left open |
| Unscheduled chores | "Anytime" bucket at the bottom; all existing chores start here |
| Day-of-week recurrence | Full Google Calendar patterns (daily, weekly, every N days, monthly, yearly) |
| Week view row structure | Hour rows (like Google Calendar) with named-period dividers |
| Grid scheduling UX | Tap empty cell → bottom sheet picker; drag existing card to move |
| Device target | Fully responsive (mobile + desktop) |
| Migration of existing data | All existing chore schedules default to `time_period = 'anytime'` |

---

## 3. Architecture Overview

### Named Time Periods

Each period maps to a clock range (local browser time on frontend, UTC stored in DB).

| Period | Icon | Hours |
|---|---|---|
| `morning` | 🌅 | 05:00 – 11:59 |
| `afternoon` | ☀️ | 12:00 – 16:59 |
| `evening` | 🌆 | 17:00 – 20:59 |
| `night` | 🌙 | 21:00 – 04:59 (next day) |
| `anytime` | 📋 | No time constraint |

### Recurrence Frequency Types

| `frequency_type` | Meaning | Active Fields |
|---|---|---|
| `daily` | Every day | `time_period`, `specific_time` |
| `weekly` | Specific weekdays | `days_of_week`, `time_period`, `specific_time` |
| `every_n_days` | Every N days from created_at | `interval_days`, `time_period`, `specific_time` |
| `monthly_by_date` | Same date each month (e.g., 15th) | `day_of_month`, `time_period`, `specific_time` |
| `monthly_by_weekday` | Nth weekday each month (e.g., 3rd Mon) | `month_weekday` JSONB, `time_period`, `specific_time` |
| `yearly` | Once a year | `day_of_month`, `month_of_year`, `time_period`, `specific_time` |

### File Map (new files marked `[NEW]`, changed files marked `[MOD]`)

```
internal/
  schedule/
    service.go           [MOD] extend ChoreSchedule struct + IsActiveForDay()
    store.go             [NEW] Store interface + memory/postgres implementations
    service_test.go      [NEW] unit tests for recurrence logic
    memory_store.go      [NEW] in-memory implementation
    postgres_store.go    [NEW] postgres implementation
  handlers/
    schedule.go          [NEW] ScheduleHandler (CRUD + for-date query)
    schedule_test.go     [NEW] handler tests
  app/
    server.go            [MOD] wire ScheduleHandler + new routes
migrations/
  003_schedule_fields.sql [NEW] ALTER TABLE chore_schedules to add new columns

web/static/js/
  schedule.js            [NEW] API calls + render schedule bottom sheet + recurrence picker
  calendar.js            [NEW] renderDayView() + renderWeekView() + drag/drop logic
  state.js               [MOD] add schedules, calendarView, calendarDate, activeSheet
  today.js               [MOD] renderTodayView() → delegate to calendar.js
  app.js                 [MOD] add event handlers for calendar, sheet, drag/drop
  tests/runner.js        [MOD] add tests for schedule.js + calendar.js
```

---

## 4. Database Migration

Create `migrations/003_schedule_fields.sql`.

> **Important:** The `chore_schedules` table already exists from `001_initial.sql`.
> This migration only adds columns and does NOT recreate the table.
> Migration files are applied in `fs.ReadDir` order at startup; name the file so it
> sorts after `002_*` files.

```sql
-- migrations/003_schedule_fields.sql

ALTER TABLE chore_schedules
  ADD COLUMN IF NOT EXISTS time_period       TEXT    NOT NULL DEFAULT 'anytime',
  ADD COLUMN IF NOT EXISTS specific_time     TEXT,
  ADD COLUMN IF NOT EXISTS day_of_month      INT,
  ADD COLUMN IF NOT EXISTS month_weekday     JSONB,
  ADD COLUMN IF NOT EXISTS month_of_year     INT,
  ADD COLUMN IF NOT EXISTS recurrence_end_date DATE;

-- Update existing rows so frequency_type is always one of the new canonical values.
-- 'daily' and 'weekly' are already valid; nothing else exists yet.
UPDATE chore_schedules
  SET time_period = 'anytime'
  WHERE time_period IS NULL OR time_period = '';

COMMENT ON COLUMN chore_schedules.time_period IS
  'Named period: morning | afternoon | evening | night | anytime';
COMMENT ON COLUMN chore_schedules.specific_time IS
  'Optional exact time within the period, HH:MM 24-hour (e.g. 08:30)';
COMMENT ON COLUMN chore_schedules.month_weekday IS
  'For monthly_by_weekday: {"week":3,"day":1} = 3rd Monday (day 0=Sun..6=Sat)';
COMMENT ON COLUMN chore_schedules.recurrence_end_date IS
  'Optional date after which the schedule is considered inactive';
```

---

## 5. Backend: Schedule Service & Store

### 5a. Extended `ChoreSchedule` struct (`internal/schedule/service.go`)

Replace the current `ChoreSchedule` struct. **Keep backward compatibility** — zero values
for new fields match old behavior (daily with no time constraint).

```go
// internal/schedule/service.go

package schedule

import "time"

// TimePeriod represents a named block of the day.
type TimePeriod string

const (
    PeriodMorning   TimePeriod = "morning"   // 05:00-11:59
    PeriodAfternoon TimePeriod = "afternoon" // 12:00-16:59
    PeriodEvening   TimePeriod = "evening"   // 17:00-20:59
    PeriodNight     TimePeriod = "night"     // 21:00-04:59
    PeriodAnytime   TimePeriod = "anytime"   // no constraint
)

// MonthWeekday encodes "Nth weekday of the month", e.g. 3rd Monday.
type MonthWeekday struct {
    Week int `json:"week"` // 1-5
    Day  int `json:"day"`  // 0=Sunday … 6=Saturday
}

// ChoreSchedule is the canonical schedule record.
type ChoreSchedule struct {
    ID              int64          `json:"id"`
    HouseholdID     int64          `json:"householdId"`
    ChoreID         int64          `json:"choreId"`
    FrequencyType   string         `json:"frequencyType"`
    TimePeriod      TimePeriod     `json:"timePeriod"`
    SpecificTime    string         `json:"specificTime,omitempty"` // "HH:MM", optional
    TimesOfDay      []string       `json:"timesOfDay"`             // legacy; kept for compat
    DaysOfWeek      []int          `json:"daysOfWeek"`
    IntervalDays    int            `json:"intervalDays"`
    DayOfMonth      int            `json:"dayOfMonth,omitempty"`
    MonthWeekday    *MonthWeekday  `json:"monthWeekday,omitempty"`
    MonthOfYear     int            `json:"monthOfYear,omitempty"`
    RecurrenceEnd   *time.Time     `json:"recurrenceEnd,omitempty"`
    TargetCount     int            `json:"targetCount"`
    IsActive        bool           `json:"isActive"`
    AssignedUserID  *int64         `json:"assignedUserId"`
    CreatedAt       time.Time      `json:"createdAt"`
    UpdatedAt       time.Time      `json:"updatedAt"`
}
```

### 5b. Store Interface (`internal/schedule/store.go`)

```go
// internal/schedule/store.go

package schedule

import "context"

// Store persists ChoreSchedule records.
type Store interface {
    Create(ctx context.Context, s ChoreSchedule) (ChoreSchedule, error)
    Get(ctx context.Context, id int64) (ChoreSchedule, error)
    ListByHousehold(ctx context.Context, householdID int64) ([]ChoreSchedule, error)
    Update(ctx context.Context, s ChoreSchedule) (ChoreSchedule, error)
    Delete(ctx context.Context, id int64) error
}
```

### 5c. `IsActiveForDay` method (`internal/schedule/service.go`)

This is the core recurrence logic. Write tests before implementation.

```go
// IsActiveForDay returns true if the schedule should show a chore card on the
// given calendar date. It does NOT check the time of day — that is handled by
// the UI bucketing logic. This lets the frontend request "which chores appear on
// Tuesday?" independently of the hour.
func (s *Service) IsActiveForDay(sch ChoreSchedule, date time.Time) bool {
    if !sch.IsActive {
        return false
    }
    if sch.RecurrenceEnd != nil && date.After(*sch.RecurrenceEnd) {
        return false
    }

    d := date.Truncate(24 * time.Hour)

    switch sch.FrequencyType {
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
// weekday (0=Sun…6=Sat) in its month. week=1 means first, week=5 means last
// occurrence (capped at actual count).
func isNthWeekdayOfMonth(t time.Time, week, weekday int) bool {
    // Count how many times this weekday has appeared so far in the month.
    count := 0
    for d := 1; d <= t.Day(); d++ {
        if int(time.Date(t.Year(), t.Month(), d, 0, 0, 0, 0, t.Location()).Weekday()) == weekday {
            count++
        }
    }
    return int(t.Weekday()) == weekday && count == week
}
```

### 5d. `GetSchedulesForDate` helper

```go
// GetSchedulesForDate filters a slice of schedules to those active on date.
// Used by the handler to answer GET /api/schedules/for-date.
func (s *Service) GetSchedulesForDate(schedules []ChoreSchedule, date time.Time) []ChoreSchedule {
    var out []ChoreSchedule
    for _, sch := range schedules {
        if s.IsActiveForDay(sch, date) {
            out = append(out, sch)
        }
    }
    return out
}
```

---

## 6. Backend: HTTP Handlers & Routes

### 6a. New Handler File (`internal/handlers/schedule.go`)

```go
// internal/handlers/schedule.go

package handlers

import (
    "net/http"
    "strconv"
    "time"

    "github.com/HammerMeetNail/nabu/internal/middleware"
    "github.com/HammerMeetNail/nabu/internal/schedule"
)

type ScheduleHandler struct {
    store   schedule.Store
    service *schedule.Service
}

func NewScheduleHandler(store schedule.Store, service *schedule.Service) *ScheduleHandler {
    return &ScheduleHandler{store: store, service: service}
}

// List returns all active schedules for the user's household.
// GET /api/schedules
func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
    user, _ := middleware.CurrentUser(r.Context())
    if user.HouseholdID == nil {
        writeError(w, http.StatusUnauthorized, "no household")
        return
    }
    schedules, err := h.store.ListByHousehold(r.Context(), *user.HouseholdID)
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error())
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"schedules": schedules})
}

// ForDate returns schedules active on a given date.
// GET /api/schedules/for-date?date=YYYY-MM-DD
func (h *ScheduleHandler) ForDate(w http.ResponseWriter, r *http.Request) {
    user, _ := middleware.CurrentUser(r.Context())
    if user.HouseholdID == nil {
        writeError(w, http.StatusUnauthorized, "no household")
        return
    }

    dateStr := r.URL.Query().Get("date")
    date := today()
    if dateStr != "" {
        if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
            date = parsed
        }
    }

    all, err := h.store.ListByHousehold(r.Context(), *user.HouseholdID)
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error())
        return
    }

    active := h.service.GetSchedulesForDate(all, date)
    writeJSON(w, http.StatusOK, map[string]any{
        "schedules": active,
        "date":      date.Format("2006-01-02"),
    })
}

// Create adds a new schedule entry.
// POST /api/schedules
func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
    user, _ := middleware.CurrentUser(r.Context())
    if user.HouseholdID == nil {
        writeError(w, http.StatusUnauthorized, "no household")
        return
    }

    var req schedule.ChoreSchedule
    if err := readJSON(r, &req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    if req.ChoreID == 0 {
        writeError(w, http.StatusBadRequest, "choreId is required")
        return
    }
    if req.TimePeriod == "" {
        req.TimePeriod = schedule.PeriodAnytime
    }
    if req.FrequencyType == "" {
        req.FrequencyType = "daily"
    }
    req.HouseholdID = *user.HouseholdID
    req.IsActive = true

    created, err := h.store.Create(r.Context(), req)
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error())
        return
    }
    writeJSON(w, http.StatusCreated, map[string]any{"schedule": created})
}

// Update replaces a schedule entry.
// PATCH /api/schedules/{id}
func (h *ScheduleHandler) Update(w http.ResponseWriter, r *http.Request) {
    user, _ := middleware.CurrentUser(r.Context())
    if user.HouseholdID == nil {
        writeError(w, http.StatusUnauthorized, "no household")
        return
    }

    idStr := r.PathValue("id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        writeError(w, http.StatusBadRequest, "invalid schedule id")
        return
    }

    existing, err := h.store.Get(r.Context(), id)
    if err != nil {
        writeError(w, http.StatusNotFound, "schedule not found")
        return
    }
    if existing.HouseholdID != *user.HouseholdID {
        writeError(w, http.StatusForbidden, "not your schedule")
        return
    }

    var req schedule.ChoreSchedule
    if err := readJSON(r, &req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    req.ID = id
    req.HouseholdID = *user.HouseholdID

    updated, err := h.store.Update(r.Context(), req)
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error())
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"schedule": updated})
}

// Delete removes a schedule entry.
// DELETE /api/schedules/{id}
func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
    user, _ := middleware.CurrentUser(r.Context())
    if user.HouseholdID == nil {
        writeError(w, http.StatusUnauthorized, "no household")
        return
    }

    idStr := r.PathValue("id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        writeError(w, http.StatusBadRequest, "invalid schedule id")
        return
    }

    existing, err := h.store.Get(r.Context(), id)
    if err != nil {
        writeError(w, http.StatusNotFound, "schedule not found")
        return
    }
    if existing.HouseholdID != *user.HouseholdID {
        writeError(w, http.StatusForbidden, "not your schedule")
        return
    }

    if err := h.store.Delete(r.Context(), id); err != nil {
        writeError(w, http.StatusInternalServerError, err.Error())
        return
    }
    writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
```

### 6b. Route Registration (`internal/app/server.go`)

Add the following inside `NewServerWithDB`, alongside the existing route registrations.
Import `"github.com/HammerMeetNail/nabu/internal/schedule"` and wire up a new store + handler.

```go
// Add after logStore and logService setup:

var scheduleStore schedule.Store
if db != nil {
    scheduleStore = schedule.NewPostgresStore(db)
} else {
    scheduleStore = schedule.NewMemoryStore()
}
scheduleService := schedule.NewService()
scheduleHandler := handlers.NewScheduleHandler(scheduleStore, scheduleService)

// Routes (add after stats routes):
mux.HandleFunc("/api/schedules",
    method(http.MethodGet,  middleware.RequireAuth(scheduleHandler.List)))
mux.HandleFunc("/api/schedules/for-date",
    method(http.MethodGet,  middleware.RequireAuth(scheduleHandler.ForDate)))
mux.HandleFunc("/api/schedules",
    method(http.MethodPost, middleware.RequireAuth(scheduleHandler.Create)))
mux.HandleFunc("/api/schedules/{id}",
    method(http.MethodPatch,  middleware.RequireAuth(scheduleHandler.Update)))
mux.HandleFunc("/api/schedules/{id}",
    method(http.MethodDelete, middleware.RequireAuth(scheduleHandler.Delete)))
```

> **Note:** Go's `http.ServeMux` (1.22+) supports method-prefixed patterns.
> Check the existing `method()` helper in `server.go`; if it conflicts with mux-level
> method routing you may need to register as `"GET /api/schedules"` style patterns
> instead, matching the pattern already used in the file.

---

## 7. Frontend: State Changes

Extend `createAppState()` in `web/static/js/state.js`:

```js
// Add these fields to the object returned by createAppState():

schedules:       [],        // ChoreSchedule[] from GET /api/schedules
calendarView:    'day',     // 'day' | 'week'
calendarDate:    null,      // ISO string — null means "use today"
activeSheet:     null,      // null | 'pick-chore' | 'edit-schedule' | 'recurrence-picker'
activeSheetData: {},        // context passed to the active bottom sheet
```

Also extend `resetAuthedState()` to clear the new fields.

---

## 8. Frontend: New Files

### 8a. `web/static/js/schedule.js` — API + recurrence helpers

```js
// web/static/js/schedule.js

import { apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";

// ─── Time period definitions ──────────────────────────────────────────────────

export const PERIODS = [
  { id: "morning",   icon: "🌅", label: "Morning",   startHour: 5,  endHour: 11 },
  { id: "afternoon", icon: "☀️",  label: "Afternoon", startHour: 12, endHour: 16 },
  { id: "evening",   icon: "🌆", label: "Evening",   startHour: 17, endHour: 20 },
  { id: "night",     icon: "🌙", label: "Night",     startHour: 21, endHour: 4  },
  { id: "anytime",   icon: "📋", label: "Anytime",   startHour: 0,  endHour: 23 },
];

/**
 * Returns the period id that contains the given hour (0-23).
 * Night wraps around midnight, so hours 21-23 and 0-4 both map to "night".
 */
export function hourToPeriod(hour) {
  if (hour >= 5  && hour <= 11) return "morning";
  if (hour >= 12 && hour <= 16) return "afternoon";
  if (hour >= 17 && hour <= 20) return "evening";
  if (hour >= 21 || hour <= 4)  return "night";
  return "anytime";
}

/**
 * Given a specific_time string ("HH:MM"), returns the period id.
 * Falls back to "anytime" if unparseable.
 */
export function timeToPeriod(timeStr) {
  if (!timeStr) return "anytime";
  const [h] = timeStr.split(":").map(Number);
  return Number.isFinite(h) ? hourToPeriod(h) : "anytime";
}

// ─── API calls ────────────────────────────────────────────────────────────────

export async function loadSchedules() {
  const { data } = await apiFetch("/api/schedules");
  return data?.schedules ?? [];
}

export async function loadSchedulesForDate(isoDate) {
  const { data } = await apiFetch(`/api/schedules/for-date?date=${isoDate}`);
  return data?.schedules ?? [];
}

export async function createSchedule(payload) {
  const { data } = await apiFetch("/api/schedules", {
    method: "POST",
    body: JSON.stringify(payload),
  });
  return data?.schedule;
}

export async function updateSchedule(id, payload) {
  const { data } = await apiFetch(`/api/schedules/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
  return data?.schedule;
}

export async function deleteSchedule(id) {
  await apiFetch(`/api/schedules/${id}`, { method: "DELETE" });
}

// ─── Recurrence helpers ───────────────────────────────────────────────────────

export const FREQ_LABELS = {
  daily:              "Every day",
  weekly:             "Weekly",
  every_n_days:       "Every N days",
  monthly_by_date:    "Monthly (same date)",
  monthly_by_weekday: "Monthly (same weekday)",
  yearly:             "Every year",
};

const DAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

/**
 * Returns a human-readable summary of the recurrence rule.
 * e.g. "Every Mon, Wed, Fri • Morning"
 */
export function recurrenceSummary(sch) {
  if (!sch || !sch.frequencyType) return "Not scheduled";
  const period = PERIODS.find(p => p.id === (sch.timePeriod || "anytime"));
  const periodLabel = period ? `${period.icon} ${period.label}` : "";

  let freq = "";
  switch (sch.frequencyType) {
    case "daily":
      freq = "Every day";
      break;
    case "weekly": {
      const days = (sch.daysOfWeek || []).map(d => DAY_NAMES[d]).join(", ");
      freq = days ? `Every ${days}` : "Weekly";
      break;
    }
    case "every_n_days":
      freq = `Every ${sch.intervalDays || 1} days`;
      break;
    case "monthly_by_date":
      freq = `Monthly on the ${ordinal(sch.dayOfMonth)}`;
      break;
    case "monthly_by_weekday": {
      const mw = sch.monthWeekday;
      freq = mw ? `Monthly on the ${ordinal(mw.week)} ${DAY_NAMES[mw.day]}` : "Monthly";
      break;
    }
    case "yearly":
      freq = `Yearly`;
      break;
    default:
      freq = sch.frequencyType;
  }

  if (sch.specificTime) {
    return `${freq} • ${fmtTime(sch.specificTime)}`;
  }
  return periodLabel ? `${freq} • ${periodLabel}` : freq;
}

function ordinal(n) {
  if (!n) return "?";
  const s = ["th", "st", "nd", "rd"];
  const v = n % 100;
  return n + (s[(v - 20) % 10] || s[v] || s[0]);
}

function fmtTime(hhmm) {
  const [h, m] = hhmm.split(":").map(Number);
  const ampm = h >= 12 ? "PM" : "AM";
  const h12 = h % 12 || 12;
  return `${h12}:${String(m).padStart(2, "0")} ${ampm}`;
}

// ─── Render: bottom sheet — pick a chore for a time slot ─────────────────────

/**
 * Renders the "pick a chore" bottom sheet.
 * @param {object[]} chores  All household chores
 * @param {object}   slot    { date: "YYYY-MM-DD", timePeriod: "morning", hour: 8 }
 * @param {object[]} schedules  Already-scheduled chore IDs for this slot
 */
export function renderPickChoreSheet(chores, slot, schedules) {
  const scheduledIds = new Set((schedules || []).map(s => s.choreId));
  const available = chores.filter(c => !scheduledIds.has(c.id));

  const items = available.length === 0
    ? `<p class="sheet-empty">All chores are already scheduled for this time.</p>`
    : available.map(c => `
        <button type="button"
          class="sheet-chore-item"
          data-action="schedule-chore-here"
          data-chore-id="${c.id}"
          data-time-period="${escapeHTML(slot.timePeriod || "anytime")}"
          data-date="${escapeHTML(slot.date || "")}"
          data-specific-hour="${slot.hour ?? ""}">
          <span class="chore-icon">${c.icon}</span>
          <span class="chore-name">${escapeHTML(c.name)}</span>
          <span class="chore-category">${escapeHTML(c.category)}</span>
        </button>`).join("");

  const period = PERIODS.find(p => p.id === (slot.timePeriod || "anytime"));
  const title  = period ? `${period.icon} Add to ${period.label}` : "Add Chore";

  return `
    <div class="bottom-sheet" role="dialog" aria-modal="true" aria-label="${title}">
      <div class="sheet-handle" aria-hidden="true"></div>
      <h2 class="sheet-title">${title}</h2>
      <div class="sheet-chore-list">${items}</div>
      <button type="button" class="btn btn-ghost btn-full" data-action="close-sheet">
        Cancel
      </button>
    </div>`;
}

// ─── Render: recurrence picker ────────────────────────────────────────────────

/**
 * Renders a full recurrence picker form.
 * @param {object} schedule  Current schedule values (may be partial/new)
 */
export function renderRecurrencePicker(sch) {
  const ft = sch?.frequencyType || "daily";
  const days = new Set(sch?.daysOfWeek || []);

  const freqOptions = Object.entries(FREQ_LABELS).map(([val, lbl]) =>
    `<option value="${val}" ${ft === val ? "selected" : ""}>${lbl}</option>`
  ).join("");

  const dayPills = DAY_NAMES.map((name, i) => `
    <button type="button"
      class="day-pill ${days.has(i) ? "day-pill--on" : ""}"
      data-action="toggle-day"
      data-day="${i}"
      aria-pressed="${days.has(i)}"
      aria-label="${name}">
      ${name}
    </button>`).join("");

  const periodOptions = PERIODS.filter(p => p.id !== "anytime").map(p =>
    `<option value="${p.id}" ${sch?.timePeriod === p.id ? "selected" : ""}>
      ${p.icon} ${p.label}
    </option>`
  ).join("");

  return `
    <div class="recurrence-picker">
      <label class="field-label" for="freq-select">Repeats</label>
      <select id="freq-select" class="select-input" data-action="change-frequency">
        ${freqOptions}
      </select>

      <div class="day-pill-row" id="weekday-row" ${ft !== "weekly" ? 'hidden' : ''}>
        <p class="field-label">On these days</p>
        <div class="day-pills">${dayPills}</div>
      </div>

      <div id="interval-row" ${ft !== "every_n_days" ? 'hidden' : ''}>
        <label class="field-label" for="interval-input">Every how many days?</label>
        <input id="interval-input" type="number" min="2" max="365"
          class="text-input" value="${sch?.intervalDays || 2}" />
      </div>

      <div id="dom-row" ${!["monthly_by_date","yearly"].includes(ft) ? 'hidden' : ''}>
        <label class="field-label" for="dom-input">Day of month</label>
        <input id="dom-input" type="number" min="1" max="31"
          class="text-input" value="${sch?.dayOfMonth || 1}" />
      </div>

      <label class="field-label" for="period-select">Time of day</label>
      <select id="period-select" class="select-input" data-action="change-period">
        <option value="anytime" ${(!sch?.timePeriod || sch?.timePeriod === "anytime") ? "selected" : ""}>
          📋 Anytime
        </option>
        ${periodOptions}
      </select>

      <div id="specific-time-row">
        <label class="field-label" for="specific-time">Specific time (optional)</label>
        <input id="specific-time" type="time" class="text-input"
          value="${sch?.specificTime || ""}" />
      </div>

      <button type="button" class="btn btn-primary btn-full" data-action="save-recurrence">
        Save Schedule
      </button>
    </div>`;
}
```

### 8b. `web/static/js/calendar.js` — Day and Week views

```js
// web/static/js/calendar.js

import { escapeHTML }     from "./utils.js";
import { PERIODS, timeToPeriod, recurrenceSummary } from "./schedule.js";
import { todayISO }       from "./today.js";

// ─── Constants ────────────────────────────────────────────────────────────────

// Hours shown in the week view grid, 0-23.
export const GRID_HOURS = Array.from({ length: 24 }, (_, i) => i);

function fmtHour(h) {
  if (h === 0)  return "12 AM";
  if (h === 12) return "12 PM";
  return h < 12 ? `${h} AM` : `${h - 12} PM`;
}

function fmtShortDate(iso) {
  const d = new Date(iso + "T00:00:00");
  return d.toLocaleDateString("en-US", { weekday: "short", day: "numeric" });
}

// ─── Day View ─────────────────────────────────────────────────────────────────

/**
 * Renders the full day view (time-bucketed chore cards + anytime section).
 *
 * @param {object}   state
 * @param {object[]} state.chores          All household chores
 * @param {object[]} state.schedules       All household schedules
 * @param {object[]} state.todayLogs       Completion logs for the viewed date
 * @param {string}   state.calendarDate    ISO date string ("YYYY-MM-DD")
 */
export function renderDayView(state) {
  const date      = state.calendarDate || todayISO(0);
  const chores    = state.chores    || [];
  const schedules = state.schedules || [];
  const logs      = state.todayLogs || [];

  // Build a lookup: choreId → log (if completed today)
  const logMap = {};
  logs.forEach(l => { logMap[l.choreId] = l; });

  // Build a lookup: choreId → schedule (for the viewed date)
  const scheduleMap = {};
  schedules.forEach(s => { scheduleMap[s.choreId] = s; });

  // Bucket chores by period
  const buckets = {};
  PERIODS.forEach(p => { buckets[p.id] = []; });

  chores.forEach(chore => {
    const sch = scheduleMap[chore.id];
    const period = sch ? (sch.timePeriod || "anytime") : "anytime";
    buckets[period].push({ chore, sch });
  });

  const sections = PERIODS.map(period => {
    const items = buckets[period.id];
    // Always show "Anytime"; hide other empty sections (they become add-targets)
    return renderPeriodSection(period, items, logMap, date);
  }).join("");

  const done  = logs.length;
  const total = chores.length;
  const pct   = total ? Math.round((done / total) * 100) : 0;

  const prev = shiftISO(date, -1);
  const next = shiftISO(date, 1);

  return `
    <div class="day-view" data-view="day">
      <div class="cal-nav">
        <button type="button" class="btn btn-icon btn-ghost"
          data-action="navigate-day" data-date="${prev}" aria-label="Previous day">←</button>
        <h2 class="cal-date">${fmtLongDate(date)}</h2>
        <button type="button" class="btn btn-icon btn-ghost"
          data-action="navigate-day" data-date="${next}" aria-label="Next day">→</button>
      </div>
      <div class="view-tabs">
        <button type="button" class="view-tab view-tab--active" data-action="switch-view" data-view="day">Day</button>
        <button type="button" class="view-tab" data-action="switch-view" data-view="week">Week</button>
      </div>
      <div class="progress-bar">
        <div class="progress-fill" style="width:${pct}%"></div>
      </div>
      <p class="progress-label">${done} of ${total} done</p>
      <div class="period-sections">${sections}</div>
    </div>`;
}

function renderPeriodSection(period, items, logMap, date) {
  const cards = items.map(({ chore, sch }) =>
    renderChoreCard(chore, sch, logMap[chore.id], date)
  ).join("");

  // Tap-to-add button for empty / non-anytime sections
  const addBtn = period.id !== "anytime"
    ? `<button type="button"
         class="add-chore-slot"
         data-action="open-pick-chore-sheet"
         data-time-period="${period.id}"
         data-date="${date}"
         aria-label="Add chore to ${period.label}">
         + Add chore
       </button>`
    : "";

  return `
    <section class="period-section" data-period="${period.id}">
      <h3 class="period-heading">
        <span class="period-icon" aria-hidden="true">${period.icon}</span>
        ${period.label}
      </h3>
      <div class="period-cards"
        data-drop-period="${period.id}"
        data-drop-date="${date}">
        ${cards}
        ${addBtn}
      </div>
    </section>`;
}

function renderChoreCard(chore, sch, log, date) {
  const done      = !!log;
  const doneClass = done ? "chore-card--done" : "";
  const action    = done ? "undo-chore" : "log-chore";
  const logId     = log ? log.id : "";
  const timeLabel = sch?.specificTime
    ? `<span class="chore-time">${fmtTime12(sch.specificTime)}</span>`
    : "";
  const assignee  = sch?.assignedUserId
    ? `<span class="chore-assignee" aria-label="Assigned">👤</span>`
    : "";

  return `
    <button type="button"
      class="chore-card ${doneClass}"
      style="border-left: 4px solid ${escapeHTML(chore.color)}"
      data-action="${action}"
      data-chore-id="${chore.id}"
      data-log-id="${logId}"
      draggable="true"
      data-drag-chore-id="${chore.id}"
      data-drag-schedule-id="${sch?.id || ""}"
      aria-pressed="${done}">
      <span class="chore-icon" aria-hidden="true">${chore.icon}</span>
      <span class="chore-name">${escapeHTML(chore.name)}</span>
      ${timeLabel}${assignee}
      ${done ? '<span class="check-overlay" aria-hidden="true">✓</span>' : ""}
    </button>`;
}

// ─── Week View ────────────────────────────────────────────────────────────────

/**
 * Renders the week view: 7 day columns × 24 hour rows with named period bands.
 *
 * @param {object}   state
 * @param {object[]} state.chores
 * @param {object[]} state.schedules   All schedules (pre-filtered or all — filtering
 *                                     by day happens in JS for the week view to avoid
 *                                     7 round trips)
 * @param {object[]} state.weekLogs    Logs for the whole viewed week
 * @param {string}   state.calendarDate  ISO date of Monday of the viewed week
 */
export function renderWeekView(state) {
  const weekStart = isoMonday(state.calendarDate || todayISO(0));
  const days      = Array.from({ length: 7 }, (_, i) => shiftISO(weekStart, i));
  const chores    = state.chores    || [];
  const schedules = state.schedules || [];
  const weekLogs  = state.weekLogs  || [];

  // Build log lookup: "choreId-YYYY-MM-DD" → log
  const logKey    = (choreId, iso) => `${choreId}-${iso}`;
  const logMap    = {};
  weekLogs.forEach(l => {
    const iso = l.completedAt ? l.completedAt.slice(0, 10) : "";
    logMap[logKey(l.choreId, iso)] = l;
  });

  const dayHeaders = days.map(iso =>
    `<div class="week-col-header">${fmtShortDate(iso)}</div>`
  ).join("");

  // Build rows: one per hour
  const rows = GRID_HOURS.map(hour => {
    const periodId = hourToPeriodDirect(hour);
    const periodBand = PERIODS.find(p => p.id === periodId);
    const bandClass  = `hour-row--${periodId}`;

    const cells = days.map(iso => {
      // Find chores scheduled at this hour on this day
      const choreCells = schedules
        .filter(sch => {
          if (!isActiveForDayJS(sch, iso)) return false;
          if (sch.timePeriod === "anytime") return false;
          // Match if specific time hour matches, or fall back to period's start hour
          const schHour = sch.specificTime
            ? parseInt(sch.specificTime.split(":")[0], 10)
            : (PERIODS.find(p => p.id === sch.timePeriod)?.startHour ?? -1);
          return schHour === hour;
        })
        .map(sch => {
          const chore = chores.find(c => c.id === sch.choreId);
          if (!chore) return "";
          const log  = logMap[logKey(chore.id, iso)];
          return renderWeekChoreCard(chore, sch, log, iso);
        }).join("");

      return `<div class="week-cell"
        data-drop-date="${iso}"
        data-drop-hour="${hour}"
        data-action-empty="open-pick-chore-sheet"
        data-time-period="${periodId}"
        data-date="${iso}"
        data-hour="${hour}">
        ${choreCells || `<span class="week-cell-empty" aria-hidden="true"></span>`}
      </div>`;
    }).join("");

    const hourLabel = `<div class="hour-label">${fmtHour(hour)}</div>`;

    return `<div class="hour-row ${bandClass}" data-hour="${hour}">
      ${hourLabel}${cells}
    </div>`;
  }).join("");

  // "Anytime" section below the grid
  const anytimeChores = schedules.filter(s => s.timePeriod === "anytime");
  const anytimeSection = renderAnytimeWeekSection(anytimeChores, chores, days, logMap);

  const prevWeek = shiftISO(weekStart, -7);
  const nextWeek = shiftISO(weekStart, 7);

  return `
    <div class="week-view" data-view="week">
      <div class="cal-nav">
        <button type="button" class="btn btn-icon btn-ghost"
          data-action="navigate-week" data-date="${prevWeek}" aria-label="Previous week">←</button>
        <h2 class="cal-date">${fmtWeekRange(weekStart)}</h2>
        <button type="button" class="btn btn-icon btn-ghost"
          data-action="navigate-week" data-date="${nextWeek}" aria-label="Next week">→</button>
      </div>
      <div class="view-tabs">
        <button type="button" class="view-tab" data-action="switch-view" data-view="day">Day</button>
        <button type="button" class="view-tab view-tab--active" data-action="switch-view" data-view="week">Week</button>
      </div>
      <div class="week-grid-wrapper">
        <div class="week-grid">
          <div class="week-header-row">
            <div class="hour-label-spacer"></div>
            ${dayHeaders}
          </div>
          <div class="week-body">${rows}</div>
        </div>
      </div>
      ${anytimeSection}
    </div>`;
}

function renderWeekChoreCard(chore, sch, log, iso) {
  const done   = !!log;
  const action = done ? "undo-chore" : "log-chore";
  return `<button type="button"
    class="week-chore-card ${done ? "chore-card--done" : ""}"
    style="background:${escapeHTML(chore.color)}22; border-left: 3px solid ${escapeHTML(chore.color)}"
    data-action="${action}"
    data-chore-id="${chore.id}"
    data-log-id="${log?.id || ""}"
    draggable="true"
    data-drag-chore-id="${chore.id}"
    data-drag-schedule-id="${sch?.id || ""}"
    aria-label="${escapeHTML(chore.name)}${done ? " (done)" : ""}"
    title="${escapeHTML(chore.name)}">
    <span class="chore-icon" aria-hidden="true">${chore.icon}</span>
    <span class="chore-name">${escapeHTML(chore.name)}</span>
    ${done ? '<span class="check-overlay" aria-hidden="true">✓</span>' : ""}
  </button>`;
}

function renderAnytimeWeekSection(anytimeSchedules, chores, days, logMap) {
  if (anytimeSchedules.length === 0) return "";
  // Show a flat list — one row per chore, one cell per day
  const rows = anytimeSchedules.map(sch => {
    const chore = chores.find(c => c.id === sch.choreId);
    if (!chore) return "";
    const cells = days.map(iso => {
      const log = logMap[`${chore.id}-${iso}`];
      return `<div class="week-cell week-cell--anytime">
        ${renderWeekChoreCard(chore, sch, log, iso)}
      </div>`;
    }).join("");
    return `<div class="anytime-row">
      <div class="hour-label">📋</div>
      ${cells}
    </div>`;
  }).join("");

  return `<div class="anytime-week-section">
    <h3 class="period-heading">📋 Anytime</h3>
    <div class="week-grid anytime-grid">${rows}</div>
  </div>`;
}

// ─── Utility functions ────────────────────────────────────────────────────────

export function shiftISO(iso, days) {
  const d = new Date(iso + "T00:00:00");
  d.setDate(d.getDate() + days);
  return d.toISOString().split("T")[0];
}

function isoMonday(iso) {
  const d = new Date(iso + "T00:00:00");
  const day = d.getDay();
  const diff = day === 0 ? -6 : 1 - day; // Mon=1, move back to Monday
  d.setDate(d.getDate() + diff);
  return d.toISOString().split("T")[0];
}

function fmtLongDate(iso) {
  const d = new Date(iso + "T00:00:00");
  return d.toLocaleDateString("en-US", { weekday: "long", month: "long", day: "numeric" });
}

function fmtWeekRange(mondayISO) {
  const start = new Date(mondayISO + "T00:00:00");
  const end   = new Date(mondayISO + "T00:00:00");
  end.setDate(end.getDate() + 6);
  const opts = { month: "short", day: "numeric" };
  return `${start.toLocaleDateString("en-US", opts)} – ${end.toLocaleDateString("en-US", opts)}`;
}

function fmtTime12(hhmm) {
  const [h, m] = hhmm.split(":").map(Number);
  const ampm = h >= 12 ? "PM" : "AM";
  return `${h % 12 || 12}:${String(m).padStart(2, "0")} ${ampm}`;
}

function hourToPeriodDirect(hour) {
  if (hour >= 5  && hour <= 11) return "morning";
  if (hour >= 12 && hour <= 16) return "afternoon";
  if (hour >= 17 && hour <= 20) return "evening";
  return "night";
}

/**
 * Pure JS recurrence check — mirrors backend IsActiveForDay.
 * Used in the week view so we don't need 7 API calls.
 */
export function isActiveForDayJS(sch, isoDate) {
  if (!sch.isActive) return false;
  const d = new Date(isoDate + "T00:00:00");
  if (sch.recurrenceEnd) {
    if (d > new Date(sch.recurrenceEnd)) return false;
  }
  const wd = d.getDay(); // 0=Sun
  switch (sch.frequencyType) {
    case "daily":
      return true;
    case "weekly":
      return (sch.daysOfWeek || []).includes(wd);
    case "every_n_days": {
      if (!sch.intervalDays || sch.intervalDays <= 0) return false;
      const origin = new Date(sch.createdAt);
      origin.setHours(0, 0, 0, 0);
      const diffDays = Math.round((d - origin) / 86400000);
      return diffDays >= 0 && diffDays % sch.intervalDays === 0;
    }
    case "monthly_by_date":
      return d.getDate() === sch.dayOfMonth;
    case "monthly_by_weekday": {
      const mw = sch.monthWeekday;
      if (!mw) return false;
      if (d.getDay() !== mw.day) return false;
      let count = 0;
      for (let day = 1; day <= d.getDate(); day++) {
        if (new Date(d.getFullYear(), d.getMonth(), day).getDay() === mw.day) count++;
      }
      return count === mw.week;
    }
    case "yearly":
      return d.getDate() === sch.dayOfMonth && (d.getMonth() + 1) === sch.monthOfYear;
    default:
      return false;
  }
}
```

---

## 9. Frontend: Modified Files

### 9a. `web/static/js/state.js` — add new fields

Add inside the object returned by `createAppState()`:

```js
schedules:       [],
calendarView:    "day",
calendarDate:    null,       // null = use today
weekLogs:        [],
activeSheet:     null,
activeSheetData: {},
```

Add to `resetAuthedState()`:
```js
state.schedules       = [];
state.calendarView    = "day";
state.calendarDate    = null;
state.weekLogs        = [];
state.activeSheet     = null;
state.activeSheetData = {};
```

### 9b. `web/static/js/today.js` — delegate to calendar.js

Replace `renderTodayView` with a thin wrapper:

```js
// In today.js, replace renderTodayView with:
import { renderDayView } from "./calendar.js";

export function renderTodayView(state) {
  // calendarDate defaults to todayDate for backward compat
  const merged = { ...state, calendarDate: state.calendarDate || state.todayDate };
  return renderDayView(merged);
}
```

Add a new data loader for schedules (called alongside `loadToday`):

```js
import { loadSchedulesForDate } from "./schedule.js";

export async function loadTodayWithSchedules(state) {
  const date = state.calendarDate || todayISO(0);
  const [todayData, schedules] = await Promise.all([
    loadToday(date),
    loadSchedulesForDate(date),
  ]);
  return { ...todayData, schedules };
}
```

### 9c. `web/static/js/app.js` — new event handlers

Add the following `data-action` handlers inside the main event delegation switch
(or if/else chain — match the existing pattern):

```js
// Inside the click handler in app.js:

case "switch-view":
  state.calendarView = el.dataset.view;
  if (state.calendarView === "week") {
    await loadAndRenderWeek(state);
  } else {
    await loadAndRenderDay(state);
  }
  render(state);
  break;

case "navigate-week":
  state.calendarDate = el.dataset.date;
  await loadAndRenderWeek(state);
  render(state);
  break;

case "open-pick-chore-sheet":
  state.activeSheet = "pick-chore";
  state.activeSheetData = {
    date:       el.dataset.date,
    timePeriod: el.dataset.timePeriod,
    hour:       el.dataset.hour ? parseInt(el.dataset.hour, 10) : null,
  };
  render(state);
  break;

case "schedule-chore-here": {
  const choreId    = parseInt(el.dataset.choreId, 10);
  const timePeriod = el.dataset.timePeriod;
  const hour       = el.dataset.specificHour
    ? `${el.dataset.specificHour.padStart(2,"0")}:00`
    : null;
  await createSchedule({
    choreId,
    timePeriod,
    specificTime: hour,
    frequencyType: "daily",  // default; user can change via chore settings
    isActive: true,
  });
  state.activeSheet = null;
  state.schedules = await loadSchedules();
  render(state);
  break;
}

case "close-sheet":
  state.activeSheet = null;
  state.activeSheetData = {};
  render(state);
  break;
```

**Drag and drop** (add to app.js after DOM setup):

```js
// Drag-and-drop: move a chore card to a new time period / day cell.
// Uses HTML5 Drag & Drop API. On mobile, fallback to touch events (see below).

document.addEventListener("dragstart", e => {
  const card = e.target.closest("[data-drag-chore-id]");
  if (!card) return;
  e.dataTransfer.setData("text/plain", JSON.stringify({
    choreId:    parseInt(card.dataset.dragChoreId,    10),
    scheduleId: parseInt(card.dataset.dragScheduleId, 10) || null,
  }));
  card.classList.add("dragging");
});

document.addEventListener("dragend", e => {
  e.target.closest("[data-drag-chore-id]")?.classList.remove("dragging");
});

document.addEventListener("dragover", e => {
  const cell = e.target.closest("[data-drop-period], [data-drop-hour]");
  if (cell) { e.preventDefault(); cell.classList.add("drop-target"); }
});

document.addEventListener("dragleave", e => {
  e.target.closest(".drop-target")?.classList.remove("drop-target");
});

document.addEventListener("drop", async e => {
  const cell = e.target.closest("[data-drop-period], [data-drop-hour]");
  if (!cell) return;
  e.preventDefault();
  cell.classList.remove("drop-target");

  const { choreId, scheduleId } = JSON.parse(e.dataTransfer.getData("text/plain"));
  const newPeriod = cell.dataset.dropPeriod || cell.dataset.timePeriod || "anytime";
  const newHour   = cell.dataset.dropHour != null
    ? `${String(cell.dataset.dropHour).padStart(2,"0")}:00`
    : null;

  if (scheduleId) {
    // Update existing schedule
    await updateSchedule(scheduleId, { timePeriod: newPeriod, specificTime: newHour });
  } else {
    // Create new schedule for this chore
    await createSchedule({
      choreId,
      timePeriod:    newPeriod,
      specificTime:  newHour,
      frequencyType: "daily",
      isActive:      true,
    });
  }

  state.schedules = await loadSchedules();
  render(state);
});
```

---

## 10. UI/UX Specification

### Grandmother Principles

1. **Every interactive element is at least 48×48px** — no tiny tap targets.
2. **Text labels on everything** — icons never appear alone without a label.
3. **One action per screen step** — the bottom sheet picker shows one list; the recurrence
   picker is a separate step.
4. **Undo is always available** — tapping a completed chore card immediately undoes it,
   same as today.
5. **Color is additive, never the only signal** — time periods also have icons + text labels.
6. **Empty states teach, not confuse** — an empty period section says "Tap + Add chore".

### Day View Wireframe (mobile, ~375px wide)

```
┌─────────────────────────────────────┐
│  ←    Monday, April 28    →         │
│  [ Day ]  [ Week ]                  │
├─────────────────────────────────────┤
│ ████████████░░░░  5 of 8 done       │
├─────────────────────────────────────┤
│ 🌅 Morning                          │
│ ┌──────────┐  ┌──────────┐          │
│ │ 🐱 Feed  │  │ 🌿 Water │          │
│ │ Cats ✓   │  │ Plants   │          │
│ └──────────┘  └──────────┘          │
│ [ + Add chore ]                     │
├─────────────────────────────────────┤
│ ☀️ Afternoon                         │
│ ┌──────────┐                        │
│ │ 🍽️ Dishes│                        │
│ └──────────┘                        │
│ [ + Add chore ]                     │
├─────────────────────────────────────┤
│ 🌆 Evening                          │
│ ┌──────────┐  ┌──────────┐          │
│ │ 🐱 Feed  │  │ 🧹 Sweep │          │
│ │ Cats     │  │ Floor ✓  │          │
│ └──────────┘  └──────────┘          │
│ [ + Add chore ]                     │
├─────────────────────────────────────┤
│ 🌙 Night                            │
│ (empty)                             │
│ [ + Add chore ]                     │
├─────────────────────────────────────┤
│ 📋 Anytime                          │
│ ┌──────────┐  ┌──────────┐          │
│ │ 👕Laundry│  │ 🧹 Vacuum│          │
│ └──────────┘  └──────────┘          │
└─────────────────────────────────────┘
```

### Week View Wireframe (desktop, simplified)

```
┌──────────────────────────────────────────────────────────────────┐
│  ←   Apr 28 – May 4   →                                         │
│  [ Day ]  [ Week ]                                               │
├────────┬────────┬────────┬────────┬────────┬────────┬────────────┤
│        │ Mon 28 │ Tue 29 │ Wed 30 │ Thu  1 │ Fri  2 │ Sat 3 ...  │
├────────┼────────┼────────┼────────┼────────┼────────┼────────────┤
│  5 AM  │        │        │        │        │        │            │
│🌅─────│        │        │        │        │        │            │
│  6 AM  │        │        │        │        │        │            │
│  7 AM  │        │        │        │        │        │            │
│  8 AM  │ 🐱Feed │        │ 🐱Feed │        │ 🐱Feed │            │
│  9 AM  │ 🌿Wtr  │        │        │        │        │            │
│ 10 AM  │        │        │        │        │        │            │
│ 11 AM  │        │        │        │        │        │            │
│☀️─────│        │        │        │        │        │            │
│ 12 PM  │        │ 🍽️Dish │        │ 🍽️Dish │        │            │
│  1 PM  │        │        │        │        │        │            │
│  ...   │        │        │        │        │        │            │
│🌆─────│        │        │        │        │        │            │
│  6 PM  │ 🐱Feed │ 🐱Feed │ 🐱Feed │ 🐱Feed │ 🐱Feed │ 🐱Feed     │
│  7 PM  │        │        │        │        │        │            │
│🌙─────│        │        │        │        │        │            │
│  9 PM  │        │        │        │        │        │            │
├────────┴────────┴────────┴────────┴────────┴────────┴────────────┤
│ 📋 Anytime                                                        │
│        │👕Laun │👕Laun │👕Laun │👕Laun │👕Laun │👕Laun         │
└──────────────────────────────────────────────────────────────────┘
```

### Bottom Sheet — Pick Chore

```
┌─────────────────────────────────────┐
│           ────                      │  ← drag handle
│  🌅 Add to Morning                  │
│  ┌──────────────────────────────┐   │
│  │ 🐱  Feed Cats       feeding  │   │
│  └──────────────────────────────┘   │
│  ┌──────────────────────────────┐   │
│  │ 👕  Laundry         cleaning │   │
│  └──────────────────────────────┘   │
│  ┌──────────────────────────────┐   │
│  │ 🧹  Vacuum          cleaning │   │
│  └──────────────────────────────┘   │
│                                     │
│  [ Cancel ]                         │
└─────────────────────────────────────┘
```

### Recurrence Picker

```
┌─────────────────────────────────────┐
│  Repeats                            │
│  ┌──────────────────────────────┐   │
│  │ Every day               ▼   │   │
│  └──────────────────────────────┘   │
│                                     │
│  On these days  (shown for Weekly)  │
│  [ Sun ] [Mon] [Tue] [Wed] [Thu]    │
│  [ Fri ] [Sat]                      │
│                                     │
│  Time of day                        │
│  ┌──────────────────────────────┐   │
│  │ 🌅 Morning              ▼   │   │
│  └──────────────────────────────┘   │
│                                     │
│  Specific time (optional)           │
│  ┌──────────────┐                   │
│  │  08:30       │                   │
│  └──────────────┘                   │
│                                     │
│  [ Save Schedule ]                  │
└─────────────────────────────────────┘
```

---

## 11. CSS Changes

Add to `web/static/css/` (or the existing stylesheet if there is one) or inline into
`web/templates/index.html` inside a `<style>` block.

Key new classes:

```css
/* ── View tabs ────────────────────────────────────── */
.view-tabs        { display: flex; gap: 8px; margin: 8px 0; }
.view-tab         { flex: 1; padding: 10px; border: 2px solid var(--color-border);
                    border-radius: 8px; background: transparent;
                    font-size: 1rem; cursor: pointer; font-weight: 600; }
.view-tab--active { background: var(--color-primary); color: #fff;
                    border-color: var(--color-primary); }

/* ── Period sections (day view) ───────────────────── */
.period-section  { margin-bottom: 20px; }
.period-heading  { font-size: 1rem; font-weight: 700; margin: 0 0 8px;
                   color: var(--color-text-secondary); text-transform: uppercase;
                   letter-spacing: 0.05em; display: flex; align-items: center; gap: 6px; }
.period-cards    { display: flex; flex-wrap: wrap; gap: 10px;
                   min-height: 60px; padding: 8px;
                   border: 2px dashed transparent; border-radius: 12px;
                   transition: border-color 0.15s; }
.period-cards.drop-target { border-color: var(--color-primary); background: #f0f8ff; }

.add-chore-slot  { display: flex; align-items: center; justify-content: center;
                   padding: 10px 16px; border: 2px dashed var(--color-border);
                   border-radius: 10px; color: var(--color-text-secondary);
                   background: transparent; cursor: pointer; font-size: 0.9rem;
                   min-height: 48px; min-width: 120px; }
.add-chore-slot:hover { border-color: var(--color-primary); color: var(--color-primary); }

/* ── Chore card enhancements ──────────────────────── */
.chore-card      { position: relative; min-width: 120px; min-height: 70px;
                   padding: 10px 12px; border-radius: 12px;
                   background: var(--color-surface); border: 1px solid var(--color-border);
                   display: flex; flex-direction: column; gap: 4px;
                   cursor: pointer; font-size: 0.9rem; text-align: left; }
.chore-card--done { opacity: 0.6; }
.chore-card--done .check-overlay { position: absolute; top: 6px; right: 8px;
                                    font-size: 1.2rem; color: var(--color-success); }
.chore-card.dragging { opacity: 0.4; }
.chore-time      { font-size: 0.75rem; color: var(--color-text-secondary); }

/* ── Week grid ────────────────────────────────────── */
.week-grid-wrapper { overflow-x: auto; -webkit-overflow-scrolling: touch; }
.week-grid         { display: grid;
                     grid-template-columns: 64px repeat(7, minmax(80px, 1fr));
                     width: max(100%, 640px); }
.week-header-row   { display: contents; }
.week-col-header   { padding: 8px 4px; font-weight: 700; font-size: 0.8rem;
                     text-align: center; background: var(--color-surface-raised);
                     position: sticky; top: 0; z-index: 1; }
.hour-label-spacer { /* occupies first column of header row */ }
.hour-row          { display: contents; }
.hour-label        { padding: 4px 8px; font-size: 0.75rem;
                     color: var(--color-text-secondary); text-align: right;
                     align-self: start; white-space: nowrap; }
.week-cell         { min-height: 48px; border: 1px solid var(--color-border-faint);
                     padding: 2px; display: flex; flex-direction: column; gap: 2px;
                     cursor: pointer; }
.week-cell.drop-target { background: #e8f4fd; border-color: var(--color-primary); }

/* Named period band coloring on week rows */
.hour-row--morning   .hour-label { background: #fff8e1; }
.hour-row--afternoon .hour-label { background: #fff3e0; }
.hour-row--evening   .hour-label { background: #fce4ec; }
.hour-row--night     .hour-label { background: #ede7f6; }

.week-chore-card { font-size: 0.75rem; border-radius: 4px; padding: 2px 4px;
                   min-height: 32px; text-align: left; cursor: pointer;
                   display: flex; align-items: center; gap: 4px; }

/* ── Bottom sheet ─────────────────────────────────── */
.bottom-sheet    { position: fixed; bottom: 0; left: 0; right: 0;
                   background: var(--color-surface);
                   border-radius: 20px 20px 0 0;
                   box-shadow: 0 -4px 32px rgba(0,0,0,0.15);
                   padding: 16px 20px 32px;
                   max-height: 85vh; overflow-y: auto;
                   z-index: 100; animation: slideUp 0.2s ease; }
.sheet-handle    { width: 40px; height: 4px; background: var(--color-border);
                   border-radius: 2px; margin: 0 auto 16px; }
.sheet-title     { font-size: 1.1rem; font-weight: 700; margin-bottom: 16px; }
.sheet-chore-item { display: flex; align-items: center; gap: 12px;
                    width: 100%; padding: 14px 12px; margin-bottom: 8px;
                    border: 1px solid var(--color-border);
                    border-radius: 12px; background: transparent;
                    cursor: pointer; min-height: 56px; font-size: 1rem; }
.sheet-chore-item:active { background: var(--color-border-faint); }
.sheet-overlay   { position: fixed; inset: 0; background: rgba(0,0,0,0.3);
                   z-index: 99; }

/* ── Recurrence picker ────────────────────────────── */
.day-pills       { display: flex; flex-wrap: wrap; gap: 8px; margin: 8px 0; }
.day-pill        { min-width: 48px; min-height: 48px;
                   border: 2px solid var(--color-border);
                   border-radius: 24px; background: transparent;
                   font-size: 0.85rem; font-weight: 600; cursor: pointer; }
.day-pill--on    { background: var(--color-primary); color: #fff;
                   border-color: var(--color-primary); }

@keyframes slideUp {
  from { transform: translateY(100%); }
  to   { transform: translateY(0); }
}

/* ── Responsive ───────────────────────────────────── */
@media (max-width: 600px) {
  .week-grid { grid-template-columns: 48px repeat(7, minmax(52px, 1fr)); }
  .week-chore-card { font-size: 0.65rem; }
  .chore-icon { display: none; }         /* hide icons in very compact week cells */
  .week-col-header { font-size: 0.7rem; }
}
```

---

## 12. Implementation Phases

Work in this order to keep the app functional at every step. Each phase ends with
all existing tests passing plus new tests green.

### Phase 1 — Database migration (no app changes, can land first)
1. Create `migrations/003_schedule_fields.sql`.
2. Verify with `make local-fresh` that the migration applies cleanly.
3. No handler or UI changes yet.

### Phase 2 — Backend: Schedule Store + extended struct (TDD)
1. Write `internal/schedule/service_test.go` with all `IsActiveForDay` test cases.
2. Extend `ChoreSchedule` struct in `service.go`.
3. Make `IsActiveForDay` pass all tests.
4. Write `internal/schedule/memory_store.go` + test for Store interface.
5. Write `internal/schedule/postgres_store.go`.
6. Run `make test-go`.

### Phase 3 — Backend: HTTP Handler (TDD)
1. Write `internal/handlers/schedule_test.go` with tests for List, ForDate, Create,
   Update, Delete.
2. Write `internal/handlers/schedule.go` to pass those tests.
3. Wire routes in `internal/app/server.go`.
4. Run `make test-go`.

### Phase 4 — Frontend: State + schedule.js (TDD)
1. Extend `state.js`.
2. Write `schedule.js` (API calls + helpers).
3. Add tests in `tests/runner.js` for `hourToPeriod`, `timeToPeriod`,
   `recurrenceSummary`, `isActiveForDayJS`.
4. Run `make test-js`.

### Phase 5 — Frontend: calendar.js + day view (TDD)
1. Write tests for `renderDayView` (correct period buckets, correct chore cards).
2. Write `calendar.js` `renderDayView`.
3. Update `today.js` `renderTodayView` to delegate.
4. Update `app.js` with `switch-view`, `navigate-week`, sheet-open handlers.
5. Add CSS to template.
6. Run `make test-js` + manual smoke test with `make run`.

### Phase 6 — Frontend: week view
1. Write tests for `renderWeekView` (headers, anytime section, day column count).
2. Implement `renderWeekView` in `calendar.js`.
3. Add `navigate-week` handler and `loadWeek` call in `app.js`.
4. Run `make test-js`.

### Phase 7 — Frontend: bottom sheet + recurrence picker
1. Write tests for `renderPickChoreSheet` (available chores, slot info).
2. Write tests for `renderRecurrencePicker` (frequency options, day pills).
3. Wire `open-pick-chore-sheet`, `schedule-chore-here`, `close-sheet` in `app.js`.
4. Wire `change-frequency` to show/hide conditional rows.
5. Wire `save-recurrence` to call `updateSchedule`.

### Phase 8 — Drag and drop
1. Add HTML5 drag/drop event listeners in `app.js`.
2. Add CSS drag states (`.dragging`, `.drop-target`).
3. Manual testing on mobile using touch emulation in DevTools.
4. Add a JS test that `isActiveForDayJS` output matches expected for each frequency type.

### Phase 9 — E2E tests
1. Write Playwright tests in `tests/e2e/` (or wherever existing e2e tests live):
   - User can see day view with period sections.
   - User can tap "+ Add chore" → bottom sheet appears → tap chore → it appears in section.
   - User can check off a chore in a time period.
   - User can switch to week view → 7 columns visible.
   - User can drag a chore card to a different period.
2. Run `make e2e`.

---

## 13. Full Code Examples (TDD-first)

### 13a. Go: `IsActiveForDay` test file

Write this file **before** implementing `IsActiveForDay`.

```go
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
    tests := []struct{ day time.Time; want bool }{
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
    tests := []struct{ day time.Time; want bool }{
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
    tests := []struct{ day time.Time; want bool }{
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
    tests := []struct{ day time.Time; want bool }{
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
        {ID: 1, IsActive: true, FrequencyType: "daily",  ChoreID: 10},
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
    for _, r := range result { ids[r.ID] = true }
    if !ids[1] || !ids[2] {
        t.Errorf("got IDs %v, want {1,2}", ids)
    }
}
```

### 13b. Go: Handler test file

Write this **before** implementing `ScheduleHandler`.

```go
// internal/handlers/schedule_test.go

package handlers

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/HammerMeetNail/nabu/internal/schedule"
)

func setupScheduleTest(t *testing.T) (*ScheduleHandler, string, *auth.Service) {
    t.Helper()
    // Reuse auth + household setup from chore_test.go's setupChoreTest pattern
    authStore    := auth.NewMemoryStore()
    authService  := auth.NewService(authStore)
    mailer       := mail.NewMemorySender()
    authService.SetMailer(mailer, "http://localhost:8080")
    authService.SetAuditLogger(nil)

    householdStore   := household.NewMemoryStore()
    householdService := household.NewService(householdStore, authService)

    scheduleStore := schedule.NewMemoryStore()
    scheduleService := schedule.NewService()
    handler := NewScheduleHandler(scheduleStore, scheduleService)

    user, session, _ := authService.Register(
        httptest.NewRequest(http.MethodGet, "/", nil).Context(),
        "alice@example.com", "password123",
    )
    householdService.CreateHousehold(
        httptest.NewRequest(http.MethodGet, "/", nil).Context(),
        "My Home", user.ID,
    )
    return handler, session.ID, authService
}

func TestScheduleListEmpty(t *testing.T) {
    handler, sessionID, authService := setupScheduleTest(t)
    req := withUser(httptest.NewRequest(http.MethodGet, "/api/schedules", nil), authService, sessionID)
    rec := httptest.NewRecorder()

    handler.List(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
    }
    var resp map[string]any
    json.Unmarshal(rec.Body.Bytes(), &resp)
    if resp["schedules"] == nil {
        t.Fatal("response missing 'schedules' key")
    }
}

func TestScheduleCreate(t *testing.T) {
    handler, sessionID, authService := setupScheduleTest(t)
    body := `{"choreId":1,"frequencyType":"daily","timePeriod":"morning"}`
    req := withUser(
        httptest.NewRequest(http.MethodPost, "/api/schedules", strings.NewReader(body)),
        authService, sessionID,
    )
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()

    handler.Create(rec, req)

    if rec.Code != http.StatusCreated {
        t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
    }
    if !strings.Contains(rec.Body.String(), `"morning"`) {
        t.Fatalf("body = %s", rec.Body.String())
    }
}

func TestScheduleCreateMissingChoreID(t *testing.T) {
    handler, sessionID, authService := setupScheduleTest(t)
    body := `{"frequencyType":"daily","timePeriod":"morning"}`
    req := withUser(
        httptest.NewRequest(http.MethodPost, "/api/schedules", strings.NewReader(body)),
        authService, sessionID,
    )
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()

    handler.Create(rec, req)

    if rec.Code != http.StatusBadRequest {
        t.Fatalf("expected 400, got %d", rec.Code)
    }
}

func TestScheduleForDate(t *testing.T) {
    handler, sessionID, authService := setupScheduleTest(t)

    // Create a daily schedule first
    createReq := withUser(
        httptest.NewRequest(http.MethodPost, "/api/schedules",
            strings.NewReader(`{"choreId":1,"frequencyType":"daily","timePeriod":"morning"}`)),
        authService, sessionID,
    )
    createReq.Header.Set("Content-Type", "application/json")
    handler.Create(httptest.NewRecorder(), createReq)

    // Now query for-date
    req := withUser(
        httptest.NewRequest(http.MethodGet, "/api/schedules/for-date?date=2026-04-28", nil),
        authService, sessionID,
    )
    rec := httptest.NewRecorder()
    handler.ForDate(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
    }
    if !strings.Contains(rec.Body.String(), `"morning"`) {
        t.Fatalf("body = %s", rec.Body.String())
    }
}

func TestScheduleRequiresHousehold(t *testing.T) {
    handler, _, _ := setupScheduleTest(t)
    req := httptest.NewRequest(http.MethodGet, "/api/schedules", nil)
    rec := httptest.NewRecorder()

    handler.List(rec, req)

    if rec.Code != http.StatusUnauthorized {
        t.Fatalf("expected 401, got %d", rec.Code)
    }
}
```

### 13c. JS: schedule.js tests

Add to `web/static/js/tests/runner.js`:

```js
// Append to existing runner.js

describe("Schedule helpers", () => {
  it("hourToPeriod: morning", async () => {
    const { hourToPeriod } = await import("../schedule.js");
    assert.equal(hourToPeriod(8), "morning");
    assert.equal(hourToPeriod(5), "morning");
    assert.equal(hourToPeriod(11), "morning");
  });

  it("hourToPeriod: afternoon", async () => {
    const { hourToPeriod } = await import("../schedule.js");
    assert.equal(hourToPeriod(12), "afternoon");
    assert.equal(hourToPeriod(16), "afternoon");
  });

  it("hourToPeriod: evening", async () => {
    const { hourToPeriod } = await import("../schedule.js");
    assert.equal(hourToPeriod(17), "evening");
    assert.equal(hourToPeriod(20), "evening");
  });

  it("hourToPeriod: night (late)", async () => {
    const { hourToPeriod } = await import("../schedule.js");
    assert.equal(hourToPeriod(21), "night");
    assert.equal(hourToPeriod(23), "night");
  });

  it("hourToPeriod: night (early AM)", async () => {
    const { hourToPeriod } = await import("../schedule.js");
    assert.equal(hourToPeriod(0), "night");
    assert.equal(hourToPeriod(4), "night");
  });

  it("timeToPeriod parses HH:MM", async () => {
    const { timeToPeriod } = await import("../schedule.js");
    assert.equal(timeToPeriod("08:30"), "morning");
    assert.equal(timeToPeriod("18:00"), "evening");
    assert.equal(timeToPeriod(null), "anytime");
    assert.equal(timeToPeriod(""),   "anytime");
  });

  it("recurrenceSummary: daily with period", async () => {
    const { recurrenceSummary } = await import("../schedule.js");
    const sch = { frequencyType: "daily", timePeriod: "morning" };
    const s = recurrenceSummary(sch);
    assert.ok(s.includes("Every day"), `got: ${s}`);
    assert.ok(s.includes("Morning"),   `got: ${s}`);
  });

  it("recurrenceSummary: weekly specific days", async () => {
    const { recurrenceSummary } = await import("../schedule.js");
    const sch = { frequencyType: "weekly", daysOfWeek: [1, 3, 5], timePeriod: "anytime" };
    const s = recurrenceSummary(sch);
    assert.ok(s.includes("Mon"), `got: ${s}`);
    assert.ok(s.includes("Wed"), `got: ${s}`);
    assert.ok(s.includes("Fri"), `got: ${s}`);
  });

  it("recurrenceSummary: monthly by date", async () => {
    const { recurrenceSummary } = await import("../schedule.js");
    const sch = { frequencyType: "monthly_by_date", dayOfMonth: 15, timePeriod: "anytime" };
    assert.ok(recurrenceSummary(sch).includes("15th"));
  });
});

describe("isActiveForDayJS recurrence", () => {
  it("daily is always active", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = { isActive: true, frequencyType: "daily" };
    assert.equal(isActiveForDayJS(sch, "2026-04-28"), true);
    assert.equal(isActiveForDayJS(sch, "2026-01-01"), true);
  });

  it("weekly fires only on correct days", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = { isActive: true, frequencyType: "weekly", daysOfWeek: [1, 3] }; // Mon, Wed
    assert.equal(isActiveForDayJS(sch, "2026-04-27"), true);  // Monday
    assert.equal(isActiveForDayJS(sch, "2026-04-29"), true);  // Wednesday
    assert.equal(isActiveForDayJS(sch, "2026-04-28"), false); // Tuesday
  });

  it("every_n_days: correct interval", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = {
      isActive: true, frequencyType: "every_n_days",
      intervalDays: 3, createdAt: "2026-04-01T00:00:00Z",
    };
    assert.equal(isActiveForDayJS(sch, "2026-04-01"), true);  // day 0
    assert.equal(isActiveForDayJS(sch, "2026-04-04"), true);  // day 3
    assert.equal(isActiveForDayJS(sch, "2026-04-03"), false); // day 2
  });

  it("monthly_by_date: matches date", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = { isActive: true, frequencyType: "monthly_by_date", dayOfMonth: 15 };
    assert.equal(isActiveForDayJS(sch, "2026-04-15"), true);
    assert.equal(isActiveForDayJS(sch, "2026-05-15"), true);
    assert.equal(isActiveForDayJS(sch, "2026-04-14"), false);
  });

  it("respects recurrenceEnd", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = {
      isActive: true, frequencyType: "daily",
      recurrenceEnd: "2026-05-01",
    };
    assert.equal(isActiveForDayJS(sch, "2026-04-30"), true);
    assert.equal(isActiveForDayJS(sch, "2026-05-02"), false);
  });
});

describe("renderDayView", () => {
  it("renders period section headings", async () => {
    const { renderDayView } = await import("../calendar.js");
    const state = {
      chores: [
        { id: 1, name: "Feed Cats", icon: "🐱", color: "#f00", category: "feeding" },
      ],
      schedules: [
        { id: 1, choreId: 1, isActive: true, timePeriod: "morning",
          frequencyType: "daily" },
      ],
      todayLogs: [],
      calendarDate: "2026-04-28",
    };
    const html = renderDayView(state);
    assert.ok(html.includes("Morning"),   `missing Morning section`);
    assert.ok(html.includes("Feed Cats"), `missing chore in Morning`);
    assert.ok(html.includes("Anytime"),   `missing Anytime section`);
  });

  it("places unscheduled chores in Anytime", async () => {
    const { renderDayView } = await import("../calendar.js");
    const state = {
      chores: [
        { id: 2, name: "Laundry", icon: "👕", color: "#00f", category: "cleaning" },
      ],
      schedules: [],  // no schedule → Anytime
      todayLogs: [],
      calendarDate: "2026-04-28",
    };
    const html = renderDayView(state);
    // Laundry should appear inside the Anytime section
    const anytimeIdx = html.indexOf("Anytime");
    const laundryIdx = html.indexOf("Laundry");
    assert.ok(laundryIdx > anytimeIdx, "Laundry should appear after Anytime heading");
  });

  it("marks completed chores as done", async () => {
    const { renderDayView } = await import("../calendar.js");
    const state = {
      chores: [
        { id: 1, name: "Dishes", icon: "🍽️", color: "#0f0", category: "cleaning" },
      ],
      schedules: [
        { id: 1, choreId: 1, isActive: true, timePeriod: "morning", frequencyType: "daily" },
      ],
      todayLogs: [{ id: 99, choreId: 1, completedAt: "2026-04-28T09:00:00Z" }],
      calendarDate: "2026-04-28",
    };
    const html = renderDayView(state);
    assert.ok(html.includes("chore-card--done"), "done chore should have done class");
    assert.ok(html.includes("undo-chore"),       "done chore should have undo action");
  });
});
```

### 13d. Memory Store for Schedule (`internal/schedule/memory_store.go`)

```go
// internal/schedule/memory_store.go

package schedule

import (
    "context"
    "errors"
    "sync"
    "time"
)

type MemoryStore struct {
    mu      sync.RWMutex
    records map[int64]ChoreSchedule
    nextID  int64
}

func NewMemoryStore() *MemoryStore {
    return &MemoryStore{records: make(map[int64]ChoreSchedule), nextID: 1}
}

func (s *MemoryStore) Create(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    sch.ID        = s.nextID
    sch.CreatedAt = time.Now().UTC()
    sch.UpdatedAt = sch.CreatedAt
    s.nextID++
    s.records[sch.ID] = sch
    return sch, nil
}

func (s *MemoryStore) Get(ctx context.Context, id int64) (ChoreSchedule, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    sch, ok := s.records[id]
    if !ok {
        return ChoreSchedule{}, errors.New("schedule not found")
    }
    return sch, nil
}

func (s *MemoryStore) ListByHousehold(ctx context.Context, householdID int64) ([]ChoreSchedule, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    var out []ChoreSchedule
    for _, sch := range s.records {
        if sch.HouseholdID == householdID {
            out = append(out, sch)
        }
    }
    return out, nil
}

func (s *MemoryStore) Update(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if _, ok := s.records[sch.ID]; !ok {
        return ChoreSchedule{}, errors.New("schedule not found")
    }
    sch.UpdatedAt = time.Now().UTC()
    s.records[sch.ID] = sch
    return sch, nil
}

func (s *MemoryStore) Delete(ctx context.Context, id int64) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if _, ok := s.records[id]; !ok {
        return errors.New("schedule not found")
    }
    delete(s.records, id)
    return nil
}
```

### 13e. Postgres Store for Schedule (`internal/schedule/postgres_store.go`)

```go
// internal/schedule/postgres_store.go

package schedule

import (
    "context"
    "database/sql"
    "encoding/json"
    "time"
)

type PostgresStore struct {
    db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
    return &PostgresStore{db: db}
}

const scheduleColumns = `
    id, household_id, chore_id, frequency_type,
    time_period, specific_time, times_of_day, days_of_week,
    interval_days, day_of_month, month_weekday, month_of_year,
    recurrence_end_date, target_count, is_active, assigned_to_user_id,
    created_at, updated_at`

func (s *PostgresStore) scan(row interface{ Scan(...any) error }) (ChoreSchedule, error) {
    var sch ChoreSchedule
    var timesRaw, daysRaw, mwRaw []byte
    var specificTime sql.NullString
    var endDate sql.NullTime

    err := row.Scan(
        &sch.ID, &sch.HouseholdID, &sch.ChoreID, &sch.FrequencyType,
        &sch.TimePeriod, &specificTime, &timesRaw, &daysRaw,
        &sch.IntervalDays, &sch.DayOfMonth, &mwRaw, &sch.MonthOfYear,
        &endDate, &sch.TargetCount, &sch.IsActive, &sch.AssignedUserID,
        &sch.CreatedAt, &sch.UpdatedAt,
    )
    if err != nil {
        return sch, err
    }
    if specificTime.Valid { sch.SpecificTime = specificTime.String }
    if endDate.Valid      { t := endDate.Time; sch.RecurrenceEnd = &t }
    if len(timesRaw) > 0  { json.Unmarshal(timesRaw, &sch.TimesOfDay) }
    if len(daysRaw)  > 0  { json.Unmarshal(daysRaw,  &sch.DaysOfWeek) }
    if len(mwRaw)    > 0  { json.Unmarshal(mwRaw,    &sch.MonthWeekday) }
    return sch, nil
}

func (s *PostgresStore) Create(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
    timesRaw, _ := json.Marshal(sch.TimesOfDay)
    daysRaw,  _ := json.Marshal(sch.DaysOfWeek)
    mwRaw,    _ := json.Marshal(sch.MonthWeekday)
    now := time.Now().UTC()

    row := s.db.QueryRowContext(ctx, `
        INSERT INTO chore_schedules
            (household_id, chore_id, frequency_type,
             time_period, specific_time, times_of_day, days_of_week,
             interval_days, day_of_month, month_weekday, month_of_year,
             recurrence_end_date, target_count, is_active, assigned_to_user_id,
             created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)
        RETURNING `+scheduleColumns,
        sch.HouseholdID, sch.ChoreID, sch.FrequencyType,
        sch.TimePeriod, nullString(sch.SpecificTime), timesRaw, daysRaw,
        sch.IntervalDays, sch.DayOfMonth, mwRaw, sch.MonthOfYear,
        sch.RecurrenceEnd, sch.TargetCount, sch.IsActive, sch.AssignedUserID,
        now,
    )
    return s.scan(row)
}

func (s *PostgresStore) Get(ctx context.Context, id int64) (ChoreSchedule, error) {
    row := s.db.QueryRowContext(ctx,
        `SELECT `+scheduleColumns+` FROM chore_schedules WHERE id=$1`, id)
    return s.scan(row)
}

func (s *PostgresStore) ListByHousehold(ctx context.Context, householdID int64) ([]ChoreSchedule, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT `+scheduleColumns+` FROM chore_schedules WHERE household_id=$1 ORDER BY id`,
        householdID)
    if err != nil { return nil, err }
    defer rows.Close()
    var out []ChoreSchedule
    for rows.Next() {
        sch, err := s.scan(rows)
        if err != nil { return nil, err }
        out = append(out, sch)
    }
    return out, rows.Err()
}

func (s *PostgresStore) Update(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
    timesRaw, _ := json.Marshal(sch.TimesOfDay)
    daysRaw,  _ := json.Marshal(sch.DaysOfWeek)
    mwRaw,    _ := json.Marshal(sch.MonthWeekday)

    row := s.db.QueryRowContext(ctx, `
        UPDATE chore_schedules SET
            frequency_type=$1, time_period=$2, specific_time=$3,
            times_of_day=$4, days_of_week=$5, interval_days=$6,
            day_of_month=$7, month_weekday=$8, month_of_year=$9,
            recurrence_end_date=$10, target_count=$11, is_active=$12,
            assigned_to_user_id=$13, updated_at=$14
        WHERE id=$15
        RETURNING `+scheduleColumns,
        sch.FrequencyType, sch.TimePeriod, nullString(sch.SpecificTime),
        timesRaw, daysRaw, sch.IntervalDays,
        sch.DayOfMonth, mwRaw, sch.MonthOfYear,
        sch.RecurrenceEnd, sch.TargetCount, sch.IsActive, sch.AssignedUserID,
        time.Now().UTC(), sch.ID,
    )
    return s.scan(row)
}

func (s *PostgresStore) Delete(ctx context.Context, id int64) error {
    _, err := s.db.ExecContext(ctx, `DELETE FROM chore_schedules WHERE id=$1`, id)
    return err
}

func nullString(s string) sql.NullString {
    return sql.NullString{String: s, Valid: s != ""}
}
```

---

*End of plan. Implement phases in order, run `make test` after each phase.*
