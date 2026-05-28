// tests/e2e/sw-update-reload.spec.js
// Verifies that the page renders correctly after a page reload,
// simulating the refresh triggered by the "App updated" service worker toast.
// Regression test for blank-screen-after-SW-update bug.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-reload-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  const email = uniqueEmail();
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#user-avatar:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies())
    .find(c => c.name === 'choresy_csrf')?.value || '';
  await page.request.post('/api/household', {
    data: { name: `SW Reload Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
}

test.describe('SW Update Reload', () => {
  test('page renders home grid after initial reload', async ({ page }) => {
    await setupWithChores(page);
    await expect(page.locator('.home-grid')).toBeVisible();
    await expect(page.locator('.home-chore-card').first()).toBeVisible();
  });

  test('page renders home grid after multiple reloads', async ({ page }) => {
    await setupWithChores(page);

    for (let i = 0; i < 3; i++) {
      await page.reload();
      await page.waitForSelector('.home-grid', { timeout: 15000 });
      await expect(page.locator('.home-grid')).toBeVisible();
      await expect(page.locator('.home-chore-card').first()).toBeVisible();
    }
  });

  test('page renders correctly on first load without SW update toast', async ({ page }) => {
    await setupWithChores(page);

    const toast = page.locator('.sw-update-toast');
    await expect(toast).toHaveCount(0);
  });

  test('page does not show Welcome message after reload for existing user', async ({ page }) => {
    await setupWithChores(page);

    for (let i = 0; i < 2; i++) {
      await page.reload();
      await page.waitForSelector('.home-grid', { timeout: 15000 });
      await expect(page.locator('.home-grid')).toBeVisible();
    }

    // Verify the page shows chore content, not the setup/welcome message
    const body = await page.textContent('body');
    expect(body).not.toContain('Set Up Household');
    expect(body).not.toContain('No chores set up yet');
  });

  test('navigating between tabs after reload works correctly', async ({ page }) => {
    await setupWithChores(page);

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    await page.click('[data-nav="chores"]');
    await expect(page.locator('.chores-view')).toBeVisible({ timeout: 5000 });

    await page.click('[data-nav="activity"]');
    await expect(page.locator('.history-view')).toBeVisible({ timeout: 5000 });

    await page.click('[data-nav="today"]');
    await expect(page.locator('.home-grid')).toBeVisible({ timeout: 5000 });
  });
});
