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
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';

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

async function openFilter(page) {
  const chips = page.locator('.hist-filter-chips');
  const isOpen = await chips.evaluate(el => el.classList.contains('hist-filter-chips--open'));
  if (!isOpen) {
    await page.locator('.hist-filter-btn').click();
  }
  await expect(chips).toHaveClass(/hist-filter-chips--open/);
}

test.describe('History filter', () => {
  test('filter dropdown opens with nothing selected by default', async ({ page }) => {
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

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    // Filter button FAB is visible; chips start closed
    await expect(page.locator('.hist-filter-fab')).toBeVisible();

    // Tap to open (reuse helper that handles the toggle)
    await openFilter(page);

    await expect(page.locator('.hist-filter-all')).toBeVisible();
    const chips = page.locator('.hist-filter-chip[data-action="history-filter-chore"]');
    await expect(chips).toHaveCount(chores.length);

    // No chore chips selected by default; "All activity" is the active state.
    for (let i = 0; i < chores.length; i++) {
      await expect(chips.nth(i)).not.toHaveClass(/active/);
    }
    await expect(page.locator('.hist-filter-all')).toHaveClass(/active/);
  });

  test('tapping a chore chip shows only that chore', async ({ page }) => {
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

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    await expect(page.locator('.hist-row')).toHaveCount(3);

    await openFilter(page);
    const chip0 = page.locator(`.hist-filter-chip[data-chore-id="${chores[0].id}"]`);
    await chip0.click();
    await page.waitForTimeout(300);

    await expect(chip0).toHaveClass(/active/);
    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(page.locator('.hist-filter-all')).not.toHaveClass(/active/);
  });

  test('selecting multiple chores is additive', async ({ page }) => {
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

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    await expect(page.locator('.hist-row')).toHaveCount(3);

    await openFilter(page);
    const chip0 = page.locator(`.hist-filter-chip[data-chore-id="${chores[0].id}"]`);
    const chip1 = page.locator(`.hist-filter-chip[data-chore-id="${chores[1].id}"]`);

    // First selection narrows to a single chore.
    await chip0.click();
    await page.waitForTimeout(300);
    await expect(page.locator('.hist-row')).toHaveCount(1);

    // Adding a second selection widens to two chores.
    await chip1.click();
    await page.waitForTimeout(300);
    await expect(page.locator('.hist-row')).toHaveCount(2);
    await expect(chip0).toHaveClass(/active/);
    await expect(chip1).toHaveClass(/active/);
    await expect(page.locator('.hist-filter-all')).not.toHaveClass(/active/);
  });

  test('tapping "All activity" clears the filter', async ({ page }) => {
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

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    await expect(page.locator('.hist-row')).toHaveCount(2);

    await openFilter(page);

    const chip0 = page.locator(`.hist-filter-chip[data-chore-id="${chores[0].id}"]`);
    await chip0.click();
    await page.waitForTimeout(300);
    await expect(page.locator('.hist-row')).toHaveCount(1);

    // "All activity" clears any selection and shows everything again.
    await page.locator('.hist-filter-all').click();
    await page.waitForTimeout(300);
    await expect(page.locator('.hist-row')).toHaveCount(2);
    await expect(page.locator('.hist-filter-all')).toHaveClass(/active/);
    await expect(chip0).not.toHaveClass(/active/);
  });

  test('tapping a selected chip again removes it', async ({ page }) => {
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

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    await openFilter(page);

    const chip0 = page.locator(`.hist-filter-chip[data-chore-id="${chores[0].id}"]`);
    const chip1 = page.locator(`.hist-filter-chip[data-chore-id="${chores[1].id}"]`);
    await chip0.click();
    await page.waitForTimeout(300);
    await chip1.click();
    await page.waitForTimeout(300);

    await expect(page.locator('.hist-row')).toHaveCount(2);

    // Deselect chip0: only chip1's chore remains.
    await chip0.click();
    await page.waitForTimeout(300);
    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(chip0).not.toHaveClass(/active/);
    await expect(chip1).toHaveClass(/active/);

    // Deselect the last chip: filter clears, everything shows again.
    await chip1.click();
    await page.waitForTimeout(300);
    await expect(page.locator('.hist-row')).toHaveCount(3);
    await expect(page.locator('.hist-filter-all')).toHaveClass(/active/);
    await expect(chip0).not.toHaveClass(/active/);
    await expect(chip1).not.toHaveClass(/active/);
  });

  test('shows empty message when selection matches no logs', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(2);

    // Only chores[0] has a log; selecting a different chore matches nothing.
    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: new Date().toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    await expect(page.locator('.hist-row')).toHaveCount(1);

    await openFilter(page);
    await page.locator(`.hist-filter-chip[data-chore-id="${chores[1].id}"]`).click();
    await page.waitForTimeout(300);

    // Single page, no older logs: definitive "nothing matches" message.
    await expect(page.locator('text=No activity matches the selected chores.')).toBeVisible();
    await expect(page.locator('.hist-row')).toHaveCount(0);
  });

  test('empty filtered page still offers Load more to page further back', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const chores = (await choresRes.json()).chores;
    expect(chores.length).toBeGreaterThanOrEqual(2);

    const now = new Date();
    const tenDaysAgo = new Date(now.getTime() - 10 * 86400000);

    // chores[0]: recent (in the current 7-day window).
    await page.request.post('/api/logs', {
      data: { choreId: chores[0].id, note: '', indicators: [], completedAt: now.toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });
    // chores[1]: older than a week, so it lives on a later page.
    await page.request.post('/api/logs', {
      data: { choreId: chores[1].id, note: '', indicators: [], completedAt: tenDaysAgo.toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await page.waitForSelector('.hist-row', { timeout: 10000 });

    // First page shows only the recent log, and a Load more button (older logs exist).
    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(page.locator('.load-more-btn')).toBeVisible();

    // Filter to the older chore: nothing on this page matches, but the match is
    // further back — so we show the time-range hint AND keep Load more.
    await openFilter(page);
    await page.locator(`.hist-filter-chip[data-chore-id="${chores[1].id}"]`).click();
    await page.waitForTimeout(300);

    await expect(page.locator('.hist-row')).toHaveCount(0);
    await expect(page.locator('text=No matching activity in this time range.')).toBeVisible();
    await expect(page.locator('.load-more-btn')).toBeVisible();

    // Paging back reveals the older matching log.
    await page.locator('.load-more-btn').click();
    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(page.locator(`.hist-row[data-chore-id="${chores[1].id}"]`)).toHaveCount(1);
  });
});
