import { getCSRFToken } from "./api.js";

function escapeHTML(str) {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

export async function loadSession() {
  const res = await fetch("/api/me");
  try {
    const data = await res.json();
    return data.user || null;
  } catch {
    return null;
  }
}

export async function handleLogin(email, password) {
  const csrfToken = getCSRFToken();
  const res = await fetch("/api/auth/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({ email, password }),
  });
  const data = await res.json();
  return { ok: res.ok, data };
}

export async function handleRegister(email, password) {
  const csrfToken = getCSRFToken();
  const res = await fetch("/api/auth/register", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({ email, password }),
  });
  const data = await res.json();
  return { ok: res.ok, data };
}

export async function handleLogout() {
  const csrfToken = getCSRFToken();
  await fetch("/api/auth/logout", {
    method: "POST",
    headers: { "X-CSRF-Token": csrfToken },
  });
}

export async function handleMagicLinkRequest(email) {
  const csrfToken = getCSRFToken();
  await fetch("/api/auth/magic-link/request", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({ email }),
  });
}

export async function handleForgotPassword(email) {
  const csrfToken = getCSRFToken();
  await fetch("/api/auth/password/forgot", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({ email }),
  });
}

export async function handleResetPassword(token, password) {
  const csrfToken = getCSRFToken();
  const res = await fetch("/api/auth/password/reset", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({ token, password }),
  });
  return { ok: res.ok, data: await res.json() };
}

export function renderLoginView() {
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
      <div id="login-error" class="form-error hidden"></div>
      <button type="submit" class="btn btn-primary btn-block">Sign In</button>
    </form>
    <div class="mt-2">
      <button type="button" class="btn btn-ghost btn-block" data-action="show-magic-link">Sign in with magic link</button>
    </div>
    <div class="auth-divider">or</div>
    <button type="button" class="btn btn-secondary btn-block" data-action="show-register">Create Account</button>
  </div>`;
}

export function renderRegisterView() {
  return `<div class="auth-card">
    <h1 class="auth-title">Create Account</h1>
    <form id="register-form" data-action="register">
      <div class="form-group">
        <label class="form-label" for="reg-email">Email</label>
        <input id="reg-email" type="email" name="email" required autocomplete="email">
      </div>
      <div class="form-group">
        <label class="form-label" for="reg-password">Password</label>
        <input id="reg-password" type="password" name="password" required minlength="8">
      </div>
      <div class="form-group">
        <label class="form-label" for="reg-confirm">Confirm Password</label>
        <input id="reg-confirm" type="password" name="confirm" required minlength="8">
      </div>
      <div id="register-error" class="form-error hidden"></div>
      <button type="submit" class="btn btn-primary btn-block">Create Account</button>
    </form>
    <p class="text-center mt-2">
      <button type="button" class="btn btn-ghost" data-action="show-login">Already have an account? Sign in</button>
    </p>
  </div>`;
}

export function renderMagicLinkRequestView() {
  return `<div class="auth-card">
    <h1 class="auth-title">Magic Link</h1>
    <p class="text-center text-secondary mb-3">Enter your email and we'll send you a link to sign in.</p>
    <form id="magic-link-form" data-action="magic-link-request">
      <div class="form-group">
        <label class="form-label" for="magic-email">Email</label>
        <input id="magic-email" type="email" name="email" required autocomplete="email">
      </div>
      <div id="magic-link-status" class="form-error hidden"></div>
      <button type="submit" class="btn btn-primary btn-block">Send Magic Link</button>
    </form>
    <p class="text-center mt-2">
      <button type="button" class="btn btn-ghost" data-action="show-login">Back to sign in</button>
    </p>
  </div>`;
}

export function renderMagicLinkNoticeView() {
  return `<div class="auth-card">
    <h1 class="auth-title">Check Your Email</h1>
    <p class="text-center">We sent a magic link to your email address. Click the link to sign in.</p>
    <p class="text-center mt-2">
      <button type="button" class="btn btn-ghost" data-action="show-login">Back to sign in</button>
    </p>
  </div>`;
}

export function renderVerifyEmailView(success) {
  return `<div class="auth-card">
    <h1 class="auth-title">${success ? "Email Verified!" : "Verify Your Email"}</h1>
    <p class="text-center">${success ? "Your email has been verified. You can now sign in." : "Check your email for a verification link."}</p>
    <div class="mt-3">
      <button type="button" class="btn btn-primary btn-block" data-action="show-login">Sign In</button>
    </div>
  </div>`;
}

export function renderForgotPasswordView() {
  return `<div class="auth-card">
    <h1 class="auth-title">Forgot Password</h1>
    <p class="text-center text-secondary mb-3">We'll send you a password reset link.</p>
    <form id="forgot-password-form" data-action="password-forgot">
      <div class="form-group">
        <label class="form-label" for="forgot-email">Email</label>
        <input id="forgot-email" type="email" name="email" required autocomplete="email">
      </div>
      <button type="submit" class="btn btn-primary btn-block">Send Reset Link</button>
    </form>
    <p class="text-center mt-2">
      <button type="button" class="btn btn-ghost" data-action="show-login">Back to sign in</button>
    </p>
  </div>`;
}

export function renderResetPasswordView(token) {
  return `<div class="auth-card">
    <h1 class="auth-title">Reset Password</h1>
    <form id="reset-password-form" data-action="password-reset">
      <input type="hidden" name="token" value="${escapeHTML(token || "")}">
      <div class="form-group">
        <label class="form-label" for="reset-password">New Password</label>
        <input id="reset-password" type="password" name="password" required minlength="8">
      </div>
      <div class="form-group">
        <label class="form-label" for="reset-confirm">Confirm Password</label>
        <input id="reset-confirm" type="password" name="confirm" required minlength="8">
      </div>
      <div id="reset-error" class="form-error hidden"></div>
      <button type="submit" class="btn btn-primary btn-block">Reset Password</button>
    </form>
  </div>`;
}
