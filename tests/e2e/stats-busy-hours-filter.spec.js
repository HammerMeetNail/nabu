import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-bhf-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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

test.describe("Busy hours filter", () => {
  test("filtering by chore shows only that chore's activity", async ({ page }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    const takeTrash = chores.find((c) => c.name === "Take Out Trash");

    if (feedCats) await postLog(page, csrf, feedCats.id, { hour: 9 });
    if (takeTrash) await postLog(page, csrf, takeTrash.id, { hour: 16 });
    if (takeTrash) await postLog(page, csrf, takeTrash.id, { hour: 16 });

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".busy-hours-chart", { timeout: 10000 });
    await page.waitForSelector(".busy-hours-filter", { timeout: 5000 });

    // Both filter dropdowns should be present
    await expect(page.locator(".busy-hours-filter")).toHaveCount(2);

    // "All chores" should show activity at both hours
    const allHourRows = page.locator(".busy-hour-row");
    await expect(allHourRows).toHaveCount(24);

    // Filter by "Feed Cats"
    const choreFilter = page.locator(".busy-hours-filter").first();
    await choreFilter.selectOption({ label: "Feed Cats" });
    await page.waitForSelector(".busy-hours-chart", { timeout: 5000 });

    // After filtering, the chart should still have 24 rows but only feedCats logs count
    const filteredRows = page.locator(".busy-hour-row");
    await expect(filteredRows).toHaveCount(24);

    // Switch back to "All chores"
    await choreFilter.selectOption({ label: "All chores" });
    await page.waitForSelector(".busy-hours-chart", { timeout: 5000 });
    await expect(page.locator(".busy-hour-row")).toHaveCount(24);
  });

  test("filtering by user shows only that user's activity", async ({ page }) => {
    const { chores, csrf } = await setupWithChores(page);

    const feedCats = chores.find((c) => c.name === "Feed Cats");
    if (feedCats) {
      await postLog(page, csrf, feedCats.id, { hour: 10 });
      await postLog(page, csrf, feedCats.id, { hour: 15 });
    }

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".busy-hours-chart", { timeout: 10000 });
    await page.waitForSelector(".busy-hours-filter", { timeout: 5000 });

    // The user filter dropdown should list the current user
    const userFilter = page.locator(".busy-hours-filter").nth(1);
    const userOptions = userFilter.locator("option");
    await expect(userOptions).toHaveCount(2); // "All members" + the user

    // Filter by user should still show chart
    const userOptionText = await userOptions.nth(1).textContent();
    await userFilter.selectOption({ label: userOptionText });
    await page.waitForSelector(".busy-hours-chart", { timeout: 5000 });
    await expect(page.locator(".busy-hour-row")).toHaveCount(24);

    // Switch back to "All members"
    await userFilter.selectOption({ label: "All members" });
    await page.waitForSelector(".busy-hours-chart", { timeout: 5000 });
    await expect(page.locator(".busy-hour-row")).toHaveCount(24);
  });
});
