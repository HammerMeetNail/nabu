// tests/e2e/day-notes.spec.js
// Phase 5.4 — per-day household diary notes on the Activity day headers.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-daynote-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithActivity(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', {
    data: { name: `DayNotes ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  const res = await page.request.post('/api/chores', {
    data: { name: 'Feed', icon: '🍼', color: '#EC4899' },
    headers: { 'X-CSRF-Token': csrf },
  });
  const chore = (await res.json()).chore;
  // A log so Activity has at least one day header.
  await page.request.post('/api/logs', {
    data: { choreId: chore.id },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { csrf };
}

test.describe('Per-day notes (Phase 5.4)', () => {
  test('add a note to a day header and see it persist', async ({ page }) => {
    await setupWithActivity(page);

    await page.click('a[data-nav="activity"]');
    await page.waitForSelector('.hist-date-header');

    // The empty affordance is the "+ note" button.
    await page.click('.hist-day-note');
    await page.waitForSelector('.day-note-sheet');
    await page.fill('#day-note-input', 'first solid food!');
    await page.click('[data-action="save-day-note"]');

    await expect(page.locator('.hist-day-note')).toContainText('first solid food!');

    // Persists across reload.
    await page.reload();
    await page.click('a[data-nav="activity"]');
    await page.waitForSelector('.hist-date-header');
    await expect(page.locator('.hist-day-note')).toContainText('first solid food!');
  });
});
