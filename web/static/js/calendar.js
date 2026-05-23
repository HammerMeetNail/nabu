// web/static/js/calendar.js

import { escapeHTML }     from "./utils.js";
import { todayISO }       from "./today.js";

// ─── Constants ────────────────────────────────────────────────────────────────

// Hours shown in the day and week view grids, 0-23.
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
 * Renders the full day view: 24 hourly rows (drag-and-droppable).
 * Scheduled chores appear in their designated hour rows.  Logs without a
 * slotHour (e.g. logged from the home tab) appear in an "Anytime" row above
 * the hour grid.
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

  // Filter to schedules active on the viewed date (respects "once", "weekly", etc.)
  const activeSchedules = schedules.filter(sch => isActiveForDayJS(sch, date));

  // Anytime row: logs with no slotHour (e.g. logged from the home tab).
  // Always shown regardless of whether the chore also has a timed schedule.
  const anytimeLogs = logs.filter(l => l.slotHour == null);
  const anytimeRow = anytimeLogs.length > 0
    ? `<div class="day-anytime-row">
        <div class="hour-label hour-label--anytime">Anytime</div>
        <div class="day-hour-cell">
          ${anytimeLogs.map(l => {
            const chore = chores.find(c => c.id === l.choreId);
            if (!chore) return "";
            return renderChoreCard(chore, null, l, date, true);
          }).join("")}
        </div>
      </div>`
    : "";

  // Build rows: one per hour 0-23.
  // Schedules with a specificTime matching the hour appear here, as do any
  // ad-hoc logs (no matching timed schedule) whose slotHour matches.
  const rows = GRID_HOURS.map(hour => {
    const scheduledAtHour = activeSchedules.filter(sch => {
      if (!sch.specificTime) return false;
      return parseInt(sch.specificTime.split(":")[0], 10) === hour;
    });
    const scheduledChoreIdsAtHour = new Set(scheduledAtHour.map(s => s.choreId));

    // Ad-hoc logged chores at this hour: logged with slotHour === hour but not
    // already shown via a timed schedule in this same row.
    const adHocCells = logs
      .filter(l => l.slotHour === hour && !scheduledChoreIdsAtHour.has(l.choreId))
      .map(l => {
        const chore = chores.find(c => c.id === l.choreId);
        if (!chore) return "";
        return renderChoreCard(chore, null, l, date, true);
      }).join("");

    const scheduledCells = scheduledAtHour
      .map(sch => {
        const chore = chores.find(c => c.id === sch.choreId);
        if (!chore) return "";
        return renderChoreCard(chore, sch, logMap[chore.id], date, true);
      }).join("");

    return `<div class="day-hour-row" data-hour="${hour}">
      <button type="button" class="hour-label"
        data-action="open-pick-chore-sheet"
        data-date="${date}"
        data-hour="${hour}">${fmtHour(hour)}</button>
      <div class="day-hour-cell"
        data-drop-hour="${hour}"
        data-drop-date="${date}"
        data-action="open-pick-chore-sheet"
        data-date="${date}"
        data-hour="${hour}">
        ${scheduledCells}${adHocCells}
      </div>
    </div>`;
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
      <div class="day-hour-grid-wrapper">
        <div class="day-hour-grid">${anytimeRow}${rows}</div>
      </div>
    </div>`;
}

function renderChoreCard(chore, sch, log, date, compact = false) {
  const done        = !!log;
  const doneClass   = done ? "chore-card--done" : "";
  const compactClass = compact ? "chore-card--compact" : "";
  const action      = done ? "view-log" : "log-chore";
  const logId       = log ? log.id : "";
  // Time label only shown in full-size (anytime) cards — the hour row itself
  // already indicates the time, so showing it again in compact chips is noise.
  const timeLabel = (!compact && sch?.specificTime)
    ? `<span class="chore-time">${fmtTime12(sch.specificTime)}</span>`
    : "";
  const assignee  = sch?.assignedUserId
    ? `<span class="chore-assignee" aria-label="Assigned">👤</span>`
    : "";
  const pencil = sch?.id
    ? `<button type="button" class="chore-card-edit-btn"
        data-action="edit-schedule"
        data-chore-id="${chore.id}"
        data-schedule-id="${sch.id}"
        aria-label="Edit schedule"
        title="Edit schedule">✎</button>`
    : "";

  return `
    <div class="chore-card-wrap">
      <button type="button"
        class="chore-card ${compactClass} ${doneClass}"
        style="border-left: 4px solid ${escapeHTML(chore.color)}"
        data-action="${action}"
        data-chore-id="${chore.id}"
        data-log-id="${logId}"
        data-date="${date}"
        draggable="true"
        data-drag-chore-id="${chore.id}"
        data-drag-schedule-id="${sch?.id || ""}"
        aria-pressed="${done}">
        <span class="chore-icon" aria-hidden="true">${chore.icon}</span>
        <span class="chore-name">${escapeHTML(chore.name)}</span>
        ${timeLabel}${assignee}
        ${done ? '<span class="check-overlay" aria-hidden="true">✓</span>' : ""}
      </button>
      ${pencil}
    </div>`;
}

// ─── Week View ────────────────────────────────────────────────────────────────

/**
 * Renders the week view: 7 day columns × 24 hour rows.
 *
 * @param {object}   state
 * @param {object[]} state.chores
 * @param {object[]} state.schedules   All schedules
 * @param {object[]} state.weekLogs    Logs for the whole viewed week
 * @param {string}   state.calendarDate  ISO date of any day in the viewed week
 */
export function renderWeekView(state) {
  const weekStart = isoMonday(state.calendarDate || todayISO(0));
  const days      = Array.from({ length: 7 }, (_, i) => shiftISO(weekStart, i));
  const chores    = state.chores    || [];
  const schedules = state.schedules || [];
  const weekLogs  = state.weekLogs  || [];

  // Build log lookup: "choreId-YYYY-MM-DD" → log
  const logKey = (choreId, iso) => `${choreId}-${iso}`;
  const logMap = {};
  weekLogs.forEach(l => {
    const iso = l.completedAt ? l.completedAt.slice(0, 10) : "";
    logMap[logKey(l.choreId, iso)] = l;
  });

  const dayHeaders = days.map(iso =>
    `<div class="week-col-header">${fmtShortDate(iso)}</div>`
  ).join("");

  // Anytime row: one cell per day showing logs with no slotHour.
  const anytimeCells = days.map(iso => {
    const dayLogs = weekLogs.filter(l => {
      if (l.slotHour != null) return false;
      const logIso = l.completedAt ? l.completedAt.slice(0, 10) : "";
      return logIso === iso;
    });
    const cards = dayLogs.map(l => {
      const chore = chores.find(c => c.id === l.choreId);
      if (!chore) return "";
      return renderWeekChoreCard(chore, null, l, iso);
    }).join("");
    return `<div class="week-cell" data-date="${iso}">${cards}</div>`;
  }).join("");

  const hasAnytime = weekLogs.some(l => l.slotHour == null);
  const anytimeRow = hasAnytime
    ? `<div class="hour-row week-anytime-row">
        <div class="hour-label hour-label--anytime">Anytime</div>
        ${anytimeCells}
      </div>`
    : "";

  // Build rows: one per hour.
  // Only schedules with a specificTime are shown in the hourly grid.
  const rows = GRID_HOURS.map(hour => {
    const cells = days.map(iso => {
      const choreCells = schedules
        .filter(sch => {
          if (!isActiveForDayJS(sch, iso)) return false;
          if (!sch.specificTime) return false;
          const schHour = parseInt(sch.specificTime.split(":")[0], 10);
          return schHour === hour;
        })
        .map(sch => {
          const chore = chores.find(c => c.id === sch.choreId);
          if (!chore) return "";
          const log = logMap[logKey(chore.id, iso)];
          return renderWeekChoreCard(chore, sch, log, iso);
        }).join("");

      return `<div class="week-cell"
        data-drop-date="${iso}"
        data-drop-hour="${hour}"
        data-action="open-pick-chore-sheet"
        data-date="${iso}"
        data-hour="${hour}">
        ${choreCells}
      </div>`;
    }).join("");

    const hourLabel = `<div class="hour-label">${fmtHour(hour)}</div>`;

    return `<div class="hour-row" data-hour="${hour}">
      ${hourLabel}${cells}
    </div>`;
  }).join("");

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
          <div class="week-body">${anytimeRow}${rows}</div>
        </div>
      </div>
    </div>`;
}

function renderWeekChoreCard(chore, sch, log, iso) {
  const done   = !!log;
  const action = done ? "view-log" : "log-chore";
  const pencil = sch?.id
    ? `<button type="button" class="chore-card-edit-btn"
        data-action="edit-schedule"
        data-chore-id="${chore.id}"
        data-schedule-id="${sch.id}"
        aria-label="Edit schedule"
        title="Edit schedule">✎</button>`
    : "";
  return `<div class="chore-card-wrap">
    <button type="button"
      class="week-chore-card ${done ? "chore-card--done" : ""}"
      style="background:${escapeHTML(chore.color)}22; border-left: 3px solid ${escapeHTML(chore.color)}"
      data-action="${action}"
      data-chore-id="${chore.id}"
      data-log-id="${log?.id || ""}"
      data-date="${iso}"
      draggable="true"
      data-drag-chore-id="${chore.id}"
      data-drag-schedule-id="${sch?.id || ""}"
      aria-label="${escapeHTML(chore.name)}${done ? " (done)" : ""}"
      title="${escapeHTML(chore.name)}">
      <span class="chore-icon" aria-hidden="true">${chore.icon}</span>
      <span class="chore-name">${escapeHTML(chore.name)}</span>
      ${done ? '<span class="check-overlay" aria-hidden="true">✓</span>' : ""}
    </button>
    ${pencil}
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
  const diff = day === 0 ? -6 : 1 - day; // Move back to Monday
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
    case "once":
      // startDate is serialized as "YYYY-MM-DD" from the DateOnly backend type
      return !!sch.startDate && isoDate === sch.startDate.slice(0, 10);
    case "daily":
      return true;
    case "weekly":
      return (sch.daysOfWeek || []).includes(wd);
    case "every_n_days": {
      if (!sch.intervalDays || sch.intervalDays <= 0) return false;
      // Use the date portion of createdAt to keep timezone consistent with isoDate.
      const createdDateStr = sch.createdAt
        ? String(sch.createdAt).slice(0, 10)
        : isoDate;
      const origin = new Date(createdDateStr + "T00:00:00");
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
