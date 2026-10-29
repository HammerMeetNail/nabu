import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-apikey-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  const email = uniqueEmail();
  await page.goto("/register");
  await page.waitForSelector("#register-form");
  await page.fill("#reg-email", email);
  await page.fill("#reg-password", "test123456");
  await page.click("button[type=\"submit\"]");
  await page.waitForSelector("#hh-indicator:not([hidden])", { timeout: 10000 });

  const csrf =
    (await page.context().cookies()).find((c) => c.name === "nabu_csrf")
      ?.value || "";
  await page.request.post("/api/household", {
    data: { name: `API Key Test ${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });
  await page.request.post("/api/chores/seed-defaults", {
    headers: { "X-CSRF-Token": csrf },
  });
  await page.reload();
  await page.waitForSelector(".home-grid", { timeout: 15000 });
  const chores =
    (await (await page.request.get("/api/chores")).json()).chores || [];
  const changeBaby = chores.find((c) => c.name === "Change Baby");
  return { email, csrf, changeBaby };
}

async function openSettings(page) {
  await page.click("a[data-nav=\"settings\"]");
  await page.waitForSelector("[data-testid=\"api-keys\"]", { timeout: 10000 });
}

async function createKey(page, name) {
  await page.fill("#api-key-name", name);
  await page.click("#create-api-key-form button[type=\"submit\"]");
  const token = await page.locator("#api-key-token").inputValue();
  expect(token).toMatch(/^nabu_pat_[A-Za-z0-9_-]{16}\.[A-Za-z0-9_-]{43}$/);
  return token;
}

test.describe("Settings API keys", () => {
  test("a key created in Settings can backfill a log without a browser session", async ({ page, playwright, baseURL }) => {
    const { changeBaby } = await setupWithChores(page);
    expect(changeBaby).toBeTruthy();
    await openSettings(page);
    const token = await createKey(page, "Backfill script");

    // A fresh request context has no cookies and no CSRF token.
    const api = await playwright.request.newContext({
      baseURL,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` },
    });
    const created = await api.post("/api/logs", {
      data: {
        choreId: changeBaby.id,
        date: "2026-10-15",
        hour: 3,
        completedAt: "2026-10-15T03:10:00Z",
        note: "api backfill",
      },
    });
    expect(created.status()).toBe(201);
    const { log } = await created.json();

    const edited = await api.patch(`/api/logs/${log.id}`, { data: { note: "api backfill edited" } });
    expect(edited.status()).toBe(200);

    // The browser session sees the API-created log.
    const history = await (await page.request.get("/api/logs/history?q=api%20backfill")).json();
    expect(history.logs.map((l) => l.note)).toContain("api backfill edited");

    // Keys are limited to activity logs.
    expect((await api.post("/api/chores", { data: { name: "Nope" } })).status()).toBe(403);
    expect((await api.get("/api/api-keys")).status()).toBe(403);

    expect((await api.delete(`/api/logs/${log.id}`)).status()).toBeLessThan(300);
    await api.dispose();

    // The token is shown once: after Done and a reload only the name remains.
    await page.click("[data-action=\"dismiss-api-key\"]");
    await expect(page.locator("[data-testid=\"api-key-issued\"]")).toHaveCount(0);
    await page.reload();
    await openSettings(page);
    const row = page.locator("[data-testid=\"api-key-row\"]");
    await expect(row).toHaveCount(1);
    await expect(row).toContainText("Backfill script");
    await expect(row).toContainText("Active");
    await expect(page.locator("[data-testid=\"api-keys\"]")).not.toContainText(token);
  });

  test("revoking a key stops it working; cancelling keeps it", async ({ page, playwright, baseURL }) => {
    await setupWithChores(page);
    await openSettings(page);
    const token = await createKey(page, "Temporary");
    await page.click("[data-action=\"dismiss-api-key\"]");
    const api = await playwright.request.newContext({
      baseURL,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` },
    });
    expect((await api.get("/api/me")).status()).toBe(200);

    // Cancel the confirm: key stays active.
    page.once("dialog", (dialog) => dialog.dismiss());
    await page.click("[data-action=\"revoke-api-key\"]");
    await expect(page.locator("[data-testid=\"api-key-row\"]")).toContainText("Active");
    expect((await api.get("/api/me")).status()).toBe(200);

    page.once("dialog", (dialog) => dialog.accept());
    await page.click("[data-action=\"revoke-api-key\"]");
    await expect(page.locator("[data-testid=\"api-key-row\"]")).toContainText("Revoked");
    await expect(page.locator("[data-action=\"revoke-api-key\"]")).toHaveCount(0);
    expect((await api.get("/api/me")).status()).toBe(401);
    await api.dispose();

    await page.reload();
    await openSettings(page);
    await expect(page.locator("[data-testid=\"api-key-row\"]")).toContainText("Revoked");
  });

  test("a failed create keeps the typed name for a retry", async ({ page }) => {
    await setupWithChores(page);
    await openSettings(page);
    let failed = false;
    await page.route("**/api/api-keys", async (route) => {
      if (route.request().method() === "POST" && !failed) {
        failed = true;
        await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "server error" }) });
        return;
      }
      await route.continue();
    });
    await page.fill("#api-key-name", "Nightly sync");
    await page.click("#create-api-key-form button[type=\"submit\"]");
    await expect(page.locator("[data-testid=\"api-key-error\"]")).toBeVisible();
    await expect(page.locator("#api-key-name")).toHaveValue("Nightly sync");

    await page.click("#create-api-key-form button[type=\"submit\"]");
    await expect(page.locator("#api-key-token")).toHaveValue(/^nabu_pat_/);
    await expect(page.locator("#api-key-name")).toHaveValue("");
    await expect(page.locator("[data-testid=\"api-key-row\"]")).toContainText("Nightly sync");
  });

  test("a name typed before another render stays in the field", async ({ page }) => {
    await setupWithChores(page);
    await openSettings(page);
    await createKey(page, "First");
    await page.fill("#api-key-name", "Second");
    // Focus leaves the field, then dismissing the token re-renders the card.
    await page.click("[data-action=\"dismiss-api-key\"]");
    await expect(page.locator("[data-testid=\"api-key-issued\"]")).toHaveCount(0);
    await expect(page.locator("#api-key-name")).toHaveValue("Second");
  });
});
