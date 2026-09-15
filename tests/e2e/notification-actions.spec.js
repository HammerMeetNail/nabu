import {test, expect} from '@playwright/test';
import {fixture, register, headers, postLog} from './review-fixtures.js';

test('notification bulk actions have matching large targets and clear all history with retry and persistence', async ({page, browser}) => {
  const owner = await fixture(page);
  const invite = await page.request.post('/api/household/invites', {headers: owner.headers});
  const code = (await invite.json()).invite.code;
  // Keep the injected HTTP failure observable in WebKit as well as Chromium.
  const context = await browser.newContext({viewport: {width: 390, height: 844}, serviceWorkers: 'block'});
  const member = await context.newPage();
  try {
    await register(member);
    const memberHeaders = await headers(member);
    expect((await member.request.post('/api/household/join', {headers: memberHeaders, data: {inviteCode: code}})).ok()).toBeTruthy();
    for (let i = 0; i < 55; i++) await postLog(page, owner.chore, {note: `Notification actions ${i}`});
    const notifications = async () => (await member.request.get('/api/notifications')).json();
    await expect.poll(async () => (await notifications()).unreadCount).toBe(55);
    const first = await notifications();
    expect(first.nextCursor).toBeTruthy();
    expect((await member.request.post(`/api/notifications/${first.notifications[0].id}/read`, {headers: memberHeaders})).ok()).toBeTruthy();
    await member.goto('/');
    await expect(member.locator('.home-grid')).toBeVisible();
    await member.locator('#notifications-bell').click();
    await expect(member.locator('.notif-item')).toHaveCount(50);
    const markAll = member.getByRole('button', {name: 'Mark all read', exact: true});
    const clearAll = member.getByRole('button', {name: 'Clear all', exact: true});
    for (const width of [320, 390, 1280]) {
      await member.setViewportSize({width, height: 844});
      await expect(markAll).toBeVisible();
      await expect(clearAll).toBeVisible();
      const mark = await markAll.boundingBox(), clear = await clearAll.boundingBox();
      expect(mark.height).toBeGreaterThanOrEqual(48);
      expect(clear.height).toBeGreaterThanOrEqual(48);
      expect(Math.abs(mark.width - clear.width)).toBeLessThan(1);
      expect(Math.abs(mark.height - clear.height)).toBeLessThan(1);
      expect(mark.y).toBe(clear.y);
      expect(clear.x - mark.x - mark.width).toBeGreaterThanOrEqual(8);
      expect(clear.x + clear.width).toBeLessThanOrEqual(width);
      if (process.env.NABU_NOTIFICATION_SCREENSHOTS) {
        await member.screenshot({path: `${process.env.NABU_NOTIFICATION_SCREENSHOTS}/notification-actions-${width}.png`});
      }
    }
    await member.setViewportSize({width: 390, height: 844});

    // An HTTP failure must preserve both the visible rows and older history.
    await member.route('**/api/notifications', route => route.request().method() === 'DELETE'
      ? route.fulfill({status: 500, json: {error: 'Could not clear notifications. Please retry.'}})
      : route.continue());
    await clearAll.click();
    await expect(member.getByTestId('notification-error')).toHaveText('Could not clear notifications. Please retry.');
    await expect(member.locator('.notif-item')).toHaveCount(50);
    await expect(member.locator('[data-action="more-notifications"]')).toBeEnabled();
    await expect(clearAll).toBeEnabled();
    expect((await notifications()).unreadCount).toBe(54);
    await member.unroute('**/api/notifications');

    await markAll.click();
    await expect(markAll).toBeDisabled();
    await expect(member.locator('.notif-read')).toHaveCount(50);
    await expect(member.locator('#notification-badge')).toBeHidden();
    expect((await notifications()).unreadCount).toBe(0);
    await expect(clearAll).toBeEnabled();
    await clearAll.click();
    await expect(member.locator('.notif-empty')).toHaveText('No notifications');
    await expect(clearAll).toBeDisabled();
    await expect(markAll).toBeDisabled();
    await expect(member.locator('[data-action="more-notifications"]')).toHaveCount(0);
    expect(await notifications()).toMatchObject({notifications: [], unreadCount: 0});
    // The first page and unloaded older page are gone; another recipient is intact.
    const olderPage = await member.request.get(`/api/notifications?cursor=${encodeURIComponent(first.nextCursor)}`);
    expect(olderPage.ok()).toBeTruthy();
    expect((await olderPage.json()).notifications).toEqual([]);
    expect((await (await page.request.get('/api/notifications')).json()).notifications.length).toBeGreaterThan(0);
    await member.reload();
    await expect(member.locator('.home-grid')).toBeVisible();
    await member.locator('#notifications-bell').click();
    await expect(member.locator('.notif-empty')).toHaveText('No notifications');
    await expect(clearAll).toBeDisabled();
    // Clearing history must not suppress future notifications or their badge.
    await postLog(page, owner.chore);
    await expect.poll(async () => (await notifications()).unreadCount).toBe(1);
    await member.getByRole('button', {name: 'Refresh notifications', exact: true}).click();
    await expect(member.locator('.notif-item')).toHaveCount(1);
    await expect(markAll).toBeEnabled();
    await clearAll.click();
    await expect(member.locator('.notif-empty')).toBeVisible();
    await expect(member.locator('#notification-badge')).toBeHidden();
    expect((await notifications()).unreadCount).toBe(0);
  } finally {
    await context.close();
  }
});
