import { createAppState, resetAuthedState } from "./state.js";
import { morphInnerHTML } from "./morph.js";
import { apiMe, apiLogin, apiRegister, apiLogout } from "./api.js";

let state;

export function render(root) {
  const route = state.currentRoute || "/";
  let html = "";

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

  morphInnerHTML(root, html);
  updateTabs(route);
  updateTopBar();
}

function renderTodayView() {
  if (!state.user) {
    return renderLoginForm();
  }
  return `<div class="today-view">
    <h2>Today</h2>
    <p>Welcome, ${escapeHTML(state.user.email)}</p>
    <div class="empty-state">
      <div class="empty-state-icon">🏠</div>
      <div class="empty-state-title">No chores logged today</div>
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
  return `<div class="settings-view">
    <h2>Settings</h2>
    <p>Household and account settings will appear here.</p>
  </div>`;
}

function renderLoginForm() {
  return `<div class="auth-card">
    <h1 class="auth-title">Choresy</h1>
    <form id="login-form" data-action="login">
      <div class="form-group">
        <label class="form-label" for="login-email">Email</label>
        <input id="login-email" type="email" name="email" required autocomplete="email">
      </div>
      <div class="form-group">
        <label class="form-label" for="login-password">Password</label>
        <input id="login-password" type="password" name="password" required minlength="8" autocomplete="current-password">
      </div>
      <button type="submit" class="btn btn-primary btn-block">Sign In</button>
    </form>
    <div class="auth-divider">or</div>
    <button type="button" class="btn btn-secondary btn-block" data-action="show-register">Create Account</button>
  </div>`;
}

function updateTabs(route) {
  const tabs = document.querySelector("#bottom-tabs");
  if (!tabs) return;
  tabs.querySelectorAll(".tab-item").forEach((tab) => {
    tab.classList.toggle("active", tab.dataset.nav === route.slice(1) || (route === "/" && tab.dataset.nav === "today"));
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

async function handleLogin(form) {
  const email = form.querySelector("#login-email").value;
  const password = form.querySelector("#login-password").value;
  const { response, data } = await apiLogin(email, password);
  if (response.ok && data.user) {
    state.user = data.user;
    const app = document.querySelector("#app");
    if (app) render(app);
  } else {
    showToast("Invalid email or password", "error");
  }
}

async function loadSession() {
  const { response, data } = await apiMe();
  if (response.ok && data.user) {
    state.user = data.user;
  }
}

function showToast(message, type = "info") {
  const container = document.querySelector("#toast-container");
  if (!container) return;
  const toast = document.createElement("div");
  toast.className = `toast toast-${type}`;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 3000);
}

export async function init() {
  state = createAppState();
  await loadSession();

  const app = document.querySelector("#app");
  if (!app) return;

  document.addEventListener("DOMContentLoaded", () => render(app));

  document.addEventListener("click", (e) => {
    const action = e.target.closest("[data-action]")?.dataset?.action;
    switch (action) {
      case "login": {
        e.preventDefault();
        const form = e.target.closest("form");
        if (form) handleLogin(form);
        break;
      }
      case "logout": {
        e.preventDefault();
        apiLogout().then(() => {
          resetAuthedState(state);
          render(app);
        });
        break;
      }
    }
    const nav = e.target.closest("[data-nav]");
    if (nav) {
      e.preventDefault();
      state.currentRoute = `/${nav.dataset.nav}`;
      render(app);
    }
  });

  document.addEventListener("submit", (e) => {
    const form = e.target;
    const action = form.dataset.action;
    if (action === "login") {
      e.preventDefault();
      handleLogin(form);
    }
  });

  render(app);
}
