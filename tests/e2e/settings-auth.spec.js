import { test, expect } from "@playwright/test";

const BASE = "http://localhost:8080";
const MAILPIT = "http://localhost:8025";

function uniqueEmail() {
  return `e2e-sa-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function registerUser(page, email, password = "test123456") {
  await page.goto(`${BASE}/register`);
  await page.waitForSelector("#register-form");
  await page.fill("#reg-email", email);
  await page.fill("#reg-password", password);
  await page.fill("#reg-confirm", password);
  await page.click("button[type=\"submit\"]");
  await page.waitForSelector("#user-avatar:not([hidden])", { timeout: 10000 });
  const csrf =
    (await page.context().cookies()).find((c) => c.name === "choresy_csrf")
      ?.value || "";
  return { csrf };
}

async function setupFullAccount(page) {
  const email = uniqueEmail();
  const { csrf } = await registerUser(page, email);

  await page.request.post(`${BASE}/api/household`, {
    data: { name: `SA Test ${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });
  await page.request.post(`${BASE}/api/chores/seed-defaults`, {
    headers: { "X-CSRF-Token": csrf },
  });
  await page.reload();
  await page.waitForSelector(".home-grid", { timeout: 15000 });
  return { email, csrf };
}

async function waitForEmailToken(request, subjectSubstring) {
  for (let attempt = 0; attempt < 15; attempt++) {
    await new Promise((r) => setTimeout(r, 1000));
    const res = await request.get(`${MAILPIT}/api/v1/messages`);
    const data = await res.json();
    for (const msg of data.messages || []) {
      if (msg.Subject && msg.Subject.includes(subjectSubstring)) {
        const rawRes = await request.get(
          `${MAILPIT}/api/v1/message/${msg.ID}/raw`
        );
        const raw = await rawRes.text();
        const match = raw.match(/token=([A-Za-z0-9_-]+)/);
        if (match) return match[1];
      }
    }
  }
  return null;
}

async function clearMailpit(request) {
  const res = await request.get(`${MAILPIT}/api/v1/messages`);
  const data = await res.json();
  for (const msg of data.messages || []) {
    await request.delete(`${MAILPIT}/api/v1/message/${msg.ID}`);
  }
}

async function navigateToSettings(page) {
  await page.click("a[data-nav=\"settings\"]");
  await page.waitForSelector(".settings-view", { timeout: 5000 });
}

