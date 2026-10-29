import { escapeHTML } from "./utils.js";
import { state, rerender, withCurrentContext } from "./app-core.js";
import { registerActions } from "./actions.js";
import { apiFetch } from "./api.js";
import { setError, hideError } from "./form-errors.js";
import { captureScope } from "./request-scope.js";
import { showToast } from "./toast.js";

// Saves the name and color the household sees, for Settings and onboarding.
// Returns false when the answer arrives for an identity that is no longer
// current.
export async function saveProfile(displayName, avatarColor) {
  const contextScope = captureScope(state);
  const { data } = await withCurrentContext(apiFetch("/api/me", { method: "PATCH",
    body: JSON.stringify(avatarColor ? { displayName, avatarColor } : { displayName }) }), contextScope);
  if (!contextScope.current() || data?.user?.id !== state.user?.id) return false;
  const { displayName: name, avatarColor: color } = data.user;
  state.user = { ...state.user, displayName: name, avatarColor: color };
  state.members = (state.members || []).map(m => m.userId === state.user.id ? { ...m, displayName: name, avatarColor: color } : m);
  return true;
}

// Settings → Account: the name and color the household sees.
async function doSaveProfile(form) {
  if (form.dataset.busy) return;
  hideError("#profile-error");
  const displayName = form.querySelector("#profile-name").value.trim();
  if (!displayName) { setError("#profile-error", "Enter your name"); return; }
  const avatarColor = form.querySelector('input[name="avatarColor"]:checked')?.value;
  const contextScope = captureScope(state);
  form.dataset.busy = "true";
  try {
    if (!await saveProfile(displayName, avatarColor)) return;
    rerender();
    showToast("Profile saved", "success");
  } catch (err) {
    if (contextScope.current() && form.isConnected) setError("#profile-error", err.message || "Could not save your profile. Please retry.");
  } finally { delete form.dataset.busy; }
}

registerActions("submit", {
  "save-profile": (_e, form) => { void doSaveProfile(form); },
});

export function renderProfileSheet(user, household, userHouseholds = [], activeHouseholdId = null) {
  const email = user?.email || "";
  const name = user?.displayName || "";
  const initial = (name || email).charAt(0).toUpperCase();
  const color = /^#[0-9A-Fa-f]{6}$/.test(user?.avatarColor || "") ? user.avatarColor : "";
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
      <div class="profile-avatar-lg" aria-hidden="true"${color ? ` style="background:${color}"` : ""}>${escapeHTML(initial)}</div>
      <div class="profile-info">
        ${name ? `<span class="profile-name">${escapeHTML(name)}</span>` : ""}
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

// Avatar colors to choose from. Each carries white initials legibly; the
// first is the color every account starts with.
export const PROFILE_COLORS = [
  ["#19323C", "Dark teal"], ["#2E86AB", "Blue"], ["#7B5EA7", "Purple"], ["#C2185B", "Magenta"],
  ["#D84315", "Orange"], ["#2E7D32", "Green"], ["#00838F", "Teal"], ["#6D4C41", "Brown"],
];

// The name and color the household sees, edited in Settings → Account
// (PATCH /api/me). A color outside the palette stays selected by leaving
// every swatch unchecked, so saving a new name never changes it.
export function renderProfileForm(user) {
  const current = String(user?.avatarColor || "").toUpperCase();
  return `<form id="profile-form" data-action="save-profile" class="profile-form">
    <div class="form-group">
      <label class="form-label" for="profile-name">Your name</label>
      <input id="profile-name" name="displayName" required maxlength="50" autocomplete="name" value="${escapeHTML(user?.displayName || "")}">
      <p class="text-secondary text-sm mt-1">How your household sees you in Activity, Stats and Settings.</p>
    </div>
    <fieldset class="form-group profile-colors">
      <legend class="form-label">Color</legend>
      ${PROFILE_COLORS.map(([hex, name]) => `<label class="profile-color" title="${name}">
        <input type="radio" name="avatarColor" value="${hex}"${hex === current ? " checked" : ""}>
        <span class="profile-color-swatch" style="background:${hex}" aria-hidden="true"></span>
        <span class="sr-only">${name}</span>
      </label>`).join("")}
    </fieldset>
    <div id="profile-error" class="form-error hidden" role="alert"></div>
    <button type="submit" class="btn btn-primary btn-sm">Save</button>
  </form>`;
}
