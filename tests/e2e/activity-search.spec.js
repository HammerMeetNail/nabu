import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-search-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setup(page) {
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
    data: { name: `Search ${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });
  await page.request.post("/api/chores/seed-defaults", {
    headers: { "X-CSRF-Token": csrf },
  });
  await page.reload();
  await page.waitForSelector(".home-grid", { timeout: 15000 });
  const chores =
    (await (await page.request.get("/api/chores")).json()).chores || [];
  return { csrf, chores };
}

test.describe("Activity: search + day counts", () => {
  test("text search filters history by note; day header shows a count", async ({
    page,
  }) => {
    const { csrf, chores } = await setup(page);
    const chore = chores[0];
    const now = new Date();
    const date = now.toISOString().slice(0, 10);
    for (const note of ["water filter changed", "took out recycling", "fed the fish"]) {
      await page.request.post("/api/logs", {
        data: { choreId: chore.id, note, date, completedAt: new Date().toISOString(), hour: now.getHours() },
        headers: { "X-CSRF-Token": csrf },
      });
    }

    await page.click("a[data-nav=\"activity\"]");
    await page.waitForSelector(".history-view", { timeout: 10000 });

    // Day header shows a count chip (3 logs today).
    await expect(page.locator(".hist-day-count").first()).toHaveText("3");

    // Search narrows to the matching note.
    const input = page.locator("#history-search-input");
    await expect(input).toBeVisible();
    await input.fill("filter");
    await expect(page.locator(".hist-row")).toHaveCount(1, { timeout: 5000 });
    await expect(page.locator(".hist-row").first()).toContainText("water filter changed");

    // Clearing the search restores the full list.
    await input.fill("");
    await expect(page.locator(".hist-row")).toHaveCount(3, { timeout: 5000 });
  });
});
