import { apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";
import { sortChoresByOrder } from "./preferences.js";

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
 * Render the home grid view (iPhone home-screen style chore cards).
 * @param {object} state  App state
 * @returns {string}  HTML string
 */
export function renderHomeView(state) {
  const hiddenSet = new Set(state.hiddenHomeChoreIDs || []);
  const chores = sortChoresByOrder(state.chores || [], state.choreOrder || [])
    .filter(c => !hiddenSet.has(c.id));
  const latestLogs = state.latestLogs || {};
  const jiggleMode = state.jiggleMode || false;

  if (!state.household && state.user) {
    return `<div class="card mt-3"><h2>Welcome!</h2>
      <p>Hi ${escapeHTML(state.user.email || '')}! Set up your household to get started.</p>
      <a class="btn btn-primary mt-2" href="#" data-nav="settings">Set Up Household</a></div>`;
  }

  if (chores.length === 0) {
    return `<div class="home-view">
      <div class="empty-state">
        <div class="empty-state-icon">🏠</div>
        <div class="empty-state-title">No chores set up yet</div>
        <p>Add chores via the <a href="#" data-nav="chores">Chores</a> tab.</p>
      </div>
  </div>`;
}

export function renderVolumePicker(selectedML = null) {
  const options = Array.from({ length: 41 }, (_, i) => i * 5);
  const optsHTML = options.map(v => {
    const selected = selectedML === v ? " selected" : "";
    return `<option value="${v}"${selected}>${v} mL</option>`;
  }).join("");
  return `<div class="sheet-volume-row">
    <label for="log-volume" class="field-label">Volume</label>
    <select id="log-volume" class="select-input volume-select">
      <option value=""${selectedML == null ? " selected" : ""}>--</option>
      ${optsHTML}
    </select>
  </div>`;
}

  const cards = chores.map(chore => {
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
        <span class="home-card-icon">${chore.icon}</span>
        <span class="home-card-name">${escapeHTML(chore.name)}</span>
        ${timeHTML}
      </button>
    </div>`;
    }
    return `<button type="button"
      class="home-chore-card"
      data-home-chore-id="${chore.id}"
      data-action="home-tap-chore"
      style="--chore-color:${chore.color}">
      <span class="home-card-icon">${chore.icon}</span>
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

/**
 * Render the confirmation bottom sheet for removing a chore from the home grid.
 * @param {object} chore
 * @returns {string}  HTML string
 */
export function renderConfirmRemoveFromHomeSheet(chore) {
  return `<div class="bottom-sheet">
    <div class="sheet-handle"></div>
    <div class="sheet-title">Remove from Home?</div>
    <p class="confirm-remove-msg">${escapeHTML(chore.icon)} <strong>${escapeHTML(chore.name)}</strong> will still be available in the Chores tab.</p>
    <button type="button" class="btn btn-danger btn-block"
      data-action="confirm-remove-home-chore"
      data-chore-id="${chore.id}">Remove from Home</button>
    <button type="button" class="btn btn-secondary btn-block mt-2"
      data-action="close-sheet">Cancel</button>
  </div>`;
}

/**
 * Render the bottom sheet for logging a chore from the home grid.
 * Used for chores that have indicator labels (must choose before logging)
 * and for backdating (datetime-local input).
 * @param {object} chore
 * @returns {string}  HTML string
 */
export function renderHomeLogSheet(chore) {
  const labels = chore.indicatorLabels || [];
  const hasVolume = chore.hasVolumeML === true;

  const now = new Date();
  const pad = n => String(n).padStart(2, "0");
  const dtLocal = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T${pad(now.getHours())}:${pad(now.getMinutes())}`;

  const chipsHTML = labels.length > 0
    ? `<div class="sheet-chip-row">
        <span class="field-label">How did it go?</span>
        <div class="chip-list">
          ${labels.map(label =>
            `<button type="button" class="log-chip" data-action="toggle-indicator" data-label="${escapeHTML(label)}">${escapeHTML(label)}</button>`
          ).join("")}
        </div>
      </div>`
    : "";

  const volumeHTML = hasVolume
    ? renderVolumePicker()
    : "";

  return `<div class="bottom-sheet">
    <div class="sheet-handle"></div>
    <div class="sheet-title">${chore.icon} ${escapeHTML(chore.name)}</div>
    ${chipsHTML}
    ${volumeHTML}
    <div class="sheet-note-row">
      <span class="field-label">Note (optional)</span>
      <textarea id="home-log-note" class="text-input" rows="2" placeholder="Any notes..."></textarea>
    </div>
    <div class="sheet-time-row">
      <span class="field-label" style="white-space:nowrap;flex-shrink:0">When</span>
      <input type="datetime-local" id="home-log-when" class="sheet-time-input text-input" value="${dtLocal}">
    </div>
    <button type="button" class="btn btn-primary btn-block mt-3"
      data-action="save-home-log"
      data-chore-id="${chore.id}">
      Log
    </button>
    <button type="button" class="btn btn-ghost btn-full sheet-cancel-btn mt-1"
      data-action="close-sheet">
      Cancel
    </button>
  </div>`;
}

