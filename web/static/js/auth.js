import { getCSRFToken } from "./api.js";
import { escapeHTML } from "./utils.js";
import { changeIdentity, logoutIdentity, originHeaders, resultIsCurrent } from "./browser-context.js";
import { registerActions } from "./actions.js";
import { state, rerender, withCurrentContext, finishIdentity, recoverIdentity, joinPendingInvite } from "./app-core.js";
import { setError, hideError } from "./form-errors.js";
import { captureScope } from "./request-scope.js";
import { routeForPath, tabForPath } from "./router.js";
import { showToast } from "./toast.js";

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
  return changeIdentity(async () => {
  const csrfToken = getCSRFToken();
  const res = await fetch("/api/auth/login", {
    method: "POST",
    signal: AbortSignal.timeout(15000),
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({ email, password }),
  });
  const data = await res.json();
  return { ok: res.ok, data };
  });
}

export async function handleRegister(email, password) {
  return changeIdentity(async () => {
  const csrfToken = getCSRFToken();
  const res = await fetch("/api/auth/register", {
    method: "POST",
    signal: AbortSignal.timeout(15000),
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({ email, password }),
  });
  const data = await res.json();
  return { ok: res.ok, data };
  });
}

export async function handleLogout(onPending) {
  return logoutIdentity(async origin => {
    const response = await fetch("/api/auth/logout", {
    method: "POST",
    signal: AbortSignal.timeout(15000),
    headers: { "X-CSRF-Token": getCSRFToken(), ...(origin?.userId ? { "X-Nabu-User-ID": String(origin.userId) } : {}) },
  });
    return { ok:response.ok, error:`Sign-out is unfinished (${response.status}). Please retry.` };
  }, onPending);
}

export async function handleMagicLinkRequest(email) {
  const csrfToken = getCSRFToken();
  await fetch("/api/auth/magic-link/request", {
    method: "POST",
    signal: AbortSignal.timeout(15000),
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
    signal: AbortSignal.timeout(15000),
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({ email }),
  });
}

export async function handleResetPassword(token, password) {
 return changeIdentity(async () => {
  const csrfToken = getCSRFToken();
  const res = await fetch("/api/auth/password/reset", {
    method: "POST",
    signal: AbortSignal.timeout(15000),
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({ token, password }),
  });
  return { ok: res.ok, data: await res.json() };
 });
}

