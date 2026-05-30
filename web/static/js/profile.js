import { escapeHTML } from "./utils.js";

export function renderProfileSheet(user, household, userHouseholds = [], activeHouseholdId = null) {
  const email = user?.email || "";
  const initial = email.charAt(0).toUpperCase();
  const hhName = household?.name || "";
  const activeId = activeHouseholdId || household?.id;

  const householdsSection = userHouseholds.length > 0 ? `
    <div class="profile-households">
      <p class="profile-households-label">Your Households</p>
      ${userHouseholds.map(h => {
        const isActive = h.id === activeId;
        const ini = h.initials || h.name.charAt(0).toUpperCase();
        return `<button type="button" class="profile-household-item${isActive ? ' profile-household-item--active' : ''}"
          data-action="activate-household" data-household-id="${h.id}">
          <span class="hh-initials-badge-sm" aria-hidden="true">${escapeHTML(ini)}</span>
          <span class="profile-household-name">${escapeHTML(h.name)}</span>
          <span class="profile-household-role text-secondary">${escapeHTML(h.role)}</span>
          ${isActive ? '<span class="profile-household-check" aria-label="Active">&#10003;</span>' : ''}
        </button>`;
      }).join('')}
    </div>` : "";

  return `
  <div class="profile-backdrop" data-action="close-profile"></div>
  <div class="profile-panel" id="profile-panel">
    <div class="profile-panel-handle" aria-hidden="true"></div>
    <div class="profile-header">
      <div class="profile-avatar-lg" aria-hidden="true">${escapeHTML(initial)}</div>
      <div class="profile-info">
        <span class="profile-email">${escapeHTML(email)}</span>
        ${hhName ? `<span class="profile-household">${escapeHTML(hhName)}</span>` : ""}
      </div>
    </div>
    ${householdsSection}
    <div class="profile-actions">
      <button type="button" class="profile-action-btn" data-action="profile-nav-settings">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <circle cx="12" cy="12" r="3"></circle>
          <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
        </svg>
        <span>Settings</span>
      </button>
      <button type="button" class="profile-action-btn profile-action-signout" data-action="logout">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path>
          <polyline points="16 17 21 12 16 7"></polyline>
          <line x1="21" y1="12" x2="9" y2="12"></line>
        </svg>
        <span>Sign Out</span>
      </button>
    </div>
  </div>`;
}
