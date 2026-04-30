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
  renderLoginView,
  renderRegisterView,
  renderMagicLinkRequestView,
  renderMagicLinkNoticeView,
  renderVerifyEmailView,
  renderForgotPasswordView,
  renderResetPasswordView,
} from "./auth.js";
import { loadHousehold, createHousehold, joinHousehold, createInvite, deleteInvite, leaveHousehold, renderHouseholdView } from "./household.js";
import { loadToday, loadWeek, logChore, undoLog, loadChores, loadHistory, renderHistoryView as renderHistoryPage, todayISO } from "./today.js";
import { renderStatsView, loadOverview } from "./stats.js";
import { renderDayView, renderWeekView } from "./calendar.js";
import { loadSchedules, createSchedule, updateSchedule, renderPickChoreSheet } from "./schedule.js";

let state;

export function render(root) {
  const route = state.currentRoute || window.location.pathname || "/";
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
  } else if (!state.user) {
    switch (route) {
      case "/register":
        html = renderRegisterView();
        break;
      case "/magic-link":
        html = renderMagicLinkRequestView();
        break;
      case "/forgot-password":
        html = renderForgotPasswordView();
        break;
      default:
        html = renderLoginView();
    }
  } else {
    switch (route) {
      case "/":
      case "/today":
        html = renderTodayView();
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
        html = renderTodayView();
    }
  }

  morphInnerHTML(root, html);
  updateTabs(route);
  updateTopBar();
}

function renderChoresView() {
  const chores = state.chores || [];
  if (chores.length === 0) {
    return `<div class="chores-view"><h2>Chores</h2>
    <div class="empty-state"><div class="empty-state-icon">📋</div><div class="empty-state-title">No chores yet</div>
    <p>Use settings to set up your household chores.</p></div></div>`;
  }
  const grid = chores.map(c => `<div class="card mb-2" style="border-left:4px solid ${c.color}">
    <span style="font-size:1.5rem">${c.icon}</span> <strong>${escapeHTML(c.name)}</strong>
    <span class="text-secondary"> — ${c.category}</span>
    ${c.isPredefined ? '<span class="role-badge">built-in</span>' : ''}
  </div>`).join("");
  return `<div class="chores-view"><h2>Chores</h2><div class="mt-3">${grid}</div></div>`;
}

function renderHistoryView() {
  return renderHistoryPage(state);
}

function renderTodayView() {
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

  if (state.activeSheet === "pick-chore") {
    const sheetHTML = renderPickChoreSheet(
      state.chores,
      state.activeSheetData || {},
      state.schedules || []
    );
    return `<div class="sheet-overlay-wrapper">
      ${mainView}
      <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
      ${sheetHTML}
    </div>`;
  }
  return mainView;
}

