// web/static/js/schedule.js

import { apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";

const MANAGE_ICON = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/></svg>`;

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
  once:               "Does not repeat",
  daily:              "Every day",
  weekly:             "Weekly",
  every_n_days:       "Every N days",
  monthly_by_date:    "Monthly (same date)",
  monthly_by_weekday: "Monthly (same weekday)",
  yearly:             "Every year",
};

const DAY_NAMES  = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MONTH_NAMES = ["Jan","Feb","Mar","Apr","May","Jun",
                     "Jul","Aug","Sep","Oct","Nov","Dec"];

/**
 * Returns a human-readable summary of the recurrence rule.
 * e.g. "Every Mon, Wed, Fri • 8:00 AM"
 */
export function recurrenceSummary(sch) {
  if (!sch || !sch.frequencyType) return "Not scheduled";

  let freq = "";
  switch (sch.frequencyType) {
    case "once":
      freq = sch.startDate ? `Once on ${fmtDateShort(sch.startDate)}` : "Once";
      break;
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

function fmtDateShort(isoDate) {
  // isoDate is "YYYY-MM-DD"
  const d = new Date(isoDate.slice(0, 10) + "T00:00:00");
  return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

function fmtHour(h) {
  if (h === 0)  return "12 AM";
  if (h === 12) return "12 PM";
  return h < 12 ? `${h} AM` : `${h - 12} PM`;
}

// ─── Frequency selector ───────────────────────────────────────────────────────

/**
 * Renders a compact Google-Calendar-style "Repeat" selector for use inside
 * bottom sheets.
 *
 * Smart defaults are derived from `date` (YYYY-MM-DD).  When editing an
 * existing schedule, pass `sch` so the current frequency is pre-selected and
 * the options reflect its stored values.
 *
 * @param {string} date   ISO date string "YYYY-MM-DD" (slot date / viewed date)
 * @param {object} sch    Existing schedule object or null for new
 * @param {string} prefix DOM id prefix: "sheet" or "edit-sheet"
 */
export function renderFreqSelect(date, sch, prefix) {
  const d = date ? new Date(date.slice(0, 10) + "T00:00:00") : new Date();
  const slotWeekday  = d.getDay();
  const slotDom      = d.getDate();
  const slotMoy      = d.getMonth() + 1;

  const ft = sch?.frequencyType || "once";

  // For each frequency type, prefer existing sch values when editing, otherwise
  // fall back to smart defaults from the slot date.
  const dom     = (["monthly_by_date","yearly"].includes(ft) && sch?.dayOfMonth) ? sch.dayOfMonth : slotDom;
  const moy     = (ft === "yearly"             && sch?.monthOfYear) ? sch.monthOfYear : slotMoy;
  const interval = (ft === "every_n_days" && sch?.intervalDays > 1) ? sch.intervalDays : 2;

  // For weekly schedules: when editing, show all selected days; for new
  // schedules, default to the slot's weekday.
  const wkDays = (ft === "weekly" && sch?.daysOfWeek?.length) ? sch.daysOfWeek : [slotWeekday];
  const wkLabel = wkDays.map(d => DAY_NAMES[d]).join(", ");

  const options = [
    {
      value: "once",
      label: "Does not repeat",
      extra: "",
    },
    {
      value: "daily",
      label: "Every day",
      extra: "",
    },
    {
      value: "every_n_days",
      label: `Every ${interval} days`,
      extra: `data-interval-days="${interval}"`,
    },
    {
      value: "weekly",
      label: `Every week on ${wkLabel}`,
      extra: `data-days-of-week="${escapeHTML(JSON.stringify(wkDays))}"`,
    },
    {
      value: "monthly_by_date",
      label: `Monthly on the ${ordinal(dom)}`,
      extra: `data-day-of-month="${dom}"`,
    },
    {
      value: "yearly",
      label: `Annually on ${MONTH_NAMES[moy - 1]} ${dom}`,
      extra: `data-day-of-month="${dom}" data-month-of-year="${moy}"`,
    },
  ].map(({ value, label, extra }) =>
    `<option value="${value}" ${ft === value ? "selected" : ""} ${extra}>${escapeHTML(label)}</option>`
  ).join("");

  // Day-of-week pills (shown only when "weekly" is selected).
  const dayPills = DAY_NAMES.map((name, i) => `
    <button type="button"
      class="day-pill ${wkDays.includes(i) ? "day-pill--on" : ""}"
      data-action="toggle-day"
      data-day="${i}"
      aria-pressed="${wkDays.includes(i)}"
      aria-label="${name}">${name}</button>`).join("");

  return `
    <div class="sheet-freq-row">
      <label for="${prefix}-freq" class="field-label">Repeat</label>
      <select id="${prefix}-freq" class="select-input" data-action="change-frequency">
        ${options}
      </select>
    </div>
    <div id="${prefix}-weekday-row" class="sheet-weekday-row" ${ft !== "weekly" ? "hidden" : ""}>
      <p class="field-label">On these days</p>
      <div class="day-pills" id="${prefix}-day-pills">${dayPills}</div>
    </div>
    <div id="${prefix}-interval-row" class="sheet-interval-row" ${ft !== "every_n_days" ? "hidden" : ""}>
      <label for="${prefix}-interval" class="field-label">Every</label>
      <div class="interval-input-group">
        <input type="number" id="${prefix}-interval" class="text-input interval-input"
               min="2" max="365" value="${interval}" inputmode="numeric">
        <span class="interval-unit">days</span>
      </div>
    </div>
    <div id="${prefix}-end-date-row" class="sheet-end-date-row" ${ft === "once" ? "hidden" : ""}>
      <label for="${prefix}-end-date" class="field-label">Stop repeating</label>
      <input type="date" id="${prefix}-end-date" class="text-input"
        value="${sch?.recurrenceEnd ? String(sch.recurrenceEnd).slice(0, 10) : ""}" />
    </div>`;
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
          draggable="true"
          data-action="schedule-chore-here"
          data-chore-id="${c.id}"
          data-reorder-chore-id="${c.id}"
          data-time-period="anytime"
          data-date="${escapeHTML(slot.date || "")}">
          <span class="drag-handle" aria-hidden="true">⠿</span>
          <span class="chore-icon">${escapeHTML(c.icon)}</span>
          <span class="chore-name">${escapeHTML(c.name)}</span>
          <span class="chore-category">${escapeHTML(c.category)}</span>
        </button>`).join("");

  const title = slot.hour != null
    ? `Add to ${fmtHour(slot.hour)}`
    : "Add Chore";

  const freqHTML = renderFreqSelect(slot.date || null, null, "sheet");

  return `
    <div class="bottom-sheet" role="dialog" aria-modal="true" aria-label="${title}">
      <div class="sheet-handle" aria-hidden="true"></div>
      <h2 class="sheet-title">${title}</h2>
      <p class="sheet-hint">Tap to schedule · Hold to log with notes</p>
      <div class="sheet-time-row">
        <label for="sheet-time" class="field-label">Time</label>
        <input type="time" id="sheet-time" class="text-input sheet-time-input"
          step="300" value="${defaultTime}" />
      </div>
      ${freqHTML}
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
 * @param {object} sch    The schedule object { id, specificTime, frequencyType, … }
 * @param {string} date   The currently-viewed date (YYYY-MM-DD), used as default
 *                        for "once" startDate if the user switches to it.
 */
export function renderEditScheduleSheet(chore, sch, date) {
  const currentTime = sch?.specificTime || "";
  const scheduleId  = sch?.id ?? "";
  const freqHTML    = renderFreqSelect(sch?.startDate || date || null, sch, "edit-sheet");

  return `
    <div class="bottom-sheet" role="dialog" aria-modal="true" aria-label="Edit ${escapeHTML(chore.name)}">
      <div class="sheet-handle" aria-hidden="true"></div>
      <h2 class="sheet-title">${escapeHTML(chore.icon)} ${escapeHTML(chore.name)}</h2>
      <div class="sheet-time-row">
        <label for="edit-sheet-time" class="field-label">Time</label>
        <input type="time" id="edit-sheet-time" class="text-input sheet-time-input"
          step="300" value="${escapeHTML(currentTime)}" />
      </div>
      ${freqHTML}
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

