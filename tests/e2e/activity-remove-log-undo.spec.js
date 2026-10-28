import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-rmundo-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `RmUndo ${Date.now()}` },
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

test.describe("Activity: remove-log undo", () => {
  test("removing a log from the edit sheet is undoable via toast", async ({
    page,
  }) => {
    const { csrf, chores } = await setupWithChores(page);
    const chore = chores.find((c) => c.name === "Wash Dishes") || chores[0];
    const now = new Date();
    await page.request.post("/api/logs", {
      data: {
        choreId: chore.id,
        note: "e2e-undo-marker",
        date: now.toISOString().slice(0, 10),
        completedAt: now.toISOString(),
        hour: now.getHours(),
      },
      headers: { "X-CSRF-Token": csrf },
    });

    await page.click("a[data-nav=\"activity\"]");
    await page.waitForSelector(".history-view", { timeout: 10000 });

    // Open the log's edit sheet and remove it.
    const row = page.locator(".hist-row", { hasText: "e2e-undo-marker" }).first();
    await expect(row).toBeVisible();
    await row.click();
    await expect(page.locator(".bottom-sheet")).toBeVisible({ timeout: 5000 });
    await page.click("[data-action=\"undo-chore\"]");

    // The row disappears and a "Log removed" undo toast appears.
    await expect(
      page.locator(".hist-row", { hasText: "e2e-undo-marker" })
    ).toHaveCount(0, { timeout: 5000 });
    const toast = page.locator("#toast-container .toast", { hasText: "Log removed" });
    await expect(toast).toBeVisible({ timeout: 5000 });

    // Undo restores the log.
    await toast.locator("button", { hasText: "Undo" }).click();
    await expect(
      page.locator(".hist-row", { hasText: "e2e-undo-marker" })
    ).toHaveCount(1, { timeout: 5000 });
  });
});
