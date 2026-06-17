import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-sc-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Customize Test ${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });

  await page.request.post("/api/chores/seed-defaults", {
    headers: { "X-CSRF-Token": csrf },
  });

  await page.reload();
  await page.waitForSelector(".home-grid", { timeout: 15000 });

  return { email, csrf };
}

test.describe("Stats customize", () => {
  test("customize panel toggles open and closed", async ({ page }) => {
    await setupWithChores(page);

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page");

    await expect(page.locator(".customize-panel")).toHaveCount(0);

    await page.click("button[data-action=\"toggle-customize-stats\"]");
    await expect(page.locator(".customize-panel")).toBeVisible();
    await expect(page.locator("button[data-action=\"toggle-customize-stats\"]")).toHaveText("Done");

    await page.click("button[data-action=\"toggle-customize-stats\"]");
    await expect(page.locator(".customize-panel")).toHaveCount(0);
    await expect(page.locator("button[data-action=\"toggle-customize-stats\"]")).toHaveText("Customize");
  });

  test("stats overview cannot be hidden", async ({ page }) => {
    await setupWithChores(page);

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page");

    await page.click("button[data-action=\"toggle-customize-stats\"]");
    await expect(page.locator(".customize-panel")).toBeVisible();

    const overviewCheckbox = page.locator(".customize-row[data-section=\"overview\"] input[type=\"checkbox\"]");
    await expect(overviewCheckbox).toBeChecked();
    await expect(overviewCheckbox).toBeDisabled();
  });

  test("stats section can be hidden and persists after reload", async ({ page }) => {
    await setupWithChores(page);

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page");
    await expect(page.locator("h3:has-text(\"Leaderboard\")")).toBeVisible();

    await page.click("button[data-action=\"toggle-customize-stats\"]");
    await expect(page.locator(".customize-panel")).toBeVisible();

    const leaderboardRow = page.locator(".customize-row[data-section=\"leaderboard\"]");
    await leaderboardRow.locator("input[type=\"checkbox\"]").uncheck();

    await expect(page.locator("h3:has-text(\"Leaderboard\")")).toHaveCount(0);

    await page.reload();
    await page.waitForSelector(".home-grid", { timeout: 15000 });
    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page");
    await expect(page.locator("h3:has-text(\"Leaderboard\")")).toHaveCount(0);
  });

  test("stats baby section stays visible by default for seeded household", async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Log a Feed Baby entry so baby data exists
    const choresResp = await (await page.request.get("/api/chores")).json();
    const chores = choresResp.chores || [];
    const feedBaby = chores.find(c => c.name === "Feed Baby");
    if (feedBaby) {
      await page.request.post("/api/logs", {
        data: {
          choreId: feedBaby.id,
          note: "",
          indicators: ["🤱 breast"],
          volumeML: 120,
          date: new Date().toISOString().slice(0, 10),
          completedAt: new Date().toISOString(),
          hour: 12,
        },
        headers: { "X-CSRF-Token": csrf },
      });
    }

    await page.reload();
    await page.waitForSelector(".home-grid", { timeout: 15000 });
    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page");

    // Baby section should be visible without opening customize
    await expect(page.locator("h3:has-text(\"Baby\")")).toBeVisible();
  });

  test("stats baby section can be hidden and re-shown", async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresResp = await (await page.request.get("/api/chores")).json();
    const chores = choresResp.chores || [];
    const feedBaby = chores.find(c => c.name === "Feed Baby");
    if (feedBaby) {
      await page.request.post("/api/logs", {
        data: {
          choreId: feedBaby.id,
          note: "",
          indicators: ["🤱 breast"],
          volumeML: 120,
          date: new Date().toISOString().slice(0, 10),
          completedAt: new Date().toISOString(),
          hour: 12,
        },
        headers: { "X-CSRF-Token": csrf },
      });
    }

    await page.reload();
    await page.waitForSelector(".home-grid", { timeout: 15000 });
    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page");
    await expect(page.locator("h3:has-text(\"Baby\")")).toBeVisible();

    await page.click("button[data-action=\"toggle-customize-stats\"]");
    const babyRow = page.locator(".customize-row[data-section=\"baby\"]");
    await babyRow.locator("input[type=\"checkbox\"]").uncheck();
    await expect(page.locator("h3:has-text(\"Baby\")")).toHaveCount(0);

    await page.reload();
    await page.waitForSelector(".home-grid", { timeout: 15000 });
    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page");
    await expect(page.locator("h3:has-text(\"Baby\")")).toHaveCount(0);

    await page.click("button[data-action=\"toggle-customize-stats\"]");
    await babyRow.locator("input[type=\"checkbox\"]").check();
    await expect(page.locator("h3:has-text(\"Baby\")")).toBeVisible();
  });
});