// ─── Render: configure-schedule bottom sheet ─────────────────────────────────

export function renderConfigureScheduleSheet(chore, date, hour, presetTime, presetFreq) {
  const defaultTime = presetTime != null
    ? presetTime
    : (hour != null ? `${String(hour).padStart(2, "0")}:00` : "");
  const sch = {
    specificTime: defaultTime,
    ...(presetFreq || {}),
  };
  const freqHTML = renderFreqSelect(date, sch, "config-sheet");

  return `
    <div class="bottom-sheet" role="dialog" aria-modal="true" aria-label="Schedule ${escapeHTML(chore.name)}">
      <div class="sheet-handle" aria-hidden="true"></div>
      <h2 class="sheet-title">${escapeHTML(chore.icon)} ${escapeHTML(chore.name)}</h2>
      <p class="sheet-hint">Set time and repeat before scheduling</p>
      <div class="sheet-time-row">
        <label for="config-sheet-time" class="field-label">Time</label>
        <input type="time" id="config-sheet-time" class="text-input sheet-time-input"
          step="300" value="${escapeHTML(defaultTime)}" />
      </div>
      ${freqHTML}
      <button type="button" class="btn btn-primary btn-full"
        data-action="save-configure-schedule"
        data-chore-id="${chore.id}"
        data-date="${escapeHTML(date || "")}">
        Schedule
      </button>
      <button type="button" class="btn btn-ghost btn-full sheet-cancel-btn" data-action="close-sheet">
        Cancel
      </button>
    </div>`;
}

