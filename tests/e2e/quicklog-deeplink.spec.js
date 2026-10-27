import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-deeplink-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Deeplink ${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });
  await page.request.post("/api/chores/seed-defaults", {
    headers: { "X-CSRF-Token": csrf },
  });
  await page.reload();
  await page.waitForSelector(".home-grid", { timeout: 15000 });
  const chores =
    (await (await page.request.get("/api/chores")).json()).chores || [];
  return { chores };
}

test.describe("Quick-log deep links", () => {
  test("?quicklog=chore:<id> opens the pre-filled log sheet (Log now action)", async ({
    page,
  }) => {
    const { chores } = await setup(page);
    const chore = chores.find((c) => c.name === "Wash Dishes") || chores[0];

    await page.goto(`/?quicklog=chore:${chore.id}`);
    // The boot handler opens the home-log sheet for that chore.
    await expect(page.locator(".bottom-sheet")).toBeVisible({ timeout: 10000 });
    await expect(page.locator(".bottom-sheet")).toContainText(chore.name);
    // Query string is cleaned so a refresh doesn't reopen it.
    await expect(page).toHaveURL(/\/$|\/\?$|localhost:\d+\/$/);
  });

  test("?quicklog=feed-baby opens the Feed Baby sheet (manifest shortcut)", async ({
    page,
  }) => {
    const { chores } = await setup(page);
    const feedBaby = chores.find((c) => c.name === "Feed Baby");
    expect(feedBaby).toBeTruthy();

    await page.goto("/?quicklog=feed-baby");
    await expect(page.locator(".bottom-sheet")).toBeVisible({ timeout: 10000 });
    await expect(page.locator(".bottom-sheet")).toContainText("Feed Baby");
  });
});
