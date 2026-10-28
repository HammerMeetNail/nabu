// tests/e2e/recent-volume-chips.spec.js
// Phase 5.3 — recent-value chips fill the volume input on the log sheet.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-recentvol-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `RecentVol ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  const res = await page.request.post('/api/chores', {
    data: { name: 'Bottle', icon: '🍼', color: '#EC4899', metricType: 'amount', metricUnit: 'mL', indicatorLabels: ['🍼 formula'] },
    headers: { 'X-CSRF-Token': csrf },
  });
  const chore = (await res.json()).chore;
  // Prior log establishes a recent value of 95 mL.
  await page.request.post('/api/logs', {
    data: { choreId: chore.id, indicators: ['🍼 formula'], indicatorVolumes: { '🍼 formula': 95 } },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { csrf, chore };
}

test.describe('Recent-value chips (Phase 5.3)', () => {
  test('tapping a recent chip fills the indicator volume select', async ({ page }) => {
    const { chore } = await setup(page);

    await page.click(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    await page.waitForSelector('.volume-recent-chip');
    await expect(page.locator('.volume-recent-chip').first()).toContainText('95');

    // The recent chip is its own class, not .log-chip (regression guard).
    await expect(page.locator('.log-chip.volume-recent-chip')).toHaveCount(0);

    await page.click('.volume-recent-chip:has-text("95")');
    await expect(page.locator('.indicator-volume-select').first()).toHaveValue('95');
  });
});