test.describe("Settings: auth features", () => {
  test("change password from settings — happy path", async ({ page }) => {
    const email = uniqueEmail();
    await registerUser(page, email);
    await navigateToSettings(page);

    await expect(page.locator("#change-password-form")).toBeVisible({
      timeout: 3000,
    });
    await expect(page.locator("#current-password")).toBeVisible();
    await expect(page.locator("#new-password")).toBeVisible();
    await expect(page.locator("#confirm-password")).toBeVisible();

    await page.fill("#current-password", "test123456");
    await page.fill("#new-password", "newpassword789");
    await page.fill("#confirm-password", "newpassword789");
    await page.click("#change-password-form button[type=\"submit\"]");

    await page.waitForTimeout(500);
    await expect(page.locator("#change-password-error")).toHaveClass(/hidden/);

    const toast = page.locator("#toast-container .toast-success");
    await expect(toast.first()).toBeVisible({ timeout: 3000 });

    await page.locator("button[data-action=\"logout\"]").click();
    await page.waitForTimeout(500);
    await expect(page.locator("#login-form")).toBeVisible({ timeout: 5000 });

    await page.fill("#login-email", email);
    await page.fill("#login-password", "newpassword789");
    await page.click("#login-form button[type=\"submit\"]");
    await expect(page.locator("#user-avatar:not([hidden])")).toBeVisible({
      timeout: 10000,
    });
  });

  test("change password — wrong current password shows error", async ({
    page,
  }) => {
    const email = uniqueEmail();
    await registerUser(page, email);
    await navigateToSettings(page);

    await expect(page.locator("#change-password-form")).toBeVisible({
      timeout: 3000,
    });
    await page.fill("#current-password", "wrongpass");
    await page.fill("#new-password", "newpassword789");
    await page.fill("#confirm-password", "newpassword789");
    await page.click("#change-password-form button[type=\"submit\"]");

    await page.waitForTimeout(500);
    await expect(page.locator("#change-password-error")).not.toHaveClass(
      /hidden/
    );
    await expect(page.locator("#change-password-error")).toContainText(
      /incorrect|failed/
    );
  });

  test("change password — mismatched confirm shows error", async ({ page }) => {
    const email = uniqueEmail();
    await registerUser(page, email);
    await navigateToSettings(page);

    await expect(page.locator("#change-password-form")).toBeVisible({
      timeout: 3000,
    });
    await page.fill("#current-password", "test123456");
    await page.fill("#new-password", "newpassword789");
    await page.fill("#confirm-password", "different");
    await page.click("#change-password-form button[type=\"submit\"]");

    await page.waitForTimeout(500);
    await expect(page.locator("#change-password-error")).not.toHaveClass(
      /hidden/
    );
    await expect(page.locator("#change-password-error")).toContainText(
      /not match/
    );
  });

  test("resend verification email from settings", async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();
    const request = context.request;

    await clearMailpit(request);

    const email = uniqueEmail();
    await registerUser(page, email);
    await navigateToSettings(page);

    const verifySection = page.locator("text=Email Verification");
    await expect(verifySection).toBeVisible({ timeout: 3000 });
    await expect(page.locator("button[data-action=\"resend-verification\"]")).toBeVisible();

    await page.click("button[data-action=\"resend-verification\"]");
    await page.waitForTimeout(500);

    const token = await waitForEmailToken(request, "Verify your");
    expect(token).not.toBeNull();

    await context.close();
  });

  test("verified email hides verification section in settings", async ({
    browser,
  }) => {
    const context = await browser.newContext();
    const page = await context.newPage();
    const request = context.request;

    await clearMailpit(request);

    const email = uniqueEmail();
    await registerUser(page, email);

    const verifyToken = await waitForEmailToken(request, "Verify your");
    expect(verifyToken).not.toBeNull();

    const verifyResp = await request.get(
      `${BASE}/api/auth/email/verify?token=${encodeURIComponent(verifyToken)}`
    );
    expect(verifyResp.ok()).toBeTruthy();

    const meResp = await request.get(`${BASE}/api/me`);
    const meData = await meResp.json();
    expect(meData.user?.emailVerified).toBe(true);

    await page.goto(BASE);
    await page.waitForSelector("#user-avatar:not([hidden])", { timeout: 10000 });

    await navigateToSettings(page);

    const verifySection = page.locator("text=Email Verification");
    await expect(verifySection).not.toBeVisible({ timeout: 3000 });

    await context.close();
  });

  test("reloading after password change still works with new session", async ({
    page,
  }) => {
    const email = uniqueEmail();
    await registerUser(page, email);
    await navigateToSettings(page);

    await page.fill("#current-password", "test123456");
    await page.fill("#new-password", "newpassword789");
    await page.fill("#confirm-password", "newpassword789");
    await page.click("#change-password-form button[type=\"submit\"]");
    await page.waitForTimeout(500);

    await page.reload();
    await page.waitForSelector("#user-avatar:not([hidden])", {
      timeout: 10000,
    });
    await expect(page.locator("#top-bar")).not.toBeHidden({ timeout: 5000 });
  });
});

test.describe("Login page elements", () => {
  test("login page shows forgot password link", async ({ page }) => {
    await page.goto(BASE);
    await page.waitForSelector("#login-form");

    const forgotBtn = page.locator(
      "button[data-action=\"show-forgot-password\"]"
    );
    await expect(forgotBtn).toBeVisible({ timeout: 3000 });
    await expect(forgotBtn).toContainText("Forgot password");
  });

  test("forgot password link navigates to forgot password view", async ({
    page,
  }) => {
    await page.goto(BASE);
    await page.waitForSelector("#login-form");

    await page.click("button[data-action=\"show-forgot-password\"]");
    await page.waitForSelector("#forgot-password-form", { timeout: 5000 });

    await expect(page.locator("#forgot-email")).toBeVisible();
    await expect(
      page.locator("#forgot-password-form button[type=\"submit\"]")
    ).toContainText("Send Reset Link");
  });

  test("Google button not visible when OAuth disabled", async ({ page }) => {
    await page.goto(BASE);
    await page.waitForSelector("#login-form");

    const googleBtn = page.locator(".btn-google");
    await expect(googleBtn).toHaveCount(0);

    await page.goto(`${BASE}/register`);
    await page.waitForSelector("#register-form");
    await expect(page.locator(".btn-google")).toHaveCount(0);
  });
});
