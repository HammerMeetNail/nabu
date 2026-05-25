import { createAppState, resetAuthedState } from "./state.js";
import { morphInnerHTML } from "./morph.js";
import { apiMe, apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";
import {
  loadSession,
  handleLogin,
  handleRegister,
  handleLogout,
  handleMagicLinkRequest,
  handleForgotPassword,
  handleResetPassword,
  handleChangePassword,
  renderLoginView,
  renderRegisterView,
  renderMagicLinkRequestView,
  renderMagicLinkNoticeView,
  renderVerifyEmailView,
  renderForgotPasswordView,
  renderResetPasswordView,
} from "./auth.js";
import { loadHousehold, createHousehold, joinHousehold, createInvite, deleteInvite, leaveHousehold, renderHouseholdView, renderJoinView } from "./household.js?v=2";
import { loadToday, loadWeek, logChore, undoLog, updateLog, loadChores, loadHistory, loadMoreHistory, renderHistoryView as renderHistoryPage, todayISO } from "./today.js";
import { renderStatsView, loadOverview } from "./stats.js";
import { renderDayView, renderWeekView, isActiveForDayJS } from "./calendar.js";
import { loadSchedules, createSchedule, updateSchedule, deleteSchedule, renderPickChoreSheet, renderEditScheduleSheet, renderLogSheet, renderQuickLogSheet } from "./schedule.js";
import { loadPreferences, saveChoreOrder, saveHiddenHomeChores, sortChoresByOrder } from "./preferences.js";
import { loadLatestLogs, renderHomeView as renderHomeViewGrid, renderHomeLogSheet, renderConfirmRemoveFromHomeSheet } from "./home.js";
import { renderChoresView as renderChoresViewList, renderChoreSheet } from "./chores.js";
import { loadNotifications, markRead, markAllRead, deleteNotification, renderNotificationPanel, maybeSubscribePush, requestNotificationPermission, clearAppBadge } from "./notifications.js";

/**
 * Reads the current frequency settings from a bottom sheet's freq <select>
 * and weekday pills, returning a partial schedule payload.
 *
 * @param {string} prefix  "sheet" | "edit-sheet"
 * @param {string} date    ISO date string used as startDate for "once"
 */
function readSheetFreq(prefix, date) {
  const sel = document.querySelector(`#${prefix}-freq`);
  if (!sel) return {};
  const freqVal  = sel.value;
  const selOpt   = sel.options[sel.selectedIndex];
  const payload  = { frequencyType: freqVal };

  switch (freqVal) {
    case "once":
      payload.startDate = date || null;
      break;
    case "every_n_days": {
      const intervalInput = document.querySelector(`#${prefix}-interval`);
      payload.intervalDays = Math.max(2, parseInt(intervalInput?.value || "2", 10));
      payload.startDate = date || todayISO(0);
      break;
    }
    case "weekly": {
      // Read which day pills are currently toggled on.
      const sheet   = sel.closest(".bottom-sheet");
      const pills   = sheet ? [...sheet.querySelectorAll(".day-pill--on")] : [];
      payload.daysOfWeek = pills.map(p => parseInt(p.dataset.day, 10));
      if (payload.daysOfWeek.length === 0) {
        // Fall back to the option's data attribute (set at render time).
        const raw = selOpt?.dataset?.daysOfWeek;
        try { payload.daysOfWeek = raw ? JSON.parse(raw) : []; } catch { payload.daysOfWeek = []; }
      }
      break;
    }
    case "monthly_by_date":
      payload.dayOfMonth = parseInt(selOpt?.dataset?.dayOfMonth || "1", 10);
      break;
    case "yearly":
      payload.dayOfMonth   = parseInt(selOpt?.dataset?.dayOfMonth   || "1",  10);
      payload.monthOfYear  = parseInt(selOpt?.dataset?.monthOfYear  || "1",  10);
      break;
    default:
      break;
  }
  return payload;
}

let state;

export function render(root) {
  const route = state.currentRoute || window.location.pathname || "/";
  // Effective route for tab highlighting: unknown/auth-only paths fall back to
  // home ("/") so the "today" tab is always active when the home grid renders.
  const knownTabRoutes = ["/", "/today", "/calendar", "/chores", "/history", "/settings", "/stats"];
  const tabRoute = knownTabRoutes.includes(route) ? route : "/";
  let html = "";

  if (route.startsWith("/verify-email")) {
    const url = new URL(window.location.href);
    const token = url.searchParams.get("token");
    if (token) {
      html = renderVerifyEmailView(true);
      if (!state._emailVerified) {
        state._emailVerified = true;
        verifyEmail(token);
      }
    } else {
      html = renderVerifyEmailView(false);
    }
  } else if (route.startsWith("/magic-login")) {
    const url = new URL(window.location.href);
    const token = url.searchParams.get("token");
    if (token) {
      html = renderMagicLinkNoticeView();
      if (!state._magicLinkConsumed) {
        state._magicLinkConsumed = true;
        consumeMagicLink(token);
      }
    } else {
      html = `<div class="auth-card"><p class="text-center">Invalid magic link.</p></div>`;
    }
  } else if (route.startsWith("/reset-password")) {
    const url = new URL(window.location.href);
    const token = url.searchParams.get("token");
    html = renderResetPasswordView(token);
  } else if (route.startsWith("/join")) {
    const code = new URL(window.location.href).searchParams.get("code") || state._pendingInviteCode;
    if (code) state._pendingInviteCode = code;
    if (!state.user) {
      html = renderJoinView(code);
    } else if (!state.household && code) {
      html = `<div class="auth-card"><p class="text-center">Joining household…</p></div>`;
      if (!state._joinAttempted) {
        state._joinAttempted = true;
        doJoinWithCode(code);
      }
    } else {
      state.currentRoute = "/";
      html = renderHomeViewWrapper();
    }
  } else if (!state.user) {
    switch (route) {
      case "/register":
        html = renderRegisterView(state.googleOAuthEnabled);
        break;
      case "/magic-link":
        html = renderMagicLinkRequestView();
        break;
      case "/forgot-password":
        html = renderForgotPasswordView();
        break;
      default:
        html = renderLoginView(state.googleOAuthEnabled);
    }
  } else {
    switch (route) {
      case "/":
      case "/today":
        html = renderHomeViewWrapper();
        break;
      case "/calendar":
        html = renderCalendarView();
        break;
      case "/chores":
        html = renderChoresView();
        break;
      case "/history":
        html = renderHistoryView();
        break;
      case "/settings":
      case "/stats":
        html = renderSettingsView();
        break;
      default:
        html = renderHomeViewWrapper();
    }
  }

  // Preserve the day-hour-grid-wrapper scroll position across re-renders.
  // morph.js reuses DOM nodes by position, but template whitespace differences
  // (e.g. when a sheet opens/closes) can cause it to destroy and recreate the
  // wrapper element, resetting scrollTop to 0 and triggering the auto-scroll.
  const prevWrapper = root.querySelector(".day-hour-grid-wrapper");
  const savedScroll = prevWrapper ? prevWrapper.scrollTop : -1;

  morphInnerHTML(root, html);
  updateTabs(tabRoute);
  updateTopBar();

  // Auto-scroll the day-hour-grid-wrapper to show the current time when it is
  // first rendered (scrollTop === 0).  This prevents the grid from always
  // starting at midnight — an hour that is rarely relevant — and ensures that
  // cards in the 9 AM–3 PM range are visible without manual scrolling.
  const wrapper = root.querySelector(".day-hour-grid-wrapper");
  if (wrapper) {
    if (savedScroll > 0) {
      // Restore the position the user was at before this re-render.
      wrapper.scrollTop = savedScroll;
    } else if (savedScroll === -1) {
      // First render (no prior wrapper): scroll to current hour.
      const h = new Date().getHours();
      const ROW_HEIGHT = 48; // must match CSS .day-hour-row height
      // Show 2 rows before the current hour; clamp between 7 AM and 11 AM so
      // that mid-morning and noon chores are always in the visible area without
      // requiring the user to scroll.
      wrapper.scrollTop = Math.min(Math.max(7, h - 2), 11) * ROW_HEIGHT;
    }
  }
}

function renderChoresView() {
  const mainView = renderChoresViewList(state);
  if (state.activeSheet === "chore-edit") {
    const { choreId } = state.activeSheetData || {};
    const isNew = choreId === null || choreId === undefined;
    const chore = isNew ? null : (state.chores || []).find(c => c.id === choreId);
    if (isNew || chore) {
      const sheetHTML = renderChoreSheet(isNew ? null : chore);
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  return mainView;
}

function renderHistoryView() {
  return renderHistoryPage(state);
}

function renderHomeViewWrapper() {
  const mainView = renderHomeViewGrid(state);
  if (state.activeSheet === "home-log") {
    const { choreId } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    if (chore) {
      const sheetHTML = renderHomeLogSheet(chore);
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  if (state.activeSheet === "confirm-remove-home-chore") {
    const { choreId } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    if (chore) {
      const sheetHTML = renderConfirmRemoveFromHomeSheet(chore);
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  return mainView;
}

function renderCalendarView() {
  const chores = state.chores || [];
  if (!state.household && state.user) {
    return `<div class="card mt-3"><h2>Welcome!</h2>
      <p>Hi ${escapeHTML(state.user.email || '')}! Set up your household to get started.</p>
      <a class="btn btn-primary mt-2" href="#" data-nav="settings">Set Up Household</a></div>`;
  }
  if (chores.length === 0) {
    return `<div class="today-view"><h2>Today</h2>
    <div class="empty-state"><div class="empty-state-icon">🏠</div>
    <div class="empty-state-title">No chores set up yet</div>
    <p>Add chores via settings or the chores tab.</p></div></div>`;
  }
  const mainView = state.calendarView === "week"
    ? renderWeekView(state)
    : renderDayView(state);

  const fab = `<button type="button" class="fab" data-action="open-quick-log" aria-label="Log a chore">+</button>`;

  if (state.activeSheet === "pick-chore") {
    const sheetHTML = renderPickChoreSheet(
      sortChoresByOrder(state.chores, state.choreOrder),
      state.activeSheetData || {},
      state.schedules || []
    );
    return `<div class="sheet-overlay-wrapper">
      ${mainView}
      ${fab}
      <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
      ${sheetHTML}
    </div>`;
  }
  if (state.activeSheet === "edit-schedule") {
    const { choreId, scheduleId } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    const sch   = (state.schedules || []).find(s => s.id === scheduleId);
    if (chore && sch) {
      const sheetHTML = renderEditScheduleSheet(chore, sch, state.calendarDate);
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        ${fab}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  if (state.activeSheet === "log") {
    const { choreId, logId, date } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    if (chore) {
      const allLogs = state.calendarView === "week"
        ? (state.weekLogs || [])
        : (state.todayLogs || []);
      const log = logId ? (allLogs.find(l => l.id === logId) || null) : null;
      const sheetHTML = renderLogSheet(chore, log, date || "");
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        ${fab}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  if (state.activeSheet === "quick-log") {
    const date = state.activeSheetData?.date || "";
    const sheetHTML = renderQuickLogSheet(sortChoresByOrder(state.chores, state.choreOrder), date);
    return `<div class="sheet-overlay-wrapper">
      ${mainView}
      ${fab}
      <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
      ${sheetHTML}
    </div>`;
  }
  return `<div class="sheet-overlay-wrapper">${mainView}${fab}</div>`;
}

function renderSettingsView() {
  const hh = state.household;
  const user = state.user;
  let statsHTML = '';
  try {
    if (state.stats && state.stats.leaderboard) {
      statsHTML = renderStatsView(state);
    } else {
      statsHTML = '<p class="text-center text-secondary">Loading stats...</p>';
    }
  } catch {
    statsHTML = '<p class="text-center text-secondary">Stats unavailable</p>';
  }

  const verificationSection = user && !user.emailVerified ? `
    <div class="card mt-3" style="border-left: 4px solid #F4A261;">
      <h3>Email Verification</h3>
      <p class="text-secondary">Your email <strong>${escapeHTML(user.email)}</strong> is not verified.</p>
      <button type="button" class="btn btn-sm btn-secondary mt-2" data-action="resend-verification">Resend verification email</button>
    </div>
  ` : "";

  const passwordSection = `
    <div class="card mt-3">
      <h3>Change Password</h3>
      <form id="change-password-form" data-action="change-password">
        <div class="form-group">
          <label class="form-label" for="current-password">Current Password</label>
          <input id="current-password" type="password" name="currentPassword" required autocomplete="current-password">
        </div>
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

  if (!hh) {
    return `<div class="settings-view">${renderHouseholdView(null)}<div class="card mt-3"><h3>Account</h3><p class="text-secondary">${escapeHTML(state.user ? state.user.email : '')}</p>${verificationSection}${passwordSection}<button type="button" class="btn btn-sm btn-secondary mt-2" data-action="logout">Sign Out</button></div></div>`;
  }
  return `<div class="settings-view"><h2>Settings</h2>${renderHouseholdView(hh, state.members, state.invites)}<div class="card mt-3"><h3>Account</h3><p class="text-secondary">${escapeHTML(state.user ? state.user.email : '')}</p>${verificationSection}${passwordSection}<button type="button" class="btn btn-sm btn-secondary mt-2" data-action="logout">Sign Out</button></div>${statsHTML}</div>`;
}

async function loadStatsData() {
  try {
    const data = await loadOverview();
    if (data && data.overview) {
      state.stats = {
        leaderboard: data.overview.leaderboard || [],
        streaks: data.overview.streaks || {},
        breakdown: data.overview.breakdown || [],
        recap: data.overview.recap || {},
      };
    }
  } catch {}
}

async function loadLatestLogsData() {
  if (!state.household) return;
  try {
    const data = await loadLatestLogs();
    state.latestLogs = data?.latestLogs || {};
  } catch {}
}

async function loadNotifData() {
  if (!state.user) return;
  try {
    const data = await loadNotifications();
    state.notifications = data.notifications || [];
    state.unreadNotifications = data.unreadCount || 0;
  } catch {}

  // Check push diagnostic from service worker
  if ('serviceWorker' in navigator && navigator.serviceWorker.controller) {
    try {
      const mc = new MessageChannel();
      const swResult = new Promise((resolve) => {
        mc.port1.onmessage = (e) => resolve(e.data);
        setTimeout(() => resolve(null), 1000);
      });
      navigator.serviceWorker.controller.postMessage("last-push", [mc.port2]);
      const lastPush = await swResult;
      if (lastPush && lastPush.time) {
        window.__pushDiag = lastPush;
      }
    } catch {}
  }
}

function showToastWithUndo(message, logId) {
  const container = document.querySelector("#toast-container");
  if (!container) return;
  const toast = document.createElement("div");
  toast.className = "toast toast-success";
  toast.style.cssText = "display:flex;align-items:center;gap:8px;";
  const label = document.createElement("span");
  label.textContent = message;
  const undoBtn = document.createElement("button");
  undoBtn.type = "button";
  undoBtn.textContent = "Undo";
  undoBtn.style.cssText = "background:rgba(255,255,255,0.2);border:none;color:white;padding:4px 10px;border-radius:6px;cursor:pointer;font-size:13px;font-weight:600;margin-left:auto;min-height:32px;";
  undoBtn.addEventListener("click", () => {
    toast.remove();
    undoLog(logId).then(async () => {
      await loadLatestLogsData();
      render(document.querySelector("#app"));
    }).catch(() => showToast("Failed to undo", "error"));
  });
  toast.appendChild(label);
  toast.appendChild(undoBtn);
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 4000);
}

function updateTabs(route) {
  const tabs = document.querySelector("#bottom-tabs");
  if (!tabs || !state.user) return;
  tabs.querySelectorAll(".tab-item").forEach((tab) => {
    const active = route === "/" + tab.dataset.nav || (route === "/" && tab.dataset.nav === "today");
    tab.classList.toggle("active", active);
  });
}

function updateTopBar() {
  const topBar = document.querySelector("#top-bar");
  const tabs = document.querySelector("#bottom-tabs");
  if (!topBar || !tabs) return;

  if (state.user) {
    topBar.hidden = false;
    tabs.hidden = false;
    const avatar = document.querySelector("#user-avatar");
    if (avatar) {
      avatar.hidden = false;
      avatar.style.backgroundColor = "#19323C";
      avatar.textContent = state.user.email.charAt(0).toUpperCase();
      avatar.title = state.user.email;
    }
    const bell = document.querySelector("#notifications-bell");
    const badge = document.querySelector("#notification-badge");
    if (bell) {
      bell.hidden = false;
      bell.title = "Notifications";
    }
    if (badge) {
      if (state.unreadNotifications > 0) {
        badge.hidden = false;
        badge.textContent = String(state.unreadNotifications);
      } else {
        badge.hidden = true;
        badge.textContent = "";
      }
    }
  } else {
    topBar.hidden = true;
    tabs.hidden = true;
  }
}

function showToast(message, type) {
  const container = document.querySelector("#toast-container");
  if (!container) return;
  const toast = document.createElement("div");
  toast.className = `toast toast-${type}`;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 3000);
}

function setError(containerId, message) {
  const el = document.querySelector(containerId);
  if (!el) return;
  el.textContent = message;
  el.classList.remove("hidden");
}

function hideError(containerId) {
  const el = document.querySelector(containerId);
  if (!el) return;
  el.classList.add("hidden");
}

async function doLogin(form) {
  hideError("#login-error");
  requestNotificationPermission();
  const email = form.querySelector("#login-email").value;
  const password = form.querySelector("#login-password").value;
  const { ok, data } = await handleLogin(email, password);
  if (ok && data.user) {
    state.user = data.user;
    state.currentRoute = "/";
    maybeSubscribePush().catch(() => {});
    await reloadAfterAuth();
    if (state._pendingInviteCode && !state.household) {
      await doJoinWithCode(state._pendingInviteCode);
      return;
    }
    const app = document.querySelector("#app");
    if (app) render(app);
  } else {
    setError("#login-error", data.error || "Invalid email or password");
  }
}

async function reloadAfterAuth() {
  try {
    await Promise.all([loadHouseholdData(), loadPreferences(state)]);
    if (state.household) {
      await Promise.all([
        loadChoreData(),
        loadTodayData(),
        loadLatestLogsData(),
        loadStatsData(),
        loadNotifData(),
      ]);
    }
  } catch {}
}

async function doRegister(form) {
  hideError("#register-error");
  requestNotificationPermission();
  const email = form.querySelector("#reg-email").value;
  const password = form.querySelector("#reg-password").value;
  const confirm = form.querySelector("#reg-confirm").value;
  if (password !== confirm) {
    setError("#register-error", "Passwords do not match");
    return;
  }
  const { ok, data } = await handleRegister(email, password);
  if (ok && data.user) {
    state.user = data.user;
    state.currentRoute = "/";
    maybeSubscribePush().catch(() => {});
    await reloadAfterAuth();
    if (state._pendingInviteCode && !state.household) {
      await doJoinWithCode(state._pendingInviteCode);
      return;
    }
    const app = document.querySelector("#app");
    if (app) render(app);
  } else {
    setError("#register-error", data.error || "Registration failed");
  }
}

async function doMagicLinkRequest(form) {
  const email = form.querySelector("#magic-email").value;
  await handleMagicLinkRequest(email);
  const el = document.querySelector("#magic-link-status");
  if (el) {
    el.textContent = "Check your email for the magic link!";
    el.classList.add("form-error");
    el.classList.remove("hidden");
  }
}

async function doForgotPassword(form) {
  const email = form.querySelector("#forgot-email").value;
  await handleForgotPassword(email);
  showToast("If an account exists, a reset link has been sent.", "info");
}

async function doResetPassword(form) {
  hideError("#reset-error");
  const token = form.querySelector("input[name='token']").value;
  const password = form.querySelector("#reset-password").value;
  const confirm = form.querySelector("#reset-confirm").value;
  if (password !== confirm) {
    setError("#reset-error", "Passwords do not match");
    return;
  }
  const { ok, data } = await handleResetPassword(token, password);
  if (ok && data.user) {
    state.user = data.user;
    state.currentRoute = "/";
    const app = document.querySelector("#app");
    if (app) render(app);
  } else {
    setError("#reset-error", data.error || "Password reset failed");
  }
}

async function doChangePassword(form) {
  hideError("#change-password-error");
  const currentPassword = form.querySelector("#current-password").value;
  const newPassword = form.querySelector("#new-password").value;
  const confirmPassword = form.querySelector("#confirm-password").value;
  if (newPassword !== confirmPassword) {
    setError("#change-password-error", "New passwords do not match");
    return;
  }
  if (newPassword.length < 8) {
    setError("#change-password-error", "Password must be at least 8 characters");
    return;
  }
  const { ok, data } = await handleChangePassword(currentPassword, newPassword);
  if (ok && data.user) {
    state.user = data.user;
    form.reset();
    showToast("Password updated", "success");
  } else {
    setError("#change-password-error", data.error || "Password change failed");
  }
}

async function doResendVerification() {
  const csrfToken = document.cookie.match(/(?:^|;\s*)choresy_csrf=([^;]*)/)?.[1] || "";
  try {
    await fetch("/api/auth/email/verification/resend", {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken },
    });
    showToast("Verification email sent", "info");
  } catch {
    showToast("Failed to resend verification email", "error");
  }
}

async function verifyEmail(token) {
  const csrfToken = document.cookie.match(/(?:^|;\s*)choresy_csrf=([^;]*)/)?.[1] || "";
  const res = await fetch(`/api/auth/email/verify?token=${encodeURIComponent(token)}`, {
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (res.ok) {
    if (state.user) {
      state.user.emailVerified = true;
    }
    try {
      state.user = await loadSession();
    } catch {}
  }
}

async function doJoinWithCode(code) {
  try {
    const data = await joinHousehold(code);
    if (data.household) {
      state._pendingInviteCode = null;
      state.currentRoute = "/";
      await Promise.all([loadHouseholdData(), loadChoreData(), loadTodayData()]);
    }
  } catch {}
  state._joinAttempted = false;
  const app = document.querySelector("#app");
  if (app) render(app);
}

async function consumeMagicLink(token) {
  try {
    const csrfToken = document.cookie.match(/(?:^|;\s*)choresy_csrf=([^;]*)/)?.[1] || "";
    const res = await fetch(`/api/auth/magic-link/consume?token=${encodeURIComponent(token)}`, {
      headers: { "X-CSRF-Token": csrfToken },
    });
    const data = await res.json();
    if (data.user) {
      state.user = data.user;
      state.currentRoute = "/";
      const app = document.querySelector("#app");
      if (app) render(app);
    }
  } catch {}
}

export async function init() {
  state = createAppState();

  state.googleOAuthEnabled = document.body?.dataset?.googleOauthEnabled === "true";

  try {
    state.user = await loadSession();
  } catch {
    state.user = null;
  }

  if (state.user) {
    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.register('/service-worker.js').catch(() => {});
    }
    maybeSubscribePush().catch(() => {});
  }

  const app = document.querySelector("#app");
  if (!app) return;

  let longPressTimer    = null;
  let longPressJustFired = false;
  let pressStartX = 0;
  let pressStartY = 0;
  const jiggleDrag = { active: false, choreId: null, targetChoreId: null };

  document.addEventListener("click", (e) => {
    // Always prevent default for nav links so a long-press residual click
    // never causes a full-page navigation via the href attribute.
    const navEl = e.target.closest("[data-nav]");
    if (navEl) e.preventDefault();

    // Only swallow the residual click on the card / cell that was long-pressed.
    // Sheet buttons (e.g. the "Log" save button) are inside .bottom-sheet and
    // must always be processed, even if the user taps within the 50 ms grace
    // period after lifting their finger from the long-press.
    if (longPressJustFired) {
      longPressJustFired = false;
      if (!e.target.closest(".bottom-sheet")) return;
    }
    const actionEl = e.target.closest("[data-action]");

    // data-nav SPA navigation: check first so it works without data-action
    if (navEl) {
      state.currentRoute = `/${navEl.dataset.nav}`;
      if (state.currentRoute === "/settings") {
        state._loadedHousehold = true;
      }
      if (state.currentRoute === "/history") {
        loadHistory().then(data => {
          state.historyLogs = data.logs || [];
          state.historyHasMore = data.hasMore;
          state.historyBefore = data.start || null;
          render(app);
        });
        return;
      }
      if (state.currentRoute === "/today") {
        loadLatestLogsData().then(() => render(app));
        return;
      }
      // Navigating to the calendar tab: always refresh log data so that chores
      // logged from the home tab (or any other source) are immediately visible.
      if (state.currentRoute === "/calendar") {
        render(app); // render immediately with current state (shows skeleton)
        (state.calendarView === "week" ? loadWeekData() : loadTodayData())
          .then(() => render(app));
        return;
      }
      render(app);
      return;
    }

    const action = actionEl?.dataset?.action;
    if (!action) return;

    // ── Weekday pill: toggle on/off ─────────────────────────────────────────
    if (action === "toggle-day") {
      actionEl.classList.toggle("day-pill--on");
      actionEl.setAttribute("aria-pressed",
        String(actionEl.classList.contains("day-pill--on")));
      return;
    }

    // ── Indicator chip: toggle on/off (log sheet) ───────────────────────────
    if (action === "toggle-indicator") {
      actionEl.classList.toggle("log-chip--on");
      actionEl.setAttribute("aria-pressed",
        String(actionEl.classList.contains("log-chip--on")));
      return;
    }

    switch (action) {
      case "show-login":
      case "show-register":
      case "show-magic-link":
      case "show-forgot-password": {
        e.preventDefault();
        const routes = {
          "show-login": "/",
          "show-register": "/register",
          "show-magic-link": "/magic-link",
          "show-forgot-password": "/forgot-password",
        };
        state.currentRoute = routes[action];
        render(app);
        break;
      }
      case "logout":
        e.preventDefault();
        handleLogout().then(() => {
          resetAuthedState(state);
          state.currentRoute = "/";
          render(app);
        });
        break;
      case "resend-verification":
        e.preventDefault();
        doResendVerification();
        break;
      case "open-notifications": {
        e.preventDefault();
        clearAppBadge();
        loadNotifData().then(() => {
          const container = document.querySelector("#notif-panel-container");
          if (container) {
            container.hidden = false;
            container.innerHTML = renderNotificationPanel(state.notifications);
          }
        });
        // Show panel immediately with current state while loading
        const container = document.querySelector("#notif-panel-container");
        if (container) {
          container.hidden = false;
          container.innerHTML = renderNotificationPanel(state.notifications);
        }
        break;
      }
      case "close-notifications": {
        e.preventDefault();
        const container = document.querySelector("#notif-panel-container");
        if (container) {
          container.hidden = true;
          container.innerHTML = "";
        }
        break;
      }
      case "mark-all-read": {
        e.preventDefault();
        clearAppBadge();
        markAllRead().then(() => loadNotifData()).then(() => {
          state.unreadNotifications = 0;
          updateTopBar();
          const container = document.querySelector("#notif-panel-container");
          if (container && !container.hidden) {
            container.innerHTML = renderNotificationPanel(state.notifications);
          }
        });
        break;
      }
      case "dismiss-notification": {
        e.preventDefault();
        const nid = parseInt(actionEl.dataset.notifId, 10);
        deleteNotification(nid).then(() => loadNotifData()).then(() => {
          updateTopBar();
          const container = document.querySelector("#notif-panel-container");
          if (container && !container.hidden) {
            container.innerHTML = renderNotificationPanel(state.notifications);
          }
        });
        break;
      }
      case "mark-notif-read": {
        e.preventDefault();
        const nid = parseInt(actionEl.dataset.notifId, 10);
        markRead(nid).then(() => loadNotifData()).then(() => {
          updateTopBar();
          const container = document.querySelector("#notif-panel-container");
          if (container && !container.hidden) {
            container.innerHTML = renderNotificationPanel(state.notifications);
          }
        });
        break;
      }
      case "create-invite":
        e.preventDefault();
        createInvite().then((data) => {
          if (data.invite) {
            const url = `${window.location.origin}/join?code=${data.invite.code}`;
            state.invites = [...(state.invites || []), data.invite];
            navigator.clipboard.writeText(url).then(
              () => showToast("Invite link copied to clipboard!", "info"),
              () => showToast("Invite link: " + url, "info")
            );
            render(app);
          }
        });
        break;
      case "copy-invite-link": {
        e.preventDefault();
        const code = actionEl.dataset.code;
        const url = `${window.location.origin}/join?code=${code}`;
        navigator.clipboard.writeText(url).then(
          () => showToast("Invite link copied!", "info"),
          () => showToast(`Link: ${url}`, "info")
        );
        break;
      }
      case "delete-invite":
        e.preventDefault();
        deleteInvite(parseInt(actionEl.dataset.inviteId)).then(() => render(app));
        break;
      case "leave-household":
        e.preventDefault();
        leaveHousehold().then(() => {
          state.household = null;
          state.chores = [];
          render(app);
        });
        break;
      case "log-chore":
        e.preventDefault();
        logChore(parseInt(actionEl.dataset.choreId), "", actionEl.dataset.date || "", []).then(async () => {
          await (state.calendarView === "week" ? loadWeekData() : loadTodayData());
          render(app);
        });
        break;
      case "undo-chore":
        e.preventDefault();
        undoLog(parseInt(actionEl.dataset.logId)).then(async () => {
          state.activeSheet     = null;
          state.activeSheetData = {};
          await (state.calendarView === "week" ? loadWeekData() : loadTodayData());
          render(app);
        }).catch((err) => {
          console.error('undo-chore failed:', err);
          (state.calendarView === "week" ? loadWeekData() : loadTodayData()).then(() => render(app));
        });
        break;
      case "view-log": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const logId   = actionEl.dataset.logId ? parseInt(actionEl.dataset.logId, 10) : null;
        const date    = actionEl.dataset.date || "";
        const chore   = (state.chores || []).find(c => c.id === choreId);
        if (chore) {
          state.activeSheet     = "log";
          state.activeSheetData = { choreId, logId, date };
          render(app);
        }
        break;
      }

      case "edit-schedule": {
        e.preventDefault();
        const choreId    = parseInt(actionEl.dataset.choreId, 10);
        const scheduleId = parseInt(actionEl.dataset.scheduleId, 10);
        state.activeSheet     = "edit-schedule";
        state.activeSheetData = { choreId, scheduleId };
        render(app);
        break;
      }

      case "save-log": {
        e.preventDefault();
        const logId   = actionEl.dataset.logId;
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const date    = actionEl.dataset.date || "";
        const note    = (document.querySelector('#log-note')?.value || "").trim();
        const indicators = [...document.querySelectorAll('.log-chip--on')]
          .map(el => el.dataset.label);
        const slotHour = state.activeSheetData?.slotHour ?? null;
        const doLog = logId
          ? updateLog(parseInt(logId, 10), note, indicators)
          : logChore(choreId, note, date, indicators, slotHour);
        doLog.then(async () => {
          state.activeSheet     = null;
          state.activeSheetData = {};
          await (state.calendarView === "week" ? loadWeekData() : loadTodayData());
          render(app);
        }).catch(() => showToast("Failed to save log", "error"));
        break;
      }

      case "open-quick-log": {
        e.preventDefault();
        state.activeSheet     = "quick-log";
        state.activeSheetData = { date: state.calendarDate || todayISO(0) };
        render(app);
        break;
      }

      case "quick-log-chore": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const date    = actionEl.dataset.date || "";
        const note    = (document.querySelector('#quick-log-note')?.value || "").trim();
        logChore(choreId, note, date, []).then(async () => {
          state.activeSheet     = null;
          state.activeSheetData = {};
          await (state.calendarView === "week" ? loadWeekData() : loadTodayData());
          render(app);
        }).catch(() => showToast("Failed to log chore", "error"));
        break;
      }

      case "navigate-day":
        e.preventDefault();
        state.calendarDate = actionEl.dataset.date;
        state.todayDate = actionEl.dataset.date;
        loadTodayData().then(() => render(app));
        break;

      case "switch-view":
        e.preventDefault();
        state.calendarView = actionEl.dataset.view;
        if (state.calendarView === "week") {
          loadWeekData().then(() => render(app));
        } else {
          loadTodayData().then(() => render(app));
        }
        break;

      case "navigate-week":
        e.preventDefault();
        state.calendarDate = actionEl.dataset.date;
        loadWeekData().then(() => render(app));
        break;

      case "open-pick-chore-sheet":
        e.preventDefault();
        state.activeSheet = "pick-chore";
        state.activeSheetData = {
          date: actionEl.dataset.date,
          hour: actionEl.dataset.hour ? parseInt(actionEl.dataset.hour, 10) : null,
        };
        render(app);
        break;

      case "schedule-chore-here": {
        e.preventDefault();
        const choreId      = parseInt(actionEl.dataset.choreId, 10);
        const slotDate     = actionEl.dataset.date || state.activeSheetData?.date || null;
        const timeInput    = document.querySelector("#sheet-time");
        const specificTime = timeInput?.value || null;
        const freqPayload  = readSheetFreq("sheet", slotDate);
        createSchedule({
          choreId,
          timePeriod:    "anytime",
          specificTime,
          isActive:      true,
          ...freqPayload,
        }).then(async () => {
          state.activeSheet = null;
          state.activeSheetData = {};
          state.schedules = await loadSchedules();
          render(app);
        }).catch(() => showToast("Failed to schedule chore", "error"));
        break;
      }

      case "close-sheet":
        e.preventDefault();
        state.activeSheet = null;
        state.activeSheetData = {};
        render(app);
        break;

      case "save-schedule-edit": {
        e.preventDefault();
        const scheduleId   = parseInt(actionEl.dataset.scheduleId, 10);
        const timeInput    = document.querySelector("#edit-sheet-time");
        const specificTime = timeInput?.value || null;
        const freqPayload  = readSheetFreq("edit-sheet", state.calendarDate);
        updateSchedule(scheduleId, { specificTime, ...freqPayload })
          .then(async () => {
            state.activeSheet     = null;
            state.activeSheetData = {};
            state.schedules = await loadSchedules();
            render(app);
          }).catch(() => showToast("Failed to update schedule", "error"));
        break;
      }

      case "delete-schedule": {
        e.preventDefault();
        const scheduleId = parseInt(actionEl.dataset.scheduleId, 10);
        deleteSchedule(scheduleId)
          .then(async () => {
            state.activeSheet     = null;
            state.activeSheetData = {};
            state.schedules = await loadSchedules();
            render(app);
          }).catch(() => showToast("Failed to remove schedule", "error"));
        break;
      }

      case "home-tap-chore": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.homeChoreId, 10);
        const chore = (state.chores || []).find(c => c.id === choreId);
        if (!chore) break;
        state.activeSheet     = "home-log";
        state.activeSheetData = { choreId };
        render(app);
        break;
      }

      case "save-home-log": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const note = (document.querySelector('#home-log-note')?.value || "").trim();
        const indicators = [...document.querySelectorAll('.log-chip--on')].map(el => el.dataset.label);
        const whenInput = document.querySelector('#home-log-when');
        const completedAt = whenInput?.value ? new Date(whenInput.value).toISOString() : null;
        // Extract the hour from the selected time so the calendar places the
        // log in the correct time slot instead of the catch-all Anytime row.
        const slotHour = whenInput?.value ? new Date(whenInput.value).getHours() : null;
        logChore(choreId, note, "", indicators, slotHour, completedAt).then(async (data) => {
          const logId = data?.log?.id;
          state.activeSheet     = null;
          state.activeSheetData = {};
          await loadLatestLogsData();
          render(app);
          if (logId) {
            const chore = (state.chores || []).find(c => c.id === choreId);
            showToastWithUndo(`${chore ? chore.icon + " " + chore.name : "Chore"}`, logId);
          }
        }).catch(() => showToast("Failed to log chore", "error"));
        break;
      }

      case "home-remove-chore": {
        e.preventDefault();
        e.stopPropagation();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        state.activeSheet = "confirm-remove-home-chore";
        state.activeSheetData = { choreId };
        render(app);
        break;
      }

      case "confirm-remove-home-chore": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const newHidden = [...new Set([...(state.hiddenHomeChoreIDs || []), choreId])];
        state.activeSheet = null;
        state.activeSheetData = {};
        // Optimistically update state and re-render; persist in the background.
        saveHiddenHomeChores(state, newHidden).then(() => render(app));
        render(app);
        break;
      }

      case "exit-jiggle-mode":
        e.preventDefault();
        state.jiggleMode = false;
        render(app);
        break;

      // ── Chores tab management ─────────────────────────────────────────────

      case "chore-add": {
        e.preventDefault();
        state.activeSheet = "chore-edit";
        state.activeSheetData = { choreId: null };
        render(app);
        break;
      }

      case "chore-edit": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        state.activeSheet = "chore-edit";
        state.activeSheetData = { choreId };
        render(app);
        break;
      }

      case "chore-toggle-home": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const hidden = new Set(state.hiddenHomeChoreIDs || []);
        if (hidden.has(choreId)) {
          hidden.delete(choreId);
        } else {
          hidden.add(choreId);
        }
        const newHidden = [...hidden];
        saveHiddenHomeChores(state, newHidden).then(() => render(app));
        render(app);
        break;
      }

      case "save-chore": {
        e.preventDefault();
        const isNew = actionEl.dataset.isNew === "true";
        const choreId = actionEl.dataset.choreId ? parseInt(actionEl.dataset.choreId, 10) : null;
        const nameEl = document.querySelector("#chore-edit-name");
        const name = (nameEl?.value || "").trim();
        if (!name) {
          nameEl?.focus();
          break;
        }
        const iconEl = document.querySelector("#chore-icon-input");
        const icon = (iconEl?.value || "").trim() || "📋";
        // Read the selected color swatch.
        const selectedSwatch = document.querySelector(".color-swatch--selected");
        const color = selectedSwatch?.dataset?.color || "#2E86AB";
        // Collect indicator labels (skip empty ones).
        const indicatorLabels = [...document.querySelectorAll(".indicator-label-input")]
          .map(el => el.value.trim())
          .filter(v => v.length > 0);

        if (isNew) {
          apiFetch("/api/chores", {
            method: "POST",
            body: JSON.stringify({ name, icon, color, category: "custom", indicatorLabels }),
          }).then(async ({ data }) => {
            const newChore = data?.chore;
            if (!newChore) { showToast("Failed to create chore", "error"); return; }
            state.activeSheet = null;
            state.activeSheetData = {};
            await loadChoreData();
            // Append new chore to order so it appears at the bottom.
            if (newChore.id) {
              const newOrder = [...(state.choreOrder || []), newChore.id];
              await saveChoreOrder(state, newOrder);
            }
            render(app);
            showToast(`${icon} ${name} added`, "success");
          }).catch(() => showToast("Failed to create chore", "error"));
        } else {
          apiFetch(`/api/chores/${choreId}`, {
            method: "PATCH",
            body: JSON.stringify({ name, icon, color, indicatorLabels }),
          }).then(async () => {
            state.activeSheet = null;
            state.activeSheetData = {};
            await loadChoreData();
            render(app);
            showToast("Chore updated", "success");
          }).catch(() => showToast("Failed to update chore", "error"));
        }
        break;
      }

      case "delete-chore": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const chore = (state.chores || []).find(c => c.id === choreId);
        if (!chore) break;
        // eslint-disable-next-line no-alert
        if (!confirm(`Delete "${chore.name}"? This cannot be undone.`)) break;
        apiFetch(`/api/chores/${choreId}`, { method: "DELETE" })
          .then(async ({ response }) => {
            if (!response.ok) { showToast("Cannot delete this chore", "error"); return; }
            state.activeSheet = null;
            state.activeSheetData = {};
            // Remove from chore order.
            state.choreOrder = (state.choreOrder || []).filter(id => id !== choreId);
            // Remove from hidden list.
            state.hiddenHomeChoreIDs = (state.hiddenHomeChoreIDs || []).filter(id => id !== choreId);
            await loadChoreData();
            render(app);
            showToast("Chore deleted", "info");
          })
          .catch(() => showToast("Failed to delete chore", "error"));
        break;
      }

      case "restore-chore-default": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const chore = (state.chores || []).find(c => c.id === choreId);
        if (!chore) break;
        // eslint-disable-next-line no-alert
        if (!confirm(`Restore "${chore.name}" to its original default values?`)) break;
        apiFetch(`/api/chores/${choreId}/restore-default`, { method: "POST" })
          .then(async ({ response }) => {
            if (!response.ok) { showToast("Could not restore default", "error"); return; }
            state.activeSheet = null;
            state.activeSheetData = {};
            await loadChoreData();
            render(app);
            showToast("Restored to default", "success");
          })
          .catch(() => showToast("Failed to restore default", "error"));
        break;
      }

      // ── Chore sheet: inline interactions ─────────────────────────────────

      case "pick-chore-color": {
        // Update selected swatch without a full re-render.
        document.querySelectorAll(".color-swatch").forEach(el => {
          const isSelected = el.dataset.color === actionEl.dataset.color;
          el.classList.toggle("color-swatch--selected", isSelected);
          el.setAttribute("aria-pressed", String(isSelected));
        });
        // Update the icon preview background.
        const preview = document.querySelector("#chore-icon-preview");
        if (preview) preview.style.background = actionEl.dataset.color;
        break;
      }

      case "pick-chore-emoji": {
        const emoji = actionEl.dataset.emoji;
        const iconInput = document.querySelector("#chore-icon-input");
        const preview = document.querySelector("#chore-icon-preview");
        if (iconInput) iconInput.value = emoji;
        if (preview) preview.textContent = emoji;
        break;
      }

      case "add-indicator-label": {
        e.preventDefault();
        const list = document.querySelector("#indicator-labels-list");
        if (!list) break;
        const idx = list.children.length;
        const row = document.createElement("div");
        row.className = "indicator-chip-row";
        row.dataset.index = idx;
        row.innerHTML = `<input type="text" class="indicator-label-input input" data-index="${idx}"
          value="" placeholder="e.g. 💩 poo" maxlength="30" />
          <button type="button" class="indicator-remove-btn"
            data-action="remove-indicator-label" data-index="${idx}"
            aria-label="Remove label">×</button>`;
        list.appendChild(row);
        row.querySelector("input")?.focus();
        break;
      }

      case "remove-indicator-label": {
        e.preventDefault();
        const row = actionEl.closest(".indicator-chip-row");
        if (row) row.remove();
        break;
      }

      case "load-more-history": {
        e.preventDefault();
        const before = state.historyBefore;
        if (!before) break;
        const btn = actionEl;
        btn.disabled = true;
        btn.textContent = "Loading...";
        loadMoreHistory(before).then(data => {
          state.historyLogs = [...(state.historyLogs || []), ...(data.logs || [])];
          state.historyHasMore = data.hasMore;
          state.historyBefore = data.start || null;
          render(app);
        }).catch(() => {
          btn.disabled = false;
          btn.textContent = "Load more";
        });
        break;
      }
    }
  });

  // ── Frequency selector: show/hide weekday pill row ─────────────────────────
  // Uses "change" (not "click") because <select> fires "change" on selection.
  document.addEventListener("change", (e) => {
    const actionEl = e.target.closest("[data-action]");
    if (actionEl?.dataset?.action === "change-frequency") {
      const sheet   = actionEl.closest(".bottom-sheet");
      const freqVal = actionEl.value;
      const wkRow   = sheet?.querySelector(".sheet-weekday-row");
      const intvRow = sheet?.querySelector(".sheet-interval-row");
      if (wkRow)   wkRow.hidden   = (freqVal !== "weekly");
      if (intvRow) intvRow.hidden = (freqVal !== "every_n_days");
    }
  });

  // Keep the "Every N days" option label in sync as the user edits the interval.
  // Also keep the emoji preview in sync with the chore icon input.
  document.addEventListener("input", (e) => {
    const input = e.target;
    // Interval sync
    if (input.classList.contains("interval-input")) {
      const sheet = input.closest(".bottom-sheet");
      const sel   = sheet?.querySelector("[data-action='change-frequency']");
      if (!sel) return;
      const opt = sel.options[sel.selectedIndex];
      if (opt?.value !== "every_n_days") return;
      const n = Math.max(2, parseInt(input.value || "2", 10));
      opt.textContent = `Every ${n} days`;
      return;
    }
    // Emoji input sync: update the large icon preview.
    if (input.id === "chore-icon-input") {
      const preview = document.querySelector("#chore-icon-preview");
      if (preview) preview.textContent = input.value || "📋";
    }
  });

  document.addEventListener("submit", (e) => {
    const form = e.target;
    const action = form.dataset.action;
    e.preventDefault();

    switch (action) {
      case "login":
        doLogin(form);
        break;
      case "register":
        doRegister(form);
        break;
      case "magic-link-request":
        doMagicLinkRequest(form);
        break;
      case "password-forgot":
        doForgotPassword(form);
        break;
      case "password-reset":
        doResetPassword(form);
        break;
      case "change-password":
        doChangePassword(form);
        break;
      case "create-household":
        doCreateHousehold(form);
        break;
      case "join-household":
        doJoinHousehold(form);
        break;
      case "new-chore-from-sheet":
        doCreateChoreFromSheet(form);
        break;
    }
  });

  try {
    await Promise.all([loadHouseholdData(), loadPreferences(state)]);
    if (state.household) {
      await Promise.all([
        loadChoreData(),
        loadTodayData(),
        loadLatestLogsData(),
        loadStatsData(),
        loadNotifData(),
      ]);
    }
  } catch {}

  // ── Drag and drop ──────────────────────────────────────────────────────────
  document.addEventListener("dragstart", e => {
    clearTimeout(longPressTimer);
    // Sheet chore reorder drag — must check before calendar card drag because
    // sheet items have [data-reorder-chore-id] but not [data-drag-chore-id].
    const reorderItem = e.target.closest("[data-reorder-chore-id]");
    if (reorderItem) {
      const choreId = parseInt(reorderItem.dataset.reorderChoreId, 10);
      e.dataTransfer.setData("text/plain", JSON.stringify({ reorderChoreId: choreId }));
      e.dataTransfer.effectAllowed = "move";
      reorderItem.classList.add("sheet-chore-item--dragging");
      return;
    }
    // Chores-tab list reorder drag.
    const choresTabItem = e.target.closest("[data-chores-tab-reorder-id]");
    if (choresTabItem) {
      const choreId = parseInt(choresTabItem.dataset.choresTabReorderId, 10);
      e.dataTransfer.setData("text/plain", JSON.stringify({ choresTabReorderId: choreId }));
      e.dataTransfer.effectAllowed = "move";
      choresTabItem.classList.add("chore-row--dragging");
      return;
    }
    // Home grid jiggle-mode reorder drag.
    const homeReorderItem = e.target.closest("[data-home-reorder-chore-id]");
    if (homeReorderItem) {
      const choreId = parseInt(homeReorderItem.dataset.homeReorderChoreId, 10);
      e.dataTransfer.setData("text/plain", JSON.stringify({ homeReorderChoreId: choreId }));
      e.dataTransfer.effectAllowed = "move";
      homeReorderItem.classList.add("home-chore-card--dragging");
      return;
    }
    // Calendar card drag.
    const card = e.target.closest("[data-drag-chore-id]");
    if (!card) return;
    e.dataTransfer.setData("text/plain", JSON.stringify({
      choreId:    parseInt(card.dataset.dragChoreId,    10),
      scheduleId: parseInt(card.dataset.dragScheduleId, 10) || null,
    }));
    card.classList.add("dragging");
  });

  document.addEventListener("dragend", e => {
    e.target.closest("[data-drag-chore-id]")?.classList.remove("dragging");
    e.target.closest("[data-reorder-chore-id]")?.classList.remove("sheet-chore-item--dragging");
    document.querySelectorAll(".sheet-chore-item--drag-over-top, .sheet-chore-item--drag-over-bottom")
      .forEach(el => el.classList.remove("sheet-chore-item--drag-over-top", "sheet-chore-item--drag-over-bottom"));
    e.target.closest("[data-home-reorder-chore-id]")?.classList.remove("home-chore-card--dragging");
    document.querySelectorAll(".home-chore-card--drag-over")
      .forEach(el => el.classList.remove("home-chore-card--drag-over"));
    e.target.closest("[data-chores-tab-reorder-id]")?.classList.remove("chore-row--dragging");
    document.querySelectorAll(".chore-row--drag-over-top, .chore-row--drag-over-bottom")
      .forEach(el => el.classList.remove("chore-row--drag-over-top", "chore-row--drag-over-bottom"));
  });

  document.addEventListener("dragover", e => {
    const cell = e.target.closest("[data-drop-period], [data-drop-hour]");
    if (cell) { e.preventDefault(); cell.classList.add("drop-target"); }
    // Sheet chore reorder: show insert position indicator.
    const item = e.target.closest("[data-reorder-chore-id]");
    if (item) {
      e.preventDefault();
      const rect = item.getBoundingClientRect();
      const half = rect.top + rect.height / 2;
      // Clear indicators on all siblings, then set the right one on this item.
      item.closest(".sheet-chore-list")
        ?.querySelectorAll("[data-reorder-chore-id]")
        .forEach(el => el.classList.remove("sheet-chore-item--drag-over-top", "sheet-chore-item--drag-over-bottom"));
      item.classList.add(e.clientY < half
        ? "sheet-chore-item--drag-over-top"
        : "sheet-chore-item--drag-over-bottom");
    }
    // Chores-tab list reorder: show insert position indicator.
    const choresTabItem = e.target.closest("[data-chores-tab-reorder-id]");
    if (choresTabItem) {
      e.preventDefault();
      const rect = choresTabItem.getBoundingClientRect();
      const half = rect.top + rect.height / 2;
      choresTabItem.closest("#chore-list")
        ?.querySelectorAll("[data-chores-tab-reorder-id]")
        .forEach(el => el.classList.remove("chore-row--drag-over-top", "chore-row--drag-over-bottom"));
      choresTabItem.classList.add(e.clientY < half
        ? "chore-row--drag-over-top"
        : "chore-row--drag-over-bottom");
    }
    // Home grid jiggle reorder: highlight target card.
    const homeItem = e.target.closest("[data-home-reorder-chore-id]");
    if (homeItem) {
      e.preventDefault();
      document.querySelectorAll(".home-chore-card--drag-over")
        .forEach(el => el.classList.remove("home-chore-card--drag-over"));
      homeItem.classList.add("home-chore-card--drag-over");
    }
  });

  document.addEventListener("dragleave", e => {
    const cell = e.target.closest(".drop-target");
    if (cell) {
      // Only remove when the cursor truly leaves the cell, not when it moves
      // into a child element (dragleave bubbles from children to the cell).
      if (!cell.contains(e.relatedTarget)) {
        cell.classList.remove("drop-target");
      }
    }
    // Sheet chore reorder: remove indicator when leaving the item.
    const item = e.target.closest("[data-reorder-chore-id]");
    if (item && !item.contains(e.relatedTarget)) {
      item.classList.remove("sheet-chore-item--drag-over-top", "sheet-chore-item--drag-over-bottom");
    }
    // Chores-tab reorder: remove indicator when leaving the row.
    const choresTabItem = e.target.closest("[data-chores-tab-reorder-id]");
    if (choresTabItem && !choresTabItem.contains(e.relatedTarget)) {
      choresTabItem.classList.remove("chore-row--drag-over-top", "chore-row--drag-over-bottom");
    }
  });

  document.addEventListener("drop", async e => {
    // ── Chores-tab list reorder ──────────────────────────────────────────────
    const choresTabTargetItem = e.target.closest("[data-chores-tab-reorder-id]");
    if (choresTabTargetItem) {
      e.preventDefault();
      document.querySelectorAll(".chore-row--drag-over-top, .chore-row--drag-over-bottom")
        .forEach(el => el.classList.remove("chore-row--drag-over-top", "chore-row--drag-over-bottom"));
      let payload;
      try { payload = JSON.parse(e.dataTransfer.getData("text/plain")); } catch { return; }
      if (!payload.choresTabReorderId) return;
      const draggedId = payload.choresTabReorderId;
      const targetId  = parseInt(choresTabTargetItem.dataset.choresTabReorderId, 10);
      if (draggedId === targetId) return;
      const rect = choresTabTargetItem.getBoundingClientRect();
      const insertBefore = e.clientY < rect.top + rect.height / 2;
      const sorted = sortChoresByOrder(state.chores, state.choreOrder);
      const ids = sorted.map(c => c.id);
      const fromIdx = ids.indexOf(draggedId);
      const toIdx   = ids.indexOf(targetId);
      if (fromIdx === -1 || toIdx === -1) return;
      ids.splice(fromIdx, 1);
      const insertIdx = ids.indexOf(targetId);
      ids.splice(insertBefore ? insertIdx : insertIdx + 1, 0, draggedId);
      await saveChoreOrder(state, ids);
      render(app);
      return;
    }

    // ── Home grid chore reorder ──────────────────────────────────────────────
    const homeTargetItem = e.target.closest("[data-home-reorder-chore-id]");
    if (homeTargetItem) {
      e.preventDefault();
      document.querySelectorAll(".home-chore-card--drag-over")
        .forEach(el => el.classList.remove("home-chore-card--drag-over"));
      let payload;
      try { payload = JSON.parse(e.dataTransfer.getData("text/plain")); } catch { return; }
      if (!payload.homeReorderChoreId) return;
      const draggedId = payload.homeReorderChoreId;
      const targetId  = parseInt(homeTargetItem.dataset.homeReorderChoreId, 10);
      if (draggedId === targetId) return;
      const sorted = sortChoresByOrder(state.chores, state.choreOrder);
      const ids = sorted.map(c => c.id);
      const fromIdx = ids.indexOf(draggedId);
      if (fromIdx === -1 || ids.indexOf(targetId) === -1) return;
      ids.splice(fromIdx, 1);
      const insertIdx = ids.indexOf(targetId);
      ids.splice(insertIdx, 0, draggedId);
      await saveChoreOrder(state, ids);
      render(app);
      return;
    }

    // ── Sheet chore reorder ──────────────────────────────────────────────────
    const targetItem = e.target.closest("[data-reorder-chore-id]");
    if (targetItem) {
      e.preventDefault();
      document.querySelectorAll(".sheet-chore-item--drag-over-top, .sheet-chore-item--drag-over-bottom")
        .forEach(el => el.classList.remove("sheet-chore-item--drag-over-top", "sheet-chore-item--drag-over-bottom"));
      let payload;
      try { payload = JSON.parse(e.dataTransfer.getData("text/plain")); } catch { return; }
      if (!payload.reorderChoreId) return;
      const draggedId = payload.reorderChoreId;
      const targetId  = parseInt(targetItem.dataset.reorderChoreId, 10);
      if (draggedId === targetId) return;
      const rect = targetItem.getBoundingClientRect();
      const insertBefore = e.clientY < rect.top + rect.height / 2;
      // Build new order from the currently-displayed (sorted) chore list.
      const sorted = sortChoresByOrder(state.chores, state.choreOrder);
      const ids = sorted.map(c => c.id);
      const fromIdx = ids.indexOf(draggedId);
      const toIdx   = ids.indexOf(targetId);
      if (fromIdx === -1 || toIdx === -1) return;
      ids.splice(fromIdx, 1);
      const insertIdx = ids.indexOf(targetId);
      ids.splice(insertBefore ? insertIdx : insertIdx + 1, 0, draggedId);
      await saveChoreOrder(state, ids);
      render(app);
      return;
    }

    // ── Calendar schedule drop ───────────────────────────────────────────────
    const cell = e.target.closest("[data-drop-period], [data-drop-hour]");
    if (!cell) return;
    e.preventDefault();
    cell.classList.remove("drop-target");
    let payload;
    try { payload = JSON.parse(e.dataTransfer.getData("text/plain")); }
    catch { return; }
    const { choreId, scheduleId } = payload;
    const newPeriod = cell.dataset.dropPeriod || "anytime";
    const newHour   = cell.dataset.dropHour != null
      ? `${String(cell.dataset.dropHour).padStart(2, "0")}:00`
      : null;
    try {
      if (scheduleId) {
        // Move an existing schedule to the new time slot (PATCH preserves all
        // other fields including isActive, frequencyType, etc.).
        await updateSchedule(scheduleId, {
          timePeriod:   newPeriod,
          specificTime: newHour,
        });
      } else {
        // Unscheduled chore dragged into a slot — create a new "once" schedule
        // for the drop target's date (shown on that specific day only).
        const dropDate = cell.dataset.dropDate || state.calendarDate || null;
        await createSchedule({
          choreId,
          timePeriod:    newPeriod,
          specificTime:  newHour,
          frequencyType: "once",
          startDate:     dropDate,
          isActive:      true,
        });
      }
      state.schedules = await loadSchedules();
      render(app);
    } catch { showToast("Failed to schedule chore", "error"); }
  });

  // ── Long-press to log a chore (with indicators/note sheet) ──────────────
  function openLogSheet(card) {
    const choreId = parseInt(card.dataset.dragChoreId, 10);
    const logId   = card.dataset.logId ? parseInt(card.dataset.logId, 10) : null;
    const date    = card.dataset.date || state.calendarDate || "";
    state.activeSheet     = "log";
    state.activeSheetData = { choreId, logId, date };
    render(app);
  }

  // Long-press on a chore item inside a bottom sheet (pick-chore / quick-log)
  // opens the log detail sheet so the user can add notes and indicator chips
  // before saving — without scheduling the chore.
  function openLogSheetFromItem(item) {
    const choreId  = parseInt(item.dataset.choreId, 10);
    const date     = item.dataset.date || state.calendarDate || "";
    const slotHour = state.activeSheetData?.hour ?? null;
    state.activeSheet     = "log";
    state.activeSheetData = { choreId, logId: null, date, slotHour };
    render(app);
  }

  function cancelPress() {
    clearTimeout(longPressTimer);
    longPressTimer = null;
    document.querySelectorAll(".chore-card--pressing")
      .forEach(el => el.classList.remove("chore-card--pressing"));
    document.querySelectorAll(".sheet-chore-item--pressing")
      .forEach(el => el.classList.remove("sheet-chore-item--pressing"));
    document.querySelectorAll(".home-chore-card--pressing")
      .forEach(el => el.classList.remove("home-chore-card--pressing"));
  }

  document.addEventListener("mousedown", e => {
    const card = e.target.closest("[data-drag-chore-id]");
    const item = !card && e.target.closest(".sheet-chore-item");
    // Only trigger long-press on home cards that are NOT in jiggle/reorder mode.
    const homeCard = !card && !item && e.target.closest(".home-chore-card:not(.home-chore-card--jiggle)");
    if (!card && !item && !homeCard) return;
    pressStartX = e.clientX;
    pressStartY = e.clientY;
    if (card) card.classList.add("chore-card--pressing");
    if (item) item.classList.add("sheet-chore-item--pressing");
    if (homeCard) homeCard.classList.add("home-chore-card--pressing");
    longPressTimer = setTimeout(() => {
      longPressJustFired = true;
      if (card) { card.classList.remove("chore-card--pressing"); openLogSheet(card); }
      if (item) { item.classList.remove("sheet-chore-item--pressing"); openLogSheetFromItem(item); }
      if (homeCard) {
        homeCard.classList.remove("home-chore-card--pressing");
        state.jiggleMode = true;
        render(app);
      }
    }, 500);
  });
  // Cancel on actual cursor movement (>8px) — but NOT on DOM-triggered mouseleave
  // events that fire when morphInnerHTML removes elements from under the cursor.
  document.addEventListener("mousemove", e => {
    if (!longPressTimer) return;
    const dx = e.clientX - pressStartX;
    const dy = e.clientY - pressStartY;
    if (Math.hypot(dx, dy) > 8) cancelPress();
  });
  document.addEventListener("mouseup", e => {
    cancelPress();
    if (longPressJustFired) {
      // The DOM changed during the long-press (backdrop appeared), so the
      // browser may not synthesize a click event at all (mousedown target ≠
      // mouseup target).  If no click fires, longPressJustFired would stay
      // true forever and block the next intentional click (e.g. the save
      // button in the edit sheet).  Reset it after a short delay so any
      // genuinely synthesized residual click is still swallowed, but the
      // next real user click is never blocked.
      setTimeout(() => { longPressJustFired = false; }, 50);
    }
  });

  // { passive: false } is required to call e.preventDefault() — without it,
  // Chrome (and Android WebView) treats document-level touch listeners as
  // passive by default (since Chrome 56) and silently ignores preventDefault.
  // iOS Safari also adopted this default.
  //
  // For bottom-sheet cards/items we still preventDefault() immediately so that
  // their scroll-contained sheet doesn't fight the outer page scroll.
  //
  // For home-grid cards in normal mode we deliberately skip preventDefault()
  // here so the browser can start a vertical scroll freely.  We call
  // preventDefault() in touchend instead to suppress the synthesised click
  // (we fire our own for short taps).  In jiggle mode we must consume the
  // whole sequence so the card drag doesn't become a page scroll.
  document.addEventListener("touchstart", e => {
    const card = e.target.closest("[data-drag-chore-id]");
    const item = !card && e.target.closest(".sheet-chore-item");
    const homeCard = !card && !item && e.target.closest(".home-chore-card");
    if (!card && !item && !homeCard) return;
    if (card || item || (homeCard && state.jiggleMode)) e.preventDefault();
    const t = e.touches[0];
    pressStartX = t.clientX;
    pressStartY = t.clientY;
    // In jiggle mode a touch on a home card starts a drag — handled in touchmove/touchend.
    if (homeCard && state.jiggleMode) {
      // If the user tapped the X (remove) button, skip drag setup so touchend
      // can synthesize the click normally.
      if (e.target.closest("[data-action='home-remove-chore']")) {
        return;
      }
      jiggleDrag.active = true;
      jiggleDrag.choreId = parseInt(homeCard.dataset.homeChoreId, 10);
      jiggleDrag.targetChoreId = null;
      homeCard.closest("[data-home-reorder-chore-id]")?.classList.add("home-chore-card--dragging");
      return;
    }
    if (card) card.classList.add("chore-card--pressing");
    if (item) item.classList.add("sheet-chore-item--pressing");
    if (homeCard) homeCard.classList.add("home-chore-card--pressing");
    longPressTimer = setTimeout(() => {
      longPressJustFired = true;
      if (card) { card.classList.remove("chore-card--pressing"); openLogSheet(card); }
      if (item) { item.classList.remove("sheet-chore-item--pressing"); openLogSheetFromItem(item); }
      if (homeCard) {
        homeCard.classList.remove("home-chore-card--pressing");
        state.jiggleMode = true;
        render(app);
      }
    }, 500);
  }, { passive: false });
  document.addEventListener("touchend", e => {
    // ── Jiggle drag end ──────────────────────────────────────────────────────
    if (jiggleDrag.active) {
      document.querySelectorAll(".home-chore-card--dragging")
        .forEach(el => el.classList.remove("home-chore-card--dragging"));
      document.querySelectorAll(".home-chore-card--drag-over")
        .forEach(el => el.classList.remove("home-chore-card--drag-over"));
      const draggedId = jiggleDrag.choreId;
      const targetId  = jiggleDrag.targetChoreId;
      jiggleDrag.active = false;
      jiggleDrag.choreId = null;
      jiggleDrag.targetChoreId = null;
      if (targetId && targetId !== draggedId) {
        const sorted = sortChoresByOrder(state.chores, state.choreOrder);
        const ids = sorted.map(c => c.id);
        const fromIdx = ids.indexOf(draggedId);
        if (fromIdx !== -1 && ids.indexOf(targetId) !== -1) {
          ids.splice(fromIdx, 1);
          const insertIdx = ids.indexOf(targetId);
          ids.splice(insertIdx, 0, draggedId);
          saveChoreOrder(state, ids).then(() => render(app));
        }
      }
      return;
    }

    const fired = longPressJustFired;
    cancelPress();

    // Detect the touched element early so we can suppress the browser's
    // synthesised click for home cards in all cases (tap, long-press, scroll).
    // For card/item the touchstart already called preventDefault(), so the
    // browser won't synthesise a click and we don't need to here.
    const card = e.target.closest("[data-drag-chore-id]");
    const item = !card && e.target.closest(".sheet-chore-item");
    const homeCard = !card && !item && e.target.closest(".home-chore-card");
    // touchstart skipped preventDefault() for normal-mode home cards so the
    // browser could scroll; prevent the synthesised mouse events here instead.
    if (homeCard) e.preventDefault();

    if (fired) {
      // Long press was handled; allow a short grace period so any stray
      // synthesised event (on browsers that still fire one) is swallowed.
      setTimeout(() => { longPressJustFired = false; }, 50);
      return;
    }
    // For a short tap (finger barely moved) fire a click manually so the
    // existing data-action handler processes it.
    if (!card && !item && !homeCard) return;
    const t = e.changedTouches[0];
    if (!t) return;
    const dx = t.clientX - pressStartX;
    const dy = t.clientY - pressStartY;
    if (Math.hypot(dx, dy) <= 8) {
      (e.target.closest("[data-action]") || e.target).click();
    }
  });
  document.addEventListener("touchmove", e => {
    if (!longPressTimer) return;
    const t = e.touches[0];
    const dx = t.clientX - pressStartX;
    const dy = t.clientY - pressStartY;
    if (Math.hypot(dx, dy) > 8) cancelPress();
  }, { passive: true });

  // Jiggle-mode touch drag: separate listener so we can call preventDefault()
  // without making the regular touchmove passive listener non-passive.
  document.addEventListener("touchmove", e => {
    if (!jiggleDrag.active) return;
    e.preventDefault();
    const t = e.touches[0];
    const el = document.elementFromPoint(t.clientX, t.clientY);
    const targetCard = el?.closest("[data-home-reorder-chore-id]");
    document.querySelectorAll(".home-chore-card--drag-over")
      .forEach(c => c.classList.remove("home-chore-card--drag-over"));
    if (targetCard) {
      const tid = parseInt(targetCard.dataset.homeReorderChoreId, 10);
      jiggleDrag.targetChoreId = tid;
      if (tid !== jiggleDrag.choreId) targetCard.classList.add("home-chore-card--drag-over");
    } else {
      jiggleDrag.targetChoreId = null;
    }
  }, { passive: false });

  // ── Notification polling ─────────────────────────────────────────────────
  // Poll every 30 s for new notifications.  Pause while the tab is hidden so
  // we don't hammer the API in background tabs.
  let notifPollTimer = null;
  function startNotifPoll() {
    if (notifPollTimer) clearInterval(notifPollTimer);
    notifPollTimer = setInterval(() => {
      if (!document.hidden && state.user) {
        loadNotifData().then(() => updateTopBar());
      }
    }, 30000);
  }
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden && state.user) {
      loadNotifData().then(() => updateTopBar());
    }
  });
  if (state.user) startNotifPoll();

  render(app);
}

async function loadHouseholdData() {
  try {
    const data = await loadHousehold();
    if (data.household) {
      state.household = data.household;
      state.members = data.members;
      state.invites = data.invites;
    }
  } catch {}
}

async function loadChoreData() {
  try {
    const data = await loadChores();
    if (data.chores) {
      state.chores = data.chores;
    }
  } catch {}
}

async function loadTodayData() {
  try {
    const date = state.calendarDate || state.todayDate || todayISO(0);
    const [todayResult, scheduleList] = await Promise.all([
      loadToday(date),
      loadSchedules(),
    ]);
    state.todayLogs = todayResult.logs || [];
    state.dailySummary = todayResult.summary;
    state.schedules = scheduleList;
  } catch {}
}

async function loadWeekData() {
  try {
    const date = state.calendarDate || todayISO(0);
    const d = new Date(date + "T00:00:00");
    const day = d.getDay();
    d.setDate(d.getDate() + (day === 0 ? -6 : 1 - day));
    const weekStart = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
    const [weekResult, scheduleList] = await Promise.all([
      loadWeek(weekStart),
      loadSchedules(),
    ]);
    state.weekLogs = weekResult.logs || [];
    state.schedules = scheduleList;
  } catch {}
}

async function doCreateHousehold(form) {
  const name = form.querySelector("#hh-name").value;
  const data = await createHousehold(name);
  if (data.household) {
    state.household = data.household;
    await loadHouseholdData();
    await seedDefaultChores();
    await loadChoreData();
    await loadTodayData();
    state.currentRoute = "/";
    render(document.querySelector("#app"));
  }
}

async function seedDefaultChores() {
  try {
    await apiFetch("/api/chores/seed-defaults", { method: "POST" });
  } catch {}
}

async function doJoinHousehold(form) {
  const code = form.querySelector("#invite-code").value;
  const data = await joinHousehold(code);
  if (data.household) {
    state.household = data.household;
    await loadHouseholdData();
    await loadChoreData();
    await loadTodayData();
    state.currentRoute = "/";
    render(document.querySelector("#app"));
  }
}

async function doCreateChoreFromSheet(form) {
  const name      = form.querySelector('[name="choreName"]').value.trim();
  if (!name) return;

  try {
    const { data: choreData } = await apiFetch("/api/chores", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
    const newChore = choreData?.chore;
    if (!newChore) { showToast("Failed to create chore", "error"); return; }

    const timeInput    = document.querySelector("#sheet-time");
    const specificTime = timeInput?.value || null;
    const slotDate     = form.querySelector('[name="date"]')?.value || state.activeSheetData?.date || null;
    const freqPayload  = readSheetFreq("sheet", slotDate);
    await createSchedule({
      choreId:       newChore.id,
      timePeriod:    "anytime",
      specificTime,
      isActive:      true,
      ...freqPayload,
    });

    await loadChoreData();
    // Append new chore to the user's custom order so it appears at the bottom
    // of the sheet list rather than being sorted to an arbitrary position.
    if (newChore.id) {
      const newOrder = [...(state.choreOrder || []), newChore.id];
      await saveChoreOrder(state, newOrder);
    }
    state.schedules = await loadSchedules();
    state.activeSheet     = null;
    state.activeSheetData = {};
    const app = document.querySelector("#app");
    if (app) render(app);
  } catch {
    showToast("Failed to create chore", "error");
  }
}

function bootstrap() {
  init().catch(() => {});
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", bootstrap);
} else {
  bootstrap();
}
