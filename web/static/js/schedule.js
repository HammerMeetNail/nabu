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
 * @param {object[]} chores     All household chores
 * @param {object}   slot       { date: "YYYY-MM-DD", timePeriod: "morning", hour: 8 }
 * @param {object[]} schedules  Already-scheduled chores for this slot
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
 * @param {object} sch  Current schedule values (may be partial/new)
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

      <div class="day-pill-row" id="weekday-row" ${ft !== "weekly" ? "hidden" : ""}>
        <p class="field-label">On these days</p>
        <div class="day-pills">${dayPills}</div>
      </div>

      <div id="interval-row" ${ft !== "every_n_days" ? "hidden" : ""}>
        <label class="field-label" for="interval-input">Every how many days?</label>
        <input id="interval-input" type="number" min="2" max="365"
          class="text-input" value="${sch?.intervalDays || 2}" />
      </div>

      <div id="dom-row" ${!["monthly_by_date","yearly"].includes(ft) ? "hidden" : ""}>
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
