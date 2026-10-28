import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-heatmap-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Heatmap Test ${Date.now()}` },
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

test.describe("Stats heatmap tap tooltips", () => {
  test("tapping a cell reveals a tooltip; tapping away dismisses it", async ({
    page,
  }) => {
    const { csrf, chores } = await setupWithChores(page);
    const chore = chores[0];
    expect(chore).toBeTruthy();

    // Log a chore today so at least one heatmap cell has a non-zero count.
    const now = new Date();
    await page.request.post("/api/logs", {
      data: {
        choreId: chore.id,
        note: "",
        date: now.toISOString().slice(0, 10),
        completedAt: now.toISOString(),
        hour: now.getHours(),
      },
      headers: { "X-CSRF-Token": csrf },
    });

    await page.click("a[data-nav=\"stats\"]");
    await page.waitForSelector(".stats-page", { timeout: 10000 });

    const wrap = page.locator(".heatmap-wrap").first();
    await expect(wrap).toBeVisible({ timeout: 5000 });

    // Every cell is keyboard/tap accessible with an aria-label.
    const cells = wrap.locator(".heatmap-cell[data-action=\"heatmap-tap\"]");
    expect(await cells.count()).toBeGreaterThan(0);
    await expect(cells.first()).toHaveAttribute("aria-label", /chore/);

    // The most recent cell (bottom-right) is today's cell with our log.
    const lastCell = cells.last();
    const tooltip = wrap.locator(".heatmap-tooltip");
    await expect(tooltip).not.toHaveClass(/heatmap-tooltip--visible/);

    await lastCell.click();
    await expect(tooltip).toHaveClass(/heatmap-tooltip--visible/);
    await expect(tooltip).toContainText("chore");

    // Tapping the same cell again dismisses it.
    await lastCell.click();
    await expect(tooltip).not.toHaveClass(/heatmap-tooltip--visible/);

    // Reveal again, then tap away (on the page heading) → dismissed.
    await lastCell.click();
    await expect(tooltip).toHaveClass(/heatmap-tooltip--visible/);
    await page.locator(".stats-page h3").first().click();
    await expect(tooltip).not.toHaveClass(/heatmap-tooltip--visible/);
  });
});
