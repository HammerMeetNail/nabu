// tests/e2e/settings-disable-member.spec.js
// Verifies that an owner can disable a member ("babysitter") from the
// Settings tab: the sitter is signed out, sees an "access disabled" message,
// cannot access household APIs, and regains access when re-enabled.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-dis-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
}

async function setupOwnerWithHousehold(page, hhName) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  const email = uniqueEmail();
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = await getCSRF(page);
  await page.request.post('/api/household', {
    data: { name: hhName },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.reload();
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 15000 });
  return { csrf };
}

async function joinAsSitter(browser, code) {
  const context = await browser.newContext();
  const page = await context.newPage();
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = await getCSRF(page);
  const joinRes = await page.request.post('/api/household/join', {
    data: { inviteCode: code },
    headers: { 'X-CSRF-Token': csrf },
  });
  if (!joinRes.ok()) {
    throw new Error(`join failed: ${joinRes.status()} ${await joinRes.text()}`);
  }
  await page.goto('/');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 15000 });
  return { page, context, email };
}

test.describe('Settings - Disable Member Access', () => {
  test('owner disables the sitter; she is signed out and blocked until re-enabled', async ({ browser }) => {
    const hhName = `Dis Test ${Date.now()}`;
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();
    const { csrf: ownerCsrf } = await setupOwnerWithHousehold(ownerPage, hhName);

    // Invite the sitter.
    const inviteRes = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const { invite } = await inviteRes.json();
    const { page: sitterPage, context: sitterCtx, email: sitterEmail } =
      await joinAsSitter(browser, invite.code);

    // Identify the sitter's user ID from the household members list.
    const hhData = await (await ownerPage.request.get('/api/household')).json();
    const ownerId = hhData.members.find(m => m.role === 'owner').userId;
    const sitterId = hhData.members.find(m => m.userId !== ownerId).userId;

    // Owner: Settings → expand sitter row → Disable access (confirm).
    await ownerPage.goto('/settings');
    await ownerPage.waitForSelector('.member-list', { timeout: 10000 });
    const sitterRow = ownerPage.locator(`.member-row[data-user-id="${sitterId}"]`);
    await sitterRow.locator('.member-row-summary').click();
    const toggleBtn = sitterRow.locator('[data-action="toggle-member-access"]');
    await expect(toggleBtn).toBeVisible();
    await expect(toggleBtn).toHaveText('Disable access');

    ownerPage.once('dialog', d => d.accept());
    await toggleBtn.click();

    // The row now shows the Disabled badge and the Enable button.
    await expect(sitterRow.locator('.member-disabled-badge')).toBeVisible();
    await expect(sitterRow.locator('[data-action="toggle-member-access"]')).toHaveText('Enable access');
    // The sitter stays listed, flagged suspended in the API.
    const hhData2 = await (await ownerPage.request.get('/api/household')).json();
    expect(hhData2.members.find(m => m.userId === sitterId).suspended).toBe(true);

    // Sitter: her session was revoked — the app sends her to sign in again.
    await sitterPage.goto('/login');
    await sitterPage.waitForSelector('#login-form', { timeout: 10000 });

    // She signs back in and is told her access is disabled.
    await sitterPage.fill('#login-email', sitterEmail);
    await sitterPage.fill('#login-password', 'test123456');
    await sitterPage.click('#login-form button[type="submit"]');
    await sitterPage.waitForSelector('#login-form', { state: 'detached', timeout: 10000 });

    const sitterGet = await sitterPage.request.get('/api/household');
    expect(sitterGet.status()).toBe(403);
    const body = await sitterGet.json();
    expect(body.status).toBe('access_suspended');
    expect(body.error).toContain('temporarily disabled');
    expect(body.error).toContain(hhName);

    // The household is invisible to her other APIs too.
    const logsRes = await sitterPage.request.get('/api/logs/history');
    expect([400, 401, 403]).toContain(logsRes.status());

    // Settings shows the disabled message instead of create/join.
    await sitterPage.goto('/settings');
    await expect(sitterPage.getByText('Access disabled')).toBeVisible();
    await expect(sitterPage.getByText(/temporarily disabled/)).toBeVisible();

    // And no tab invites her to "set up a household" she already belongs to.
    for (const route of ['/', '/schedule', '/activity', '/stats']) {
      await sitterPage.goto(route);
      await expect(sitterPage.getByText('Access disabled')).toBeVisible();
      await expect(sitterPage.getByText(/Welcome to Nabu|Set up (your|a) household/)).toHaveCount(0);
    }

    // Stats explains the real reason rather than showing an empty household.
    await sitterPage.goto('/stats');
    await expect(sitterPage.getByText(/temporarily disabled/)).toBeVisible();
    await expect(sitterPage.getByText('No chores yet')).toHaveCount(0);
    await expect(sitterPage.getByText('No data yet')).toHaveCount(0);

    // Owner re-enables.
    await ownerPage.reload();
    await ownerPage.waitForSelector('.member-list', { timeout: 10000 });
    const row2 = ownerPage.locator(`.member-row[data-user-id="${sitterId}"]`);
    await row2.locator('.member-row-summary').click();
    await row2.locator('[data-action="toggle-member-access"]').click();
    await expect(row2.locator('.member-disabled-badge')).toHaveCount(0);

    // The sitter regains access: household loads again.
    await sitterPage.goto('/');
    await sitterPage.waitForSelector('#hh-indicator:not([hidden])', { timeout: 15000 });
    const sitterGet2 = await sitterPage.request.get('/api/household');
    expect(sitterGet2.status()).toBe(200);

    await sitterCtx.close();
    await ownerCtx.close();
  });
});
