import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-lb-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto("/register");
  await page.waitForSelector("#register-form");
  await page.fill("#reg-email", email);
  await page.fill("#reg-password", "test123456");
  await page.fill("#reg-confirm", "test123456");
  await page.click("button[type=\"submit\"]");
  await page.waitForSelector("#hh-indicator:not([hidden])", { timeout: 10000 });

  const csrf =
    (await page.context().cookies()).find((c) => c.name === "nabu_csrf")
      ?.value || "";

  await page.request.post("/api/household", {
    data: { name: `Leaderboard Test ${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });

  await page.request.post("/api/chores/seed-defaults", {
    headers: { "X-CSRF-Token": csrf },
  });

  await page.reload();
  await page.waitForSelector(".home-grid", { timeout: 15000 });

  const resp = await page.request.get("/api/chores");
  const chores = (await resp.json()).chores || [];

  return { email, csrf, chores };
}

async function postLog(page, csrf, choreId, opts = {}) {
  const { hour } = opts;
  const body = {
    choreId,
    note: "",
    indicators: [],
    date: new Date().toISOString().slice(0, 10),
    completedAt: new Date().toISOString(),
  };
  if (hour !== undefined) body.hour = hour;
  await page.request.post("/api/logs", {
    data: body,
    headers: { "X-CSRF-Token": csrf },
  });
}

test.describe("Stats leaderboard", () => {
  test("leaderboard section appears with period toggle and default week active", async ({
    page,
  }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    if (feedCats) {
      await postLog(page, csrf, feedCats.id, { hour: 8 });
      await postLog(page, csrf, feedCats.id, { hour: 12 });
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    await expect(page.locator(".stats-page h3:text(\"Leaderboard\")")).toBeVisible({
      timeout: 5000,
    });

    const toggle = page.locator(
      ".stats-section-header:has(h3:text(\"Leaderboard\")) .period-toggle"
    );
    await expect(toggle).toBeVisible({ timeout: 5000 });

    const buttons = toggle.locator(".period-toggle-btn");
    await expect(buttons).toHaveCount(4);
    await expect(buttons.filter({ hasText: "Day" })).toBeVisible();
    await expect(buttons.filter({ hasText: "Week" })).toBeVisible();
    await expect(buttons.filter({ hasText: "Month" })).toBeVisible();
    await expect(buttons.filter({ hasText: "All" })).toBeVisible();

    await expect(
      buttons.filter({ hasText: "Week" })
    ).toHaveClass(/\bperiod-toggle--active\b/);

    const lbCard = page.locator(".card", { has: page.locator('h3:text("Leaderboard")') });
    await expect(lbCard.locator(".stats-date-range")).toBeVisible();

    const items = lbCard.locator(".stat-list .stat-item");
    await expect(items.first()).toBeVisible({ timeout: 5000 });
    await expect(items.first().locator(".text-secondary")).toContainText(
      /chores/
    );
  });

  test("leaderboard All period shows All time label", async ({ page }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    if (feedCats) {
      await postLog(page, csrf, feedCats.id, { hour: 8 });
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    const toggle = page.locator(
      ".stats-section-header:has(h3:text(\"Leaderboard\")) .period-toggle"
    );
    await expect(toggle).toBeVisible({ timeout: 5000 });

    await toggle.locator(".period-toggle-btn", { hasText: "All" }).click();

    await expect(
      toggle.locator(".period-toggle-btn", { hasText: "All" })
    ).toHaveClass(/\bperiod-toggle--active\b/);

    const lbCard = page.locator(".card", { has: page.locator('h3:text("Leaderboard")') });
    await expect(lbCard.locator(".stats-date-range")).toHaveText("All time");
  });

  test("leaderboard Day period updates date range and entries", async ({
    page,
  }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    if (feedCats) {
      await postLog(page, csrf, feedCats.id, { hour: 7 });
      await postLog(page, csrf, feedCats.id, { hour: 19 });
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    const toggle = page.locator(
      ".stats-section-header:has(h3:text(\"Leaderboard\")) .period-toggle"
    );
    await expect(toggle).toBeVisible({ timeout: 5000 });

    await toggle.locator(".period-toggle-btn", { hasText: "Day" }).click();

    await expect(
      toggle.locator(".period-toggle-btn", { hasText: "Day" })
    ).toHaveClass(/\bperiod-toggle--active\b/);

    const lbCard = page.locator(".card", { has: page.locator('h3:text("Leaderboard")') });
    const range = lbCard.locator(".stats-date-range");
    await expect(range).not.toHaveText("All time");

    await expect(range).toContainText(/–/);

    const items = lbCard.locator(".stat-list .stat-item");
    await expect(items.first()).toBeVisible({ timeout: 5000 });
    await expect(items.first().locator(".text-secondary")).toHaveText("2 chores");
  });
});
