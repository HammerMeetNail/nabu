import { apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";
import { loadSchedulesForDate } from "./schedule.js";

function formatLocalISODate(d) {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function todayISO(offset) {
  const d = new Date();
  d.setDate(d.getDate() + (offset || 0));
  return formatLocalISODate(d);
}

function shiftDate(iso, offset) {
  const d = new Date(iso + "T00:00:00");
  d.setDate(d.getDate() + offset);
  return formatLocalISODate(d);
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
  const { data } = await apiFetch("/api/logs/history");
  return data;
}

export async function loadMoreHistory(before) {
  const { data } = await apiFetch(`/api/logs/history?before=${before}`);
  return data;
}

export async function logChore(choreId, note, date = "", indicators = [], slotHour = null, completedAt = null, volumeML = null, userId = null) {
  const body = { choreId, note, indicators };
  if (date) body.date = date;
  if (slotHour !== null) body.hour = slotHour;
  if (completedAt) body.completedAt = completedAt;
  if (volumeML !== null) body.volumeML = volumeML;
  if (userId !== null) body.userId = userId;
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

export async function updateLog(logId, note, indicators = [], volumeML = null, userId = null) {
  const body = { note, indicators };
  if (volumeML !== null) body.volumeML = volumeML;
  if (userId !== null) body.userId = userId;
  const { data } = await apiFetch(`/api/logs/${logId}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
  return data;
}

export async function loadChores() {
  const { data } = await apiFetch("/api/chores");
  return data;
}

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
      <p class="text-secondary">No completed chores yet.</p>
    </div>`;
  }
  const members = state.members || [];
  const memberMap = {};
  members.forEach(m => { memberMap[m.userId] = m.displayName || m.email; });

  const pad = n => String(n).padStart(2, '0');

  // Group by day
  const dayGroups = [];
  let currentDate = '';
  for (const l of logs) {
    const d = l.completedAt ? new Date(l.completedAt) : null;
    if (!d) continue;
    const dateKey = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    const timeStr = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
    const dayLabel = d.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });

    if (dateKey !== currentDate) {
      currentDate = dateKey;
      dayGroups.push({ date: dateKey, label: dayLabel, rows: [] });
    }
    const chore = (state.chores || []).find(c => c.id === l.choreId);
    dayGroups[dayGroups.length - 1].rows.push({
      icon: chore?.icon || '',
      name: chore?.name || `Chore #${l.choreId}`,
      color: chore?.color || '#999',
      who: memberMap[l.userId] || 'Someone',
      time: timeStr,
      note: l.note || '',
      volumeML: l.volumeML,
      logId: l.id,
      choreId: l.choreId,
      date: dateKey,
    });
  }

  // Group day groups into 7-day chunk groups
  // Logs are newest-first.  Each chunk: [chunkStart, chunkStart+7).
  // We want day groups ordered newest-to-oldest, so when we go from
  // newest to oldest, the first day group starts a new chunk, and
  // subsequent day groups belong to that chunk until we cross a
  // 7-day boundary.
  const chunked = [];
  if (dayGroups.length > 0) {
    let chunkDays = [];
    const msPerDay = 86400000;
    // Start of the first chunk: truncate the first day's date to the
    // start of its 7-day window (same as backend calculation).
    const firstDate = new Date(dayGroups[0].date + "T00:00:00");
    // Align to the same window boundary used by the server:
    // end = min(before, tomorrow), start = end - 7.
    // For rendering, we use 7-day segments anchored from the first day.
    // Walk the day groups and wrap every 7 days.
    let chunkIdx = 0;
    for (const dg of dayGroups) {
      const d = new Date(dg.date + "T00:00:00");
      const daysSinceFirst = Math.round((firstDate - d) / msPerDay);
      const newChunkIdx = Math.floor(daysSinceFirst / 7);
      if (newChunkIdx !== chunkIdx) {
        const chunkStart = new Date(firstDate);
        chunkStart.setDate(chunkStart.getDate() - chunkIdx * 7);
        const chunkEnd = new Date(chunkStart);
        chunkEnd.setDate(chunkEnd.getDate() + 6);
        chunked.push({
          label: fmtChunkRange(chunkStart, chunkEnd),
          days: chunkDays,
        });
        chunkDays = [];
        chunkIdx = newChunkIdx;
      }
      chunkDays.push(dg);
    }
    // Flush last chunk
    const chunkStart = new Date(firstDate);
    chunkStart.setDate(chunkStart.getDate() - chunkIdx * 7);
    const chunkEnd = new Date(chunkStart);
    chunkEnd.setDate(chunkEnd.getDate() + 6);
    chunked.push({
      label: fmtChunkRange(chunkStart, chunkEnd),
      days: chunkDays,
    });
  }

  const html = chunked.map(chunk => {
    const days = chunk.days.map(g => {
      const rows = g.rows.map(r => {
        const volumeStr = r.volumeML != null ? ` · ${r.volumeML}mL` : '';
        return `
        <button type="button" class="hist-row" style="--chore-color:${r.color}"
          data-action="view-log"
          data-chore-id="${r.choreId}"
          data-log-id="${r.logId}"
          data-date="${r.date}">
          <span class="hist-icon">${r.icon}</span>
          <div class="hist-body">
            <span class="hist-name">${escapeHTML(r.name)}</span>
            <span class="hist-meta">${r.time} · ${escapeHTML(r.who)}${r.note ? ` · ${escapeHTML(r.note)}` : ''}${volumeStr}</span>
          </div>
        </button>`;
      }).join('');
      return `<div class="hist-date-header">${g.label}</div>${rows}`;
    }).join('');
    return `<div class="hist-chunk">
      <div class="hist-chunk-header">${chunk.label}</div>
      ${days}
    </div>`;
  }).join('');

  const loadMore = state.historyHasMore
    ? `<div class="load-more-wrap"><button type="button" class="btn btn-secondary load-more-btn" data-action="load-more-history">Load more</button></div>`
    : '';

  return `<div class="history-view">
    <h2>History</h2>
    ${html}
    ${loadMore}
  </div>`;
}

function fmtChunkRange(start, end) {
  const opts = { month: 'short', day: 'numeric' };
  return `${start.toLocaleDateString('en-US', opts)} - ${end.toLocaleDateString('en-US', opts)}`;
}

