// tests/e2e/home-toast-sheet-overlap.spec.js
// The undo toast floats where a sheet's primary action sits. Opening a sheet
// must dismiss it so it never covers Log. Also covers the header polish: the
// bell is visible in dark mode and the build version lives in Settings.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-toast-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', { data: { name: `Toast ${Date.now()}` }, headers: { 'X-CSRF-Token': csrf } });
  await page.request.post('/api/chores/seed-defaults', { headers: { 'X-CSRF-Token': csrf } });
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { csrf };
}

test.describe('Undo toast and sheets', () => {
  test('opening a sheet dismisses the undo toast so it cannot cover Log', async ({ page }) => {
    await setupWithChores(page);
    const cards = page.locator('.home-chore-card');

    await cards.nth(0).click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await page.click('[data-action="save-log"]');
    const toast = page.locator('#toast-container .toast', { has: page.locator('button', { hasText: 'Undo' }) });
    await expect(toast).toBeVisible({ timeout: 5000 });

    // Open the next sheet while the toast would still be showing (4 s).
    await cards.nth(1).click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(toast).toHaveCount(0);

    // The sheet's Log button is the topmost element at its own center.
    const save = page.locator('[data-action="save-log"]');
    await expect(save).toBeVisible();
    const box = await save.boundingBox();
    const topmostIsSave = await page.evaluate(({ x, y }) => {
      const el = document.elementFromPoint(x, y);
      return !!el?.closest('[data-action="save-log"]');
    }, { x: box.x + box.width / 2, y: box.y + box.height / 2 });
    expect(topmostIsSave).toBe(true);
  });
});

test.describe('Header polish', () => {
  test('the bell is visible on the dark header', async ({ browser }) => {
    const context = await browser.newContext({ colorScheme: 'dark' });
    const page = await context.newPage();
    await setupWithChores(page);
    const bell = page.locator('#notifications-bell');
    await expect(bell).toBeVisible();
    const colors = await page.evaluate(() => {
      const bellColor = getComputedStyle(document.querySelector('#notifications-bell')).color;
      const headerBg = getComputedStyle(document.querySelector('#top-bar')).backgroundColor;
      return { bellColor, headerBg };
    });
    const lum = (rgb) => {
      const [r, g, b] = rgb.match(/\d+/g).map(Number).map(v => v / 255)
        .map(c => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    };
    const [hi, lo] = [lum(colors.bellColor), lum(colors.headerBg)].sort((a, b) => b - a);
    expect((hi + 0.05) / (lo + 0.05)).toBeGreaterThan(4.5);
    await context.close();
  });

  test('the version label is in Settings, not the header', async ({ page }) => {
    await setupWithChores(page);
    await expect(page.locator('#top-bar .app-version')).toHaveCount(0);
    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view');
    await expect(page.getByTestId('app-version')).toContainText(/^Nabu v/);
  });
});
