import { test, expect } from "@playwright/test";

const MAILPIT = process.env.MAILPIT_URL || "http://localhost:8025";
const uniqueEmail = () => `e2e-claim-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;

async function csrf(page) {
  return (await page.context().cookies()).find(c => c.name === "nabu_csrf")?.value || "";
}

async function register(page, email) {
  await page.goto("/register");
  await page.locator("#reg-email").fill(email);
  await page.locator("#reg-password").fill("registrant-password");
  await page.locator("#reg-confirm").fill("registrant-password");
  await page.locator("#register-form button[type=submit]").click();
  await expect(page.locator("#hh-indicator")).toBeVisible();
}

async function verificationToken(request, email) {
  let token;
  await expect.poll(async () => {
    const res = await request.get(`${MAILPIT}/api/v1/messages`);
    for (const msg of (await res.json()).messages || []) {
      if (!msg.Subject?.includes("Verify your")) continue;
      const raw = await (await request.get(`${MAILPIT}/api/v1/message/${msg.ID}/raw`)).text();
      if (raw.includes(email)) token = raw.match(/token=([A-Za-z0-9_-]+)/)?.[1];
      if (token) return true;
    }
    return false;
  }, { timeout: 20000 }).toBe(true);
  return token;
}

test("email claim revokes earlier credentials and lets the owner set a password", async ({ browser, page }) => {
  const email = uniqueEmail();
  await register(page, email);
  const token = await verificationToken(page.request, email);
  const ownerContext = await browser.newContext();
  try {
    const owner = await ownerContext.newPage();
    let verificationRequests = 0;
    owner.on('request',request=>{if(new URL(request.url()).pathname==='/api/auth/email/verify') verificationRequests++;});
    await owner.goto(`/verify-email?token=${token}`);
    await expect(owner.getByRole("heading", { name: "Email Verified!" })).toBeVisible();
    expect(verificationRequests).toBe(1);
    const session = await (await owner.request.get("/api/me")).json();
    expect(session.user.email).toBe(email);
    expect(session.user.hasPassword).toBe(false);

    // An earlier browser retains its cookie, so this probes server revocation.
    expect((await (await page.request.get("/api/me")).json()).user).toBeNull();
    expect((await page.request.post("/api/auth/login", {
      headers: { "X-CSRF-Token": await csrf(page) },
      data: { email, password: "registrant-password" },
    })).status()).toBe(401);

    await owner.goto("/settings");
    await expect(owner.getByRole("heading", { name: "Set Password", exact: true })).toBeVisible();
    await expect(owner.locator("#current-password")).toHaveCount(0);
    await owner.locator("#new-password").fill("owner-password-123");
    await owner.locator("#confirm-password").fill("mismatch-password");
    await owner.locator("#change-password-form button[type=submit]").click();
    await expect(owner.locator("#change-password-error")).toContainText("do not match");
    await owner.locator("#confirm-password").fill("owner-password-123");
    const passwordRequest = owner.waitForRequest(request =>
      request.method() === "POST" && new URL(request.url()).pathname === "/api/auth/password");
    await owner.locator("#change-password-form button[type=submit]").click();
    expect((await passwordRequest).postDataJSON()).toEqual({
      current_password: "", new_password: "owner-password-123",
    });
    await expect(owner.locator("#current-password")).toBeVisible();
    await owner.reload();
    await expect(owner.locator("#current-password")).toBeVisible();
    expect((await (await owner.request.get("/api/me")).json()).user.hasPassword).toBe(true);
    expect((await owner.request.post("/api/auth/login", {
      headers: { "X-CSRF-Token": await csrf(owner) },
      data: { email, password: "owner-password-123" },
    })).status()).toBe(200);
  } finally { await ownerContext.close(); }
});

test("registration retry gives a recovery message for an already committed email", async ({ browser, page }) => {
  const email = uniqueEmail();
  await register(page, email);
  const retryContext = await browser.newContext();
  try {
    const retry = await retryContext.newPage();
    await retry.goto("/register");
    await retry.locator("#reg-email").fill(email);
    await retry.locator("#reg-password").fill("registrant-password");
    await retry.locator("#reg-confirm").fill("registrant-password");
    await retry.locator("#register-form button[type=submit]").click();
    await expect(retry.locator("#register-status")).toContainText("sign in");
    await expect(retry.locator("#register-error")).toBeHidden();
    await retry.getByRole("button", { name: "Already have an account? Sign in" }).click();
    await retry.locator("#login-email").fill(email);
    await retry.locator("#login-password").fill("registrant-password");
    await retry.locator("#login-form button[type=submit]").click();
    await expect(retry.locator("#hh-indicator")).toBeVisible();
  } finally { await retryContext.close(); }
});

test("verification never displays success for a rejected token", async ({ page }) => {
  let verificationRequests = 0;
  page.on('request',request=>{if(new URL(request.url()).pathname==='/api/auth/email/verify') verificationRequests++;});
  await page.goto("/verify-email?token=synthetic-invalid-token");
  await expect(page.getByRole("heading", { name: "Verification Failed" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Email Verified!" })).toHaveCount(0);
  expect(verificationRequests).toBe(1);
});
