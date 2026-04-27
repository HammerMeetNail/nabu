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

let state;

export function render(root) {
  const route = state.currentRoute || "/";
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

function renderTodayView() {
  if (!state.household) {
    return `<div class="card mt-3">
      <h2>Welcome!</h2>
      <p>${state.user ? `Hi ${escapeHTML(state.user.email)}! ` : ''}Set up your household to get started with Choresy.</p>
      <div class="mt-2">
        <button type="button" class="btn btn-primary" data-nav="settings">Set Up Household</button>
      </div>
    </div>`;
  }
  return `<div class="today-view">
    <h2>Today</h2>
    <div class="empty-state">
      <div class="empty-state-icon">🏠</div>
      <div class="empty-state-title">No chores yet</div>
      <p>Your household chores will appear here.</p>
    </div>
  </div>`;
}

function renderChoresView() {
  return `<div class="chores-view">
    <h2>Chores</h2>
    <div class="empty-state">
      <div class="empty-state-icon">📋</div>
      <div class="empty-state-title">No chores yet</div>
      <p>Add chores for your household to get started.</p>
    </div>
  </div>`;
}

function renderHistoryView() {
  return `<div class="history-view">
    <h2>History</h2>
    <div class="empty-state">
      <div class="empty-state-icon">📊</div>
      <div class="empty-state-title">No history yet</div>
      <p>Completed chores will appear here.</p>
    </div>
  </div>`;
}

function renderSettingsView() {
  const hh = state.household;
  if (!hh) {
    return renderHouseholdView(null);
  }
  return `<div class="settings-view"><h2>Settings</h2>${renderHouseholdView(hh, state.members, state.invites)}<div class="card mt-3"><h3>Account</h3><p class="text-secondary">${escapeHTML(state.user.email)}</p><button type="button" class="btn btn-sm btn-secondary mt-2" data-action="logout">Sign Out</button></div></div>`;
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
    const app = document.querySelector("#app");
    if (app) render(app);
  } else {
    setError("#login-error", data.error || "Invalid email or password");
  }
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

  state.user = await loadSession();

  const app = document.querySelector("#app");
  if (!app) return;

  document.addEventListener("click", (e) => {
    const actionEl = e.target.closest("[data-action]");
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
          render(app);
        });
        break;
    }

    const nav = e.target.closest("[data-nav]");
    if (nav) {
      e.preventDefault();
      state.currentRoute = `/${nav.dataset.nav}`;
      if (state.currentRoute === "/settings") {
        state._loadedHousehold = true;
      }
      render(app);
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

  await loadHouseholdData();
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

async function doCreateHousehold(form) {
  const name = form.querySelector("#hh-name").value;
  const data = await createHousehold(name);
  if (data.household) {
    state.household = data.household;
    await loadHouseholdData();
    render(document.querySelector("#app"));
  }
}

async function doJoinHousehold(form) {
  const code = form.querySelector("#invite-code").value;
  const data = await joinHousehold(code);
  if (data.household) {
    state.household = data.household;
    await loadHouseholdData();
    render(document.querySelector("#app"));
  }
}
