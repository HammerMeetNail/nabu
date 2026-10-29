// The Settings page: its render, and the actions only it has (email
// verification, the password, notification and display preferences).
// Deleting the account and signing out stay in app.js, which owns the
// identity they end.
import { registerActions } from "./actions.js";
import { apiFetch } from "./api.js";
import { state, rerender, refreshTopBar, owned, withCurrentContext, finishIdentity, recoverIdentity } from "./app-core.js";
import { renderApiKeys } from "./api-keys.js";
import { handleChangePassword } from "./auth.js";
import { resultIsCurrent } from "./browser-context.js";
import { renderExports, renderMyDataDownload } from "./exports.js";
import { setError, hideError } from "./form-errors.js";
import { renderHouseholdView } from "./household.js";
import { maybeSubscribePush, saveNotificationPreferences } from "./notifications.js";
import { saveVolumeUnit, saveHideNotificationBadge } from "./preferences.js";
import { renderProfileForm } from "./profile.js";
import { captureScope } from "./request-scope.js";
import { showToast } from "./toast.js";
import { escapeHTML, renderAccessSuspendedCard } from "./utils.js";

export function renderSettingsView() {
  const hh = state.household;
  const user = state.user;

  const verificationSection = user && !user.emailVerified ? `
    <div class="card mt-3" style="border-left: 4px solid #F4A261;">
      <h3>Email Verification</h3>
      <p class="text-secondary">Your email <strong>${escapeHTML(user.email)}</strong> is not verified. A verification email is queued for delivery; you can request another below.</p>
      <button type="button" class="btn btn-sm btn-secondary mt-2" data-action="resend-verification">Resend verification email</button>
    </div>
  ` : "";

  // The build version lives here (Settings → About), not in the header.
  const appVersion = (typeof document !== "undefined" && document.body?.dataset.appVersion) || "";
  const aboutLine = appVersion ? `<p class="app-version" data-testid="app-version">Nabu v${escapeHTML(appVersion)}</p>` : "";

  // In-app account deletion (App Store guideline 5.1.1(v) requires it on the
  // native client; the PWA gets the same flow for parity). The server demands
  // the typed confirmation {"confirm":"DELETE"}; a sole owner of a
  // multi-member household gets a 409 telling them to transfer first.
  const deleteAccountSection = state.deleteAccountOpen ? `
    <div class="mt-3" data-testid="delete-account-confirm">
      <p class="text-secondary">Deleting your account permanently removes your
        account, email, name, sessions, devices, preferences, reminders and
        notifications. Households where you are the only member are deleted with
        all their logs.</p>
      <p class="text-secondary">In households you share, the logs you recorded
        stay so the others keep their history. They are shown as
        “Former member” and are no longer linked to you. You can also delete them.
        If you are the only owner of a household with other members, transfer
        ownership (or remove the other members) first. This cannot be undone.</p>
      ${renderMyDataDownload(state, { jsonId: "download-my-data", csvId: "download-my-logs" })}
      <label class="form-check mt-2">
        <input type="checkbox" id="delete-account-logs"${state.deleteAccountLogs ? ' checked' : ''}>
        <span>Also delete everything I logged in shared households</span>
      </label>
      <label class="form-check mt-1">
        <input type="checkbox" id="delete-account-notes"${state.deleteAccountNotes ? ' checked' : ''}>
        <span>Also delete the notes I wrote on logs that stay</span>
      </label>
      <div class="form-group mt-2">
        <label class="form-label" for="delete-account-input">Type DELETE to confirm</label>
        <input id="delete-account-input" type="text" autocomplete="off" autocapitalize="characters" placeholder="DELETE">
      </div>
      <div id="delete-account-error" class="form-error${state.deleteAccountError ? '' : ' hidden'}" role="alert">${escapeHTML(state.deleteAccountError || '')}</div>
      <button type="button" class="btn btn-danger btn-sm mt-2" data-action="confirm-delete-account"${state.deleteAccountBusy ? ' disabled' : ''}>${state.deleteAccountBusy ? 'Deleting…' : 'Permanently delete my account'}</button>
      <button type="button" class="btn btn-ghost btn-sm mt-2" data-action="cancel-delete-account">Cancel</button>
    </div>
  ` : `
    <button type="button" class="btn btn-danger btn-sm mt-3" data-action="open-delete-account">Delete account…</button>
  `;

  const passwordSection = `
    <div class="card mt-3">
      <h3>${user?.hasPassword === false ? "Set Password" : "Change Password"}</h3>
      <form id="change-password-form" data-action="change-password">
        ${user?.hasPassword === false ? "" : `<div class="form-group">
          <label class="form-label" for="current-password">Current Password</label>
          <input id="current-password" type="password" name="currentPassword" required autocomplete="current-password">
        </div>`}
        <div class="form-group">
          <label class="form-label" for="new-password">New Password</label>
          <input id="new-password" type="password" name="newPassword" required minlength="8" autocomplete="new-password">
        </div>
        <div class="form-group">
          <label class="form-label" for="confirm-password">Confirm New Password</label>
          <input id="confirm-password" type="password" name="confirmPassword" required minlength="8" autocomplete="new-password">
        </div>
        <div id="change-password-error" class="form-error hidden"></div>
        <button type="submit" class="btn btn-primary btn-sm mt-2">Update Password</button>
      </form>
    </div>
  `;

  const notifTypes = state.availableNotificationTypes || [];
  const notifPrefsLoaded = !!state.notificationPrefs;

  let notifPrefsCard = "";
  if (notifPrefsLoaded && notifTypes.length > 0) {
    const prefs = state.notificationPrefs;
    const pushEnabled = prefs?.pushEnabled !== false;
    const checked = pushEnabled ? " checked" : "";

    const permBtn = typeof Notification !== 'undefined' && Notification.permission === 'default'
      ? `<p class="mt-2"><button type="button" class="btn btn-sm btn-primary" data-action="enable-notifications">Enable Notifications</button></p>`
      : '';

    const defaultLead = prefs?.defaultReminderLeadMinutes ?? 10;
    const leadTimes = [5, 10, 15, 30, 60];

    // Per-type push toggles — only visible when push is enabled.
    // Schedule Reminder has a nested lead time selector.
    let pushTypeRows = "";
    if (pushEnabled) {
      const rows = notifTypes.map(t => {
        const allEnabled = (prefs?.enabledPushTypes || []).length === 0;
        const isEnabled = allEnabled || (prefs?.enabledPushTypes || []).includes(t.type);
        const typeChecked = isEnabled ? " checked" : "";

        let nested = "";
        if (t.type === "schedule_reminder" && isEnabled) {
          nested = `<div class="notif-pref-row notif-pref-row--nested">
            <label class="notif-pref-label">
              <span class="notif-pref-title">Reminder lead time</span>
              <span class="notif-pref-desc">Minutes before a scheduled chore's time</span>
            </label>
            <select data-action="change-default-reminder-lead" class="notif-pref-select">
              ${leadTimes.map(m => `<option value="${m}"${m === defaultLead ? ' selected' : ''}>${m} min</option>`).join("")}
            </select>
          </div>`;
        }

        return `<div class="notif-pref-row">
          <label class="notif-pref-label">
            <span class="notif-pref-title">${escapeHTML(t.label)}</span>
            <span class="notif-pref-desc">${escapeHTML(t.description)}</span>
          </label>
          <label class="notif-pref-toggle">
            <input type="checkbox" data-action="toggle-notif-pref" data-notif-type="${escapeHTML(t.type)}"${typeChecked}>
            <span class="toggle-slider"></span>
          </label>
        </div>${nested}`;
      }).join("");
      pushTypeRows = `<div class="notif-pref-list">${rows}</div>`;
    }

    notifPrefsCard = `<div class="card mt-3" data-key="notification-prefs">
      <h3>Notifications</h3>
      <p class="text-secondary">Applies to your account across all households.</p>
      <div class="notif-pref-row">
        <label class="notif-pref-label">
          <span class="notif-pref-title">Push Notifications</span>
        </label>
        <label class="notif-pref-toggle">
          <input type="checkbox" data-action="toggle-push-enabled"${checked}>
          <span class="toggle-slider"></span>
        </label>
      </div>
      ${permBtn}
      ${pushTypeRows}
    </div>`;
  }

  const volumeUnit = state.volumeUnit === "oz" ? "oz" : "ml";
  const badgeHidden = !!state.hideNotificationBadge;
  const prefsCard = `<div class="card mt-3" data-key="preferences">
    <h3>Preferences</h3>
    <div class="pref-row">
      <label class="pref-label">
        <span class="pref-title">Feed volume unit</span>
        <span class="pref-desc">How bottle volumes are shown and entered</span>
      </label>
      <div class="segmented" role="group" aria-label="Feed volume unit">
        <button type="button" class="segmented-btn${volumeUnit === "ml" ? " segmented-btn--active" : ""}" data-action="set-volume-unit" data-unit="ml" aria-pressed="${volumeUnit === "ml"}">mL</button>
        <button type="button" class="segmented-btn${volumeUnit === "oz" ? " segmented-btn--active" : ""}" data-action="set-volume-unit" data-unit="oz" aria-pressed="${volumeUnit === "oz"}">oz</button>
      </div>
    </div>
    <div class="pref-row">
      <label class="pref-label">
        <span class="pref-title">Hide notification badge</span>
        <span class="pref-desc">Notifications still collect; the unread count on the bell stays hidden</span>
      </label>
      <label class="notif-pref-toggle">
        <input type="checkbox" data-action="toggle-hide-notification-badge"${badgeHidden ? " checked" : ""} aria-label="Hide notification badge">
        <span class="toggle-slider"></span>
      </label>
    </div>
  </div>`;

  const householdRole = state.members?.find(m => m.userId === user?.id)?.role || user?.role;
  const canExportHousehold = !!hh && (householdRole === "owner" || householdRole === "admin");
  const exportCard = renderExports(state, canExportHousehold);
  const apiKeysCard = user ? renderApiKeys(state, window.location.origin) : "";
  // Everything about you, without starting a deletion (GDPR access). The
  // deletion panel offers it with the logs CSV, so it moves there while open.
  const myDataLine = user && !state.deleteAccountOpen ? renderMyDataDownload(state, { jsonId: "account-download-my-data" }) : "";

  const activeId = state.activeHouseholdId || hh?.id;
  const yourHouseholdsCard = state.userHouseholds && state.userHouseholds.length > 1 ? `
    <div class="card mt-3" data-key="your-households">
      <h3>Your Households</h3>
      <div class="profile-households">
        ${state.userHouseholds.map(h => {
          const isActive = h.id === activeId;
          const ini = h.initials || h.name.charAt(0).toUpperCase();
          return `<button type="button" class="profile-household-item${isActive ? ' profile-household-item--active' : ''}"
            data-action="activate-household" data-household-id="${escapeHTML(h.id)}">
            <span class="hh-initials-badge-sm" aria-hidden="true">${escapeHTML(ini)}</span>
            <span class="profile-household-name">${escapeHTML(h.name)}</span>
            <span class="profile-household-role text-secondary">${escapeHTML(h.role)}</span>
            ${isActive ? '<span class="profile-household-check" aria-label="Active">&#10003;</span>' : ''}
          </button>`;
        }).join('')}
      </div>
    </div>` : "";

  if (!hh) {
    const householdCard = state.accessSuspended
      ? renderAccessSuspendedCard(state)
      : renderHouseholdView(null, null, null, state.user);
    return `<div class="settings-view">${householdCard}${yourHouseholdsCard}${prefsCard}${notifPrefsCard}${exportCard}${apiKeysCard}<div class="card mt-3" data-key="account"><h3>Account</h3>${state.user ? renderProfileForm(state.user) : ''}<p class="text-secondary">${escapeHTML(state.user ? state.user.email : '')}</p>${verificationSection}${passwordSection}${myDataLine}${deleteAccountSection}</div>${aboutLine}</div>`;
  }
  return `<div class="settings-view"><h2>Settings</h2>${renderHouseholdView(hh, state.members, state.invites, state.user, { showInviteQR: !!state.showInviteQR })}${yourHouseholdsCard}${prefsCard}${notifPrefsCard}${exportCard}${apiKeysCard}<div class="card mt-3" data-key="account"><h3>Account</h3>${state.user ? renderProfileForm(state.user) : ''}<p class="text-secondary">${escapeHTML(state.user ? state.user.email : '')}</p>${verificationSection}${passwordSection}${myDataLine}${deleteAccountSection}</div>${aboutLine}</div>`;
}


