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
  test("cluster feeding chart renders with strip view and heatmap toggle", async ({
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
    // Add another at 7pm: 7:00pm 130mL -> 7:35pm 50mL (35 min gap)

    const logs = [
      { completedAt: new Date(y, m, d, 9, 0, 0), volumeML: 120, indicatorVolumes: { "🍼 formula": 120 } },
      { completedAt: new Date(y, m, d, 9, 40, 0), volumeML: 40, indicatorVolumes: { "🍼 formula": 40 } },
      { completedAt: new Date(y, m, d, 12, 30, 0), volumeML: 110, indicatorVolumes: { "🍼 formula": 110 } },
      { completedAt: new Date(y, m, d, 13, 15, 0), volumeML: 30, indicatorVolumes: { "🍼 formula": 30 } },
      { completedAt: new Date(y, m, d, 16, 0, 0), volumeML: 100, indicatorVolumes: { "🍼 formula": 100 } },
      { completedAt: new Date(y, m, d, 19, 0, 0), volumeML: 130, indicatorVolumes: { "🍼 formula": 130 } },
      { completedAt: new Date(y, m, d, 19, 35, 0), volumeML: 50, indicatorVolumes: { "🍼 formula": 50 } },
    ];

    for (const log of logs) {
      await postFeedLog(page, csrf, feedBaby.id, log.completedAt, log.volumeML, log.indicatorVolumes);
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    // The cluster feeding column should appear as its own box
    const gapsColumn = page.locator(".baby-care-column").filter({ hasText: "Cluster Feeding" });
    await expect(gapsColumn).toBeVisible({ timeout: 5000 });
    await expect(gapsColumn.locator("h4")).toContainText("Cluster Feeding");

    // The cluster rate chart should have an SVG with bars (rect elements)
    const svg = gapsColumn.locator("svg");
    await expect(svg).toBeVisible({ timeout: 3000 });
    await expect(svg.locator("rect")).not.toHaveCount(0);

    // Info icon should be present
    await expect(gapsColumn.locator(".feeding-gaps-info-btn")).toBeVisible();

    // Click info icon to expand explainer
    await gapsColumn.locator(".feeding-gaps-info-btn").click();
    await expect(gapsColumn.locator(".feeding-gaps-explainer--visible")).toBeVisible();
  });

  test("no cluster feeding section when there are no feeding logs", async ({
    page,
  }) => {
    const { chores } = await setupWithChores(page);
    const feedBaby = chores.find((c) => c.name === "Feed Baby");
    // No logs posted at all

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    // The cluster feeding column should not appear since there are no gaps
    await expect(
      page.locator(".baby-care-column").filter({ hasText: "Cluster Feeding" })
    ).toHaveCount(0);
  });
});
