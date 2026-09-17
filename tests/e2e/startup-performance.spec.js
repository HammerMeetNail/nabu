import { test, expect } from '@playwright/test';
import { fixture, postLog, deferred, observeModuleCalls } from './review-fixtures.js';

test.use({ serviceWorkers: 'block' });

test('cold startup loads independent Home data together after confirming the session', async ({ page }) => {
  const { chore, headers } = await fixture(page);
  await postLog(page, chore, { volumeML: 120 });
  const extras = [];
  for (const name of ['First in saved order', 'Hidden on Home']) {
    const response = await page.request.post('/api/chores', { headers, data: { name, icon: '📝', color: '#6080AA' } });
    expect(response.ok()).toBe(true);
    extras.push((await response.json()).chore);
  }
  const preferences = await page.request.patch('/api/preferences', { headers, data: {
    choreOrder: [extras[0].id, chore.id, extras[1].id], hiddenHomeChoreIds: [extras[1].id],
  } });
  expect(preferences.ok()).toBe(true);
  await page.addInitScript(() => {
    const observer = new MutationObserver(() => {
      const grid = document.querySelector('.home-grid');
      if (!grid) return;
      window.firstHomeCards = [...grid.querySelectorAll('[data-home-chore-id]')].map(card => Number(card.dataset.homeChoreId));
      observer.disconnect();
    });
    observer.observe(document, { childList: true, subtree: true });
  });
  const joinStartup = await observeModuleCalls(page, 'app', 'reloadAfterAuth', { exported: false });
  const session = deferred(), core = deferred();
  const paths = ['/api/household', '/api/households', '/api/preferences', '/api/chores', '/api/logs/latest-per-chore'];
  const started = new Set();
  let sessionEntered = false;
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/api/me') {
      sessionEntered = true;
      await session.promise;
    } else if (paths.includes(path) && route.request().method() === 'GET') {
      started.add(path);
      await core.promise;
    }
    await route.continue();
  });
  try {
    await page.reload();
    await expect.poll(() => sessionEntered).toBe(true);
    expect([...started]).toEqual([]);
    await expect(page.locator('.home-grid')).toHaveCount(0);
    session.resolve();
    // A held household response must not prevent the other independent reads.
    await expect.poll(() => [...started].sort()).toEqual([...paths].sort());
    await expect(page.locator('.home-grid')).toHaveCount(0);
    core.resolve();
    await joinStartup();
    await expect(page.locator('.home-grid')).toBeVisible();
    await expect(page.locator(`[data-home-chore-id="${chore.id}"] .home-card-time`)).toHaveText('just now');
    expect(await page.evaluate(() => window.firstHomeCards)).toEqual([extras[0].id, chore.id]);
  } finally {
    session.resolve();
    core.resolve();
    await page.unrouteAll({ behavior: 'wait' });
  }
});

test('Home is usable while notifications are pending and loads calendar data on navigation', async ({ page }) => {
  const { chore, headers } = await fixture(page, { metricType: '', metricUnit: '' });
  await postLog(page, chore);
  const scheduled = await page.request.post('/api/schedules', { headers, data: {
    choreId: chore.id, timePeriod: 'anytime', specificTime: '12:00', frequencyType: 'daily', isActive: true,
  } });
  expect(scheduled.ok()).toBe(true);
  const { schedule } = await scheduled.json();
  const joinNotifications = await observeModuleCalls(page, 'app', 'loadNotifData', { exported: false });
  const notifications = deferred();
  let notificationsEntered = false;
  const calendarRequests = [];
  page.on('request', request => {
    const path = new URL(request.url()).pathname;
    if (path === '/api/logs/today' || path === '/api/schedules') calendarRequests.push(path);
  });
  await page.route('**/api/notifications', async route => {
    notificationsEntered = true;
    await notifications.promise;
    await route.continue();
  });
  try {
    await page.reload();
    await expect.poll(() => notificationsEntered).toBe(true);
    await expect(page.locator('.home-grid')).toBeVisible();
    await expect(page.locator('.home-card-time')).toHaveText('just now');
    expect(calendarRequests).toEqual([]);
    await page.locator(`[data-action="home-tap-chore"][data-home-chore-id="${chore.id}"]`).click();
    await expect(page.locator('.bottom-sheet')).toBeVisible();
    const saved = page.waitForResponse(response => response.url().endsWith('/api/logs') && response.request().method() === 'POST');
    await page.locator('[data-action="save-log"]').click();
    const savedResponse = await saved;
    expect(savedResponse.ok()).toBe(true);
    const { log: savedLog } = await savedResponse.json();
    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
    await page.locator('[data-nav="schedule"]').click();
    await expect.poll(() => calendarRequests.includes('/api/schedules')).toBe(true);
    await expect(page.locator('.schedule-view')).toBeVisible();
    await expect(page.locator(`.sch-row-main[data-schedule-id="${schedule.id}"]`).first()).toContainText(chore.name);
    notifications.resolve();
    await joinNotifications();
    await expect(page.locator('.schedule-view')).toBeVisible();
    await page.reload();
    await expect(page.locator('.home-grid')).toBeVisible();
    await expect(page.locator('.home-card-time')).toHaveText('just now');
    const history = await page.request.get('/api/logs/history');
    expect(history.ok()).toBe(true);
    expect((await history.json()).logs.map(log => log.id)).toContain(savedLog.id);
  } finally {
    notifications.resolve();
    await page.unrouteAll({ behavior: 'wait' });
  }
});

test('Stats loads the Today count after cold Home startup and on a direct launch', async ({ page }) => {
  const { chore } = await fixture(page);
  await postLog(page, chore, { volumeML: 120 });
  await postLog(page, chore, { volumeML: 150 });
  await page.reload();
  await expect(page.locator('.home-grid')).toBeVisible();
  await page.locator('[data-nav="stats"]').click();
  const today = page.locator('.overview-card').filter({ has: page.getByText('Today', { exact: true }) });
  await expect(today.locator('.overview-card-value')).toHaveText('2');
  await page.goto('/stats');
  await expect(today.locator('.overview-card-value')).toHaveText('2');
});

test('a late startup notification response cannot restore data after sign-out', async ({ page }) => {
  await fixture(page);
  const joinNotifications = await observeModuleCalls(page, 'app', 'loadNotifData', { exported: false });
  const notifications = deferred();
  let notificationsEntered = false;
  await page.route('**/api/notifications', async route => {
    const response = await route.fetch();
    notificationsEntered = true;
    await notifications.promise;
    await route.fulfill({ response, json: { notifications: [{ id: 9001, title: 'Old household notification', body: 'Private startup message', isRead: false }], unreadCount: 1 } });
  });
  try {
    await page.reload();
    await expect.poll(() => notificationsEntered).toBe(true);
    await expect(page.locator('.home-grid')).toBeVisible();
    await page.locator('#hh-indicator').click();
    await page.locator('[data-action="logout"]').click();
    await expect(page.locator('#login-form')).toBeVisible();
    notifications.resolve();
    await joinNotifications();
    await expect(page.locator('#login-form')).toBeVisible();
    await expect(page.locator('#notification-badge')).toBeHidden();
    await expect(page.locator('body')).not.toContainText('Private startup message');
  } finally {
    notifications.resolve();
    await page.unrouteAll({ behavior: 'wait' });
  }
});
