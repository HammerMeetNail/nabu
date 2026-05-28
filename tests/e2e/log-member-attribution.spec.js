// tests/e2e/log-member-attribution.spec.js
// Verifies that household members can be selected when logging a chore
// and that the attribution can be changed retroactively from history.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-lma-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';
}

async function registerAndCreateHousehold(page, email) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#user-avatar:not([hidden])', { timeout: 10000 });

  const csrf = await getCSRF(page);

  await page.request.post('/api/household', {
    data: { name: `LMA Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return csrf;
}

async function getInviteCode(page, csrf) {
  const res = await page.request.post('/api/household/invites', {
    headers: { 'X-CSRF-Token': csrf },
  });
  const data = await res.json();
  return data.invite.code;
}

async function joinAsSecondUser(browser, code) {
  const context = await browser.newContext();
  const page = await context.newPage();
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#user-avatar:not([hidden])', { timeout: 10000 });

  const csrf = await getCSRF(page);

  const joinRes = await page.request.post('/api/household/join', {
    data: { inviteCode: code },
    headers: { 'X-CSRF-Token': csrf },
  });
  if (!joinRes.ok()) {
    throw new Error(`join failed: ${joinRes.status()} ${await joinRes.text()}`);
  }

  await page.goto('/');
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { page, email, csrf, context };
}

async function setupOwnerAndMember(browser) {
  const ownerCtx = await browser.newContext();
  const ownerPage = await ownerCtx.newPage();
  const ownerEmail = uniqueEmail();
  const ownerCsrf = await registerAndCreateHousehold(ownerPage, ownerEmail);

  const hhRes = await ownerPage.request.get('/api/household');
  const hhData = await hhRes.json();
  const members = hhData.members || [];
  const ownerId = members[0]?.userId;

  const code = await getInviteCode(ownerPage, ownerCsrf);
  const { page: memberPage, context: memberCtx } = await joinAsSecondUser(browser, code);

  const hhRes2 = await memberPage.request.get('/api/household');
  const hhData2 = await hhRes2.json();
  const memberMembers = hhData2.members || [];
  const memberId = memberMembers.find(m => m.userId !== ownerId)?.userId;

  await ownerPage.reload();
  await ownerPage.waitForSelector('.home-grid', { timeout: 15000 });

  return { ownerCtx, ownerPage, ownerCsrf, ownerId, memberCtx, memberPage, memberId };
}

test.describe('Log member attribution', () => {
  test('member dropdown appears and log is attributed to selected member', async ({ browser }) => {
    const { ownerPage, ownerId, memberCtx, memberId } = await setupOwnerAndMember(browser);
    const todayDate = await ownerPage.evaluate(() => {
      const d = new Date();
      return d.toLocaleDateString('en-CA');
    });

    // ── Test: log a chore from home tab, selecting the other member ──────
    // Tap the first chore card to open the home log sheet.
    const firstCard = ownerPage.locator('.home-chore-card').first();
    await expect(firstCard).toBeVisible();
    await firstCard.click();
    await expect(ownerPage.locator('.bottom-sheet')).toBeVisible();

    // The member dropdown should appear with both users.
    await expect(ownerPage.locator('#log-member')).toBeVisible();
    const memberOptions = ownerPage.locator('#log-member option');
    await expect(memberOptions).toHaveCount(2);

    // Select the other member (not the owner).
    await ownerPage.locator('#log-member').selectOption(String(memberId));

    // Submit the log.
    await ownerPage.locator('[data-action="save-log"]').click();
    await ownerPage.waitForTimeout(500);

    // Verify the log was attributed to the selected member via API.
    const logsRes = await ownerPage.request.get(`/api/logs/today?date=${todayDate}`);
    const logsData = await logsRes.json();
    const logs = logsData.logs || [];
    expect(logs.length).toBeGreaterThan(0);
    const ourLog = logs.find(l => l.userId === memberId);
    expect(ourLog).toBeTruthy();
    expect(ourLog.userId).toBe(memberId);

    // ── Test: edit the log from history and change the member back ──────
    await ownerPage.locator('[data-nav="activity"]').click();
    await ownerPage.waitForSelector('.hist-row', { timeout: 10000 });

    // Tap the history row to open the edit sheet.
    await ownerPage.locator('.hist-row').first().click();
    await expect(ownerPage.locator('.bottom-sheet')).toBeVisible();
    await expect(ownerPage.locator('#log-member')).toBeVisible();

    // The member dropdown should show the current member selected.
    const selectedVal = await ownerPage.locator('#log-member').inputValue();
    expect(Number(selectedVal)).toBe(memberId);

    // Change back to the owner.
    await ownerPage.locator('#log-member').selectOption(String(ownerId));

    // Save the edit.
    await ownerPage.locator('[data-action="save-log"]').click();
    await ownerPage.waitForTimeout(500);

    // Verify the log was updated via API.
    const logsRes2 = await ownerPage.request.get(`/api/logs/today?date=${todayDate}`);
    const logsData2 = await logsRes2.json();
    const logs2 = logsData2.logs || [];
    const updatedLog = logs2.find(l => l.id === ourLog.id);
    expect(updatedLog).toBeTruthy();
    expect(updatedLog.userId).toBe(ownerId);

    await memberCtx.close();
    await ownerPage.context().close();
  });

  test('log via calendar pick-chore sheet attributes to selected member', async ({ browser }) => {
    const { ownerPage, memberCtx, memberId } = await setupOwnerAndMember(browser);
    const todayDate = await ownerPage.evaluate(() => {
      const d = new Date();
      return d.toLocaleDateString('en-CA');
    });

    // Navigate to calendar day view.
    await ownerPage.locator('[data-nav="activity"]').click();
    await ownerPage.locator('[data-action="switch-view"][data-view="day"]').click();
    await ownerPage.waitForSelector('.day-hour-grid', { timeout: 10000 });

    // Tap an hour label to open the pick-chore sheet.
    const hourBtn = ownerPage.locator('.hour-label').first();
    await hourBtn.click();
    await expect(ownerPage.locator('.bottom-sheet')).toBeVisible();

    // Long-press a chore in the pick-chore sheet to open the log sheet.
    const choreItem = ownerPage.locator('.sheet-chore-item').first();
    await choreItem.hover();
    await ownerPage.mouse.down();
    await ownerPage.waitForTimeout(650);
    await ownerPage.mouse.up();
    await expect(ownerPage.locator('#log-member')).toBeVisible({ timeout: 5000 });

    // Select the other member.
    await ownerPage.locator('#log-member').selectOption(String(memberId));

    // Log the chore.
    await ownerPage.locator('[data-action="save-log"]').click();
    await ownerPage.waitForTimeout(500);

    // Verify attribution via API.
    const logsRes = await ownerPage.request.get(`/api/logs/today?date=${todayDate}`);
    const logs = (await logsRes.json()).logs || [];
    const memberLog = logs.find(l => l.userId === memberId);
    expect(memberLog).toBeTruthy();

    await memberCtx.close();
    await ownerPage.context().close();
  });
});
