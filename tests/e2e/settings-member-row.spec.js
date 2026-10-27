// tests/e2e/settings-member-row.spec.js
// A member row opened in Settings stays open when Settings re-renders, as it
// does whenever data arrives. It used to snap shut, hiding the row's controls
// (household-lifecycle.spec.js failed now and then on exactly this).

import { test, expect } from '@playwright/test';

test('an opened member row survives a re-render', async ({ page }) => {
  const email = `e2e-member-row-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
  await page.goto('/register');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await expect(page.getByRole('heading', { name: 'Welcome to Nabu' })).toBeVisible({ timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  expect((await page.request.post('/api/household', { data: { name: 'Home' }, headers: { 'X-CSRF-Token': csrf } })).ok()).toBeTruthy();

  await page.goto('/settings');
  const row = page.locator('.member-row').first();
  await row.locator('summary').click();
  await expect(row).toHaveAttribute('open', '');

  // Re-render Settings, and know that it happened: morph removes an
  // attribute the markup doesn't have.
  const settings = page.locator('.settings-view');
  await settings.evaluate(el => el.setAttribute('data-e2e-before-render', ''));
  await page.evaluate(() => window.dispatchEvent(new Event('nabu-journal-change')));
  await expect(settings).not.toHaveAttribute('data-e2e-before-render', '');
  await expect(row).toHaveAttribute('open', '');
});