async function doChangePassword(form) {
  if (form.dataset.busy) return;
  hideError("#change-password-error");
  const password = form.querySelector("#new-password").value;
  if (password !== form.querySelector("#confirm-password").value) { setError("#change-password-error","New passwords do not match"); return; }
  if (password.length < 8) { setError("#change-password-error","Password must be at least 8 characters"); return; }
  form.dataset.busy = "true";
  try {
    const result = await handleChangePassword(form.querySelector("#current-password")?.value || "",password);
    if (!resultIsCurrent(result)) return;
    if (result.ok && result.user) {
      if (await finishIdentity(result,"/settings")) showToast("Password updated","success");
    } else {
      maybeSubscribePush().catch(() => {});
      if (form.isConnected) setError("#change-password-error",result.data?.error || "Password change failed");
    }
  } catch (err) { await recoverIdentity(err,"/settings"); if (form.isConnected) setError("#change-password-error",err.message || "Password change failed"); }
  finally { delete form.dataset.busy; }
}

async function doResendVerification() {
  const contextScope = captureScope(state);
  const scope = captureScope(state);
  try {
    const {response} = await withCurrentContext(apiFetch("/api/auth/email/verification/resend",{method:"POST"}), contextScope);
    if (!response.ok) throw new Error("Verification request failed");
    if (scope.current()) showToast("Verification email queued","info");
  } catch {
    if (!contextScope.current()) return; if (scope.current()) showToast("Failed to request verification email. Please try again.","error"); }
}

