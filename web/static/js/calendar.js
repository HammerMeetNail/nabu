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

  // Bucket by schedule — a chore can appear in multiple periods if it has
  // multiple schedules (e.g. "morning" + "afternoon").
  const buckets = {};
  PERIODS.forEach(p => { buckets[p.id] = []; });

  const scheduledChoreIds = new Set();
  schedules.forEach(sch => {
    const chore = chores.find(c => c.id === sch.choreId);
    if (!chore) return;
    const period = sch.timePeriod || "anytime";
    buckets[period].push({ chore, sch });
    scheduledChoreIds.add(chore.id);
  });

  // Chores with no schedule at all fall into Anytime
  chores.forEach(chore => {
    if (!scheduledChoreIds.has(chore.id)) {
      buckets["anytime"].push({ chore, sch: null });
    }
  });

  const sections = PERIODS.map(period =>
    renderPeriodSection(period, buckets[period.id], logMap, date)
  ).join("");

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

  // Tap-to-add button for non-anytime sections
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
  // Schedules with only a timePeriod (no specificTime) are shown in the
  // period sections below the grid so they don't all pile up at startHour.
  const rows = GRID_HOURS.map(hour => {
    const periodId  = hourToPeriodDirect(hour);
    const bandClass = `hour-row--${periodId}`;

    const cells = days.map(iso => {
      const choreCells = schedules
        .filter(sch => {
          if (!isActiveForDayJS(sch, iso)) return false;
          if (sch.timePeriod === "anytime") return false;
          if (!sch.specificTime) return false; // period-only → shown below grid
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

  // Period sections below the grid: schedules with timePeriod but no specificTime.
  const periodOnlySchedules = schedules.filter(s =>
    s.timePeriod && s.timePeriod !== "anytime" && !s.specificTime
  );
  const periodSection = renderPeriodWeekSection(periodOnlySchedules, chores, days, logMap);

  // "Anytime" section below the grid
  const anytimeSchedules = schedules.filter(s => s.timePeriod === "anytime");
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
      ${periodSection}
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
    <h3 class="period-heading">📋 Anytime</h3>
    <div class="week-grid anytime-grid">${rows}</div>
  </div>`;
}

/**
 * Renders below-grid period bands for schedules that have a timePeriod but
 * no specificTime.  Groups by period so morning / afternoon / etc. each get
 * their own labelled row (mirrors how renderAnytimeWeekSection works).
 */
function renderPeriodWeekSection(periodSchedules, chores, days, logMap) {
  if (periodSchedules.length === 0) return "";

  // Group schedules by their timePeriod, preserving PERIODS display order.
  const groups = {};
  PERIODS.forEach(p => { groups[p.id] = []; });
  periodSchedules.forEach(sch => {
    const pid = sch.timePeriod || "anytime";
    if (groups[pid]) groups[pid].push(sch);
  });

  const sections = PERIODS.filter(p => p.id !== "anytime" && groups[p.id].length > 0).map(period => {
    const rows = groups[period.id].map(sch => {
      const chore = chores.find(c => c.id === sch.choreId);
      if (!chore) return "";
      const cells = days.map(iso => {
        const log = logMap[`${chore.id}-${iso}`];
        return `<div class="week-cell week-cell--anytime">
          ${renderWeekChoreCard(chore, sch, log, iso)}
        </div>`;
      }).join("");
      return `<div class="anytime-row">
        <div class="hour-label">${period.icon}</div>
        ${cells}
      </div>`;
    }).join("");

    return `<div class="period-week-section">
      <h3 class="period-heading">${period.icon} ${period.label}</h3>
      <div class="week-grid anytime-grid">${rows}</div>
    </div>`;
  }).join("");

  return sections;
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
