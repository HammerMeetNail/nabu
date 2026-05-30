// tests/e2e/history-filter.spec.js
// End-to-end tests for chore filtering on the history activity page.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-filt-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Filter Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { email, csrf };
}

test.describe('History filter', () => {
  test('filter bar appears with chore chips', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Log one chore so history is not empty
    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(3);

    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to activity tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    // Filter bar should be visible
    await expect(page.locator('.hist-filter-fab')).toBeVisible();

    // Should have "All" button and chore chips
    await expect(page.locator('.hist-filter-all')).toBeVisible();
    const chips = page.locator('.hist-filter-chip[data-action="history-filter-chore"]');
    await expect(chips).toHaveCount(chores.length);

    // All chips should be active by default (no filter applied)
    for (let i = 0; i < chores.length; i++) {
      await expect(chips.nth(i)).toHaveClass(/active/);
    }
    await expect(page.locator('.hist-filter-all')).toHaveClass(/active/);
  });

  test('tapping a chore chip hides that chore from history', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(3);

    // Log first three chores to build history
    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.request.post('/api/logs', {
      data: { choreId: chores[1].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.request.post('/api/logs', {
      data: { choreId: chores[2].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to activity tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // Should see 3 rows
    await expect(page.locator('.hist-row')).toHaveCount(3);

    // Tap the first chore chip to exclude it
    await page.locator('.hist-filter-chip[data-action="history-filter-chore"]').first().click();
    await page.waitForTimeout(300); // wait for re-render

    // That chip should no longer be active
    await expect(page.locator('.hist-filter-chip[data-action="history-filter-chore"]').first()).not.toHaveClass(/active/);

    // Should now see only 2 rows (the other two chores)
    await expect(page.locator('.hist-row')).toHaveCount(2);

    // "All" should not be active
    await expect(page.locator('.hist-filter-all')).not.toHaveClass(/active/);
  });

  test('tapping "All" resets the filter', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(3);

    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.request.post('/api/logs', {
      data: { choreId: chores[1].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to activity tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    await expect(page.locator('.hist-row')).toHaveCount(2);

    // Exclude first chore
    await page.locator('.hist-filter-chip[data-action="history-filter-chore"]').first().click();
    await page.waitForTimeout(300);
    await expect(page.locator('.hist-row')).toHaveCount(1);

    // Tap "All" to reset
    await page.locator('.hist-filter-all').click();
    await page.waitForTimeout(300);

    // All chips should be active again
    const chips = page.locator('.hist-filter-chip[data-action="history-filter-chore"]');
    for (let i = 0; i < chores.length; i++) {
      await expect(chips.nth(i)).toHaveClass(/active/);
    }
    await expect(page.locator('.hist-filter-all')).toHaveClass(/active/);

    // All rows should be back
    await expect(page.locator('.hist-row')).toHaveCount(2);
  });

  test('filter persists when switching between history sub-views', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(3);

    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.request.post('/api/logs', {
      data: { choreId: chores[1].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to activity tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // Exclude first chore
    await page.locator('.hist-filter-chip[data-action="history-filter-chore"]').first().click();
    await page.waitForTimeout(300);
    await expect(page.locator('.hist-row')).toHaveCount(1);

    // Switch to Day view
    await page.click('[data-action="switch-view"][data-view="day"]');
    await page.waitForSelector('.day-view', { timeout: 10000 });

    // Switch back to History
    await page.click('[data-action="switch-view"][data-view="history"]');
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // Filter should still be applied — only 1 row
    await expect(page.locator('.hist-row')).toHaveCount(1);

    // The first chip should still be inactive
    await expect(page.locator('.hist-filter-chip[data-action="history-filter-chore"]').first()).not.toHaveClass(/active/);
  });

  test('re-adding a chore via chip toggles it back in', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(3);

    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.request.post('/api/logs', {
      data: { choreId: chores[1].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.request.post('/api/logs', {
      data: { choreId: chores[2].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to activity tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // Hide first two chores
    const chip0 = page.locator('.hist-filter-chip[data-action="history-filter-chore"]').nth(0);
    const chip1 = page.locator('.hist-filter-chip[data-action="history-filter-chore"]').nth(1);
    await chip0.click();
    await page.waitForTimeout(300);
    await chip1.click();
    await page.waitForTimeout(300);

    // Only chore 2 should be visible
    await expect(page.locator('.hist-row')).toHaveCount(1);

    // Re-add first chore by tapping its chip
    await chip0.click();
    await page.waitForTimeout(300);

    // Now 2 rows should be visible (chore 0 and chore 2)
    await expect(page.locator('.hist-row')).toHaveCount(2);
    await expect(chip0).toHaveClass(/active/);
    await expect(chip1).not.toHaveClass(/active/);

    // Re-add second chore — now all back, filter should reset to null
    await chip1.click();
    await page.waitForTimeout(300);

    await expect(page.locator('.hist-row')).toHaveCount(3);
    await expect(page.locator('.hist-filter-all')).toHaveClass(/active/);
    await expect(chip0).toHaveClass(/active/);
    await expect(chip1).toHaveClass(/active/);
  });

  test('shows empty message when filter excludes all logs', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(2);

    // Log only chore 0
    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Navigate to activity tab
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    await expect(page.locator('.hist-row')).toHaveCount(1);

    // Exclude chore 0 by tapping its chip
    await page.locator('.hist-filter-chip[data-action="history-filter-chore"]').first().click();
    await page.waitForTimeout(300);

    // Should show "No logs match" message
    await expect(page.locator('text=No logs match the selected chores.')).toBeVisible();
    await expect(page.locator('.hist-row')).toHaveCount(0);
  });
});