// Saves the push types the toggles leave on; a failed save flips the toggle back.
function saveNotificationToggle(prefs, el, isChecked) {
  saveNotificationPreferences(prefs)
    .then(owned(data => {
      state.notificationPrefs = data.preferences;
      rerender();
    }))
    .catch(owned(() => {
      el.checked = !isChecked;
    }));
}

registerActions("click", {
  "resend-verification": (e) => {
    e.preventDefault();
    doResendVerification();
  },
  "toggle-push-enabled": (e, el) => {
    e.preventDefault();
    const isChecked = el.checked;
    const enabledTypes = state.notificationPrefs?.enabledPushTypes || [];
    saveNotificationToggle({ pushEnabled: isChecked, enabledPushTypes: isChecked ? enabledTypes : [] }, el, isChecked);
  },
  "enable-notifications": (e) => {
    e.preventDefault();
    if (typeof Notification === 'undefined') return;
    Notification.requestPermission().then(owned(result => {
      if (result === 'granted') {
        maybeSubscribePush().catch(() => {});
      }
    })).catch(() => {});
  },
  "toggle-notif-pref": (e, el) => {
    e.preventDefault();
    const notifType = el.dataset.notifType;
    const isChecked = el.checked;
    let enabledTypes = [...(state.notificationPrefs?.enabledPushTypes || [])];
    const prevPushEnabled = state.notificationPrefs?.pushEnabled;
    const allTypes = (state.availableNotificationTypes || []).map(t => t.type);

    // If no explicit preferences have been set yet (pushEnabled defaults
    // to true and enabledTypes is empty), start from all types.
    if (enabledTypes.length === 0 && prevPushEnabled !== false) {
      enabledTypes = [...allTypes];
    }

    if (isChecked) {
      if (!enabledTypes.includes(notifType)) enabledTypes.push(notifType);
    } else {
      enabledTypes = enabledTypes.filter(t => t !== notifType);
    }
    saveNotificationToggle({ enabledPushTypes: enabledTypes, pushEnabled: enabledTypes.length > 0 }, el, isChecked);
  },
  "set-volume-unit": (e, el) => {
    e.preventDefault();
    const unit = el.dataset.unit === "oz" ? "oz" : "ml";
    if (unit === state.volumeUnit) return;
    // saveVolumeUnit updates state optimistically (and rolls back on
    // failure); render now for snappy feedback and again on completion.
    saveVolumeUnit(state, unit).then(owned(() => rerender()));
    rerender();
  },
  "toggle-hide-notification-badge": (e, el) => {
    // No preventDefault: the checkbox keeps its native toggled state so
    // the UI reflects the tap instantly. saveHideNotificationBadge
    // applies the value optimistically and rolls back if the PATCH
    // fails; the completion render redraws the toggle from persisted
    // state either way.
    const hide = el.checked;
    if (hide === state.hideNotificationBadge) return;
    const saved = saveHideNotificationBadge(state, hide);
    refreshTopBar();
    saved.then(owned(() => {
      refreshTopBar();
      rerender();
    }));
  },
});

registerActions("change", {
  "change-default-reminder-lead": (e, el) => {
    const leadMinutes = parseInt(el.value, 10);
    if (isNaN(leadMinutes)) return;
    saveNotificationPreferences({ defaultReminderLeadMinutes: leadMinutes })
      .then(owned(data => {
        state.notificationPrefs = data.preferences;
        rerender();
      }))
      .catch(() => {});
  },
});

registerActions("submit", {
  "change-password": (e, form) => { doChangePassword(form); },
});
