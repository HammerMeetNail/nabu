// tests/e2e/nav-tabs-position.spec.js
// Regression test: bottom tabs must be locked to the bottom of the viewport
// at all times — on initial load and after navigation.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-tabs-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#user-avatar:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Nav Tabs Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { email, csrf };
}

async function tabsBottomGap(page) {
  return page.evaluate(() => {
    const tabs = document.querySelector('#bottom-tabs');
    if (!tabs) return null;
    const rect = tabs.getBoundingClientRect();
    return {
      bottom: rect.bottom,
      viewportHeight: window.innerHeight,
      gap: window.innerHeight - rect.bottom,
    };
  });
}

test.describe('Navigation Tabs: Positioning', () => {
  test('page versions CSS and manifest URLs', async ({ page }) => {
    await page.goto('/register');

    const assets = await page.evaluate(() => {
      return {
        stylesheetHref: document.querySelector('link[rel="stylesheet"]')?.getAttribute('href'),
        manifestHref: document.querySelector('link[rel="manifest"]')?.getAttribute('href'),
      };
    });

    expect(assets.stylesheetHref).toMatch(/^\/static\/css\/app\.css\?v=/);
    expect(assets.manifestHref).toMatch(/^\/static\/manifest\.webmanifest\?v=/);
  });

  test('showing tabs does not leave temporary body padding behind', async ({ page }) => {
    await setupWithChores(page);

    const paddingBottom = await page.evaluate(() => {
      return window.getComputedStyle(document.body).paddingBottom;
    });

    expect(paddingBottom).toBe('0px');
  });

  test('bottom tabs are flush with the viewport bottom on initial load', async ({ page }) => {
    await setupWithChores(page);

    const result = await tabsBottomGap(page);
    expect(result).not.toBeNull();
    // Bottom edge of the tabs should be within 2px of the viewport bottom.
    // A small tolerance accounts for sub-pixel rendering.
    expect(Math.abs(result.gap)).toBeLessThan(2);
  });

  test('bottom tabs stay flush after navigating to another tab', async ({ page }) => {
    await setupWithChores(page);

    // Navigate to Calendar
    await page.click('[data-nav="calendar"]');
    await expect(page.locator('.cal-date')).toBeVisible({ timeout: 10000 });

    const result = await tabsBottomGap(page);
    expect(result).not.toBeNull();
    expect(Math.abs(result.gap)).toBeLessThan(2);
  });

  test('bottom tabs stay flush after navigating back to Home', async ({ page }) => {
    await setupWithChores(page);

    // Navigate away and back
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 10000 });
    await page.click('[data-nav="today"]');
    await expect(page.locator('.home-grid')).toBeVisible({ timeout: 5000 });

    const result = await tabsBottomGap(page);
    expect(result).not.toBeNull();
    expect(Math.abs(result.gap)).toBeLessThan(2);
  });

  test('bottom tabs stay flush when content height changes', async ({ page }) => {
    await setupWithChores(page);

    // Log a chore to add content, then verify tabs stay put
    const firstCard = page.locator('.home-chore-card').first();
    await firstCard.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await page.click('[data-action="save-home-log"]');
    await page.waitForTimeout(1500);

    const result = await tabsBottomGap(page);
    expect(result).not.toBeNull();
    expect(Math.abs(result.gap)).toBeLessThan(2);
  });

  test('bottom tabs stay flush after a full page reload', async ({ page }) => {
    await setupWithChores(page);

    // Verify initial positioning
    let result = await tabsBottomGap(page);
    expect(result).not.toBeNull();
    expect(Math.abs(result.gap)).toBeLessThan(2);

    // Reload and check again — this simulates reopening the app
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    result = await tabsBottomGap(page);
    expect(result).not.toBeNull();
    expect(Math.abs(result.gap)).toBeLessThan(2);
  });
});
