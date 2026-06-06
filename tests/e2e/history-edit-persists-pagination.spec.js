// tests/e2e/history-edit-persists-pagination.spec.js
// Regression: clicking "Load more" then editing an activity must not
// lose the extra activities or make "Load more" reappear.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-hedit-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `HistoryEdit Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { email, csrf };
}

test.describe('History: pagination survives editing', () => {
  test('loaded pages persist after updating an activity', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(2);

    // Log chore[0] today
    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: 'original note', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Log chore[1] 10 days ago (outside the first 7-day window)
    const tenDaysAgo = new Date();
    tenDaysAgo.setDate(tenDaysAgo.getDate() - 10);
    await page.request.post('/api/logs', {
      data: { choreId: chores[1].id, note: 'old note', indicators: [], completedAt: tenDaysAgo.toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to activity tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // Should have 1 row visible (the today log) and a Load more button
    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(page.locator('.load-more-btn')).toBeVisible();

    // Click load more
    await page.locator('.load-more-btn').click();
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // Now both rows should be visible
    await expect(page.locator('.hist-row')).toHaveCount(2);
    // Load more button should be gone (no more data before 10 days ago)
    await expect(page.locator('.load-more-btn')).toHaveCount(0);

    // Remember chunks and chunk headers visible
    await expect(page.locator('.hist-chunk-header')).toHaveCount(2);

    // Click the first history row (today's log) to open the edit sheet
    const firstRow = page.locator('.hist-row').first();
    await firstRow.click();

    // Wait for the log sheet to appear
    await expect(page.locator('#log-note')).toBeVisible({ timeout: 5000 });

    // Change the note
    await page.fill('#log-note', 'updated note');

    // Click the Update button
    await page.locator('[data-action="save-log"]').click();

    // Wait for sheet to close and history to re-render
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });
    await page.waitForTimeout(300); // let morph finish

    // BOTH rows must still be visible — the older chunk must not have vanished
    await expect(page.locator('.hist-row')).toHaveCount(2);

    // Both chunk headers must still be present
    await expect(page.locator('.hist-chunk-header')).toHaveCount(2);

    // Load more button must NOT have reappeared
    await expect(page.locator('.load-more-btn')).toHaveCount(0);

    // The first row should reflect the updated note
    const firstNote = await firstRow.locator('.hist-meta').textContent();
    expect(firstNote).toContain('updated note');
  });

  test('loaded pages persist after removing an activity', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(2);

    // Log chore[0] today
    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Log chore[1] 10 days ago
    const tenDaysAgo = new Date();
    tenDaysAgo.setDate(tenDaysAgo.getDate() - 10);
    await page.request.post('/api/logs', {
      data: { choreId: chores[1].id, note: '', indicators: [], completedAt: tenDaysAgo.toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to activity tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(page.locator('.load-more-btn')).toBeVisible();

    // Load more
    await page.locator('.load-more-btn').click();
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    await expect(page.locator('.hist-row')).toHaveCount(2);
    await expect(page.locator('.load-more-btn')).toHaveCount(0);
    await expect(page.locator('.hist-chunk-header')).toHaveCount(2);

    // Click the older chunk's row (second row) to open its sheet
    const secondRow = page.locator('.hist-row').nth(1);
    await secondRow.click();

    // Wait for the log sheet
    await expect(page.locator('#log-note')).toBeVisible({ timeout: 5000 });

    // Click the Remove log button
    await page.locator('[data-action="undo-chore"]').click();

    // Wait for sheet to close and re-render
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });
    await page.waitForTimeout(300);

    // Should now have 1 row (the today log survived)
    await expect(page.locator('.hist-row')).toHaveCount(1);

    // Load more button should be gone since we started fresh
    // (the remaining log is today's; no older logs exist now)
    await expect(page.locator('.load-more-btn')).toHaveCount(0);
  });
});
