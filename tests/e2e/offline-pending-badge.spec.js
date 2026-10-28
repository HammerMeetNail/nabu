// tests/e2e/offline-pending-badge.spec.js
// Phase 2.1 — an offline-queued log shows inline in Activity with a "pending"
// badge, reconciled on the next successful replay.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-pending-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setup(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', {
    data: { name: `Pending ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  const res = await page.request.post('/api/chores', {
    data: { name: 'Trash', icon: '🗑️', color: '#6B7280' },
    headers: { 'X-CSRF-Token': csrf },
  });
  const chore = (await res.json()).chore;
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { csrf, chore };
}

test.describe('Offline pending badge (Phase 2.1)', () => {
  test('offline log shows a pending badge, cleared after reconnect', async ({ page, context }) => {
    const { chore } = await setup(page);

    await context.setOffline(true);

    // Log while offline: the POST fails and is queued; a synthetic pending row
    // is added to state.
    await page.click(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    await page.waitForSelector('[data-action="save-log"]');
    await page.click('[data-action="save-log"]');

    await page.click('a[data-nav="activity"]');
    await page.waitForSelector('.history-view');
    await expect(page.locator('.hist-pending')).toBeVisible();

    // Reconnect and flush the queue.
    await context.setOffline(false);
    await page.evaluate(() => window.dispatchEvent(new Event('online')));

    await expect(page.locator('.hist-pending')).toHaveCount(0, { timeout: 15000 });
    // The real (synced) log is now present.
    await expect(page.locator('.hist-row')).toHaveCount(1);
  });
});
