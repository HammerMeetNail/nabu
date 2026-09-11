import { test, expect } from "@playwright/test";
import {deferred,observeModuleCalls} from './review-fixtures.js';

test.use({serviceWorkers:'block'});

const BASE = "http://localhost:8080";

function uniqueEmail() {
  return `e2e-da-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function registerUser(page, email, password = "test123456") {
  await page.goto(`${BASE}/register`);
  await page.waitForSelector("#register-form");
  await page.fill("#reg-email", email);
  await page.fill("#reg-password", password);
  await page.fill("#reg-confirm", password);
  await page.click("button[type=\"submit\"]");
  await page.waitForSelector("#hh-indicator:not([hidden])", { timeout: 10000 });
  const csrf =
    (await page.context().cookies()).find((c) => c.name === "nabu_csrf")
      ?.value || "";
  return { csrf };
}

async function setupFullAccount(page) {
  const email = uniqueEmail();
  const { csrf } = await registerUser(page, email);

  await page.request.post(`${BASE}/api/household`, {
    data: { name: `DA Test ${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });
  await page.reload();
  await page.waitForSelector(".home-grid, .empty-state", { timeout: 15000 });
  return { email, csrf };
}

async function navigateToSettings(page) {
  await page.click("a[data-nav=\"settings\"]");
  await page.waitForSelector(".settings-view", { timeout: 5000 });
}

test.describe("Settings: delete account", () => {
  test('a delayed rejection preserves Cancel and subsequent navigation',async({page})=>{
    const joinDeletion=await observeModuleCalls(page,'app','doDeleteAccount',{exported:false});
    await setupFullAccount(page);
    await navigateToSettings(page);
    const entered=deferred(),release=deferred();
    await page.route('**/api/me',async route=>{
      if(route.request().method()!=='DELETE') return route.continue();
      entered.resolve();await release.promise;
      await route.fulfill({status:409,contentType:'application/json',body:JSON.stringify({error:'Transfer ownership first'})});
    });
    try {
      await page.locator('[data-action="open-delete-account"]').click();
      await page.locator('#delete-account-input').fill('DELETE');
      await page.locator('[data-action="confirm-delete-account"]').click();await entered.promise;
      await page.locator('[data-action="cancel-delete-account"]').click();
      await page.locator('[data-nav="today"]').click();
      await expect(page.locator('.settings-view')).toHaveCount(0);
      release.resolve();await joinDeletion();
      await expect(page.locator('.settings-view')).toHaveCount(0);
      await navigateToSettings(page);
      await expect(page.getByTestId('delete-account-confirm')).toHaveCount(0);
      await expect(page.locator('[data-action="open-delete-account"]')).toBeVisible();
    } finally {release.resolve();}
  });
  test("typed confirmation deletes the account and lands on login", async ({
    page,
  }) => {
    const { email } = await setupFullAccount(page);
    await navigateToSettings(page);

    // The flow is collapsed behind an explicit button.
    await page.click("[data-action=\"open-delete-account\"]");
    await expect(
      page.locator("[data-testid=\"delete-account-confirm\"]")
    ).toBeVisible();

    // Wrong confirmation text is rejected client-side.
    await page.fill("#delete-account-input", "delete");
    await page.click("[data-action=\"confirm-delete-account\"]");
    await expect(page.locator("#delete-account-error")).toBeVisible();

    // Typed DELETE destroys the account and logs the user out.
    await page.fill("#delete-account-input", "DELETE");
    await page.click("[data-action=\"confirm-delete-account\"]");
    await page.waitForSelector("#login-form", { timeout: 10000 });

    // The credentials no longer work.
    await page.fill("#login-email", email);
    await page.fill("#login-password", "test123456");
    await page.click("button[type=\"submit\"]");
    await expect(page.locator("#login-error")).toBeVisible({ timeout: 5000 });
  });

  test("cancel collapses the confirmation without deleting", async ({
    page,
  }) => {
    await setupFullAccount(page);
    await navigateToSettings(page);

    await page.click("[data-action=\"open-delete-account\"]");
    await expect(
      page.locator("[data-testid=\"delete-account-confirm\"]")
    ).toBeVisible();

    await page.click("[data-action=\"cancel-delete-account\"]");
    await expect(
      page.locator("[data-testid=\"delete-account-confirm\"]")
    ).toHaveCount(0);
    // Still logged in.
    await expect(page.locator(".settings-view")).toBeVisible();
  });

  test("sole owner of a multi-member household is blocked with guidance", async ({
    page,
    browser,
  }) => {
    const { csrf } = await setupFullAccount(page);

    // Second member joins via invite code.
    const inviteRes = await page.request.post(
      `${BASE}/api/household/invites`,
      { headers: { "X-CSRF-Token": csrf } }
    );
    const invite = (await inviteRes.json()).invite;

    const ctx2 = await browser.newContext();
    const page2 = await ctx2.newPage();
    const { csrf: csrf2 } = await registerUser(page2, uniqueEmail());
    await page2.request.post(`${BASE}/api/household/join`, {
      data: { inviteCode: invite.code },
      headers: { "X-CSRF-Token": csrf2 },
    });
    await ctx2.close();

    await page.reload();
    await navigateToSettings(page);
    await page.click("[data-action=\"open-delete-account\"]");
    await page.fill("#delete-account-input", "DELETE");
    await page.click("[data-action=\"confirm-delete-account\"]");

    // 409 from the server: transfer ownership first. Still logged in.
    await expect(page.locator("#delete-account-error")).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator("#delete-account-error")).toContainText(
      /owner/i
    );
    await expect(page.locator(".settings-view")).toBeVisible();
    await page.locator('[data-nav="today"]').click();
    await navigateToSettings(page);
    await expect(page.locator('#delete-account-error')).toContainText(/owner/i);
  });
});
