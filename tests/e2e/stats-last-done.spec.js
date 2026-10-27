import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-lastdone-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
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
    data: { name: `LastDone ${Date.now()}` },
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

test.describe("Stats: Last done section", () => {
  test("shows time-since per chore, most-recent first", async ({ page }) => {
    const { csrf, chores } = await setupWithChores(page);
    const feedBaby = chores.find((c) => c.name === "Feed Baby");
    const now = new Date();
    await page.request.post("/api/logs", {
      data: {
        choreId: feedBaby.id,
        note: "",
        date: now.toISOString().slice(0, 10),
        completedAt: now.toISOString(),
        hour: now.getHours(),
      },
      headers: { "X-CSRF-Token": csrf },
    });

    // Refresh latest-per-chore state by reloading, then go to stats.
    await page.reload();
    await page.waitForSelector(".home-grid", { timeout: 15000 });
    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    const section = page.locator(".card", { hasText: "Last done" }).first();
    await expect(section).toBeVisible();
    const list = section.locator(".last-done-list");
    await expect(list).toBeVisible();

    // Feed Baby (just logged) is first and shows a recent "ago" label.
    const firstRow = section.locator(".last-done-row").first();
    await expect(firstRow).toContainText("Feed Baby");
    await expect(firstRow.locator(".last-done-ago")).toContainText(/ago|just now/);

    // A never-logged chore shows "never".
    await expect(section.locator(".last-done-ago--never").first()).toBeVisible();
  });
});
