// tests/e2e/offline-launch.spec.js
// A cold launch without a reachable server opens the Home saved for this
// device's active identity, queues new logs, and syncs them on reconnect.
// Saved data never survives a sign-out or a session the server has ended.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-offline-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', { data: { name: `Offline ${Date.now()}` }, headers: { 'X-CSRF-Token': csrf } });
  await page.request.post('/api/chores/seed-defaults', { headers: { 'X-CSRF-Token': csrf } });
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { csrf };
}

function savedSnapshot(page) {
  return page.evaluate(() => new Promise((resolve, reject) => {
    const open = indexedDB.open('nabu-device', 1);
    open.onerror = () => reject(open.error);
    open.onsuccess = () => {
      const db = open.result;
      const request = db.transaction('state', 'readonly').objectStore('state').get('home-snapshot');
      request.onsuccess = () => { db.close(); resolve(request.result || null); };
      request.onerror = () => { db.close(); reject(request.error); };
    };
  }));
}

// The service worker must control the page before it can serve an offline launch.
async function controlledByServiceWorker(page) {
  await page.evaluate(() => navigator.serviceWorker.ready);
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  await expect.poll(() => page.evaluate(() => !!navigator.serviceWorker.controller)).toBe(true);
}

test.describe('Offline launch', () => {
  test('a cold launch without a network opens the saved Home and syncs logs on reconnect', async ({ page, context }) => {
    await setupWithChores(page);
    const cards = page.locator('.home-chore-card');
    await cards.nth(0).click();
    await page.click('[data-action="save-log"]');
    await expect(page.locator('.bottom-sheet')).toHaveCount(0, { timeout: 5000 });
    await controlledByServiceWorker(page);
    await expect.poll(async () => (await savedSnapshot(page))?.data?.chores?.length || 0).toBeGreaterThan(0);

    await context.setOffline(true);
    await page.reload();
    await expect(page.locator('.offline-banner')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('.home-grid')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Check your session' })).toHaveCount(0);
    await expect(cards.nth(0).locator('.home-card-time')).not.toContainText('never');

    // A log made offline waits in this device's journal.
    await cards.nth(1).click();
    await page.click('[data-action="save-log"]');
    await expect(page.locator('.pending-work summary')).toContainText('1 log saved on this device', { timeout: 10000 });

    await context.setOffline(false);
    await page.evaluate(() => window.dispatchEvent(new Event('online')));
    await expect(page.locator('.offline-banner')).toHaveCount(0, { timeout: 15000 });
    await expect(page.locator('.pending-work')).toHaveCount(0, { timeout: 15000 });
    // Both chores, the online log and the synced offline one, reached the server.
    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    expect(Object.keys(latestLogs).length).toBe(2);
  });

  test('after signing out, an offline launch shows no saved data', async ({ page, context }) => {
    await setupWithChores(page);
    await controlledByServiceWorker(page);
    await expect.poll(async () => !!(await savedSnapshot(page))).toBe(true);

    await page.locator('#hh-indicator').click();
    await page.locator('[data-action="logout"]').click();
    await expect(page.locator('#login-form')).toBeVisible({ timeout: 10000 });
    expect(await savedSnapshot(page)).toBeNull();

    await context.setOffline(true);
    await page.reload();
    await expect(page.locator('.home-grid')).toHaveCount(0);
    await expect(page.locator('.offline-banner')).toHaveCount(0);
  });

  test('when the server reports the session has ended, saved data is removed', async ({ page, context }) => {
    const { csrf } = await setupWithChores(page);
    await controlledByServiceWorker(page);
    await expect.poll(async () => !!(await savedSnapshot(page))).toBe(true);

    // Signed out elsewhere: the next check finds no session.
    await page.request.post('/api/auth/logout', { headers: { 'X-CSRF-Token': csrf } });
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
    await expect(page.locator('#login-form, #register-form')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.home-grid')).toHaveCount(0);
    expect(await savedSnapshot(page)).toBeNull();

    await context.setOffline(true);
    await page.reload();
    await expect(page.locator('.home-grid')).toHaveCount(0);
  });
});
