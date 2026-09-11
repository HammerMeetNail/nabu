import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-offline-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Offline ${Date.now()}` },
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

test.describe("Offline log queue", () => {
  test("a log made offline is queued and replayed on reconnect", async ({
    page,
    context,
  }) => {
    const { csrf, chores } = await setup(page);
    const chore = chores.find((c) => c.name === "Wash Dishes") || chores[0];

    // Open the log sheet for the chore.
    const card = page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    await card.click();
    await expect(page.locator(".bottom-sheet")).toBeVisible({ timeout: 5000 });

    // Go offline, then submit — the POST fails and must be queued, not lost.
    await context.setOffline(true);
    await page.click('[data-action="save-log"]');

    await expect(
      page.locator("#toast-container .toast", { hasText: "Saved on this device. Waiting to sync." })
    ).toBeVisible({ timeout: 5000 });

    // Nothing persisted server-side yet.
    let latest = await (await page.request.get("/api/logs/latest-per-chore")).json();
    expect(latest.latestLogs?.[chore.id]).toBeUndefined();

    // Reconnect: the queue replays automatically and the log lands.
    await context.setOffline(false);
    await expect(
      page.locator("#toast-container .toast", { hasText: /Synced 1 log/ })
    ).toBeVisible({ timeout: 8000 });

    latest = await (await page.request.get("/api/logs/latest-per-chore")).json();
    expect(latest.latestLogs?.[chore.id]).toBeTruthy();
  });

  test("idempotency: replaying the same key does not duplicate", async ({
    page,
  }) => {
    const { csrf, chores } = await setup(page);
    const chore = chores[0];
    const key = `e2e-idem-${Date.now()}`;
    const body = {
      choreId: chore.id,
      note: "idem",
      date: new Date().toISOString().slice(0, 10),
      completedAt: new Date().toISOString(),
      idempotencyKey: key,
    };
    const post = () =>
      page.request.post("/api/logs", { data: body, headers: { "X-CSRF-Token": csrf } });

    const r1 = await post();
    const r2 = await post();
    const l1 = (await r1.json()).log;
    const l2 = (await r2.json()).log;
    expect(l1.id).toBe(l2.id); // same log returned, not a duplicate

    const hist = await (await page.request.get("/api/logs/history")).json();
    const matches = (hist.logs || []).filter((l) => l.note === "idem");
    expect(matches.length).toBe(1);
  });
});
