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
 * Renders the full day view: 24 hourly rows (drag-and-droppable) plus an
 * "Anytime" section below for chores with no specific time set.
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

  // Track chores that have a specific-time schedule (used to find "anytime" chores)
  const timeScheduledChoreIds = new Set();
  schedules.forEach(sch => {
    if (sch.specificTime) timeScheduledChoreIds.add(sch.choreId);
  });

  // Build rows: one per hour 0-23.
  // Only schedules with a specificTime appear in the hourly grid.
  const rows = GRID_HOURS.map(hour => {
    const choreCells = schedules
      .filter(sch => {
        if (!sch.specificTime) return false;
        const schHour = parseInt(sch.specificTime.split(":")[0], 10);
        return schHour === hour;
      })
      .map(sch => {
        const chore = chores.find(c => c.id === sch.choreId);
        if (!chore) return "";
        return renderChoreCard(chore, sch, logMap[chore.id], date);
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
        ${choreCells}
      </div>
    </div>`;
  }).join("");

  // "Anytime" section: schedules without a specificTime + wholly unscheduled chores.
  const noTimeSchedules = schedules.filter(s => !s.specificTime);
  const allScheduledChoreIds = new Set([
    ...timeScheduledChoreIds,
    ...noTimeSchedules.map(s => s.choreId),
  ]);
  const unscheduledChores = chores.filter(c => !allScheduledChoreIds.has(c.id));

  const anytimeItems = [
    ...noTimeSchedules.map(sch => {
      const chore = chores.find(c => c.id === sch.choreId);
      return chore ? { chore, sch } : null;
    }).filter(Boolean),
    ...unscheduledChores.map(chore => ({ chore, sch: null })),
  ];

  const anytimeSection = anytimeItems.length > 0 ? `
    <div class="day-anytime-section">
      <h3 class="section-heading">Anytime</h3>
      <div class="day-anytime-cards"
        data-drop-period="anytime"
        data-drop-date="${date}">
        ${anytimeItems.map(({ chore, sch }) =>
          renderChoreCard(chore, sch, logMap[chore.id], date)
        ).join("")}
      </div>
    </div>` : "";

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
        <div class="day-hour-grid">${rows}</div>
      </div>
      ${anytimeSection}
    </div>`;
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
        data-action-empty="open-pick-chore-sheet"
        data-date="${iso}"
        data-hour="${hour}">
        ${choreCells || `<span class="week-cell-empty" aria-hidden="true"></span>`}
      </div>`;
    }).join("");

    const hourLabel = `<div class="hour-label">${fmtHour(hour)}</div>`;

    return `<div class="hour-row" data-hour="${hour}">
      ${hourLabel}${cells}
    </div>`;
  }).join("");

  // "Anytime" section below the grid: schedules with no specificTime.
  const anytimeSchedules = schedules.filter(s => !s.specificTime);
  const anytimeSection = renderAnytimeWeekSection(anytimeSchedules, chores, days, logMap);

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
    <h3 class="section-heading">📋 Anytime</h3>
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