export async function handleChangePassword(currentPassword, newPassword) {
 return changeIdentity(async origin => {
  const csrfToken = getCSRFToken();
  const res = await fetch("/api/auth/password", {
    method: "POST",
    signal: AbortSignal.timeout(15000),
    headers: {
      ...originHeaders(origin),
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({
      current_password: currentPassword,
      new_password: newPassword,
    }),
  });
  return { ok: res.ok, data: await res.json() };
 });
}

// Signing in at /login?next=/operator returns to the operator dashboard.
export function operatorLoginDestination() {
  if (window.location.pathname !== "/login") return null;
  return new URL(window.location.href).searchParams.get("next") === "/operator" ? "/operator" : null;
}

async function doLogin(form) {
  if (form.dataset.busy) return;
  hideError("#login-error");
  form.dataset.busy = "true";
  try {
    const result = await handleLogin(form.querySelector("#login-email").value, form.querySelector("#login-password").value);
    if (!resultIsCurrent(result)) return;
    if (result.ok && result.user) {
      const destination = operatorLoginDestination();
      if (destination) { window.location.assign(destination); return; }
      // Signing in from a tab link (e.g. a shared Stats link) opens that tab.
      const path = window.location.pathname;
      if (!await finishIdentity(result, tabForPath(path) ? routeForPath(path) : "/")) return;
      await joinPendingInvite();
    } else if (form.isConnected) setError("#login-error",result.data?.error || "Invalid email or password");
  } catch (err) {
    await recoverIdentity(err);
    if (form.isConnected) setError("#login-error",err.message || "Could not sign in. Please retry.");
  } finally { delete form.dataset.busy; }
}

async function doRegister(form) {
  if (form.dataset.busy) return;
  hideError("#register-error"); hideError("#register-status");
  const email = form.querySelector("#reg-email").value, password = form.querySelector("#reg-password").value;
  form.dataset.busy = "true";
  try {
    const result = await handleRegister(email,password);
    if (!resultIsCurrent(result)) return;
    if (result.ok && result.data?.user && result.user) {
      if (!await finishIdentity(result)) return;
      await joinPendingInvite();
    } else if (form.isConnected) {
      if (result.ok) setError("#register-status","If this email is new, check your inbox. You can also sign in or request a magic link.");
      else setError("#register-error",result.data?.error || "Registration failed");
    }
  } catch (err) {
    await recoverIdentity(err);
    if (form.isConnected) setError("#register-error","We couldn't confirm registration. Try again, or sign in if your account was created.");
  } finally { delete form.dataset.busy; }
}
async function doMagicLinkRequest(form) {
  const contextScope = captureScope(state);
  const scope = captureScope(state);
  try {
    await withCurrentContext(handleMagicLinkRequest(form.querySelector("#magic-email").value), contextScope);
    if (scope.current() && form.isConnected) setError("#magic-link-status","Check your email for the magic link!");
  } catch {
    if (!contextScope.current()) return; if (scope.current()) showToast("Could not request a link. Please retry.","error"); }
}
async function doForgotPassword(form) {
  const contextScope = captureScope(state);
  const scope = captureScope(state);
  try {
    await withCurrentContext(handleForgotPassword(form.querySelector("#forgot-email").value), contextScope);
    if (scope.current()) showToast("If an account exists, a reset link has been sent.","info");
  } catch {
    if (!contextScope.current()) return; if (scope.current()) showToast("Could not request a link. Please retry.","error"); }
}
async function doResetPassword(form) {
  if (form.dataset.busy) return;
  hideError("#reset-error");
  const password = form.querySelector("#reset-password").value;
  if (password !== form.querySelector("#reset-confirm").value) { setError("#reset-error","Passwords do not match"); return; }
  form.dataset.busy = "true";
  try {
    const result = await handleResetPassword(form.querySelector("input[name='token']").value,password);
    if (!resultIsCurrent(result)) return;
    if (result.ok && result.user) await finishIdentity(result);
    else if (form.isConnected) setError("#reset-error",result.data?.error || "Password reset failed");
  } catch (err) { await recoverIdentity(err); if (form.isConnected) setError("#reset-error",err.message || "Password reset failed"); }
  finally { delete form.dataset.busy; }
}

registerActions("submit", {
  "login": (_e, form) => { void doLogin(form); },
  "register": (_e, form) => { void doRegister(form); },
  "magic-link-request": (_e, form) => { void doMagicLinkRequest(form); },
  "password-forgot": (_e, form) => { void doForgotPassword(form); },
  "password-reset": (_e, form) => { void doResetPassword(form); },
});

function signInWith(path) {
  window.location.href = path + (operatorLoginDestination() ? "?redirect=%2Foperator" : "");
}

// The sign-in screens, by the action that opens each.
const AUTH_ROUTES = {
  "show-login": "/",
  "show-register": "/register",
  "show-magic-link": "/magic-link",
  "show-forgot-password": "/forgot-password",
};
function showAuthScreen(event, el) {
  event.preventDefault();
  state.currentRoute = AUTH_ROUTES[el.dataset.action];
  rerender();
}

registerActions("click", {
  "google-signin"() { signInWith("/api/auth/google/login"); },
  "apple-signin"() { signInWith("/api/auth/apple/web/login"); },
  "toggle-password"(event, el) {
    // Straight on the DOM: a re-render would clear the fields not in focus.
    event.preventDefault();
    const input = document.getElementById(el.dataset.target);
    if (!input) return;
    const show = input.type === "password";
    input.type = show ? "text" : "password";
    el.textContent = show ? "Hide" : "Show";
    el.setAttribute("aria-pressed", String(show));
    el.setAttribute("aria-label", show ? "Hide password" : "Show password");
  },
  "show-login": showAuthScreen,
  "show-register": showAuthScreen,
  "show-magic-link": showAuthScreen,
  "show-forgot-password": showAuthScreen,
});

// renderOAuthButtons renders the third-party sign-in block shared by the
// login and register views. Apple sits above Google, matching the iOS app's
// button order (App Review guideline 4.8 prominence).
function renderOAuthButtons(googleOAuthEnabled, appleSignInEnabled) {
  const appleButton = appleSignInEnabled ? `
    <button type="button" class="btn btn-apple btn-block" data-action="apple-signin">
      <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
        <path fill="currentColor" d="M12.152 6.896c-.948 0-2.415-1.078-3.96-1.04-2.04.027-3.91 1.183-4.961 3.014-2.117 3.675-.546 9.103 1.519 12.09 1.013 1.454 2.208 3.09 3.792 3.03 1.52-.065 2.09-.987 3.935-.987 1.831 0 2.35.987 3.96.948 1.637-.026 2.676-1.48 3.676-2.948 1.156-1.688 1.636-3.325 1.662-3.415-.039-.013-3.182-1.221-3.22-4.857-.026-3.04 2.48-4.494 2.597-4.559-1.429-2.09-3.623-2.324-4.39-2.376-2-.156-3.675 1.09-4.61 1.09zM15.53 3.83c.843-1.012 1.4-2.427 1.245-3.83-1.207.052-2.662.805-3.532 1.818-.78.896-1.454 2.338-1.273 3.714 1.338.104 2.715-.688 3.56-1.702"/>
      </svg>
      Continue with Apple
    </button>
  ` : "";
  const googleButton = googleOAuthEnabled ? `
    <button type="button" class="btn btn-google btn-block" data-action="google-signin">
      <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
        <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"/>
        <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
        <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
        <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
      </svg>
      Continue with Google
    </button>
  ` : "";
  if (!appleButton && !googleButton) return "";
  return `<div class="auth-divider">or</div>` + appleButton + googleButton;
}

// The sign-in screens open with the logo and what Nabu is for.
const AUTH_TAGLINE = "One shared log for your home: feeds, meds, walks and chores.";

function renderAuthBrand(title) {
  return `<div class="auth-brand">
    <img class="auth-logo" src="/static/icons/icon.svg" alt="" width="56" height="56">
    <h1 class="auth-title">${title}</h1>
    <p class="auth-tagline">${AUTH_TAGLINE}</p>
  </div>`;
}

// A password field with Show/Hide, which stands in for typing it twice.
function renderPasswordField(id, { autocomplete, describedBy = "" }) {
  return `<div class="password-field">
      <input id="${id}" type="password" name="password" required minlength="8" maxlength="72" autocomplete="${autocomplete}"${describedBy ? ` aria-describedby="${describedBy}"` : ""}>
      <button type="button" class="password-toggle" data-action="toggle-password" data-target="${id}" aria-pressed="false" aria-label="Show password">Show</button>
    </div>`;
}

export function renderLoginView(googleOAuthEnabled, appleSignInEnabled) {
  return `<div class="auth-card">` +
    renderAuthBrand("Nabu") +
    `<form id="login-form" data-action="login">` +
    `  <div class="form-group">` +
    `    <label class="form-label" for="login-email">Email</label>` +
    `    <input id="login-email" type="email" name="email" required autocomplete="email">` +
    `  </div>` +
    `  <div class="form-group">` +
    `    <label class="form-label" for="login-password">Password</label>` +
    renderPasswordField("login-password", { autocomplete: "current-password" }) +
    `  </div>` +
    `  <div id="login-error" class="form-error hidden"></div>` +
    `  <button type="submit" class="btn btn-primary btn-block">Sign In</button>` +
    `</form>` +
    `<div class="mt-2">` +
    `  <button type="button" class="btn btn-ghost btn-block" data-action="show-magic-link">Sign in with magic link</button>` +
    `</div>` +
    renderOAuthButtons(googleOAuthEnabled, appleSignInEnabled) +
    `<div class="auth-divider">or</div>` +
    `<button type="button" class="btn btn-secondary btn-block" data-action="show-register">Create Account</button>` +
    `<p class="text-center mt-2">` +
    `  <button type="button" class="btn btn-ghost" data-action="show-forgot-password">Forgot password?</button>` +
    `</p>` +
  `</div>`;
}

export function renderRegisterView(googleOAuthEnabled, appleSignInEnabled) {
  return `<div class="auth-card">
    ${renderAuthBrand("Create Account")}
    <form id="register-form" data-action="register">
      <div class="form-group">
        <label class="form-label" for="reg-email">Email</label>
        <input id="reg-email" type="email" name="email" required autocomplete="email">
      </div>
      <div class="form-group">
        <label class="form-label" for="reg-password">Password</label>
        ${renderPasswordField("reg-password", { autocomplete: "new-password", describedBy: "reg-password-rules" })}
        <p id="reg-password-rules" class="form-hint">At least 8 characters.</p>
      </div>
      <div id="register-error" class="form-error hidden"></div>
      <p id="register-status" class="text-secondary hidden" role="status"></p>
      <button type="submit" class="btn btn-primary btn-block">Create Account</button>
    </form>
    ${renderOAuthButtons(googleOAuthEnabled, appleSignInEnabled)}
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

export function renderVerifyEmailView(status) {
  const messages = {
    pending: ["Verifying Email…", "Please wait while we check your link."],
    success: ["Email Verified!", "You're signed in. You can set a password in Settings."],
    error: ["Verification Failed", "This link could not be verified. Sign in to request another link."],
    missing: ["Verify Your Email", "Check your email for a verification link."],
  };
  const [title, message] = messages[status] || messages.missing;
  return `<div class="auth-card" role="status">
    <h1 class="auth-title">${title}</h1>
    <p class="text-center">${message}</p>
    ${status === "pending" ? "" : `<div class="mt-3">
      <button type="button" class="btn btn-primary btn-block" data-action="show-login">${status === "success" ? "Continue" : "Sign In"}</button>
    </div>`}
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
