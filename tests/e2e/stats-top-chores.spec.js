import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-top-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Top Chores Test ${Date.now()}` },
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
  const { indicators = [], volumeML, hour } = opts;
  const body = {
    choreId,
    note: "",
    indicators,
    date: new Date().toISOString().slice(0, 10),
    completedAt: new Date().toISOString(),
  };
  if (hour !== undefined) body.hour = hour;
  if (volumeML !== undefined) body.volumeML = volumeML;
  await page.request.post("/api/logs", {
    data: body,
    headers: { "X-CSRF-Token": csrf },
  });
}

test.describe("Stats top chores", () => {
  test("top chores section appears and shows ranked chores with counts", async ({
    page,
  }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    const takeTrash = chores.find((c) => c.name === "Take Out Trash");
    const dishes = chores.find((c) => c.name === "Wash Dishes");

    if (feedCats) {
      await postLog(page, csrf, feedCats.id, { hour: 8 });
      await postLog(page, csrf, feedCats.id, { hour: 12 });
      await postLog(page, csrf, feedCats.id, { hour: 18 });
    }
    if (takeTrash) {
      await postLog(page, csrf, takeTrash.id, { hour: 9 });
      await postLog(page, csrf, takeTrash.id, { hour: 14 });
    }
    if (dishes) {
      await postLog(page, csrf, dishes.id, { hour: 10 });
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    await expect(page.locator(".stats-page h3:text(\"Top Chores\")")).toBeVisible({
      timeout: 5000,
    });

    await expect(page.locator(".top-chore-list")).toBeVisible({
      timeout: 5000,
    });

    const rows = page.locator(".top-chore-row");
    await expect(rows).toHaveCount(3);

    await expect(rows.nth(0).locator(".top-chore-name")).toContainText("Feed Cats");
    await expect(rows.nth(0).locator(".top-chore-rank")).toContainText("1");
    await expect(rows.nth(1).locator(".top-chore-name")).toContainText("Take Out Trash");
    await expect(rows.nth(1).locator(".top-chore-rank")).toContainText("2");
    await expect(rows.nth(2).locator(".top-chore-name")).toContainText("Wash Dishes");
    await expect(rows.nth(2).locator(".top-chore-rank")).toContainText("3");

    await expect(rows.nth(0).locator(".top-chore-count")).toHaveText("3");
    await expect(rows.nth(1).locator(".top-chore-count")).toHaveText("2");
    await expect(rows.nth(2).locator(".top-chore-count")).toHaveText("1");
  });

  test("top chores shows user pills and empty state when no chores logged", async ({
    page,
  }) => {
    await setupWithChores(page);

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    await expect(page.locator(".top-chore-pills")).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".top-chore-pill")).toHaveCount(1);
    await expect(page.locator(".top-chore-pill--active")).toHaveCount(1);

    await expect(page.locator(".top-chore-list")).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".top-chore-row")).toHaveCount(0);
    await expect(page.locator(".top-chore-list .text-secondary")).toBeVisible();
  });

  test("top chores period toggle defaults to month and switches between periods", async ({
    page,
  }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    if (feedCats) {
      await postLog(page, csrf, feedCats.id, { hour: 8 });
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    const toggle = page.locator(
      ".stats-section-header:has(h3:text(\"Top Chores\")) .period-toggle"
    );
    await expect(toggle).toBeVisible({ timeout: 5000 });

    const buttons = toggle.locator(".period-toggle-btn");
    await expect(buttons).toHaveCount(4);
    await expect(buttons.filter({ hasText: "Day" })).toBeVisible();
    await expect(buttons.filter({ hasText: "Week" })).toBeVisible();
    await expect(buttons.filter({ hasText: "Month" })).toBeVisible();
    await expect(buttons.filter({ hasText: "All" })).toBeVisible();

    await expect(
      buttons.filter({ hasText: "Month" })
    ).toHaveClass(/\bperiod-toggle--active\b/);

    await buttons.filter({ hasText: "Day" }).click();
    await expect(
      buttons.filter({ hasText: "Day" })
    ).toHaveClass(/\bperiod-toggle--active\b/);
    await expect(
      page.locator(".top-chore-list .top-chore-count").first()
    ).toBeVisible();

    await buttons.filter({ hasText: "All" }).click();
    await expect(
      buttons.filter({ hasText: "All" })
    ).toHaveClass(/\bperiod-toggle--active\b/);
    await expect(
      page.locator(".top-chore-list .top-chore-count").first()
    ).toBeVisible();
  });

  test("user pill toggles per-user top chores", async ({ page }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    const washDishes = chores.find((c) => c.name === "Wash Dishes");

    if (feedCats) {
      await postLog(page, csrf, feedCats.id, { hour: 8 });
      await postLog(page, csrf, feedCats.id, { hour: 10 });
    }
    if (washDishes) {
      await postLog(page, csrf, washDishes.id, { hour: 9 });
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    const pills = page.locator(".top-chore-pill");
    await expect(pills).toHaveCount(1);

    const activePill = page.locator(".top-chore-pill--active");
    await expect(activePill).toHaveCount(1);

    const rows = page.locator(".top-chore-row");
    await expect(rows).toHaveCount(2);
  });
});
