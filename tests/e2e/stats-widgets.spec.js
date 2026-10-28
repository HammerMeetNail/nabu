// tests/e2e/stats-widgets.spec.js
// Phase 4 — secure user-defined stats widgets: the "+ Add widget" wizard, the
// rendered widget, XSS-inert titles, and removal.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-widgets-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChore(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', {
    data: { name: `Widgets ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  const res = await page.request.post('/api/chores', {
    data: { name: 'Bottle', icon: '🍼', color: '#EC4899', metricType: 'amount', metricUnit: 'mL' },
    headers: { 'X-CSRF-Token': csrf },
  });
  const chore = (await res.json()).chore;
  await page.request.post('/api/logs', {
    data: { choreId: chore.id, volumeML: 100 },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { csrf, chore };
}

test.describe('Stats widgets (Phase 4)', () => {
  test('add a widget via the wizard and see it render', async ({ page }) => {
    await setupWithChore(page);
    await page.click('a[data-nav="stats"]');
    await page.waitForSelector('.stats-page');

    await page.click('button[data-action="toggle-customize-stats"]');
    await page.click('[data-action="widget-add"]');
    await page.waitForSelector('.widget-wizard-sheet');

    await page.fill('#widget-title', 'My Bottles');
    await page.check('.widget-chore-check input');
    await page.selectOption('#widget-metric', 'amount');
    await page.click('[data-action="widget-save"]');

    await expect(page.locator('.widget-card')).toContainText('My Bottles');
    // total of amount over the loaded window = 100 mL
    await expect(page.locator('.widget-card')).toContainText('100');
  });

  test('a malicious widget title renders inert (no script executes)', async ({ page }) => {
    const { csrf, chore } = await setupWithChore(page);
    await page.request.patch('/api/preferences', {
      data: {
        statsWidgets: [{
          type: 'total', metric: 'count', period: 'week', choreIds: [chore.id],
          title: '<img src=x onerror="window.__xss=1">',
        }],
      },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('a[data-nav="stats"]');
    await page.waitForSelector('.stats-page');
    await page.waitForTimeout(500); // give any (broken) onerror a chance to fire

    await expect(page.locator('.widget-card img')).toHaveCount(0);
    const xss = await page.evaluate(() => window.__xss);
    expect(xss).toBeUndefined();
    // The title text is present, escaped.
    await expect(page.locator('.widget-card')).toContainText('<img src=x');
  });

  test('widget period scopes the total (all-time counts an old log, week does not)', async ({ page }) => {
    const { csrf, chore } = await setupWithChore(page); // logs one volume today
    const old = new Date();
    old.setDate(old.getDate() - 40);
    await page.request.post('/api/logs', {
      data: { choreId: chore.id, completedAt: old.toISOString() },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.request.patch('/api/preferences', {
      data: {
        statsWidgets: [
          { type: 'total', metric: 'count', period: 'week', choreIds: [chore.id], title: 'WeekCount' },
          { type: 'total', metric: 'count', period: 'all', choreIds: [chore.id], title: 'AllCount' },
        ],
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.reload();
    await page.click('a[data-nav="stats"]');
    await page.waitForSelector('.widget-card');

    const week = page.locator('.widget-card', { hasText: 'WeekCount' });
    const all = page.locator('.widget-card', { hasText: 'AllCount' });
    await expect(week.locator('.widget-big-number')).toHaveText('1'); // today only
    await expect(all.locator('.widget-big-number')).toHaveText('2');  // today + 40d ago
  });

  test('the wizard lists pickable chores (checkbox next to its name) and has no period field', async ({ page }) => {
    const { csrf } = await setupWithChore(page); // creates a "Bottle" chore
    await page.request.post('/api/chores', { data: { name: 'Nap', icon: '😴', color: '#60A5FA' }, headers: { 'X-CSRF-Token': csrf } });
    await page.reload();
    await page.click('a[data-nav="stats"]');
    await page.waitForSelector('.stats-page');
    await page.click('button[data-action="toggle-customize-stats"]');
    await page.click('[data-action="widget-add"]');
    await page.waitForSelector('.widget-wizard-sheet');

    // Each chore is a labelled, checkable row.
    const rows = page.locator('.widget-chore-check');
    await expect(rows).toHaveCount(2);
    await expect(page.locator('.widget-chore-check', { hasText: 'Bottle' })).toBeVisible();
    await expect(page.locator('.widget-chore-check', { hasText: 'Nap' })).toBeVisible();
    // The checkbox sits immediately left of its label (not shoved to the far
    // right): the checkbox's right edge is within a few px of the label's left.
    const box = page.locator('.widget-chore-check', { hasText: 'Bottle' });
    const cb = await box.locator('input[type="checkbox"]').boundingBox();
    const sp = await box.locator('span').boundingBox();
    expect(sp.x - (cb.x + cb.width)).toBeLessThan(16);
    // Checking a chore works.
    await box.locator('input[type="checkbox"]').check();
    await expect(box.locator('input[type="checkbox"]')).toBeChecked();

    // Period is no longer chosen at create time.
    await expect(page.locator('#widget-period')).toHaveCount(0);
    await expect(page.locator('#widget-presentation')).toBeVisible();
    await expect(page.locator('#widget-metric')).toBeVisible();
  });

  test('a widget card has a day/week/month period toggle that persists', async ({ page }) => {
    const { csrf, chore } = await setupWithChore(page);
    await page.request.patch('/api/preferences', {
      data: { statsWidgets: [{ type: 'total', metric: 'count', period: 'week', choreIds: [chore.id], title: 'Counter' }] },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.reload();
    await page.click('a[data-nav="stats"]');
    const card = page.locator('.widget-card', { hasText: 'Counter' });
    await card.waitFor();

    await expect(card.locator('.period-toggle-btn', { hasText: 'Week' })).toHaveClass(/period-toggle--active/);
    await expect(card.locator('.period-toggle-btn', { hasText: 'All' })).toHaveCount(0); // day/week/month only
    await card.locator('.period-toggle-btn', { hasText: 'Day' }).click();
    await expect(card.locator('.period-toggle-btn', { hasText: 'Day' })).toHaveClass(/period-toggle--active/);

    // Persists across reload.
    await page.reload();
    await page.click('a[data-nav="stats"]');
    const card2 = page.locator('.widget-card', { hasText: 'Counter' });
    await expect(card2.locator('.period-toggle-btn', { hasText: 'Day' })).toHaveClass(/period-toggle--active/);
  });

  test('remove a widget', async ({ page }) => {
    const { csrf, chore } = await setupWithChore(page);
    await page.request.patch('/api/preferences', {
      data: { statsWidgets: [{ type: 'total', metric: 'count', period: 'week', choreIds: [chore.id], title: 'Doomed' }] },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.reload();
    await page.click('a[data-nav="stats"]');
    await page.waitForSelector('.widget-card');
    await expect(page.locator('.widget-card')).toContainText('Doomed');

    await page.click('.widget-card .widget-remove-btn');
    await expect(page.locator('.widget-card')).toHaveCount(0);
  });
});
