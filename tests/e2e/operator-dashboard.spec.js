import { test, expect } from "@playwright/test";

// Run with OPERATOR_E2E=1 against a fresh PostgreSQL stack configured with
// OPERATOR_OWNER_USER_ID=1. The standard in-memory CI stack leaves the feature off.
test.skip(process.env.OPERATOR_E2E !== "1", "operator E2E needs the enabled PostgreSQL stack");

const email = "operator-e2e@nabu.local";
const password = "operator-local-password";
const mailpit = process.env.MAILPIT_URL || "http://localhost:18026";

async function csrf(page) {
  return (await page.context().cookies()).find(cookie => cookie.name === "nabu_csrf")?.value || "";
}

async function register(page, address) {
  await page.goto("/register");
  await page.locator("#reg-email").fill(address);
  await page.locator("#reg-password").fill(password);
  await page.locator("#reg-confirm").fill(password);
  await page.locator("#register-form button[type=submit]").click();
  await expect(page.locator("#hh-indicator")).toBeVisible();
}

async function verificationToken(request, address) {
  let token;
  await expect.poll(async () => {
    const res = await request.get(`${mailpit}/api/v1/messages`);
    for (const msg of (await res.json()).messages || []) {
      if (!msg.Subject?.includes("Verify your")) continue;
      const raw = await (await request.get(`${mailpit}/api/v1/message/${msg.ID}/raw`)).text();
      if (raw.includes(address)) token = raw.match(/token=([A-Za-z0-9_-]+)/)?.[1];
      if (token) return true;
    }
    return false;
  }, { timeout: 20000 }).toBe(true);
  return token;
}

test("operator dashboard and scoped key lifecycle", async ({ browser }) => {
  const ownerContext = await browser.newContext();
  const owner = await ownerContext.newPage();
  const outsiderContext = await browser.newContext();
  const outsider = await outsiderContext.newPage();
  const keyContext = await browser.newContext();
  try {
    expect((await owner.goto("/operator")).status()).toBe(401);
    await register(owner, email);
    const profile = (await (await owner.request.get("/api/me")).json()).user;
    expect(profile.id, "start with a fresh stack so this configured owner is user 1").toBe(1);
    const token = await verificationToken(owner.request, email);
    await owner.goto(`/verify-email?token=${token}`);
    await expect(owner.getByRole("heading", { name: "Email Verified!" })).toBeVisible();

    const ownerCsrf = await csrf(owner);
    // Verification of a registration claim clears its old password. Set one
    // through the authenticated flow so this fixture can be used after E2E.
    const passwordSetup = await owner.request.post("/api/auth/password", {
      headers: { "X-CSRF-Token": ownerCsrf },
      data: { current_password: "", new_password: password },
    });
    expect(passwordSetup.status()).toBe(200);
    const reusableLogin = await owner.request.post("/api/auth/login", {
      headers: { "X-CSRF-Token": ownerCsrf }, data: { email, password },
    });
    expect(reusableLogin.status()).toBe(200);
    const household = await owner.request.post("/api/household", {
      headers: { "X-CSRF-Token": ownerCsrf }, data: { name: "Operator test household" },
    });
    expect(household.status()).toBe(201);
    const seeded = await owner.request.post("/api/chores/seed-defaults", { headers: { "X-CSRF-Token": ownerCsrf } });
    expect(seeded.ok()).toBe(true);
    const chores = (await (await owner.request.get("/api/chores")).json()).chores;
    const logged = await owner.request.post("/api/logs", {
      headers: { "X-CSRF-Token": ownerCsrf }, data: { choreId: chores[0].id, completedAt: new Date().toISOString() },
    });
    expect(logged.status()).toBe(201);

    await register(outsider, `operator-outsider-${Date.now()}@test.local`);
    const outsiderCsrf = await csrf(outsider);
    const outsideHousehold = await outsider.request.post("/api/household", {
      headers: { "X-CSRF-Token": outsiderCsrf }, data: { name: "Other household" },
    });
    expect(outsideHousehold.status()).toBe(201);
    expect((await outsider.goto("/operator")).status()).toBe(403);
    expect((await outsider.request.get("/api/operator/v1/users")).status()).toBe(403);

    await owner.goto("/operator");
    await expect(owner.getByRole("heading", { name: "App dashboard" })).toBeVisible();
    await expect(owner.locator("#users-body tr")).toHaveCount(2);
    await expect(owner.locator("#users-body")).toContainText(email);
    await expect(owner.locator("#households-body")).toContainText("Operator test household");
    await expect(owner.locator("#overview")).toContainText("Registered users");
    if (process.env.OPERATOR_SCREENSHOT) {
      await owner.screenshot({ path: process.env.OPERATOR_SCREENSHOT, fullPage: true });
    }

    // State-changing key management is protected by the existing CSRF layer.
    expect((await owner.request.post("/api/operator/v1/keys", {
      data: { name: "blocked", scope: "full", days: 7 }, headers: { "X-CSRF-Token": "invalid" },
    })).status()).toBe(403);

    await owner.locator('#key-form input[name="name"]').fill("Dashboard full access");
    await owner.locator('#key-form select[name="scope"]').selectOption("full");
    await owner.locator("#key-form button[type=submit]").click();
    await expect(owner.locator("#issued-key")).toBeVisible();
    const fullToken = await owner.locator("#token").inputValue();
    expect(fullToken).toMatch(/^nabu_op_/);
    await owner.locator("#dismiss-key").click();
    await owner.reload();
    await expect(owner.locator("#keys-body")).toContainText("Dashboard full access");
    expect(await owner.locator("#token").inputValue()).toBe("");

    const keyUsers = await keyContext.request.get("/api/operator/v1/users", { headers: { Authorization: `Bearer ${fullToken}` } });
    expect(keyUsers.status()).toBe(200);
    expect((await keyUsers.json()).users.map(user => user.email)).toContain(email);
    const fullKeyID = (await (await owner.request.get("/api/operator/v1/keys")).json()).keys.find(key => key.name === "Dashboard full access").id;
    const revoked = await owner.request.delete(`/api/operator/v1/keys/${fullKeyID}`, { headers: { "X-CSRF-Token": await csrf(owner) } });
    expect(revoked.status()).toBe(204);
    expect((await keyContext.request.get("/api/operator/v1/users", { headers: { Authorization: `Bearer ${fullToken}` } })).status()).toBe(401);

    const summaryKey = await owner.request.post("/api/operator/v1/keys", {
      headers: { "X-CSRF-Token": await csrf(owner) }, data: { name: "Summary only", scope: "summary", days: 7 },
    });
    expect(summaryKey.status()).toBe(201);
    const summaryToken = (await summaryKey.json()).token;
    expect((await keyContext.request.get("/api/operator/v1/summary", { headers: { Authorization: `Bearer ${summaryToken}` } })).status()).toBe(200);
    expect((await keyContext.request.get("/api/operator/v1/users", { headers: { Authorization: `Bearer ${summaryToken}` } })).status()).toBe(401);
  } finally {
    await ownerContext.close();
    await outsiderContext.close();
    await keyContext.close();
  }
});
