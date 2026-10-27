import { apiFetch } from "./api.js";
import { escapeHTML, renderAccessSuspendedCard, localDateStr, shiftDateStr } from "./utils.js";
import { sortChoresByOrder } from "./preferences.js";
import { renderMemberSelect } from "./schedule.js";
import { renderChoresView as renderChoresViewList } from "./chores.js";
import { historyMemberNames, historyRow, renderHistoryRow } from "./today.js";

/**
 * Fetch the most-recent log for each chore in the household.
 * Returns { latestLogs: { [choreId]: ChoreLog } }
 */
export async function loadLatestLogs() {
  const { data } = await apiFetch("/api/logs/latest-per-chore");
  return data;
}

/**
 * Format an ISO timestamp as a human-readable "X ago" string.
 * @param {string} iso  ISO 8601 string
 * @returns {string}
 */
export function formatTimeAgo(iso) {
  if (!iso) return "";
  const diffSec = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (diffSec < 60) return "just now";
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  const days = Math.floor(diffSec / 86400);
  if (days === 1) return "yesterday";
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  if (months === 1) return "1mo ago";
  if (months < 12) return `${months}mo ago`;
  const years = Math.floor(months / 12);
  return years === 1 ? "1yr ago" : `${years}yr ago`;
}

/**
 * Update the "X ago" labels on the home grid in place, without a full
 * re-render (which could disrupt an open sheet or scroll). Called on a timer
 * while the home tab is visible so "3h ago" ticks over to "4h ago" without a
 * navigation.
 * @param {object} state  App state
 */
export function refreshHomeCardTimes(state) {
  const latestLogs = state.latestLogs || {};
  document.querySelectorAll(".home-chore-card[data-home-chore-id]").forEach(card => {
    const id = parseInt(card.dataset.homeChoreId, 10);
    const span = card.querySelector(".home-card-time");
    if (!span) return;
    const latest = latestLogs[id];
    if (latest && latest.completedAt) {
      span.textContent = formatTimeAgo(latest.completedAt);
      span.classList.remove("home-card-time--never");
    }
  });
}

/**
 * Render the home header bar with Log / Manage toggle.
 * @param {object} state  App state
 * @returns {string}  HTML string
 */
export function renderHomeHeader(state) {
  const isManage = state.homeView === "manage";
  return `<div class="home-header">
    <div class="home-header-tabs">
      <button type="button" class="home-header-tab${isManage ? "" : " home-header-tab--active"}" data-action="switch-home-view" data-view="log">Log</button>
      <button type="button" class="home-header-tab${isManage ? " home-header-tab--active" : ""}" data-action="switch-home-view" data-view="manage">Manage</button>
    </div>
  </div>`;
}

/**
 * Render the home grid view (iPhone home-screen style chore cards). Keys 1–9
 * log the first nine tiles; keyHints shows each one's number on its tile.
 * @param {object} state  App state
 * @param {{keyHints?: boolean}} [options]
 * @returns {string}  HTML string
 */
