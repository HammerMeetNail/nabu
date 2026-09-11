import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-volunit-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Vol Test ${Date.now()}` },
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
  return { email, csrf, feedBaby };
}

test.describe("Volume unit preference", () => {
  test("toggling to oz persists and relabels the Feed Baby volume picker", async ({
    page,
  }) => {
    const { feedBaby } = await setupWithChores(page);
    expect(feedBaby).toBeTruthy();

    // Default is mL; the Settings toggle shows mL active.
    await page.click("a[data-nav=\"settings\"]");
    await page.waitForSelector(".settings-view", { timeout: 10000 });
    const mlBtn = page.locator("[data-action=\"set-volume-unit\"][data-unit=\"ml\"]");
    const ozBtn = page.locator("[data-action=\"set-volume-unit\"][data-unit=\"oz\"]");
    await expect(mlBtn).toHaveClass(/segmented-btn--active/);

    // Switch to oz.
    const saved = page.waitForResponse(response => response.request().method() === 'PATCH'
      && new URL(response.url()).pathname === '/api/preferences'
      && response.request().postDataJSON().volumeUnit === 'oz');
    await ozBtn.click();
    await expect(ozBtn).toHaveClass(/segmented-btn--active/);
    expect((await saved).status()).toBe(200);

    // Persisted server-side.
    const prefs = await (await page.request.get("/api/preferences")).json();
    expect(prefs.preferences.volumeUnit).toBe("oz");

    // Survives a reload.
    await page.reload();
    await page.click("a[data-nav=\"settings\"]");
    await page.waitForSelector(".settings-view", { timeout: 10000 });
    await expect(
      page.locator("[data-action=\"set-volume-unit\"][data-unit=\"oz\"]")
    ).toHaveClass(/segmented-btn--active/);

    // The Feed Baby log sheet volume picker now shows oz-labeled options,
    // while option values remain canonical mL.
    await page.click("a[data-nav=\"today\"]");
    await page.waitForSelector(".home-grid", { timeout: 10000 });
    const feedCard = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await feedCard.click();
    const sheet = page.locator(".bottom-sheet");
    await expect(sheet).toBeVisible({ timeout: 5000 });
    // Formula is default-on, so its volume select is already visible.
    const select = sheet.locator(".indicator-volume-select").first();
    await expect(select).toBeVisible();
    // At least one option is labeled in oz.
    await expect(select.locator("option", { hasText: "oz" }).first()).toHaveCount(1);
  });
});
