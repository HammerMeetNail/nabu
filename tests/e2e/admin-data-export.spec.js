import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-admin-export-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find((c) => c.name === "nabu_csrf")?.value || "";
}

async function setupOwner(page) {
  await page.goto("/register");
  await page.waitForSelector("#register-form");
  await page.fill("#reg-email", uniqueEmail());
  await page.fill("#reg-password", "test123456");
  await page.fill("#reg-confirm", "test123456");
  await page.click('button[type="submit"]');
  await page.waitForSelector("#hh-indicator:not([hidden])", { timeout: 10000 });

  const csrf = await getCSRF(page);
  await page.request.post("/api/household", {
    data: { name: `Admin Export ${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });
  await page.request.post("/api/chores/seed-defaults", {
    headers: { "X-CSRF-Token": csrf },
  });
  await page.reload();
  await page.waitForSelector(".home-grid", { timeout: 15000 });
  return { csrf };
}

async function joinAsUser(browser, code) {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto("/register");
  await page.waitForSelector("#register-form");
  await page.fill("#reg-email", uniqueEmail());
  await page.fill("#reg-password", "test123456");
  await page.fill("#reg-confirm", "test123456");
  await page.click('button[type="submit"]');
  await page.waitForSelector("#hh-indicator:not([hidden])", { timeout: 10000 });

  const csrf = await getCSRF(page);
  const response = await page.request.post("/api/household/join", {
    data: { inviteCode: code },
    headers: { "X-CSRF-Token": csrf },
  });
  expect(response.ok()).toBe(true);
  await page.goto("/");
  await page.waitForSelector(".home-grid", { timeout: 15000 });
  return { page, context, csrf };
}

test.describe("Admin household data export", () => {
  test("owners and admins can export all household data, members cannot", async ({ browser }) => {
    const ownerContext = await browser.newContext();
    const ownerPage = await ownerContext.newPage();
    const { csrf: ownerCsrf } = await setupOwner(ownerPage);

    const chores = (await (await ownerPage.request.get("/api/chores")).json()).chores;
    const chore = chores[0];
    const marker = `all-data-${Date.now()}`;
    await ownerPage.request.post("/api/logs", {
      data: { choreId: chore.id, note: marker, completedAt: new Date().toISOString() },
      headers: { "X-CSRF-Token": ownerCsrf },
    });
    await ownerPage.request.post("/api/schedules", {
      data: { choreId: chore.id, frequencyType: "daily", timePeriod: "anytime" },
      headers: { "X-CSRF-Token": ownerCsrf },
    });
    await ownerPage.request.put("/api/day-notes/2026-06-16", {
      data: { note: `${marker}-note` },
      headers: { "X-CSRF-Token": ownerCsrf },
    });
    const inviteResponse = await ownerPage.request.post("/api/household/invites", {
      headers: { "X-CSRF-Token": ownerCsrf },
    });
    const inviteCode = (await inviteResponse.json()).invite.code;

    const ownerExport = await ownerPage.request.get("/api/household/data");
    expect(ownerExport.status()).toBe(200);
    expect(ownerExport.headers()["content-type"]).toContain("text/csv");
    expect(ownerExport.headers()["content-disposition"]).toContain("nabu-household-data.csv");
    const ownerBody = await ownerExport.text();
    expect(ownerBody).toContain("record_type");
    for (const recordType of ["household", "member", "chore", "log", "schedule", "day_note", "invite"]) {
      expect(ownerBody).toContain(`\n${recordType},`);
    }
    expect(ownerBody).toContain(marker);
    expect(ownerBody).not.toContain(inviteCode);

    await ownerPage.click('a[data-nav="settings"]');
    await ownerPage.waitForSelector(".settings-view", { timeout: 10000 });
    await expect(ownerPage.locator('[data-testid="household-export-section"]')).toBeVisible();
    await expect(ownerPage.locator('button[data-action="export-csv"][data-kind="household"]')).toHaveText(/Export household data as CSV/);

    const { page: memberPage, context: memberContext, csrf: memberCsrf } =
      await joinAsUser(browser, inviteCode);
    const household = await (await ownerPage.request.get("/api/household")).json();
    const member = household.members.find((m) => m.role === "member");
    expect(member).toBeTruthy();

    const promoteResponse = await ownerPage.request.patch(`/api/household/members/${member.userId}`, {
      data: { role: "admin" },
      headers: { "X-CSRF-Token": ownerCsrf },
    });
    expect(promoteResponse.ok()).toBe(true);
    await memberPage.reload();
    await memberPage.waitForSelector(".home-grid", { timeout: 15000 });
    await memberPage.click('a[data-nav="settings"]');
    await memberPage.waitForSelector(".settings-view", { timeout: 10000 });
    await expect(memberPage.locator('[data-testid="household-export-section"]')).toBeVisible();
    expect((await memberPage.request.get("/api/household/data")).status()).toBe(200);

    const demoteResponse = await ownerPage.request.patch(`/api/household/members/${member.userId}`, {
      data: { role: "member" },
      headers: { "X-CSRF-Token": ownerCsrf },
    });
    expect(demoteResponse.ok()).toBe(true);
    await memberPage.reload();
    await memberPage.waitForSelector(".home-grid", { timeout: 15000 });
    await memberPage.click('a[data-nav="settings"]');
    await memberPage.waitForSelector(".settings-view", { timeout: 10000 });
    await expect(memberPage.locator('[data-testid="household-export-section"]')).toHaveCount(0);
    const denied = await memberPage.request.get("/api/household/data", {
      headers: { "X-CSRF-Token": memberCsrf },
    });
    expect(denied.status()).toBe(403);

    await memberContext.close();
    await ownerContext.close();
  });
});
