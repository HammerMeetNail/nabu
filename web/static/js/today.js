import { apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";
import { loadSchedulesForDate } from "./schedule.js";


export function todayISO(offset) {
  const d = new Date();
  d.setDate(d.getDate() + (offset || 0));
  return d.toISOString().split("T")[0];
}

function shiftDate(iso, offset) {
  const d = new Date(iso + "T00:00:00");
  d.setDate(d.getDate() + offset);
  return d.toISOString().split("T")[0];
}

function fmtDate(iso) {
  const d = new Date(iso + "T00:00:00");
  return d.toLocaleDateString("en-US", { weekday: "long", month: "long", day: "numeric" });
}

export async function loadToday(date) {
  const d = date || todayISO(0);
  const { data } = await apiFetch(`/api/logs/today?date=${d}`);
  return data;
}

export async function loadWeek(start) {
  const { data } = await apiFetch(`/api/logs/week?start=${start}`);
  return data;
}

export async function loadHistory() {
  const start = todayISO(0);
  const d = new Date(start + "T00:00:00");
  d.setDate(d.getDate() - d.getDay() + (d.getDay() === 0 ? -6 : 1));
  const weekStart = d.toISOString().split("T")[0];
  const { data } = await apiFetch(`/api/logs/week?start=${weekStart}`);
  return data;
}

export async function logChore(choreId, note, date = "", indicators = [], slotHour = null, completedAt = null) {
  const body = { choreId, note, indicators };
  if (date) body.date = date;
  if (slotHour !== null) body.hour = slotHour;
  if (completedAt) body.completedAt = completedAt;
  const { data } = await apiFetch("/api/logs", {
    method: "POST",
    body: JSON.stringify(body),
  });
  return data;
}

export async function undoLog(logId) {
  const { data } = await apiFetch(`/api/logs/${logId}`, { method: "DELETE" });
  return data;
}

export async function updateLog(logId, note, indicators = []) {
  const { data } = await apiFetch(`/api/logs/${logId}`, {
    method: "PATCH",
    body: JSON.stringify({ note, indicators }),
  });
  return data;
}

export async function loadChores() {
  const { data } = await apiFetch("/api/chores");
  return data;
}

/**
 * Loads today's logs AND today's active schedules in parallel.
 * Returns a merged object suitable for passing into renderDayView.
 */
export async function loadTodayWithSchedules(state) {
  const date = state.calendarDate || state.todayDate || todayISO(0);
  const [todayData, schedules] = await Promise.all([
    loadToday(date),
    loadSchedulesForDate(date),
  ]);
  return { ...todayData, schedules };
}

export function renderTodayView(state) {
  const date = state.todayDate || todayISO(0);
  const logs = state.todayLogs || [];
  const chores = state.chores || [];
  const loggedChoreIDs = new Set(logs.map(l => l.choreId));
  const done = loggedChoreIDs.size;
  const total = chores.length;

  const logMap = {};
  logs.forEach(l => { logMap[l.choreId] = l; });

  const choreCards = chores.map(chore => {
    const log = logMap[chore.id];
    const doneClass = log ? "chore-done" : "";
    const style = `border-left: 4px solid ${chore.color}`;
    const check = log ? '<span class="check-overlay">✓</span>' : '';
    const note = log && log.note ? `<span class="chore-note">${escapeHTML(log.note)}</span>` : '';
    return `<button type="button" class="chore-card ${doneClass}" data-action="${log ? 'undo-chore' : 'log-chore'}" data-chore-id="${chore.id}" data-log-id="${log ? log.id : ''}" style="${style}">
      <span class="chore-icon">${chore.icon}</span>
      <span class="chore-name">${escapeHTML(chore.name)}</span>
      <span class="chore-category">${chore.category}</span>
      ${check}${note}
    </button>`;
  }).join("");

  const prev = shiftDate(date, -1);
  const next = shiftDate(date, 1);

  return `<div class="today-view">
    <div class="date-nav">
      <button type="button" class="btn btn-icon btn-ghost" data-action="navigate-day" data-date="${prev}">←</button>
      <h2 class="today-date">${fmtDate(date)}</h2>
      <button type="button" class="btn btn-icon btn-ghost" data-action="navigate-day" data-date="${next}">→</button>
    </div>
    <div class="progress-bar mb-3">
      <div class="progress-fill" style="width:${total ? (done / total) * 100 : 0}%"></div>
    </div>
    <p class="text-center text-secondary mb-3">${done} of ${total} chores done</p>
    <div class="chore-grid">${choreCards}</div>
    ${chores.length === 0 ? '<div class="empty-state"><div class="empty-state-icon">📋</div><p>No chores set up yet. <a href="#" data-nav="chores">Add chores</a> to get started.</p></div>' : ''}
  </div>`;
}

export function renderHistoryView(state) {
  const logs = state.historyLogs || [];
  if (logs.length === 0) {
    return `<div class="history-view">
      <h2>History</h2>
      <p class="text-secondary">No completed chores yet this week.</p>
    </div>`;
  }
  const items = logs.map(l => {
    const chore = (state.chores || []).find(c => c.id === l.choreId);
    const choreName = chore ? `${chore.icon} ${escapeHTML(chore.name)}` : `Chore #${l.choreId}`;
    return `<li class="member-item">
    <span>${l.completedAt ? new Date(l.completedAt).toLocaleDateString("en-US", { weekday: "short", month: "short", day: "numeric" }) : ''}</span>
    <span>${choreName}</span>
    ${l.note ? `<span class="text-secondary">${escapeHTML(l.note)}</span>` : ''}
  </li>`;
  }).join("");
  return `<div class="history-view">
    <h2>History</h2>
    <ul class="member-list">${items}</ul>
  </div>`;
}


