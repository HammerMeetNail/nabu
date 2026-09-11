import { apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";
import { formatAmount } from "./metrics.js";
import { loadSchedulesForDate } from "./schedule.js";
import { submitLog } from "./offline-queue.js";
import { contextSnapshot, sameOrigin, ContextChangedError } from "./browser-context.js";
import { newKey } from "./device-store.js";

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
  return d.toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric" });
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

export async function loadHistory(query = "") {
  const q = (query || "").trim();
  const url = q ? `/api/logs/history?q=${encodeURIComponent(q)}` : "/api/logs/history";
  const { data } = await apiFetch(url);
  return data;
}

export async function loadMoreHistory(before) {
  const { data } = await apiFetch(`/api/logs/history?before=${before}`);
  return data;
}

export async function logChore(choreId, note, date = "", indicators = [], slotHour = null, completedAt = null, volumeML = null, userId = null, indicatorVolumes = {}, followUpMinutes = 0, followUpTime = null, rating = null, title = null, durationSeconds = null, subject = null, { submission = {} } = {}) {
  if (submission.promise) return submission.promise;
  const body = { choreId, note, indicators };
  if (Object.keys(indicatorVolumes).length > 0) body.indicatorVolumes = indicatorVolumes;
  if (date) body.date = date;
  if (slotHour !== null) body.hour = slotHour;
  if (completedAt) body.completedAt = completedAt;
  if (volumeML !== null) body.volumeML = volumeML;
  if (userId !== null) body.userId = userId;
  if (followUpMinutes > 0) body.followUpMinutes = followUpMinutes;
  if (followUpTime) body.followUpTime = followUpTime;
  if (rating !== null) body.rating = rating;
  if (title) body.title = title;
  if (durationSeconds !== null) body.durationSeconds = durationSeconds;
  if (subject !== null) body.subject = subject;
  if (!submission.body) {
    if (!body.completedAt) {
      const when = date ? (slotHour !== null ? new Date(`${date}T${String(slotHour).padStart(2,"0")}:00:00`) : new Date(`${date}T12:00:00Z`)) : new Date();
      body.completedAt = when.toISOString();
    }
    body.idempotencyKey = submission.idempotencyKey || newKey();
    submission.idempotencyKey = body.idempotencyKey;
    submission.body = JSON.parse(JSON.stringify(body));
    submission.origin = contextSnapshot();
  }
  const active = contextSnapshot();
  if (submission.origin && !sameOrigin(submission.origin, active)) throw new ContextChangedError();
  submission.origin = active;
  submission.promise = submitLog(submission.body, apiFetch, submission.origin)
    .finally(() => { delete submission.promise; });
  return submission.promise;
}

export async function undoLog(logId) {
  const { response, data } = await apiFetch(`/api/logs/${logId}`, { method: "DELETE" });
  if (!response.ok) throw new Error(data?.error || `Delete failed (${response.status})`);
  return data;
}

export async function updateLog(logId, patch) {
  const { response, data } = await apiFetch(`/api/logs/${logId}`, { method:"PATCH", body:JSON.stringify(patch) });
  if (!response.ok) throw new Error(data?.error || `Update failed (${response.status})`);
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
    const rating = log && log.rating != null ? `<span class="chore-note">${renderStarRatingDisplay(log.rating)}</span>` : '';
    const title = log && log.title ? `<span class="chore-note">${escapeHTML(log.title)}</span>` : '';
    return `<button type="button" class="chore-card ${doneClass}" data-action="${log ? 'undo-chore' : 'log-chore'}" data-chore-id="${chore.id}" data-log-id="${log ? log.id : ''}" style="${style}">
      <span class="chore-icon">${escapeHTML(chore.icon)}</span>
      <span class="chore-name">${escapeHTML(chore.name)}</span>
      <span class="chore-category">${escapeHTML(chore.category)}</span>
      ${check}${title}${note}${rating}
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
    ${chores.length === 0 ? '<div class="empty-state"><div class="empty-state-icon">📋</div><p>No chores set up yet. <button type="button" class="btn btn-sm btn-primary" data-action="switch-home-view" data-view="manage">Add chores</button></p></div>' : ''}
  </div>`;
}

export function renderHistoryFilter(state) {
  const filter = state.historyChoreFilter;
  const chores = state.chores || [];
  const sorted = [...chores].sort((a, b) =>
    (a.name || '').localeCompare(b.name || '', undefined, { sensitivity: 'base' }));
  const open = state.historyFilterOpen;

  // Empty/null filter = show everything, with nothing highlighted. Selecting
  // chores narrows the view to only those chores (additive).
  const hasFilter = Array.isArray(filter) && filter.length > 0;
  const allActive = !hasFilter;
  let html = '<div class="hist-filter-fab">';
  html += `<div class="hist-filter-chips${open ? ' hist-filter-chips--open' : ''}">`;
  html += `<button type="button" class="hist-filter-chip hist-filter-all${allActive ? ' active' : ''}" data-action="history-filter-all">All activity</button>`;
  for (const c of sorted) {
    const active = hasFilter && filter.includes(c.id);
    html += `<button type="button" class="hist-filter-chip${active ? ' active' : ''}" data-action="history-filter-chore" data-chore-id="${c.id}" style="--chore-color:${c.color}">
      <span class="hist-filter-chip-icon">${escapeHTML(c.icon)}</span>
      <span class="hist-filter-chip-name">${escapeHTML(c.name)}</span>
    </button>`;
  }
  html += '</div>';
  html += `<button type="button" class="hist-filter-btn${open ? ' hist-filter-btn--open' : ''}" data-action="toggle-history-filter" aria-expanded="${open ? 'true' : 'false'}" aria-label="Filter chores">`;
  html += '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"></polygon></svg>';
  html += '</button>';
  html += '</div>';
  return html;
}

export function renderHistorySearchBar(state) {
  const q = state.historySearch || "";
  return `<div class="hist-search">
    <input type="search" id="history-search-input" class="hist-search-input"
      placeholder="Search notes &amp; titles…" value="${escapeHTML(q)}"
      data-action="history-search" aria-label="Search activity" autocomplete="off">
  </div>`;
}

export function renderHistoryView(state) {
  // Merge any offline-queued logs (Phase 2.1) so they show inline with a
  // "pending" badge until they sync. They carry _pending; search results skip
  // them (they aren't on the server yet).
  const searchingNow = !!(state.historySearch && state.historySearch.trim());
  const pending = (!searchingNow ? (state.pendingLogs || []) : []);
  const logs = [...pending, ...(state.historyLogs || [])];
  const chores = state.chores || [];
  const filter = state.historyChoreFilter;
  const searching = !!(state.historySearch && state.historySearch.trim());
  // Chore chips filter the loaded (windowed) pages; they don't apply to a
  // flat text search, so hide the filter FAB while searching.
  const filterFab = (chores.length > 0 && !searching) ? renderHistoryFilter(state) : '';
  const status = state.historyError ? `<p class="form-error" role="status">${escapeHTML(state.historyError)} <button class="btn btn-sm" data-action="retry-activity">Retry</button></p>`
    : state.historyLoading ? '<p role="status" class="text-secondary">Loading activity…</p>' : '';
  const searchBar = renderHistorySearchBar(state) + status;
  const loadMore = state.historyHasMore && !searching
    ? `<div class="load-more-wrap"><button type="button" class="btn btn-secondary load-more-btn" data-action="load-more-history"${state._historyLoadingMore ? ' disabled' : ''}>${state._historyLoadingMore ? 'Loading…' : 'Load more'}</button></div>`
    : '';


  if (logs.length === 0) {
    if (state.historyLoading || state.historyError) return `<div class="history-view">${searchBar}${filterFab}</div>`;
    const emptyMsg = searching
      ? '<p class="text-secondary">No activity matches your search.</p>'
      : state.historyHasMore
        ? '<p class="text-secondary">No activity in this time range. Load more to look further back.</p>'
        : '<p class="text-secondary">No completed chores yet.</p>';
    return `<div class="history-view">
      ${searchBar}
      ${emptyMsg}
      ${loadMore}
      ${filterFab}
    </div>`;
  }
  const members = [...(state.members || []), ...(state.historicalMembers || [])];
  const memberMap = {};
  members.forEach(m => { memberMap[m.userId] = m.displayName || m.email; });
  const volumeUnit = state.volumeUnit === "oz" ? "oz" : "ml";

  const pad = n => String(n).padStart(2, '0');

  // Group by day
  const rawDayGroups = [];
  let currentDate = '';
  for (const l of logs) {
    const d = l.completedAt ? new Date(l.completedAt) : null;
    if (!d) continue;
    const dateKey = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    const h = d.getHours();
    const ampm = h >= 12 ? 'PM' : 'AM';
    const h12 = h % 12 || 12;
    const timeStr = `${h12}:${pad(d.getMinutes())} ${ampm}`;
    const dayLabel = d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' });

    if (dateKey !== currentDate) {
      currentDate = dateKey;
      rawDayGroups.push({ date: dateKey, label: dayLabel, rows: [] });
    }
    const chore = (state.chores || []).find(c => c.id === l.choreId);
    const indicatorVolumes = l.indicatorVolumes || {};
    const volKeys = new Set(Object.keys(indicatorVolumes));
    const indicatorIcons = (l.indicators || [])
      .filter(label => !volKeys.has(label))
      .map(label => escapeHTML(label.split(' ')[0]));
    rawDayGroups[rawDayGroups.length - 1].rows.push({
      chore: chore || { hasVolumeML: true },
      icon: chore?.icon || '',
      name: chore?.name || `Chore #${l.choreId}`,
      color: chore?.color || '#999',
      who: memberMap[l.userId] || 'Someone',
      time: timeStr,
      note: l.note || '',
      volumeML: l.volumeML,
      indicatorVolumes,
      indicatorIcons,
      rating: l.rating,
      title: l.title || '',
      subject: l.subject || '',
      pending: l._pending || false,
      logId: l.id,
      choreId: l.choreId,
      date: dateKey,
    });
  }

  // Apply chore filter. Empty/null = show everything.
  const hasFilter = Array.isArray(filter) && filter.length > 0;
  const dayGroups = hasFilter
    ? rawDayGroups.map(g => ({
        date: g.date,
        label: g.label,
        rows: g.rows.filter(r => filter.includes(r.choreId)),
      })).filter(g => g.rows.length > 0)
    : rawDayGroups;


  if (dayGroups.length === 0) {
    // Nothing on the loaded pages matches. If more pages exist, the match may
    // simply be further back in time — keep the Load more button available.
    let emptyMsg;
    if (!hasFilter) {
      emptyMsg = '<p class="text-secondary">No completed chores yet.</p>';
    } else if (state.historyHasMore) {
      emptyMsg = '<p class="text-secondary">No matching activity in this time range. Load more to look further back.</p>';
    } else {
      emptyMsg = '<p class="text-secondary">No activity matches the selected chores.</p>';
    }
    return `<div class="history-view">
      ${searchBar}
      ${filterFab}
      ${emptyMsg}
      ${loadMore}
    </div>`;
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
        const indicatorVolParts = Object.entries(r.indicatorVolumes || {}).map(([label, ml]) => {
          const icon = escapeHTML(label.split(' ')[0]);
          return `${icon} ${escapeHTML(formatAmount(ml, r.chore, volumeUnit))}`;
        });
        const indicatorVolStr = indicatorVolParts.length > 0 ? ` · ${indicatorVolParts.join(' ')}` : '';
        const legacyVolumeStr = !indicatorVolParts.length && r.volumeML != null ? ` · ${escapeHTML(formatAmount(r.volumeML, r.chore, volumeUnit))}` : '';
        const indicatorIconsStr = r.indicatorIcons.length ? ` · ${r.indicatorIcons.join(' ')}` : '';
        const ratingStr = r.rating != null ? ` · ${renderStarRatingDisplay(r.rating)}` : '';
        const subjectStr = r.subject ? ` · <span class="hist-subject">${escapeHTML(r.subject)}</span>` : '';
        const titleStr = r.title ? `<span class="hist-title">${escapeHTML(r.title)}</span>` : '';
        const pendingBadge = r.pending ? `<span class="hist-pending">⏳ pending</span>` : '';
        // Pending rows aren't yet on the server, so they're not tappable.
        const pendingAttrs = r.pending ? `disabled aria-disabled="true"` : `data-action="view-log"`;
        return `
        <button type="button" class="hist-row${r.pending ? ' hist-row--pending' : ''}" style="--chore-color:${r.color}"
          ${pendingAttrs}
          data-chore-id="${r.choreId}"
          data-log-id="${r.logId}"
          data-date="${r.date}">
          <span class="hist-icon">${r.icon}</span>
          <div class="hist-body">
            <span class="hist-name">${escapeHTML(r.name)}${pendingBadge}</span>
            ${titleStr}
            <span class="hist-meta">${r.time} · ${escapeHTML(r.who)}${subjectStr}${r.note ? ` · ${escapeHTML(r.note)}` : ''}${indicatorVolStr}${legacyVolumeStr}${indicatorIconsStr}${ratingStr}</span>
          </div>
        </button>`;
      }).join('');
      const dayNote = (state.dayNotes || {})[g.date] || "";
      const noteHTML = dayNote
        ? `<button type="button" class="hist-day-note" data-action="edit-day-note" data-date="${g.date}">📝 ${escapeHTML(dayNote)}</button>`
        : `<button type="button" class="hist-day-note hist-day-note--empty" data-action="edit-day-note" data-date="${g.date}" aria-label="Add a note for ${escapeHTML(g.label)}">＋ note</button>`;
      return `<div class="hist-date-header">${g.label} <span class="hist-day-count">${g.rows.length}</span>${noteHTML}</div>${rows}`;
    }).join('');
    return `<div class="hist-chunk">
      <div class="hist-chunk-header">${chunk.label}</div>
      ${days}
    </div>`;
  }).join('');

  // Sentinel for infinite scroll: when it scrolls into view an
  // IntersectionObserver (app.js) auto-loads the next page. The Load more
  // button stays as an explicit fallback. Only present when more pages exist
  // and we're not in flat-search mode.
  const sentinel = (state.historyHasMore && !searching)
    ? '<div class="hist-sentinel" data-history-sentinel aria-hidden="true"></div>'
    : '';

  return `<div class="history-view">
    ${searchBar}
    ${html}
    ${sentinel}
    ${loadMore}
    ${filterFab}
  </div>`;
}

function renderStarRatingDisplay(rating) {
  const stars = rating / 10;
  // Mirror the half-star semantics from the rating input's aria-valuetext so
  // screen readers announce e.g. "4.5 out of 5 stars", not a bare "4.5 ⭐".
  return `<span aria-label="${stars} out of 5 stars">${stars} ⭐</span>`;
}

function fmtChunkRange(start, end) {
  const opts = { month: 'short', day: 'numeric' };
  return `${start.toLocaleDateString(undefined, opts)} - ${end.toLocaleDateString(undefined, opts)}`;
}
