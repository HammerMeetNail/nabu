import { escapeHTML } from "./utils.js";
import { sortChoresByOrder } from "./preferences.js";

// Preset color swatches for the chore editor (matches existing predefined chore colors + extras).
export const COLOR_SWATCHES = [
  "#F59E0B", "#EC4899", "#8B5CF6", "#10B981", "#6366F1", "#6B7280",
  "#3B82F6", "#06B6D4", "#F97316", "#EF4444", "#14B8A6", "#60A5FA",
  "#FB923C", "#1F2937", "#A78BFA", "#34D399", "#2E86AB", "#19323C",
];

// Common household/pet/chore emojis for the quick-pick grid.
const QUICK_EMOJIS = [
  "🐱","🐶","🐰","🐹","🐟","🐦","🌱","🌿","🌸","🌻",
  "🍽️","🧹","🗑️","🧺","👕","🛁","🛏️","🚿","💊","🧽",
  "🎃","🍼","👶","🛒","🧴","💡","🔧","📦","🥣","🌊",
  "🏠","⭐","❤️","✨","🧸","🎯","🔑","📋","🪴","🫙",
];

const EYE_OPEN_SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>`;
const EYE_CLOSED_SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94"/><path d="M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19"/><line x1="1" y1="1" x2="23" y2="23"/></svg>`;
const EDIT_SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>`;

/**
 * Render the full chores management tab.
 * Shows all chores (including hidden ones) sorted by user preference order,
 * with drag handles, visibility toggles, and edit buttons.
 */
export function renderChoresView(state) {
  if (!state.household) {
    return `<div class="chores-view">
      <div class="chores-view-header"><h2>Chores</h2></div>
      <div class="empty-state">
        <div class="empty-state-icon">🏠</div>
        <div class="empty-state-title">No household set up</div>
        <p>Set up your household to manage chores.</p>
        <a class="btn btn-primary mt-2" href="#" data-nav="settings">Set Up Household</a>
      </div>
    </div>`;
  }

  const sortedChores = sortChoresByOrder(state.chores || [], state.choreOrder || []);
  const hiddenSet = new Set(state.hiddenHomeChoreIDs || []);

  if (sortedChores.length === 0) {
    return `<div class="chores-view">
      <div class="chores-view-header"><h2>Chores</h2></div>
      <div class="empty-state">
        <div class="empty-state-icon">📋</div>
        <div class="empty-state-title">No chores yet</div>
        <p>Tap + to add your first chore.</p>
      </div>
      <button type="button" class="fab" data-action="chore-add" aria-label="Add chore">+</button>
    </div>`;
  }

  const rows = sortedChores.map(c => {
    const isHidden = hiddenSet.has(c.id);
    const eyeTitle = isHidden ? "Show on Home" : "Hide from Home";
    const isDefault = c.isPredefined;
    return `<div class="chore-row${isHidden ? ' chore-row--hidden' : ''}"
      data-chores-tab-reorder-id="${c.id}" draggable="true">
      <span class="chore-row-drag-handle" aria-hidden="true">⋮⋮</span>
      <span class="chore-row-icon" style="background:${escapeHTML(c.color)}">${c.icon}</span>
      <span class="chore-row-name">${escapeHTML(c.name)}</span>
      <span class="chore-row-badge${isDefault ? ' chore-row-badge--default' : ' chore-row-badge--custom'}">${isDefault ? 'Default' : 'Custom'}</span>
      <div class="chore-row-actions">
        <button type="button"
          class="chore-row-btn chore-row-eye${isHidden ? ' chore-row-eye--off' : ''}"
          data-action="chore-toggle-home" data-chore-id="${c.id}"
          aria-label="${eyeTitle}" title="${eyeTitle}">${isHidden ? EYE_CLOSED_SVG : EYE_OPEN_SVG}</button>
        <button type="button" class="chore-row-btn chore-row-edit"
          data-action="chore-edit" data-chore-id="${c.id}"
          aria-label="Edit ${escapeHTML(c.name)}">${EDIT_SVG}</button>
      </div>
    </div>`;
  }).join("");

  return `<div class="chores-view">
    <div class="chores-view-header"><h2>Chores</h2></div>
    <p class="chores-view-hint">Drag ⋮⋮ to reorder · ${EYE_OPEN_SVG} to show/hide on Home</p>
    <div class="chore-list" id="chore-list">${rows}</div>
    <button type="button" class="fab" data-action="chore-add" aria-label="Add chore">+</button>
  </div>`;
}

/**
 * Render the chore add/edit bottom sheet.
 * @param {object|null} chore  null = add new chore; object = edit existing
 */
export function renderChoreSheet(chore) {
  const isNew = !chore;
  const title = isNew ? "Add Chore" : "Edit Chore";
  const icon = chore?.icon || "📋";
  const name = chore?.name || "";
  const color = chore?.color || "#2E86AB";
  const indicatorLabels = chore?.indicatorLabels || [];
  const isPredefined = chore?.isPredefined || false;
  const choreId = chore?.id ?? null;

  const swatches = COLOR_SWATCHES.map(c =>
    `<button type="button"
      class="color-swatch${c === color ? ' color-swatch--selected' : ''}"
      data-action="pick-chore-color" data-color="${c}"
      style="background:${c}" aria-label="Color ${c}"
      aria-pressed="${c === color}"></button>`
  ).join("");

  const quickEmojis = QUICK_EMOJIS.map(e =>
    `<button type="button" class="emoji-quick" data-action="pick-chore-emoji"
      data-emoji="${escapeHTML(e)}" aria-label="${escapeHTML(e)}">${e}</button>`
  ).join("");

  const indicatorChips = indicatorLabels.map((label, i) =>
    `<div class="indicator-chip-row" data-index="${i}">
      <input type="text" class="indicator-label-input input" data-index="${i}"
        value="${escapeHTML(label)}" placeholder="e.g. 💩 poo" maxlength="30" />
      <button type="button" class="indicator-remove-btn"
        data-action="remove-indicator-label" data-index="${i}"
        aria-label="Remove label">×</button>
    </div>`
  ).join("");

  const deleteOrRestore = isNew ? "" : isPredefined
    ? `<button type="button" class="btn btn-outline btn-sm chore-sheet-restore"
        data-action="restore-chore-default" data-chore-id="${choreId}">↩ Restore default</button>`
    : `<button type="button" class="btn btn-danger btn-sm chore-sheet-delete"
        data-action="delete-chore" data-chore-id="${choreId}">Delete</button>`;

  return `<div class="bottom-sheet chore-edit-sheet">
    <div class="sheet-handle"></div>
    <div class="sheet-title">${title}</div>

    <div class="chore-edit-field">
      <label class="chore-edit-label" for="chore-edit-name">Name</label>
      <input id="chore-edit-name" type="text" class="input chore-edit-name-input"
        value="${escapeHTML(name)}" placeholder="Chore name" maxlength="60" />
    </div>

    <div class="chore-edit-field">
      <label class="chore-edit-label">Icon</label>
      <div class="chore-icon-row">
        <div class="chore-icon-preview" id="chore-icon-preview"
          style="background:${escapeHTML(color)}">${icon}</div>
        <input id="chore-icon-input" type="text" class="input chore-icon-input"
          value="${escapeHTML(icon)}" maxlength="4" placeholder="Emoji" />
      </div>
      <div class="emoji-quick-row">${quickEmojis}</div>
    </div>

    <div class="chore-edit-field">
      <label class="chore-edit-label">Color</label>
      <div class="color-swatch-grid" id="color-swatch-grid">${swatches}</div>
    </div>

    <div class="chore-edit-field">
      <label class="chore-edit-label">
        Indicator labels
        <span class="chore-edit-hint">Optional tags logged with this chore</span>
      </label>
      <div id="indicator-labels-list">${indicatorChips}</div>
      <button type="button" class="btn-add-indicator" data-action="add-indicator-label">+ Add label</button>
    </div>

    <div class="chore-sheet-footer">
      <div class="chore-sheet-footer-left">${deleteOrRestore}</div>
      <div class="chore-sheet-footer-right">
        <button type="button" class="btn btn-outline" data-action="close-sheet">Cancel</button>
        <button type="button" class="btn btn-primary" data-action="save-chore"
          ${choreId !== null ? `data-chore-id="${choreId}"` : ''}
          data-is-new="${isNew}">Save</button>
      </div>
    </div>
  </div>`;
}
