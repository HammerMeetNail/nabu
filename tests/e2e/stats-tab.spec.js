import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-stats-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    (await page.context().cookies()).find((c) => c.name === "choresy_csrf")
      ?.value || "";

  await page.request.post("/api/household", {
    data: { name: `Stats Test ${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });

  await page.request.post("/api/chores/seed-defaults", {
    headers: { "X-CSRF-Token": csrf },
  });

  await page.reload();
  await page.waitForSelector(".home-grid", { timeout: 15000 });

  const chores =
    (await (await page.request.get("/api/chores")).json()).chores || [];

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

test.describe("Stats tab", () => {
  test("stats tab is visible in bottom nav", async ({ page }) => {
    await setupWithChores(page);
    await expect(page.locator("a[data-nav=\"stats\"]")).toBeVisible({
      timeout: 5000,
    });
  });

  test("navigating to stats tab shows stats page with sections", async ({
    page,
  }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    const takeTrash = chores.find((c) => c.name === "Take Out Trash");
    if (feedCats) await postLog(page, csrf, feedCats.id, { hour: 8 });
    if (takeTrash) await postLog(page, csrf, takeTrash.id, { hour: 9 });

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    await expect(page.locator(".stats-page h2")).toContainText("Stats");
    await expect(page.locator(".overview-cards")).toBeVisible({ timeout: 5000 });
    await expect(page.locator(".heatmap-grid")).toBeVisible({ timeout: 5000 });
    await expect(page.locator(".busy-hours-chart")).toBeVisible({ timeout: 5000 });
    await expect(page.locator(".stat-list")).toBeVisible({ timeout: 5000 });
  });

  test("stats page shows overview cards with data", async ({ page }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    if (feedCats) await postLog(page, csrf, feedCats.id, { hour: 9 });

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".overview-cards", { timeout: 10000 });

    const cards = page.locator(".overview-card");
    await expect(cards).toHaveCount(4);

    const labels = ["Today", "This Week", "Day Streak", "Top Chore"];
    for (const label of labels) {
      await expect(page.locator(`.overview-card-label:text-is("${label}")`)).toHaveCount(1);
    }
  });

  test("feed baby volume stats appear in chore details", async ({ page }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedBaby = chores.find((c) => c.name === "Feed Baby");
    expect(feedBaby).toBeTruthy();

    await postLog(page, csrf, feedBaby.id, {
      indicators: ["🍼 formula"],
      hour: 8,
      volumeML: 120,
    });

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    await expect(
      page.locator(".chore-stat-name:text-is(\"Feed Baby\")")
    ).toBeVisible({ timeout: 5000 });

    const summary = page.locator(
      ".chore-stat-summary:has(.chore-stat-name:text-is(\"Feed Baby\"))"
    );
    await summary.click();
    await page.waitForTimeout(300);

    await expect(page.locator(".vol-chart")).toBeVisible({ timeout: 3000 });
  });

  test("change baby indicator stats appear in chore details", async ({
    page,
  }) => {
    const { chores, csrf } = await setupWithChores(page);

    const changeBaby = chores.find((c) => c.name === "Change Baby");
    expect(changeBaby).toBeTruthy();

    await postLog(page, csrf, changeBaby.id, {
      indicators: ["💩 poo", "💛 pee"],
      hour: 10,
    });

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    await expect(
      page.locator(".chore-stat-name:text-is(\"Change Baby\")")
    ).toBeVisible({ timeout: 5000 });

    const summary = page.locator(
      ".chore-stat-summary:has(.chore-stat-name:text-is(\"Change Baby\"))"
    );
    await summary.click();
    await page.waitForTimeout(300);

    await expect(page.locator(".ind-tag:text(\"💩 poo\")")).toBeVisible({
      timeout: 3000,
    });
    await expect(page.locator(".ind-tag:text(\"💛 pee\")")).toBeVisible({
      timeout: 3000,
    });
  });

  test("settings page no longer contains stats", async ({ page }) => {
    await setupWithChores(page);

    await page.click("a[data-nav=\"settings\"]");
    await page.waitForSelector(".settings-view", { timeout: 5000 });

    await expect(page.locator(".settings-view .stats-view")).toHaveCount(0);
    await expect(page.locator(".settings-view .stats-page")).toHaveCount(0);
  });

  test("tabs are in correct order: Stats, Activity, Home, Schedule, Settings", async ({
    page,
  }) => {
    await setupWithChores(page);

    const tabs = page.locator("#bottom-tabs a.tab-item");
    await expect(tabs).toHaveCount(5);

    const navValues = await tabs.evaluateAll((els) =>
      els.map((el) => el.dataset.nav)
    );
    expect(navValues).toEqual([
      "stats",
      "activity",
      "today",
      "schedule",
      "settings",
    ]);
  });

  test("expandable chore sections show chevron that rotates on open", async ({
    page,
  }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedBaby = chores.find((c) => c.name === "Feed Baby");
    if (feedBaby) {
      await postLog(page, csrf, feedBaby.id, {
        indicators: ["🍼 formula"],
        hour: 8,
        volumeML: 120,
      });
    }
    const changeBaby = chores.find((c) => c.name === "Change Baby");
    if (changeBaby) {
      await postLog(page, csrf, changeBaby.id, {
        indicators: ["💩 poo"],
        hour: 9,
      });
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    const feedBabyChevron = page.locator(
      ".chore-stat-summary:has(.chore-stat-name:text-is(\"Feed Baby\")) .chore-stat-chevron"
    );
    await expect(feedBabyChevron).toBeVisible({ timeout: 5000 });

    const changeBabyChevron = page.locator(
      ".chore-stat-summary:has(.chore-stat-name:text-is(\"Change Baby\")) .chore-stat-chevron"
    );
    await expect(changeBabyChevron).toBeVisible({ timeout: 5000 });

    // Non-indicator, non-volume chores should not have a chevron
    const takeTrash = chores.find((c) => c.name === "Take Out Trash");
    if (takeTrash) {
      await postLog(page, csrf, takeTrash.id, { hour: 10 });
    }
    await page.reload();
    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    const trashChevron = page.locator(
      ".chore-stat-summary:has(.chore-stat-name:text-is(\"Take Out Trash\")) .chore-stat-chevron"
    );
    await expect(trashChevron).toHaveCount(0);
  });
});