export function renderHomeView(state, { keyHints = false } = {}) {
  const hiddenSet = new Set(state.hiddenHomeChoreIDs || []);
  const chores = sortChoresByOrder(state.chores || [], state.choreOrder || [])
    .filter(c => !hiddenSet.has(c.id));
  const latestLogs = state.latestLogs || {};
  const jiggleMode = state.jiggleMode || false;

  if (!state.household && state.user) {
    if (state.accessSuspended) return renderAccessSuspendedCard(state);
    return `<div class="card mt-3"><h2>Welcome!</h2>
      <p>Hi ${escapeHTML(state.user.email || '')}! Set up your household to get started.</p>
      <a class="btn btn-primary mt-2" href="#" data-nav="settings">Set Up Household</a></div>`;
  }

  if (chores.length === 0) {
    return `<div class="home-view">
      <div class="empty-state">
        <div class="empty-state-icon">🏠</div>
        <div class="empty-state-title">No chores set up yet</div>
        <p><button type="button" class="btn btn-primary btn-sm" data-action="switch-home-view" data-view="manage">Add chores</button></p>
      </div>
  </div>`;
}

  const cards = chores.map((chore, index) => {
    const latest = latestLogs[chore.id];
    const timeAgo = latest ? formatTimeAgo(latest.completedAt) : "";
    const timeHTML = timeAgo
      ? `<span class="home-card-time">${escapeHTML(timeAgo)}</span>`
      : `<span class="home-card-time home-card-time--never">never</span>`;
    if (jiggleMode) {
      // Wrap in a div so the X badge and card button are siblings — nesting a
      // <button> inside a <button> is invalid HTML and causes browsers to eject
      // the inner content, producing empty cards.
      return `<div class="home-card-wrapper" draggable="true" data-home-reorder-chore-id="${chore.id}">
      <button type="button" class="home-card-remove" data-action="home-remove-chore" data-chore-id="${chore.id}" aria-label="Remove ${escapeHTML(chore.name)} from home">&#x2715;</button>
      <button type="button" class="home-chore-card home-chore-card--jiggle" data-home-chore-id="${chore.id}" style="--chore-color:${chore.color}">
        <span class="home-card-icon">${escapeHTML(chore.icon)}</span>
        <span class="home-card-name">${escapeHTML(chore.name)}</span>
        ${timeHTML}
      </button>
    </div>`;
    }
    const key = index < 9 ? String(index + 1) : "";
    return `<button type="button"
      class="home-chore-card"
      data-home-chore-id="${chore.id}"
      data-action="home-tap-chore"${key ? ` aria-keyshortcuts="${key}"` : ""}
      style="--chore-color:${chore.color}">
      ${key && keyHints ? `<span class="home-card-key" aria-hidden="true">${key}</span>` : ""}
      <span class="home-card-icon">${escapeHTML(chore.icon)}</span>
      <span class="home-card-name">${escapeHTML(chore.name)}</span>
      ${timeHTML}
    </button>`;
  }).join("");

  const jiggleBar = jiggleMode
    ? `<div class="home-jiggle-bar">
        <button type="button" class="btn btn-primary btn-sm" data-action="exit-jiggle-mode">Done</button>
      </div>`
    : "";

  return `<div class="home-view">
    ${jiggleBar}
    <div class="home-grid${jiggleMode ? " home-grid--jiggle" : ""}">
      ${cards}
    </div>
  </div>`;
}

// The most rows Home's activity column shows; the Activity tab has the rest.
const HOME_ACTIVITY_ROWS = 25;

/**
 * Render Home's activity column for the wide layout: the household's newest
 * logs, with this device's unsent ones marked pending, under day headings.
 * Each row opens its log, as on the Activity tab.
 * @param {object} state  App state
 * @returns {string}  HTML string
 */
export function renderHomeActivity(state) {
  const activity = state.homeActivity || {};
  const memberMap = historyMemberNames(state);
  const rows = [...(state.pendingLogs || []), ...(activity.logs || [])]
    .filter(log => log.completedAt)
    .sort((a, b) => new Date(b.completedAt) - new Date(a.completedAt))
    .slice(0, HOME_ACTIVITY_ROWS)
    .map(log => historyRow(state, log, memberMap))
    .filter(Boolean);
  const today = localDateStr(new Date());
  const dayNames = { [today]: "Today", [shiftDateStr(today, -1)]: "Yesterday" };
  let body = "";
  let day = "";
  for (const row of rows) {
    if (row.date !== day) {
      day = row.date;
      body += `<h3 class="hist-date-header">${escapeHTML(dayNames[day] || row.dayLabel)}</h3>`;
    }
    body += renderHistoryRow(row);
  }
  if (!body) {
    body = activity.error
      ? `<p class="home-activity-empty" role="status">${escapeHTML(activity.error)} <button type="button" class="btn btn-sm" data-action="retry-home-activity">Retry</button></p>`
      : `<p class="home-activity-empty">${activity.loaded ? "Nothing logged in the last 7 days." : "Loading…"}</p>`;
  }
  return `<aside class="home-activity" aria-labelledby="home-activity-title">
    <div class="home-activity-header">
      <h2 id="home-activity-title">Recent activity</h2>
      <a href="/activity" data-nav="activity">See all</a>
    </div>
    ${body}
  </aside>`;
}

/**
 * Render the confirmation bottom sheet for removing a chore from the home grid.
 * @param {object} chore
 * @returns {string}  HTML string
 */
/**
 * Render the chores management view for the Home tab.
 * @param {object} state  App state
 * @returns {string}  HTML string
 */
export function renderHomeManageView(state) {
  return renderChoresViewList(state);
}

export function renderConfirmRemoveFromHomeSheet(chore) {
  return `<div class="bottom-sheet">
    <div class="sheet-handle"></div>
    <div class="sheet-title">Remove from Home?</div>
    <p class="confirm-remove-msg">${escapeHTML(chore.icon)} <strong>${escapeHTML(chore.name)}</strong> will still be available in Manage Chores.</p>
    <button type="button" class="btn btn-danger btn-block"
      data-action="confirm-remove-home-chore"
      data-chore-id="${chore.id}">Remove from Home</button>
    <button type="button" class="btn btn-secondary btn-block mt-2"
      data-action="close-sheet">Cancel</button>
  </div>`;
}
