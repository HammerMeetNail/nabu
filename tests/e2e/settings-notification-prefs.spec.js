// tests/e2e/settings-notification-prefs.spec.js
// Verifies the notification preference toggles in the settings page.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-np-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
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

  const csrf = await getCSRF(page);

  await page.request.post('/api/household', {
    data: { name: `NP Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { email, csrf };
}

test.describe('Settings: notification preferences', () => {
  test('notification toggle appears in settings and can be toggled', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view', { timeout: 5000 });
    await expect(page.locator('.notif-pref-list')).toBeVisible({ timeout: 5000 });

    const checkbox = page.locator('.notif-pref-toggle input[data-notif-type="chore_logged"]');
    await expect(checkbox).toBeAttached();
    await expect(checkbox).toBeChecked();

    // Turn off each toggle so pushEnabled becomes false
    const toggles = page.locator('.notif-pref-toggle .toggle-slider');
    const toggleCount = await toggles.count();
    for (let i = 0; i < toggleCount; i++) {
      const slider = toggles.nth(i);
      await slider.scrollIntoViewIfNeeded();
      // Only click if it's checked (some may be already off)
      const input = page.locator('.notif-pref-toggle input').nth(i);
      if (await input.isChecked()) {
        await slider.click();
        await page.waitForTimeout(300);
      }
    }

    // Verify push is now disabled
    await expect.poll(async () => {
      const res = await page.request.get('/api/notification-preferences', {
        headers: { 'X-CSRF-Token': csrf },
      });
      if (!res.ok()) return null;
      const data = await res.json();
      return data.preferences.pushEnabled;
    }, { timeout: 10000, intervals: [300] }).toBe(false);

    // Turn chore_logged back ON
    const toggleSliderOn = page.locator('.notif-pref-toggle input[data-notif-type="chore_logged"] + .toggle-slider').first();
    await toggleSliderOn.scrollIntoViewIfNeeded();
    await toggleSliderOn.click();
    await page.waitForTimeout(500);

    // Verify push is re-enabled
    await expect.poll(async () => {
      const res = await page.request.get('/api/notification-preferences', {
        headers: { 'X-CSRF-Token': csrf },
      });
      if (!res.ok()) return null;
      const data = await res.json();
      return data.preferences.pushEnabled;
    }, { timeout: 5000, intervals: [300] }).toBe(true);
  });

  test('notification preferences persist after page reload', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view', { timeout: 5000 });
    await expect(page.locator('.notif-pref-list')).toBeVisible({ timeout: 5000 });

    const patchRes = await page.request.patch('/api/notification-preferences', {
      data: { enabledPushTypes: [], pushEnabled: false },
      headers: { 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' },
    });
    expect(patchRes.ok()).toBe(true);

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view', { timeout: 5000 });
    await page.waitForSelector('.notif-pref-list', { timeout: 5000 });

    const checkbox = page.locator('.notif-pref-toggle input[data-notif-type="chore_logged"]');
    await expect(checkbox).toBeAttached();
    await expect(checkbox).not.toBeChecked();
  });

  test('available notification types are returned by the API', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const res = await page.request.get('/api/notification-preferences', {
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(res.ok()).toBe(true);

    const data = await res.json();
    expect(data).toHaveProperty('availableTypes');
    expect(data.availableTypes.length).toBeGreaterThanOrEqual(1);

    const choreLogged = data.availableTypes.find(t => t.type === 'chore_logged');
    expect(choreLogged).toBeTruthy();
    expect(choreLogged.label).toBe('Chore Logged');
    expect(choreLogged.description).toBeTruthy();
  });
});
