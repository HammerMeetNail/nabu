import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-caps-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Caps ${Date.now()}` },
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

test.describe("Per-field input caps (audit #10)", () => {
  test("server rejects over-limit log fields and accepts at-limit", async ({
    page,
  }) => {
    const { csrf, chores } = await setup(page);
    const chore = chores[0];
    const base = {
      choreId: chore.id,
      completedAt: new Date().toISOString(),
    };
    const headers = { "X-CSRF-Token": csrf };

    const noteOver = await page.request.post("/api/logs", {
      data: { ...base, note: "a".repeat(2001) },
      headers,
    });
    expect(noteOver.status()).toBe(400);

    const titleOver = await page.request.post("/api/logs", {
      data: { ...base, title: "a".repeat(121), note: "" },
      headers,
    });
    expect(titleOver.status()).toBe(400);

    const hourBad = await page.request.post("/api/logs", {
      data: { ...base, hour: 24, note: "" },
      headers,
    });
    expect(hourBad.status()).toBe(400);

    const ok = await page.request.post("/api/logs", {
      data: { ...base, title: "a".repeat(120), note: "a".repeat(2000), hour: 23 },
      headers,
    });
    expect(ok.status()).toBe(201);
  });

  test("server rejects unknown timezone and over-limit household name", async ({
    page,
  }) => {
    const { csrf } = await setup(page);
    const headers = { "X-CSRF-Token": csrf };

    const tz = await page.request.patch("/api/preferences", {
      data: { timezone: "Mars/Olympus" },
      headers,
    });
    expect(tz.status()).toBe(400);

    const name = await page.request.patch("/api/household", {
      data: { name: "n".repeat(61), initials: "" },
      headers,
    });
    expect(name.status()).toBe(400);
  });

  test("PWA log sheet enforces the server caps in the UI", async ({ page }) => {
    const { csrf, chores } = await setup(page);
    const rated = chores.find((c) => c.hasRating) || chores[0];

    await page.request.post("/api/schedules", {
      data: {
        choreId: rated.id,
        timePeriod: "anytime",
        specificTime: "08:00",
        frequencyType: "daily",
      },
      headers: { "X-CSRF-Token": csrf },
    });

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector(".sch-row-main", { timeout: 10000 });
    await page.locator(".sch-row-main").first().click();
    await page.waitForSelector("#log-note", { timeout: 10000 });

    const title = page.locator("#log-title");
    if (await title.count()) {
      await expect(title).toHaveAttribute("maxlength", "120");
    }
    await expect(page.locator("#log-note")).toHaveAttribute("maxlength", "2000");
  });
});
