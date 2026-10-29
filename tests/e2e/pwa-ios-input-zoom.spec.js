// tests/e2e/pwa-ios-input-zoom.spec.js
// iOS zooms the page into a text field smaller than 16px when it gets focus,
// and the page stays zoomed after the keyboard closes: the tab bar ends up
// below the screen. Every text field people type into must be 16px or more.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-zoom-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', { data: { name: `Zoom ${Date.now()}` }, headers: { 'X-CSRF-Token': csrf } });
  await page.request.post('/api/chores/seed-defaults', { headers: { 'X-CSRF-Token': csrf } });
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
}

// Visible fields that take typing, with a font smaller than 16px.
async function smallFields(page) {
  return page.evaluate(() => [...document.querySelectorAll('input, select, textarea')]
    .filter(el => el.getClientRects().length > 0 &&
      !['checkbox', 'radio', 'hidden', 'range', 'color', 'button', 'submit'].includes(el.type))
    .map(el => ({ field: el.id || el.name || el.className, px: parseFloat(getComputedStyle(el).fontSize) }))
    .filter(f => f.px < 16));
}

test('text fields are 16px or more, so iOS never zooms into one', async ({ page }) => {
  await setupWithChores(page);

  await page.click('[data-action="home-tap-chore"] >> nth=0');
  await expect(page.locator('.bottom-sheet')).toBeVisible();
  expect(await smallFields(page), 'log sheet').toEqual([]);
  await page.keyboard.press('Escape');
  await expect(page.locator('.bottom-sheet')).toHaveCount(0);

  await page.click('a[data-nav="activity"]');
  await expect(page.locator('#history-search-input')).toBeVisible();
  expect(await smallFields(page), 'activity').toEqual([]);

  await page.click('a[data-nav="settings"]');
  await page.waitForSelector('[data-testid="api-keys"]');
  expect(await smallFields(page), 'settings').toEqual([]);

  await page.click('a[data-nav="stats"]');
  await expect(page.locator('.busy-hours-filter').first()).toBeVisible({ timeout: 15000 });
  await expect(page.locator('.feeding-gaps-date').first()).toBeVisible();
  expect(await smallFields(page), 'stats').toEqual([]);
});
