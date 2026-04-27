import { createAppState, resetAuthedState } from "./state.js";
import { morphInnerHTML } from "./morph.js";
import { apiMe } from "./api.js";
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
import { loadToday, logChore, undoLog, loadChores, renderTodayView as renderTodayViewImpl, renderHistoryView as renderHistoryPage, todayISO } from "./today.js";
import { renderStatsView, loadLeaderboard, loadStreaks, loadBreakdown, loadRecap } from "./stats.js";

let state;

export function render(root) {
  const route = state.currentRoute || window.location.pathname || "/";
  let html = "";

  if (route.startsWith("/verify-email")) {
    const url = new URL(window.location.href);
    const token = url.searchParams.get("token");
    if (token) {
      html = renderVerifyEmailView(true);
      verifyEmail(token);
    } else {
      html = renderVerifyEmailView(false);
    }
  } else if (route.startsWith("/magic-login")) {
    const url = new URL(window.location.href);
    const token = url.searchParams.get("token");
    if (token) {
      html = renderMagicLinkNoticeView();
      consumeMagicLink(token);
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
  return renderTodayViewImpl(state);
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
    const [lb, st, br, rp] = await Promise.all([
      loadLeaderboard("week"),
      loadStreaks(),
      loadBreakdown(),
      loadRecap(),
    ]);
    state.stats = {
      leaderboard: lb.leaderboard || [],
      streaks: st.streaks || {},
      breakdown: br.breakdown || [],
      recap: rp.recap || {},
    };
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
  } else {
    topBar.hidden = true;
    tabs.hidden = true;
  }
}

function escapeHTML(str) {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
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
      await loadChoreData();
      await loadTodayData();
      loadStatsData();
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
        state.todayDate = actionEl.dataset.date;
        loadTodayData().then(() => render(app));
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
    }
  });

  try {
    await loadHouseholdData();
    if (state.household) {
      await loadChoreData();
      await loadTodayData();
      loadStatsData();
    }
  } catch {}
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
    const date = state.todayDate || todayISO(0);
    const data = await loadToday(date);
    state.todayLogs = data.logs || [];
    state.dailySummary = data.summary;
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
    await fetch("/api/chores/seed-defaults", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": (() => {
          const m = document.cookie.match(/(?:^|;\s*)choresy_csrf=([^;]*)/);
          return m ? m[1] : "";
        })(),
      },
      body: JSON.stringify({
        names: [
          "Feed Cats (Morning)", "Feed Cats (Evening)", "Feed Baby",
          "Change Baby", "Water Plants", "Clean Litter Box",
          "Take Out Trash", "Wash Dishes", "Vacuum",
          "Laundry", "Walk Dog", "Make Bed",
        ],
      }),
    });
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

function bootstrap() {
  init().catch(() => {});
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", bootstrap);
} else {
  bootstrap();
}
