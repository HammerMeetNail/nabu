// tests/e2e/notifications-hide-badge.spec.js
// Verifies the "Hide notification badge" user preference: notifications keep
// accumulating server-side, but the unread count on the bell is suppressed
// while the preference is on — and it persists across reloads.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-badge-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
}

async function registerAndCreateHousehold(page, email) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = await getCSRF(page);

  await page.request.post('/api/household', {
    data: { name: `Badge Test ${Date.now()}` },
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

async function setupOwnerAndMember(browser) {
  const ownerCtx = await browser.newContext();
  const ownerPage = await ownerCtx.newPage();
  const ownerEmail = uniqueEmail();
  const ownerCsrf = await registerAndCreateHousehold(ownerPage, ownerEmail);
  const code = await getInviteCode(ownerPage, ownerCsrf);
  const { page: memberPage, context: memberCtx } = await joinAsSecondUser(browser, code);
  return { ownerCtx, ownerPage, ownerCsrf, memberPage, memberCtx };
}

/** Owner logs a chore so the member receives a notification. */
async function ownerLogsChore(ownerPage, ownerCsrf) {
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
}

async function openSettings(page) {
  await page.locator('#hh-indicator').click();
  await expect(page.locator('.profile-panel')).toBeVisible({ timeout: 5000 });
  await page.click('button[data-action="profile-nav-settings"]');
  await expect(page.locator('.settings-view')).toBeVisible({ timeout: 5000 });
}

test.describe('Hide notification badge preference', () => {
  test('hides the badge immediately, keeps accumulating, and persists across reload', async ({ browser }) => {
    const { ownerCtx, ownerPage, ownerCsrf, memberPage, memberCtx } = await setupOwnerAndMember(browser);
    try {
      // Member starts with a visible unread badge.
      await ownerLogsChore(ownerPage, ownerCsrf);
      await expect.poll(
        async () => (await fetchNotifications(memberPage)).unreadCount,
        { timeout: 10000, intervals: [500] }
      ).toBeGreaterThanOrEqual(1);

      await memberPage.reload();
      await memberPage.waitForSelector('.home-grid', { timeout: 15000 });
      await expect.poll(
        async () => await memberPage.locator('#notification-badge').isVisible().catch(() => false),
        { timeout: 8000, intervals: [300] }
      ).toBe(true);

      // Toggle the preference in Settings. The native checkbox is visually
      // hidden (opacity:0) behind the slider span, so click the slider —
      // same pattern as settings-notification-prefs.spec.js.
      await openSettings(memberPage);
      const slider = memberPage.locator('input[data-action="toggle-hide-notification-badge"] + .toggle-slider');
      await expect(slider).toBeVisible();
      await slider.click();

      // Badge disappears without a reload.
      await expect(memberPage.locator('#notification-badge')).toBeHidden({ timeout: 5000 });

      // Notifications still accumulate server-side while the badge is hidden.
      await ownerLogsChore(ownerPage, ownerCsrf);
      await expect.poll(
        async () => (await fetchNotifications(memberPage)).unreadCount,
        { timeout: 10000, intervals: [500] }
      ).toBeGreaterThanOrEqual(2);

      // The preference survives a reload (which keeps the Settings tab) and the
      // badge stays hidden even though there are now 2+ unread notifications.
      await memberPage.reload();
      await memberPage.waitForSelector('.settings-view', { timeout: 15000 });
      await expect.poll(
        async () => (await fetchNotifications(memberPage)).unreadCount,
        { timeout: 10000, intervals: [500] }
      ).toBeGreaterThanOrEqual(2);
      await expect(memberPage.locator('#notification-badge')).toBeHidden({ timeout: 8000 });

      // The bell itself remains usable: the panel still opens.
      await memberPage.click('#notifications-bell');
      await expect(memberPage.locator('.notif-panel')).toBeVisible({ timeout: 5000 });
    } finally {
      await memberCtx.close();
      await ownerCtx.close();
    }
  });

  test('turning the preference off shows the badge again', async ({ browser }) => {
    const { ownerCtx, ownerPage, ownerCsrf, memberPage, memberCtx } = await setupOwnerAndMember(browser);
    try {
      await ownerLogsChore(ownerPage, ownerCsrf);
      await expect.poll(
        async () => (await fetchNotifications(memberPage)).unreadCount,
        { timeout: 10000, intervals: [500] }
      ).toBeGreaterThanOrEqual(1);

      // Reload so the member's client picks up the unread count — the
      // badge must actually be visible before we can assert hiding it.
      await memberPage.reload();
      await memberPage.waitForSelector('.home-grid', { timeout: 15000 });
      await expect.poll(
        async () => await memberPage.locator('#notification-badge').isVisible().catch(() => false),
        { timeout: 8000, intervals: [300] }
      ).toBe(true);

      // Hide it first.
      await openSettings(memberPage);
      const slider = memberPage.locator('input[data-action="toggle-hide-notification-badge"] + .toggle-slider');
      await expect(slider).toBeVisible();
      await slider.click();
      await expect(memberPage.locator('#notification-badge')).toBeHidden({ timeout: 5000 });

      // Then un-hide: the badge comes back without a reload.
      await slider.click();
      await expect(memberPage.locator('#notification-badge')).toBeVisible({ timeout: 5000 });
      const badgeText = await memberPage.locator('#notification-badge').textContent();
      expect(Number(badgeText)).toBeGreaterThanOrEqual(1);
    } finally {
      await memberCtx.close();
      await ownerCtx.close();
    }
  });
});
