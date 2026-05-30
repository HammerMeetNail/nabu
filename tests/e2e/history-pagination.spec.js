// tests/e2e/history-pagination.spec.js
// End-to-end tests for the history page: reverse-chronological order,
// 7-day chunk grouping, and pagination.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-hist-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `History Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { email, csrf };
}

test.describe('History: Reverse-chronological order', () => {
  test('history shows recent days and events at the top', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Log a few chores to build history data.
    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(3);

    // Log chore[0] as today
    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });
    // Log chore[1] as yesterday
    const yesterday = new Date();
    yesterday.setDate(yesterday.getDate() - 1);
    await page.request.post('/api/logs', {
      data: { choreId: chores[1].id, note: '', indicators: [], completedAt: yesterday.toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });
    // Log chore[2] as 3 days ago
    const threeDaysAgo = new Date();
    threeDaysAgo.setDate(threeDaysAgo.getDate() - 3);
    await page.request.post('/api/logs', {
      data: { choreId: chores[2].id, note: '', indicators: [], completedAt: threeDaysAgo.toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to history tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // Should have at least 3 history rows
    const rows = page.locator('.hist-row');
    await expect(rows).toHaveCount(3);

    // Get the first row's name
    const firstName = await rows.first().locator('.hist-name').textContent();
    expect(firstName).toBe(chores[0].name);

    // Get the last row's name
    const lastName = await rows.last().locator('.hist-name').textContent();
    expect(lastName).toBe(chores[2].name);
  });

  test('history shows 7-day chunk headers and load more button', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(2);

    // Log one chore today
    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Log another chore 10 days ago (outside the first 7-day window)
    const tenDaysAgo = new Date();
    tenDaysAgo.setDate(tenDaysAgo.getDate() - 10);
    await page.request.post('/api/logs', {
      data: { choreId: chores[1].id, note: '', indicators: [], completedAt: tenDaysAgo.toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to history tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // Should show chunk header
    await expect(page.locator('.hist-chunk-header')).toBeVisible();

    // Should show load more button (hasMore should be true)
    const loadMore = page.locator('.load-more-btn');
    await expect(loadMore).toBeVisible();

    // Click load more
    await loadMore.click();
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // Should now have 2 rows visible
    const rows = page.locator('.hist-row');
    await expect(rows).toHaveCount(2);

    // Should still have chunk headers (now two chunks)
    await expect(page.locator('.hist-chunk-header')).toHaveCount(2);
  });

  test('history shows date headers within each chunk', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(2);

    // Log two chores on different days within the same week
    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    const twoDaysAgo = new Date();
    twoDaysAgo.setDate(twoDaysAgo.getDate() - 2);
    await page.request.post('/api/logs', {
      data: { choreId: chores[1].id, note: '', indicators: [], completedAt: twoDaysAgo.toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to history tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // Should show date headers (one per day)
    const dateHeaders = page.locator('.hist-date-header');
    await expect(dateHeaders).toHaveCount(2);
  });
});
