import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-tz-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

test.describe("Stats timezone awareness", () => {
  test("heatmap groups logs by local date when timezone is set", async ({ page }) => {
    const email = uniqueEmail();

    await page.goto("/register");
    await page.waitForSelector("#register-form");
    await page.fill("#reg-email", email);
    await page.fill("#reg-password", "test123456");
    await page.fill("#reg-confirm", "test123456");
    await page.click("button[type=\"submit\"]");
    await page.waitForSelector("#user-avatar:not([hidden])", { timeout: 10000 });

    const csrf =
      (await page.context().cookies()).find((c) => c.name === "choresy_csrf")
        ?.value || "";

    await page.request.post("/api/household", {
      data: { name: `TZ Test ${Date.now()}` },
      headers: { "X-CSRF-Token": csrf },
    });

    await page.request.post("/api/chores/seed-defaults", {
      headers: { "X-CSRF-Token": csrf },
    });

    const choresResp = await page.request.get("/api/chores");
    const chores = (await choresResp.json()).chores || [];
    const feedCats = chores.find((c) => c.name === "Feed Cats");

    // Set timezone to Asia/Tokyo (UTC+9)
    await page.request.patch("/api/preferences", {
      data: { timezone: "Asia/Tokyo" },
      headers: { "X-CSRF-Token": csrf },
    });

    // Create a log at 2026-05-06T23:30:00Z (UTC).
    // In UTC:  May 6
    // In JST:  May 7 08:30
    const crossBoundaryUTC = "2026-05-06T23:30:00Z";
    const utcDateStr = "2026-05-06";
    const jstDateStr = "2026-05-07";

    await page.request.post("/api/logs", {
      data: {
        choreId: feedCats.id,
        note: "",
        date: utcDateStr,
        completedAt: crossBoundaryUTC,
        hour: 8,
        indicators: [],
      },
      headers: { "X-CSRF-Token": csrf },
    });

    // Query heatmap for the date range that includes May 6-7
    const heatmapResp = await page.request.get(
      "/api/stats/heatmap?start=2026-05-01&end=2026-05-08"
    );
    const heatmap = (await heatmapResp.json()).heatmap || [];

    // The log should appear under the local JST date (May 7), not UTC (May 6)
    const may6Cell = heatmap.find((c) => c.date === "2026-05-06");
    const may7Cell = heatmap.find((c) => c.date === "2026-05-07");

    expect(may6Cell?.count || 0).toBe(0);
    expect(may7Cell?.count || 0).toBe(1);
  });

  test("busy hours reflect local timezone", async ({ page }) => {
    const email = uniqueEmail();

    await page.goto("/register");
    await page.waitForSelector("#register-form");
    await page.fill("#reg-email", email);
    await page.fill("#reg-password", "test123456");
    await page.fill("#reg-confirm", "test123456");
    await page.click("button[type=\"submit\"]");
    await page.waitForSelector("#user-avatar:not([hidden])", { timeout: 10000 });

    const csrf =
      (await page.context().cookies()).find((c) => c.name === "choresy_csrf")
        ?.value || "";

    await page.request.post("/api/household", {
      data: { name: `TZ Busy Test ${Date.now()}` },
      headers: { "X-CSRF-Token": csrf },
    });

    await page.request.post("/api/chores/seed-defaults", {
      headers: { "X-CSRF-Token": csrf },
    });

    const choresResp = await page.request.get("/api/chores");
    const chores = (await choresResp.json()).chores || [];
    const feedCats = chores.find((c) => c.name === "Feed Cats");

    // Set timezone to America/New_York (UTC-4 in May)
    await page.request.patch("/api/preferences", {
      data: { timezone: "America/New_York" },
      headers: { "X-CSRF-Token": csrf },
    });

    // Create a log at 2026-05-06T03:00:00Z (UTC).
    // In UTC:  hour 3
    // In EDT:  May 5 23:00 (hour 23)
    const logTime = "2026-05-06T03:00:00Z";
    const logDate = "2026-05-05";

    await page.request.post("/api/logs", {
      data: {
        choreId: feedCats.id,
        note: "",
        date: logDate,
        completedAt: logTime,
        hour: 23,
        indicators: [],
      },
      headers: { "X-CSRF-Token": csrf },
    });

    // Query busy hours
    const busyResp = await page.request.get(
      "/api/stats/busy-hours?start=2026-05-01&end=2026-05-08"
    );
    const busyHours = (await busyResp.json()).busyHours || [];

    // The log should appear in hour 23 (EDT), not hour 3 (UTC)
    const hour3 = busyHours.find((h) => h.hour === 3);
    const hour23 = busyHours.find((h) => h.hour === 23);

    expect(hour3?.count || 0).toBe(0);
    expect(hour23?.count || 0).toBe(1);
  });

  test("heatmap includes today when using default end date", async ({ page }) => {
    const email = uniqueEmail();

    await page.goto("/register");
    await page.waitForSelector("#register-form");
    await page.fill("#reg-email", email);
    await page.fill("#reg-password", "test123456");
    await page.fill("#reg-confirm", "test123456");
    await page.click("button[type=\"submit\"]");
    await page.waitForSelector("#user-avatar:not([hidden])", { timeout: 10000 });

    const csrf =
      (await page.context().cookies()).find((c) => c.name === "choresy_csrf")
        ?.value || "";

    // Reload the page to get a fresh session with the household state
    await page.goto("/today");
    await page.waitForSelector("#app");

    await page.request.post("/api/household", {
      data: { name: `TZ Today Test ${Date.now()}` },
      headers: { "X-CSRF-Token": csrf },
    });

    await page.request.post("/api/chores/seed-defaults", {
      headers: { "X-CSRF-Token": csrf },
    });

    const choresResp = await page.request.get("/api/chores");
    const chores = (await choresResp.json()).chores || [];
    const feedCats = chores.find((c) => c.name === "Feed Cats");

    // Set timezone to America/New_York (EDT/EST)
    await page.request.patch("/api/preferences", {
      data: { timezone: "America/New_York" },
      headers: { "X-CSRF-Token": csrf },
    });

    // Use a fixed log time that is definitely today in America/New_York.
    // noon UTC = 8 AM EDT, which is safely within today for any EDT day.
    const now = new Date();
    const logTime = new Date(
      Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate(), 16, 0, 0)
    );
    const completedAt = logTime.toISOString();
    const todayNY = logTime.toLocaleString("en-CA", {
      timeZone: "America/New_York",
    }).slice(0, 10);

    await page.request.post("/api/logs", {
      data: {
        choreId: feedCats.id,
        note: "",
        date: todayNY,
        completedAt,
        hour: 12,
        indicators: [],
      },
      headers: { "X-CSRF-Token": csrf },
    });

    // Query heatmap without explicit start/end (default behavior)
    const heatmapResp = await page.request.get("/api/stats/heatmap");
    const heatmap = (await heatmapResp.json()).heatmap || [];

    const todayCell = heatmap.find((c) => c.date === todayNY);
    expect(todayCell?.count || 0).toBeGreaterThanOrEqual(1);
  });

  test("no timezone set falls back to UTC", async ({ page }) => {
    const email = uniqueEmail();

    await page.goto("/register");
    await page.waitForSelector("#register-form");
    await page.fill("#reg-email", email);
    await page.fill("#reg-password", "test123456");
    await page.fill("#reg-confirm", "test123456");
    await page.click("button[type=\"submit\"]");
    await page.waitForSelector("#user-avatar:not([hidden])", { timeout: 10000 });

    const csrf =
      (await page.context().cookies()).find((c) => c.name === "choresy_csrf")
        ?.value || "";

    await page.request.post("/api/household", {
      data: { name: `TZ Default Test ${Date.now()}` },
      headers: { "X-CSRF-Token": csrf },
    });

    await page.request.post("/api/chores/seed-defaults", {
      headers: { "X-CSRF-Token": csrf },
    });

    const choresResp = await page.request.get("/api/chores");
    const chores = (await choresResp.json()).chores || [];
    const feedCats = chores.find((c) => c.name === "Feed Cats");

    // Do NOT set any timezone — defaults to UTC

    // Create a log at 2026-05-06T10:00:00Z
    const logTime = "2026-05-06T10:00:00Z";

    await page.request.post("/api/logs", {
      data: {
        choreId: feedCats.id,
        note: "",
        date: "2026-05-06",
        completedAt: logTime,
        hour: 10,
        indicators: [],
      },
      headers: { "X-CSRF-Token": csrf },
    });

    // Query heatmap
    const heatmapResp = await page.request.get(
      "/api/stats/heatmap?start=2026-05-01&end=2026-05-08"
    );
    const heatmap = (await heatmapResp.json()).heatmap || [];

    const may6Cell = heatmap.find((c) => c.date === "2026-05-06");
    expect(may6Cell?.count || 0).toBe(1);

    // Query busy hours — no timezone set, so falls back to system local time.
    const busyResp = await page.request.get(
      "/api/stats/busy-hours?start=2026-05-01&end=2026-05-08"
    );
    const busyHours = (await busyResp.json()).busyHours || [];

    // Exactly one log should appear, grouped into whichever local hour the
    // server's system timezone maps 10:00 UTC to (hour 6 on EDT, hour 10 on UTC).
    const totalBusyCount = busyHours.reduce((sum, h) => sum + h.count, 0);
    expect(totalBusyCount).toBe(1);
  });
});
