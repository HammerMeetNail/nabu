// web/static/js/schedule.js

import { apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";

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
 * e.g. "Every Mon, Wed, Fri • 8:00 AM"
 */
export function recurrenceSummary(sch) {
  if (!sch || !sch.frequencyType) return "Not scheduled";

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
  return freq;
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

function fmtHour(h) {
  if (h === 0)  return "12 AM";
  if (h === 12) return "12 PM";
  return h < 12 ? `${h} AM` : `${h - 12} PM`;
}

// ─── Render: bottom sheet — pick a chore for a time slot ─────────────────────

/**
 * Renders the "pick a chore" bottom sheet.
 * All household chores are always shown so a chore can be added multiple times
 * (e.g. feeding the cat at 8 AM and 6 PM both need the same chore).
 * An inline form at the bottom lets the user create a brand-new chore on the
 * fly and have it immediately scheduled for this slot.
 *
 * @param {object[]} chores     All household chores
 * @param {object}   slot       { date: "YYYY-MM-DD", hour: 8 }
 * @param {object[]} _schedules Unused (kept for call-site compatibility)
 */
export function renderPickChoreSheet(chores, slot, _schedules) {
  // Default time: top of the tapped hour, or empty for "anytime"
  const defaultTime = slot.hour != null
    ? `${String(slot.hour).padStart(2, "0")}:00`
    : "";

  const items = chores.length === 0
    ? `<p class="sheet-empty">No chores set up yet — create one below.</p>`
    : chores.map(c => `
        <button type="button"
          class="sheet-chore-item"
          data-action="schedule-chore-here"
          data-chore-id="${c.id}"
          data-time-period="anytime"
          data-date="${escapeHTML(slot.date || "")}">
          <span class="chore-icon">${c.icon}</span>
          <span class="chore-name">${escapeHTML(c.name)}</span>
          <span class="chore-category">${escapeHTML(c.category)}</span>
        </button>`).join("");

  const title = slot.hour != null
    ? `Add to ${fmtHour(slot.hour)}`
    : "Add Chore";

  return `
    <div class="bottom-sheet" role="dialog" aria-modal="true" aria-label="${title}">
      <div class="sheet-handle" aria-hidden="true"></div>
      <h2 class="sheet-title">${title}</h2>
      <div class="sheet-time-row">
        <label for="sheet-time" class="field-label">Time</label>
        <input type="time" id="sheet-time" class="text-input sheet-time-input"
          step="900" value="${defaultTime}" />
      </div>
      <div class="sheet-chore-list">${items}</div>
      <form data-action="new-chore-from-sheet" class="sheet-new-chore-form">
        <input type="hidden" name="timePeriod" value="anytime" />
        <input type="hidden" name="date" value="${escapeHTML(slot.date || "")}" />
        <p class="sheet-section-label">Create new chore</p>
        <div class="sheet-new-chore-row">
          <input type="text" name="choreName" class="text-input sheet-new-chore-input"
            placeholder="Chore name…" autocomplete="off" />
          <button type="submit" class="btn btn-primary sheet-new-chore-btn" aria-label="Create and add chore">+</button>
        </div>
      </form>
      <button type="button" class="btn btn-ghost btn-full sheet-cancel-btn" data-action="close-sheet">
        Cancel
      </button>
    </div>`;
}

// ─── Render: edit-schedule bottom sheet ──────────────────────────────────────

/**
 * Renders the "edit schedule" bottom sheet opened by long-pressing a chore card.
 *
 * @param {object} chore  The chore object { id, icon, name, … }
 * @param {object} sch    The schedule object { id, specificTime, … }
 */
export function renderEditScheduleSheet(chore, sch) {
  const currentTime = sch?.specificTime || "";
  const scheduleId  = sch?.id ?? "";
  return `
    <div class="bottom-sheet" role="dialog" aria-modal="true" aria-label="Edit ${escapeHTML(chore.name)}">
      <div class="sheet-handle" aria-hidden="true"></div>
      <h2 class="sheet-title">${chore.icon} ${escapeHTML(chore.name)}</h2>
      <div class="sheet-time-row">
        <label for="edit-sheet-time" class="field-label">Time</label>
        <input type="time" id="edit-sheet-time" class="text-input sheet-time-input"
          step="900" value="${escapeHTML(currentTime)}" />
      </div>
      <button type="button" class="btn btn-primary btn-full"
        data-action="save-schedule-edit"
        data-schedule-id="${scheduleId}">
        Save
      </button>
      <button type="button" class="btn btn-danger btn-full mt-2"
        data-action="delete-schedule"
        data-schedule-id="${scheduleId}">
        Remove from schedule
      </button>
      <button type="button" class="btn btn-ghost btn-full sheet-cancel-btn" data-action="close-sheet">
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
