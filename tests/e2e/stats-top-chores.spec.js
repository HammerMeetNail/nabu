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
  });

  test("top chores shows zero counts when no chores logged", async ({
    page,
  }) => {
    await setupWithChores(page);

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    await expect(page.locator(".top-chore-list")).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".top-chore-row")).toHaveCount(0);
    await expect(page.locator(".top-chore-list .text-secondary")).toBeVisible();
  });

  test("top chores day/week/month count labels are present", async ({
    page,
  }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    if (feedCats) {
      await postLog(page, csrf, feedCats.id, { hour: 8 });
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    await expect(page.locator(".top-chore-header-label:text(\"Day\")")).toBeVisible();
    await expect(page.locator(".top-chore-header-label:text(\"Week\")")).toBeVisible();
    await expect(page.locator(".top-chore-header-label:text(\"Month\")")).toBeVisible();

    const row = page.locator(".top-chore-row").first();
    await expect(row.locator(".top-chore-count--day")).toBeVisible();
    await expect(row.locator(".top-chore-count--week")).toBeVisible();
    await expect(row.locator(".top-chore-count--month")).toBeVisible();
  });
});