// ─── Render: log-with-indicators bottom sheet ────────────────────────────────

function renderIndicatorVolumeRow(label, on, selectedML = null) {
  const options = Array.from({ length: 41 }, (_, i) => i * 5);
  const optsHTML = options.map(v => {
    const sel = selectedML === v ? " selected" : "";
    return `<option value="${v}"${sel}>${v} mL</option>`;
  }).join("");
  return `<div class="indicator-row">
    <button type="button"
      class="log-chip${on ? " log-chip--on" : ""}"
      data-action="toggle-indicator"
      data-label="${escapeHTML(label)}"
      aria-pressed="${on}">
      ${escapeHTML(label)}
    </button>
    <select class="indicator-volume-select select-input"
      data-indicator="${escapeHTML(label)}"
      ${on ? "" : "style=\"display:none\""}>
      <option value=""${selectedML == null ? " selected" : ""}>--</option>
      ${optsHTML}
    </select>
  </div>`;
}

/**
 * Renders the log sheet for both "log" mode (log=null) and "edit log" mode.
 *
 * @param {object}      chore  { id, icon, name, color, indicatorLabels[] }
 * @param {object|null} log    Existing log entry, or null for new log
 * @param {string}      date   ISO date "YYYY-MM-DD"
 * @param {object[]}    members   Household members
 * @param {number}      currentUserId  Current auth user's ID
 * @param {number|null} cachedVolumeML Volume from previous log or null (DEPRECATED; use cachedIndicatorVolumes)
 * @param {object}      cachedIndicatorVolumes Map of indicator label -> volume mL (from latest log)
 * @param {object}      opts   { showWhen: bool, slotHour: number|null }
 */
