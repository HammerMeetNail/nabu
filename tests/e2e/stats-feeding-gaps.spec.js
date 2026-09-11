import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-gaps-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Gaps Test ${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });

  await page.request.post("/api/chores/seed-defaults", {
    headers: { "X-CSRF-Token": csrf },
  });

  await page.reload();
  await page.waitForSelector(".home-grid", { timeout: 15000 });

  const chores =
    (await (await page.request.get("/api/chores")).json()).chores || [];
  const feedBaby = chores.find((c) => c.name === "Feed Baby");

  return { email, csrf, chores, feedBaby };
}

async function postFeedLog(page, csrf, choreId, completedAt, volumeML, indicatorVolumes) {
  const d = new Date(completedAt);
  const date = d.toISOString().slice(0, 10);
  const body = {
    choreId,
    note: "",
    date,
    completedAt: new Date(completedAt).toISOString(),
  };
  if (volumeML !== undefined) body.volumeML = volumeML;
  if (indicatorVolumes) body.indicatorVolumes = indicatorVolumes;
  await page.request.post("/api/logs", {
    data: body,
    headers: { "X-CSRF-Token": csrf },
  });
}

test.describe("Feeding gaps chart", () => {
  test("cluster feeding chart renders scatter plot of gap durations", async ({
    page,
  }) => {
    const { csrf, feedBaby } = await setupWithChores(page);
    expect(feedBaby).toBeTruthy();

    const now = new Date();
    const y = now.getFullYear();
    const m = now.getMonth();
    const d = now.getDate();

    // Create feedings that form gaps:
    // 9:00am 120mL -> 9:40am 40mL (40 min gap, small top-off)
    // 9:40am 40mL  -> 12:30pm 110mL (170 min gap)
    // 12:30pm 110mL -> 1:15pm 30mL (45 min gap, small top-off)
    // 1:15pm 30mL   -> 4:00pm 100mL (165 min gap)
    // 7:00pm 130mL -> 7:35pm 50mL (35 min gap, small top-off)

    const logs = [
      { completedAt: new Date(y, m, d, 9, 0, 0), volumeML: 120, indicatorVolumes: { "\u{1F37C} formula": 120 } },
      { completedAt: new Date(y, m, d, 9, 40, 0), volumeML: 40, indicatorVolumes: { "\u{1F37C} formula": 40 } },
      { completedAt: new Date(y, m, d, 12, 30, 0), volumeML: 110, indicatorVolumes: { "\u{1F37C} formula": 110 } },
      { completedAt: new Date(y, m, d, 13, 15, 0), volumeML: 30, indicatorVolumes: { "\u{1F37C} formula": 30 } },
      { completedAt: new Date(y, m, d, 16, 0, 0), volumeML: 100, indicatorVolumes: { "\u{1F37C} formula": 100 } },
      { completedAt: new Date(y, m, d, 19, 0, 0), volumeML: 130, indicatorVolumes: { "\u{1F37C} formula": 130 } },
      { completedAt: new Date(y, m, d, 19, 35, 0), volumeML: 50, indicatorVolumes: { "\u{1F37C} formula": 50 } },
    ];

    for (const log of logs) {
      await postFeedLog(page, csrf, feedBaby.id, log.completedAt, log.volumeML, log.indicatorVolumes);
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    const gapsColumn = page.locator(".baby-care-column").filter({ hasText: "Cluster Feeding" });
    await expect(gapsColumn).toBeVisible({ timeout: 5000 });

    // 3 pink + 2 orange + 1 blue (all with hit areas) + 3 legend = 15
    const svg = gapsColumn.locator("svg.feeding-gaps-chart");
    await expect(svg).toBeVisible({ timeout: 3000 });
    const dots = svg.locator("circle");
    await expect(dots).toHaveCount(15);

    // All dots should be tappable and reveal tooltips
    const tappable = svg.locator("[data-action=\"scatter-tap\"]");
    await expect(tappable).toHaveCount(6);
    await tappable.first().click();
    const tooltip = svg.locator(".scatter-tooltip--visible");
    await expect(tooltip).toHaveCount(1);

    // Info icon expand/collapse
    await expect(gapsColumn.locator(".feeding-gaps-info-btn")).toBeVisible();
    await gapsColumn.locator(".feeding-gaps-info-btn").click();
    await expect(gapsColumn.locator(".feeding-gaps-explainer--visible")).toBeVisible();
    await gapsColumn.locator(".feeding-gaps-info-btn").click();
    await expect(gapsColumn.locator(".feeding-gaps-explainer--visible")).not.toBeVisible();

    // Compare button should no longer exist
    await expect(gapsColumn.locator("[data-action=\"stats-feeding-gaps-compare\"]")).toHaveCount(0);
  });

  test("empty cluster feeding keeps its range controls visible", async ({
    page,
  }) => {
    const { chores } = await setupWithChores(page);
    const feedBaby = chores.find((c) => c.name === "Feed Baby");
    // No logs posted at all

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    const column = page.locator(".baby-care-column").filter({ hasText: "Cluster Feeding" });
    await expect(column).toBeVisible();
    await expect(column).toContainText("No data");
    await expect(column.locator('[data-action="stats-feeding-gaps-quick"][data-days="1"]')).toBeVisible();
  });
});

test.describe('Feeding gap calendar dates', () => {
  for (const timezoneId of ['Asia/Tokyo', 'America/New_York']) {
    test(`empty Day range retains controls and sends the next date in ${timezoneId}`, async ({ browser }) => {
      const context = await browser.newContext({ timezoneId });
      try {
        const page = await context.newPage();
        // A fixed instant makes the two browsers use different local dates.
        await page.clock.setFixedTime(new Date('2026-09-10T00:30:00Z'));
        await setupWithChores(page);
        await page.click('[data-nav="stats"]');
        const day = page.locator('[data-action="stats-feeding-gaps-quick"][data-days="1"]');
        await expect(day).toBeVisible();
        const response = page.waitForResponse(r => r.url().includes('/api/stats/feeding-gaps?') && new URL(r.url()).searchParams.get('start') === (timezoneId === 'Asia/Tokyo' ? '2026-09-10' : '2026-09-09'));
        await day.click();
        const result = await response;
        expect(result.ok()).toBe(true);
        const url = new URL(result.url());
        expect(url.searchParams.get('end')).toBe(timezoneId === 'Asia/Tokyo' ? '2026-09-11' : '2026-09-10');
        await expect(day).toHaveClass(/period-toggle--active/);
        await expect(page.locator('.feeding-gaps-dates')).toBeVisible();
        await expect(page.locator('.baby-care-column').filter({ has: day })).toContainText('No data');
        const inclusive = await page.evaluate(async () => {
          const { isActiveForDayJS } = await import('/static/js/calendar.js');
          return isActiveForDayJS({ isActive: true, frequencyType: 'daily', recurrenceEnd: '2026-09-10T00:00:00Z' }, '2026-09-10');
        });
        expect(inclusive).toBe(true);
      } finally { await context.close(); }
    });
  }
});
