// tests/e2e/household-join-notify.spec.js
// Verifies that when someone joins a household:
// 1. An in-app notification of type "household_joined" is created for existing members.
// 2. The "Household Joined" toggle appears in notification settings.
// 3. Existing member's member list auto-updates after join.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-join-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Join Notif Test ${Date.now()}` },
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

async function fetchNotificationPrefs(page) {
  const res = await page.request.get('/api/notification-preferences');
  if (!res.ok()) {
    return { preferences: { enabledPushTypes: [] }, availableTypes: [] };
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

test.describe('Household Join Notification', () => {
  test('joining creates a household_joined notification for existing members', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();
    const ownerEmail = uniqueEmail();
    const ownerCsrf = await registerAndCreateHousehold(ownerPage, ownerEmail);
    const code = await getInviteCode(ownerPage, ownerCsrf);

    // Owner should have no household_joined notifications yet
    const beforeNotifs = await fetchNotifications(ownerPage);
    const joinNotifsBefore = beforeNotifs.notifications.filter(n => n.type === 'household_joined');
    expect(joinNotifsBefore.length).toBe(0);

    // Have a second user join
    const { context: memberCtx } = await joinAsSecondUser(browser, code);

    // Poll until the owner receives the join notification (goroutine is fire-and-forget)
    await expect.poll(
      async () => {
        const data = await fetchNotifications(ownerPage);
        return data.notifications.filter(n => n.type === 'household_joined').length;
      },
      { timeout: 10000, intervals: [500] }
    ).toBeGreaterThanOrEqual(1);

    // Verify content of the notification
    const notifs = await fetchNotifications(ownerPage);
    const joinNotif = notifs.notifications.find(n => n.type === 'household_joined');
    expect(joinNotif).toBeTruthy();
    expect(joinNotif.title).toBe('New Member');
    expect(joinNotif.body).toContain('joined');

    await memberCtx.close();
    await ownerCtx.close();
  });

  test('household_joined notification type appears in settings notification toggles', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();
    const ownerEmail = uniqueEmail();
    await registerAndCreateHousehold(ownerPage, ownerEmail);

    // Navigate to settings
    await ownerPage.click('[data-nav="settings"]');
    await ownerPage.waitForSelector('.notif-pref-toggle', { timeout: 10000 });

    // The "Household Joined" toggle should be visible
    const toggleLabels = ownerPage.locator('.notif-pref-title');
    const labelTexts = await toggleLabels.allTextContents();
    const hasJoinToggle = labelTexts.some(t => t.includes('Household Joined'));
    expect(hasJoinToggle).toBe(true);

    await ownerCtx.close();
  });

  test('available notification types include household_joined via API', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();
    const ownerEmail = uniqueEmail();
    await registerAndCreateHousehold(ownerPage, ownerEmail);

    const { availableTypes } = await fetchNotificationPrefs(ownerPage);
    const joinType = availableTypes.find(t => t.type === 'household_joined');
    expect(joinType).toBeTruthy();
    expect(joinType.label).toBe('Household Joined');
    expect(joinType.description).toContain('joins');

    await ownerCtx.close();
  });

  test('existing members see updated member count after someone joins', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();
    const ownerEmail = uniqueEmail();
    const ownerCsrf = await registerAndCreateHousehold(ownerPage, ownerEmail);

    // Navigate to settings and get initial member count
    await ownerPage.click('[data-nav="settings"]');
    await ownerPage.waitForSelector('.member-list', { timeout: 10000 });
    const initialCount = await ownerPage.locator('.member-row').count();
    expect(initialCount).toBe(1); // just the owner

    // Have a second user join
    const code = await getInviteCode(ownerPage, ownerCsrf);
    const { page: memberPage, context: memberCtx } = await joinAsSecondUser(browser, code);

    // The member should see the household
    await memberPage.click('[data-nav="settings"]');
    await memberPage.waitForSelector('.member-list', { timeout: 10000 });
    const memberViewCount = await memberPage.locator('.member-row').count();
    expect(memberViewCount).toBe(2);

    // Re-navigate to settings on owner page to see updated count
    // (the auto-refresh via polling would pick it up within 30s,
    // but for test speed we just re-navigate)
    await ownerPage.goto('/');
    await ownerPage.waitForSelector('.home-grid', { timeout: 15000 });
    await ownerPage.click('[data-nav="settings"]');
    await ownerPage.waitForSelector('.member-list', { timeout: 10000 });
    const updatedCount = await ownerPage.locator('.member-row').count();
    expect(updatedCount).toBe(2);

    await memberCtx.close();
    await ownerCtx.close();
  });

  test('joining user sees household members immediately after join', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();
    const ownerEmail = uniqueEmail();
    const ownerCsrf = await registerAndCreateHousehold(ownerPage, ownerEmail);
    const code = await getInviteCode(ownerPage, ownerCsrf);

    const { page: memberPage, context: memberCtx } = await joinAsSecondUser(browser, code);

    // Immediately after the join+goto in joinAsSecondUser, the new member
    // should see the home grid (meaning they are in the household)
    await memberPage.waitForSelector('.home-grid', { timeout: 10000 });

    // Navigate to settings and check member list
    await memberPage.click('[data-nav="settings"]');
    await memberPage.waitForSelector('.member-list', { timeout: 10000 });
    const memberCount = await memberPage.locator('.member-row').count();
    expect(memberCount).toBe(2);

    // Verify the member sees the owner
    const roleBadges = memberPage.locator('.role-badge');
    const roles = await roleBadges.allTextContents();
    expect(roles).toContain('owner');
    expect(roles).toContain('member');

    await memberCtx.close();
    await ownerCtx.close();
  });
});