function renderSettingsView() {
  const hh = state.household;
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
  if (!hh) {
    return `<div class="settings-view">${renderHouseholdView(null)}<div class="card mt-3"><h3>Account</h3><p class="text-secondary">${escapeHTML(state.user ? state.user.email : '')}</p><button type="button" class="btn btn-sm btn-secondary mt-2" data-action="logout">Sign Out</button></div></div>`;
  }
  return `<div class="settings-view"><h2>Settings</h2>${renderHouseholdView(hh, state.members, state.invites)}<div class="card mt-3"><h3>Account</h3><p class="text-secondary">${escapeHTML(state.user ? state.user.email : '')}</p><button type="button" class="btn btn-sm btn-secondary mt-2" data-action="logout">Sign Out</button></div>${statsHTML}</div>`;
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
    if (badge && state.unreadNotifications > 0) {
      badge.hidden = false;
      badge.textContent = String(state.unreadNotifications);
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
  const email = form.querySelector("#login-email").value;
  const password = form.querySelector("#login-password").value;
  const { ok, data } = await handleLogin(email, password);
  if (ok && data.user) {
    state.user = data.user;
    state.currentRoute = "/";
    await reloadAfterAuth();
    const app = document.querySelector("#app");
    if (app) render(app);
  } else {
    setError("#login-error", data.error || "Invalid email or password");
  }
}

async function reloadAfterAuth() {
  try {
    await loadHouseholdData();
    if (state.household) {
      await Promise.all([
        loadChoreData(),
        loadTodayData(),
        loadStatsData(),
      ]);
    }
  } catch {}
}

async function doRegister(form) {
  hideError("#register-error");
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
    await reloadAfterAuth();
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

async function verifyEmail(token) {
  const csrfToken = document.cookie.match(/(?:^|;\s*)choresy_csrf=([^;]*)/)?.[1] || "";
  await fetch(`/api/auth/email/verify?token=${encodeURIComponent(token)}`, {
    headers: { "X-CSRF-Token": csrfToken },
  });
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

  try {
    state.user = await loadSession();
  } catch {
    state.user = null;
  }

  const app = document.querySelector("#app");
  if (!app) return;

  document.addEventListener("click", (e) => {
    const actionEl = e.target.closest("[data-action]");

    // data-nav SPA navigation: check first so it works without data-action
    const navEl = e.target.closest("[data-nav]");
    if (navEl) {
      e.preventDefault();
      state.currentRoute = `/${navEl.dataset.nav}`;
      if (state.currentRoute === "/settings") {
        state._loadedHousehold = true;
      }
      if (state.currentRoute === "/history") {
        loadHistory().then(data => {
          state.historyLogs = data.logs || [];
          render(app);
        });
        return;
      }
      render(app);
      return;
    }

    const action = actionEl?.dataset?.action;
    if (!action) return;

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
      case "create-invite":
        e.preventDefault();
        createInvite().then((data) => {
          if (data.invite) {
            showToast("Invite created: " + data.invite.code, "info");
            render(app);
          }
        });
        break;
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
        logChore(parseInt(actionEl.dataset.choreId), "").then(async () => {
          await loadTodayData();
          render(app);
        });
        break;
      case "undo-chore":
        e.preventDefault();
        undoLog(parseInt(actionEl.dataset.logId)).then(async () => {
          await loadTodayData();
          render(app);
        }).catch((err) => {
          console.error('undo-chore failed:', err);
          loadTodayData().then(() => render(app));
        });
        break;
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
          date:       actionEl.dataset.date,
          timePeriod: actionEl.dataset.timePeriod,
          hour:       actionEl.dataset.hour ? parseInt(actionEl.dataset.hour, 10) : null,
        };
        render(app);
        break;

      case "schedule-chore-here": {
        e.preventDefault();
        const choreId    = parseInt(actionEl.dataset.choreId, 10);
        const timePeriod = actionEl.dataset.timePeriod;
        const rawHour    = actionEl.dataset.specificHour;
        const specificTime = rawHour
          ? `${String(rawHour).padStart(2, "0")}:00`
          : null;
        createSchedule({
          choreId,
          timePeriod,
          specificTime,
          frequencyType: "daily",
          isActive: true,
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
    await loadHouseholdData();
    if (state.household) {
      await Promise.all([
        loadChoreData(),
        loadTodayData(),
        loadStatsData(),
      ]);
    }
  } catch {}

  // ── Drag and drop ──────────────────────────────────────────────────────────
  document.addEventListener("dragstart", e => {
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
  });

  document.addEventListener("dragover", e => {
    const cell = e.target.closest("[data-drop-period], [data-drop-hour]");
    if (cell) { e.preventDefault(); cell.classList.add("drop-target"); }
  });

  document.addEventListener("dragleave", e => {
    e.target.closest(".drop-target")?.classList.remove("drop-target");
  });

  document.addEventListener("drop", async e => {
    const cell = e.target.closest("[data-drop-period], [data-drop-hour]");
    if (!cell) return;
    e.preventDefault();
    cell.classList.remove("drop-target");
    let payload;
    try { payload = JSON.parse(e.dataTransfer.getData("text/plain")); }
    catch { return; }
    const { choreId, scheduleId } = payload;
    const newPeriod = cell.dataset.dropPeriod || cell.dataset.timePeriod || "anytime";
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
        // Unscheduled chore dragged into a slot — create a new schedule.
        await createSchedule({
          choreId,
          timePeriod:    newPeriod,
          specificTime:  newHour,
          frequencyType: "daily",
          isActive:      true,
        });
      }
      state.schedules = await loadSchedules();
      render(app);
    } catch { showToast("Failed to schedule chore", "error"); }
  });

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
    const weekStart = d.toISOString().split("T")[0];
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
  const timePeriod = form.querySelector('[name="timePeriod"]').value;
  const rawHour   = form.querySelector('[name="specificHour"]').value;
  if (!name) return;

  try {
    const { data: choreData } = await apiFetch("/api/chores", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
    const newChore = choreData?.chore;
    if (!newChore) { showToast("Failed to create chore", "error"); return; }

    const specificTime = rawHour ? `${String(rawHour).padStart(2, "0")}:00` : null;
    await createSchedule({
      choreId:       newChore.id,
      timePeriod,
      specificTime,
      frequencyType: "daily",
      isActive:      true,
    });

    await loadChoreData();
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
