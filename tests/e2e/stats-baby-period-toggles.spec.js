import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-bp-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Baby Period Test ${Date.now()}` },
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
  const changeBaby = chores.find((c) => c.name === "Change Baby");

  return { email, csrf, chores, feedBaby, changeBaby };
}

async function postLog(page, csrf, choreId, opts = {}) {
  const { volumeML, indicatorVolumes, hours, minutes } = opts;
  const body = {
    choreId,
    note: "",
    date: new Date().toISOString().slice(0, 10),
    completedAt: new Date().toISOString(),
  };
  if (hours !== undefined || minutes !== undefined) {
    const now = new Date();
    const h = hours ?? now.getHours();
    const m = minutes ?? now.getMinutes();
    const d = new Date(
      now.getFullYear(),
      now.getMonth(),
      now.getDate(),
      h,
      m,
      0,
    );
    body.completedAt = d.toISOString();
    body.hour = h;
  }
  if (volumeML !== undefined) body.volumeML = volumeML;
  if (indicatorVolumes) body.indicatorVolumes = indicatorVolumes;
  await page.request.post("/api/logs", {
    data: body,
    headers: { "X-CSRF-Token": csrf },
  });
}

test.describe("Stats baby period toggles", () => {
  test("each baby column has its own period selector; cluster feeding selector sits in the header", async ({
    page,
  }) => {
    const { csrf, feedBaby, changeBaby } = await setupWithChores(page);
    expect(feedBaby).toBeTruthy();
    expect(changeBaby).toBeTruthy();

    // Feed Baby logs with volumes close together so the cluster feeding column
    // (which needs 2+ feeds within 2 hours) also renders.
    await postLog(page, csrf, feedBaby.id, {
      hours: 9,
      minutes: 0,
      volumeML: 120,
      indicatorVolumes: { "\u{1F37C} formula": 120 },
    });
    await postLog(page, csrf, feedBaby.id, {
      hours: 9,
      minutes: 40,
      volumeML: 40,
      indicatorVolumes: { "\u{1F37C} formula": 40 },
    });

    // Change Baby logs so its column renders.
    await postLog(page, csrf, changeBaby.id, { hours: 10, minutes: 0 });
    await postLog(page, csrf, changeBaby.id, { hours: 14, minutes: 30 });

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    // The shared period toggle in the section header has been removed.
    await expect(
      page.locator(".baby-care-header .period-toggle"),
    ).toHaveCount(0);

    const feedCol = page
      .locator(".baby-care-column")
      .filter({ hasText: "Feed Baby" });
    const changeCol = page
      .locator(".baby-care-column")
      .filter({ hasText: "Change Baby" });
    const gapsCol = page
      .locator(".baby-care-column")
      .filter({ hasText: "Cluster Feeding" });

    await expect(feedCol).toBeVisible({ timeout: 5000 });
    await expect(changeCol).toBeVisible();
    await expect(gapsCol).toBeVisible();

    // Each baby column has its own Daily/Weekly/Monthly toggle, defaulting to Daily.
    for (const col of [feedCol, changeCol]) {
      const toggle = col.locator(".baby-col-header .period-toggle");
      await expect(toggle).toBeVisible();
      await expect(toggle.locator(".period-toggle-btn")).toHaveCount(3);
      await expect(
        toggle.locator(".period-toggle-btn", { hasText: "Daily" }),
      ).toHaveClass(/\bperiod-toggle--active\b/);
    }

    // The cluster feeding quick selector lives inside the column header row,
    // to the right of the title (not as a separate block below it).
    const gapsHeader = gapsCol.locator(".feeding-gaps-header");
    await expect(gapsHeader).toBeVisible();
    await expect(
      gapsHeader.locator(".feeding-gaps-quick .period-toggle-btn"),
    ).toHaveCount(3);

    // The whole quick toggle group must be inside the header, so a button
    // clicked from there updates state.
    await gapsHeader.locator('.feeding-gaps-quick [data-days="14"]').click();
    await expect(
      gapsHeader.locator('.feeding-gaps-quick [data-days="14"]'),
    ).toHaveClass(/\bperiod-toggle--active\b/);

    // Toggling a period only affects that column: switch Feed Baby to Weekly.
    await feedCol
      .locator(".baby-col-header .period-toggle-btn", { hasText: "Weekly" })
      .click();

    await expect(
      feedCol.locator(".baby-col-header .period-toggle-btn", {
        hasText: "Weekly",
      }),
    ).toHaveClass(/\bperiod-toggle--active\b/);
    // Change Baby column should remain on Daily.
    await expect(
      changeCol.locator(".baby-col-header .period-toggle-btn", {
        hasText: "Daily",
      }),
    ).toHaveClass(/\bperiod-toggle--active\b/);
  });
});
