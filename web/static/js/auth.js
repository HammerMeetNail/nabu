import { createAppState, resetAuthedState } from "../state.js";
import { apiMe, apiFetch, getCSRFToken } from "../api.js";

export async function handleLogin({ state, apiFetch, email, password }) {
  return await apiLogin(email, password);
}

export async function loadSession() {
  return await apiMe();
}

export function renderLoginView() {
  return `<div class="auth-card">
    <h1 class="auth-title">Choresy</h1>
    <form id="login-form" data-action="login">
      <div class="form-group">
        <label class="form-label" for="login-email">Email</label>
        <input id="login-email" type="email" name="email" required>
      </div>
      <div class="form-group">
        <label class="form-label" for="login-password">Password</label>
        <input id="login-password" type="password" name="password" required minlength="8">
      </div>
      <button type="submit" class="btn btn-primary btn-block">Sign In</button>
    </form>
    <div class="auth-divider">or</div>
    <button type="button" class="btn btn-secondary btn-block" data-action="show-register">Create Account</button>
  </div>`;
}
