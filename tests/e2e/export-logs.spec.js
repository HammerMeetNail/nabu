import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-export-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setup(page) {
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
    data: { name: `Export ${Date.now()}` },
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

test.describe("CSV export", () => {
  test("exports logged activity as CSV and offers a Settings button", async ({
    page,
  }) => {
    const { csrf, chores } = await setup(page);
    const chore = chores[0];
    await page.request.post("/api/logs", {
      data: {
        choreId: chore.id,
        note: "csv-export-marker",
        completedAt: new Date().toISOString(),
      },
      headers: { "X-CSRF-Token": csrf },
    });

    const res = await page.request.get("/api/logs/export?start=2000-01-01");
    expect(res.status()).toBe(200);
    expect(res.headers()["content-type"]).toContain("text/csv");
    expect(res.headers()["content-disposition"]).toContain("attachment");
    const body = await res.text();
    expect(body).toContain("date,time,chore,member,title,note");
    expect(body).toContain("csv-export-marker");
    expect(body).toContain(chore.name);

    // Settings surfaces the export link.
    await page.click("a[data-nav=\"settings\"]");
    await page.waitForSelector(".settings-view", { timeout: 10000 });
    const link = page.locator('button[data-action="export-csv"][data-kind="logs"]');
    await expect(link).toBeVisible();
    await expect(link).toHaveText(/Export logs as CSV/);
  });
});
