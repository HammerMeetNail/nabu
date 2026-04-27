import { getCSRFToken } from "./api.js";

function apiFetch(path, options = {}) {
  const headers = new Headers(options.headers || {});
  headers.set("Content-Type", "application/json");
  const csrfToken = getCSRFToken();
  if (csrfToken) headers.set("X-CSRF-Token", csrfToken);
  if (!options.method) options.method = "GET";
  return fetch(path, { ...options, headers }).then(r => r.json());
}

function todayISO(offset) {
  const d = new Date();
  d.setDate(d.getDate() + (offset || 0));
  return d.toISOString().split("T")[0];
}

function fmtDate(iso) {
  const d = new Date(iso + "T00:00:00");
  return d.toLocaleDateString("en-US", { weekday: "long", month: "long", day: "numeric" });
}

export async function loadToday(date) {
  const d = date || todayISO(0);
  return apiFetch(`/api/logs/today?date=${d}`);
}

export async function loadWeek(start) {
  return apiFetch(`/api/logs/week?start=${start}`);
}

export async function logChore(choreId, note) {
  return apiFetch("/api/logs", {
    method: "POST",
    body: JSON.stringify({ choreId, note }),
  });
}

export async function undoLog(logId) {
  return apiFetch(`/api/logs/${logId}`, { method: "DELETE" });
}

export async function loadChores() {
  return apiFetch("/api/chores");
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

  const prev = todayISO(-1);
  const next = todayISO(1);

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
  return `<div class="history-view">
    <h2>History</h2>
    <p class="text-secondary">Completed chores will appear here.</p>
  </div>`;
}

function escapeHTML(str) {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}