export function renderLogSheet(chore, log, date, members, currentUserId, cachedVolumeML = null, opts = {}) {
  const title = `${escapeHTML(chore.icon)} ${escapeHTML(chore.name)}`;
  const noteVal = log ? escapeHTML(log.note || "") : "";
  const titleVal = log?.title ? escapeHTML(log.title) : "";
  const activeIndicators = new Set(log?.indicators || (chore.indicatorDefaults || []));
  const logIndicatorVolumes = log?.indicatorVolumes || {};
  const cachedIndicatorVolumes = (log ? null : (opts.cachedIndicatorVolumes || null));

  const indicatorSection = (() => {
    const labels = chore.indicatorLabels || [];
    if (labels.length === 0) return "";

    if (chore.hasVolumeML) {
      const rows = labels.map(label => {
        const on = activeIndicators.has(label);
        const volume = log ? (logIndicatorVolumes[label] ?? null)
          : ((cachedIndicatorVolumes?.[label]) ?? null);
        return renderIndicatorVolumeRow(label, on, volume);
      }).join("");
      return `<div class="sheet-indicator-row">
        <p class="field-label">Type</p>
        <div class="indicator-rows">${rows}</div>
      </div>`;
    }

    const chips = labels.map(label => {
      const on = activeIndicators.has(label);
      return `<button type="button"
        class="log-chip${on ? " log-chip--on" : ""}"
        data-action="toggle-indicator"
        data-label="${escapeHTML(label)}"
        aria-pressed="${on}">
        ${escapeHTML(label)}
      </button>`;
    }).join("");
    return `<div class="sheet-chip-row">
      <p class="field-label">How did it go?</p>
      <div class="chip-list">${chips}</div>
    </div>`;
  })();

  const selectedMemberId = log?.userId ?? (currentUserId || null);
  const memberSection = renderMemberSelect(members, currentUserId, selectedMemberId, "log");

  const followUpSection = !log && chore.followUpEnabled ? (() => {
    const totalMins = chore.lastFollowUpMinutes || 0;
    const d = Math.floor(totalMins / 1440);
    const h = Math.floor((totalMins % 1440) / 60);
    const m = Math.round((totalMins % 60) / 5) * 5;

    const dayOpts = Array.from({ length: 15 }, (_, i) =>
      `<option value="${i}"${i === d ? ' selected' : ''}>${i}d</option>`).join('');
    const hourOpts = Array.from({ length: 24 }, (_, i) =>
      `<option value="${i}"${i === h ? ' selected' : ''}>${i}h</option>`).join('');
    const minOpts = [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55].map(v =>
      `<option value="${v}"${v === m ? ' selected' : ''}>${v}m</option>`).join('');

    return `<div class="sheet-followup-row">
      <span class="field-label">Follow-up in</span>
      <div class="followup-inputs">
        <select id="followup-days" class="followup-select">${dayOpts}</select>
        <select id="followup-hours" class="followup-select">${hourOpts}</select>
        <select id="followup-mins" class="followup-select">${minOpts}</select>
      </div>
    </div>`;
  })() : "";

    const titleSection = chore.hasRating ? `
    <div class="sheet-note-row">
      <label for="log-title" class="field-label">Title</label>
      <input id="log-title" class="text-input" placeholder="Enter a title…" value="${titleVal}" maxlength="200">
    </div>` : "";

    const ratingSection = chore.hasRating ? (() => {
      const rating = log?.rating || 0;
      const pct = (rating / 50) * 100;
      return `<div class="star-rating-row">
        <label class="field-label">Rating</label>
        <div class="star-rating" data-action="set-rating" data-rating="${rating}" role="slider" aria-valuemin="0" aria-valuemax="50" aria-valuenow="${rating}" aria-valuetext="${rating / 10} stars">
          <span class="star-rating-bg">☆☆☆☆☆</span>
          <span class="star-rating-fg" style="width:${pct}%">★★★★★</span>
        </div>
        ${rating > 0 ? `<button type="button" class="star-clear-btn" data-action="clear-rating">clear</button>` : `<button type="button" class="star-clear-btn" data-action="clear-rating" style="display:none">clear</button>`}
      </div>`;
    })() : "";

    const noteSection = `
    <div class="sheet-note-row">
      <label for="log-note" class="field-label">Note (optional)</label>
      <textarea id="log-note" class="text-input" rows="2" placeholder="Add a note…">${noteVal}</textarea>
    </div>`;

  const showWhen = opts.showWhen === true;
  const whenSection = showWhen ? (() => {
    const now = new Date();
    const pad = n => String(n).padStart(2, "0");
    let whenVal = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T${pad(now.getHours())}:${pad(now.getMinutes())}`;
    if (log?.completedAt) {
      const d = new Date(log.completedAt);
      whenVal = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    } else if (date && opts.slotHour != null) {
      whenVal = `${date}T${pad(opts.slotHour)}:00`;
    } else if (date) {
      whenVal = `${date}T${pad(now.getHours())}:${pad(now.getMinutes())}`;
    }
    return `<div class="sheet-time-row">
      <span class="field-label" style="white-space:nowrap;flex-shrink:0">When</span>
      <input type="datetime-local" id="log-when" class="sheet-time-input text-input" value="${whenVal}" step="300">
    </div>`;
  })() : "";

  const actions = log
    ? `<button type="button" class="btn btn-primary btn-full"
        data-action="save-log"
        data-log-id="${log.id}"
        data-chore-id="${chore.id}"
        data-date="${escapeHTML(date)}">
        Update
      </button>
      <button type="button" class="btn btn-danger btn-full mt-2"
        data-action="undo-chore"
        data-log-id="${log.id}">
        Remove log
      </button>`
    : `<button type="button" class="btn btn-primary btn-full"
        data-action="save-log"
        data-log-id=""
        data-chore-id="${chore.id}"
        data-date="${escapeHTML(date)}"
        data-slot-hour="${opts.slotHour ?? new Date().getHours()}"
        data-completed-at="${new Date().toISOString()}">
        Log
      </button>`;

  const removeScheduleBtn = !log && opts.scheduleId
    ? `<button type="button" class="btn btn-danger btn-full mt-2"
        data-action="delete-schedule"
        data-schedule-id="${opts.scheduleId}">
        Remove from schedule
      </button>`
    : "";

  return `
    <div class="bottom-sheet" role="dialog" aria-modal="true" aria-label="${log ? "Edit log" : "Log chore"}">
      <div class="sheet-handle" aria-hidden="true"></div>
      <div class="sheet-title-row">
        <h2 class="sheet-title">${title}</h2>
        <button type="button" class="sheet-manage-btn" data-action="chore-edit" data-chore-id="${chore.id}" aria-label="Manage ${escapeHTML(chore.name)}" title="Manage chore">${MANAGE_ICON}</button>
      </div>
      ${whenSection}
      ${indicatorSection}
      ${followUpSection}
      ${titleSection}
      ${ratingSection}
      ${memberSection}
      ${noteSection}
      ${actions}
      ${removeScheduleBtn}
      <button type="button" class="btn btn-ghost btn-full sheet-cancel-btn" data-action="close-sheet">
        Cancel
      </button>
    </div>`;
}

// ─── Render: quick-log bottom sheet (FAB) ─────────────────────────────────────

/**
 * Renders the quick-log sheet: pick a chore and add an optional note.
 *
 * @param {object[]} chores  All household chores
 * @param {string}   date    ISO date "YYYY-MM-DD"
 */
export function renderQuickLogSheet(chores, date) {
  const items = chores.length === 0
    ? `<p class="sheet-empty">No chores set up yet.</p>`
    : chores.map(c => `
        <button type="button"
          class="sheet-chore-item"
          draggable="true"
          data-action="quick-log-chore"
          data-chore-id="${c.id}"
          data-reorder-chore-id="${c.id}"
          data-date="${escapeHTML(date)}">
          <span class="drag-handle" aria-hidden="true">⠿</span>
          <span class="chore-icon">${escapeHTML(c.icon)}</span>
          <span class="chore-name">${escapeHTML(c.name)}</span>
        </button>`).join("");

  return `
    <div class="bottom-sheet" role="dialog" aria-modal="true" aria-label="Quick Log">
      <div class="sheet-handle" aria-hidden="true"></div>
      <h2 class="sheet-title">Log a chore</h2>
      <p class="sheet-hint">Tap to log instantly · Hold to add notes</p>
      <div class="sheet-note-row">
        <label for="quick-log-note" class="field-label">Note (optional)</label>
        <textarea id="quick-log-note" class="text-input" rows="2" placeholder="Add a note…"></textarea>
      </div>
      <div class="sheet-chore-list">${items}</div>
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
          value="${sch?.specificTime || ""}" step="300" />
      </div>

      <button type="button" class="btn btn-primary btn-full" data-action="save-recurrence">
        Save Schedule
      </button>
    </div>`;
}

export function renderVolumeSelect(selectedML = null) {
  const options = Array.from({ length: 41 }, (_, i) => i * 5);
  const optsHTML = options.map(v => {
    const sel = selectedML === v ? " selected" : "";
    return `<option value="${v}"${sel}>${v} mL</option>`;
  }).join("");
  return `<div class="sheet-volume-row">
    <label for="log-volume" class="field-label">Volume</label>
    <select id="log-volume" class="select-input volume-select">
      <option value=""${selectedML == null ? " selected" : ""}>--</option>
      ${optsHTML}
    </select>
  </div>`;
}

export function renderMemberSelect(members, currentUserId, selectedUserId = null, prefix = "log") {
  if (!members || members.length <= 1) return "";
  const selected = selectedUserId ?? currentUserId ?? "";
  const options = members.map(m =>
    `<option value="${m.userId}" ${m.userId === selected ? "selected" : ""}>${escapeHTML(m.displayName || m.email)}</option>`
  ).join("");
  return `<div class="sheet-member-row">
    <label for="${prefix}-member" class="field-label">Done by</label>
    <select id="${prefix}-member" class="select-input member-select">
      ${options}
    </select>
  </div>`;
}
