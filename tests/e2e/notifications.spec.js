// tests/e2e/notifications.spec.js
// Verifies in-app notifications: when one household member logs a chore,
// every other member receives a notification.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-notif-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Notif Test ${Date.now()}` },
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

async function fetchNotifications(page) {
  const csrf = await getCSRF(page);
  const res = await page.request.get('/api/notifications', {
    headers: { 'X-CSRF-Token': csrf },
  });
  if (!res.ok()) {
    return { notifications: [], unreadCount: 0 };
  }
  return res.json();
}

test.describe('In-app notifications', () => {
  test('logging a chore creates a notification for other household members', async ({ browser }) => {
    const ownerPage = await browser.newPage();
    const ownerEmail = uniqueEmail();
    const ownerCsrf = await registerAndCreateHousehold(ownerPage, ownerEmail);
    const code = await getInviteCode(ownerPage, ownerCsrf);

    const { page: memberPage, context: memberCtx } = await joinAsSecondUser(browser, code);

    // Verify member sees no unread badge initially
    await memberPage.waitForSelector('#notifications-bell:not([hidden])', { timeout: 5000 });
    const badgeBefore = await memberPage.locator('#notification-badge').isVisible().catch(() => false);
    expect(badgeBefore).toBe(false);

    // Owner logs a chore via API
    const choresRes = await ownerPage.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];
    expect(firstChore).toBeTruthy();

    await ownerPage.request.post('/api/logs', {
      data: { choreId: firstChore.id, note: '', indicators: [] },
      headers: { 'X-CSRF-Token': ownerCsrf },
    });

    // Poll API until notification appears (goroutine is fire-and-forget)
    await expect.poll(
      async () => (await fetchNotifications(memberPage)).unreadCount,
      { timeout: 10000, intervals: [500] }
    ).toBeGreaterThanOrEqual(1);

    // Reload and verify badge
    await memberPage.reload();
    await memberPage.waitForSelector('.home-grid', { timeout: 15000 });
    await expect.poll(
      async () => {
        const badge = memberPage.locator('#notification-badge');
        return await badge.isVisible().catch(() => false);
      },
      { timeout: 8000, intervals: [300] }
    ).toBe(true);

    await memberCtx.close();
    await ownerPage.close();
  });

  test('notification panel opens, shows items, and mark-all-read clears badge', async ({ browser }) => {
    const ownerPage = await browser.newPage();
    const ownerEmail = uniqueEmail();
    const ownerCsrf = await registerAndCreateHousehold(ownerPage, ownerEmail);
    const code = await getInviteCode(ownerPage, ownerCsrf);

    const { page: memberPage, context: memberCtx } = await joinAsSecondUser(browser, code);

    // Trigger a notification
    const choresRes = await ownerPage.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];

    await ownerPage.request.post('/api/logs', {
      data: { choreId: firstChore.id, note: '', indicators: [] },
      headers: { 'X-CSRF-Token': ownerCsrf },
    });

    // Wait for notification via API
    await expect.poll(
      async () => (await fetchNotifications(memberPage)).unreadCount,
      { timeout: 10000, intervals: [500] }
    ).toBeGreaterThanOrEqual(1);

    // Reload and wait for badge
    await memberPage.reload();
    await memberPage.waitForSelector('.home-grid', { timeout: 15000 });
    await expect.poll(
      async () => {
        const badge = memberPage.locator('#notification-badge');
        return await badge.isVisible().catch(() => false);
      },
      { timeout: 8000, intervals: [300] }
    ).toBe(true);

    // Open notification panel
    await memberPage.click('#notifications-bell');
    await memberPage.waitForSelector('.notif-panel', { timeout: 5000 });

    // Should see at least one notification item
    const items = memberPage.locator('.notif-item');
    await expect(items).toHaveCount(1);

    // Mark all read
    await memberPage.click('button[data-action="mark-all-read"]');
    await expect.poll(
      async () => {
        const badge = memberPage.locator('#notification-badge');
        return await badge.isHidden().catch(() => false);
      },
      { timeout: 5000, intervals: [200] }
    ).toBe(true);

    await memberCtx.close();
    await ownerPage.close();
  });

  test('deleting a notification removes it from the panel', async ({ browser }) => {
    const ownerPage = await browser.newPage();
    const ownerEmail = uniqueEmail();
    const ownerCsrf = await registerAndCreateHousehold(ownerPage, ownerEmail);
    const code = await getInviteCode(ownerPage, ownerCsrf);

    const { page: memberPage, context: memberCtx } = await joinAsSecondUser(browser, code);

    const choresRes = await ownerPage.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];

    await ownerPage.request.post('/api/logs', {
      data: { choreId: firstChore.id, note: '', indicators: [] },
      headers: { 'X-CSRF-Token': ownerCsrf },
    });

    await expect.poll(
      async () => (await fetchNotifications(memberPage)).notifications.length,
      { timeout: 10000, intervals: [500] }
    ).toBeGreaterThanOrEqual(1);

    // Open panel and dismiss
    await memberPage.reload();
    await memberPage.waitForSelector('.home-grid', { timeout: 15000 });
    await memberPage.click('#notifications-bell');
    await memberPage.waitForSelector('.notif-item', { timeout: 5000 });

    await memberPage.click('button[data-action="dismiss-notification"]');
    await expect(memberPage.locator('.notif-item')).toHaveCount(0, { timeout: 5000 });

    await memberCtx.close();
    await ownerPage.close();
  });

  test('notification survives page reload', async ({ browser }) => {
    const ownerPage = await browser.newPage();
    const ownerEmail = uniqueEmail();
    const ownerCsrf = await registerAndCreateHousehold(ownerPage, ownerEmail);
    const code = await getInviteCode(ownerPage, ownerCsrf);

    const { page: memberPage, context: memberCtx } = await joinAsSecondUser(browser, code);

    const choresRes = await ownerPage.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];

    await ownerPage.request.post('/api/logs', {
      data: { choreId: firstChore.id, note: '', indicators: [] },
      headers: { 'X-CSRF-Token': ownerCsrf },
    });

    await expect.poll(
      async () => (await fetchNotifications(memberPage)).notifications.length,
      { timeout: 10000, intervals: [500] }
    ).toBeGreaterThanOrEqual(1);

    // Verify via API after reload
    const data = await fetchNotifications(memberPage);
    expect(data.notifications.length).toBeGreaterThanOrEqual(1);
    expect(data.notifications[0].type).toBe('chore_logged');

    await memberCtx.close();
    await ownerPage.close();
  });
});
