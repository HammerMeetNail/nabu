import { getCSRFToken } from "./api.js";
import { escapeHTML } from "./utils.js";

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

export async function handleChangePassword(currentPassword, newPassword) {
  const csrfToken = getCSRFToken();
  const res = await fetch("/api/auth/password", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({
      current_password: currentPassword,
      new_password: newPassword,
    }),
  });
  return { ok: res.ok, data: await res.json() };
}

export function renderLoginView(googleOAuthEnabled) {
  const googleButton = googleOAuthEnabled ? `
    <div class="auth-divider">or</div>
    <a href="/api/auth/google/login" class="btn btn-google btn-block">
      <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
        <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"/>
        <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
        <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
        <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
      </svg>
      Continue with Google
    </a>
  ` : "";
  return `<div class="auth-card">` +
    `<h1 class="auth-title">Nabu</h1>` +
    `<form id="login-form" data-action="login">` +
    `  <div class="form-group">` +
    `    <label class="form-label" for="login-email">Email</label>` +
    `    <input id="login-email" type="email" name="email" required autocomplete="email">` +
    `  </div>` +
    `  <div class="form-group">` +
    `    <label class="form-label" for="login-password">Password</label>` +
    `    <input id="login-password" type="password" name="password" required minlength="8" autocomplete="current-password">` +
    `  </div>` +
    `  <div id="login-error" class="form-error hidden"></div>` +
    `  <button type="submit" class="btn btn-primary btn-block">Sign In</button>` +
    `</form>` +
    `<div class="mt-2">` +
    `  <button type="button" class="btn btn-ghost btn-block" data-action="show-magic-link">Sign in with magic link</button>` +
    `</div>` +
    googleButton +
    `<div class="auth-divider">or</div>` +
    `<button type="button" class="btn btn-secondary btn-block" data-action="show-register">Create Account</button>` +
    `<p class="text-center mt-2">` +
    `  <button type="button" class="btn btn-ghost" data-action="show-forgot-password">Forgot password?</button>` +
    `</p>` +
  `</div>`;
}

export function renderRegisterView(googleOAuthEnabled) {
  const googleButton = googleOAuthEnabled ? `
    <div class="auth-divider">or</div>
    <a href="/api/auth/google/login" class="btn btn-google btn-block">
      <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
        <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"/>
        <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
        <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
        <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
      </svg>
      Continue with Google
    </a>
  ` : "";
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
    ${googleButton}
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
