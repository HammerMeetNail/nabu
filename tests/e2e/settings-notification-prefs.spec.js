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
    // Per-type toggles are visible (push is enabled by default)
    await expect(page.locator('.notif-pref-list')).toBeVisible({ timeout: 5000 });

    const checkbox = page.locator('.notif-pref-toggle input[data-notif-type="chore_logged"]');
    await expect(checkbox).toBeAttached();
    await expect(checkbox).toBeChecked();

    // Click push notifications master toggle to disable all
    const pushSlider = page.locator('.notif-pref-toggle input[data-action="toggle-push-enabled"] + .toggle-slider');
    await pushSlider.scrollIntoViewIfNeeded();
    await pushSlider.click();
    await page.waitForTimeout(500);

    // Per-type toggles should be hidden when push is off
    await expect(page.locator('.notif-pref-list')).not.toBeVisible({ timeout: 5000 });

    // Re-enable push
    await pushSlider.scrollIntoViewIfNeeded();
    await pushSlider.click();
    await page.waitForTimeout(500);

    // Per-type toggles should be visible again
    await expect(page.locator('.notif-pref-list')).toBeVisible({ timeout: 5000 });
  });

  test('schedule reminder toggle shows/hides lead time selector', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view', { timeout: 5000 });

    // Schedule Reminder toggle should be visible
    const srToggle = page.locator('.notif-pref-toggle input[data-notif-type="schedule_reminder"]');
    await expect(srToggle).toBeAttached();
    await expect(srToggle).toBeChecked();

    // Lead time selector should be visible when schedule_reminder is enabled
    await expect(page.locator('.notif-pref-select')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.notif-pref-select')).toHaveValue('10');

    // Change lead time to 15
    await page.locator('.notif-pref-select').selectOption('15');
    await page.waitForTimeout(300);

    // Verify via API
    await expect.poll(async () => {
      const res = await page.request.get('/api/notification-preferences', {
        headers: { 'X-CSRF-Token': csrf },
      });
      if (!res.ok()) return null;
      const data = await res.json();
      return data.preferences.defaultReminderLeadMinutes;
    }, { timeout: 5000, intervals: [300] }).toBe(15);

    // Turn off schedule_reminder
    const srSlider = srToggle.locator('+ .toggle-slider');
    await srSlider.scrollIntoViewIfNeeded();
    await srSlider.click();
    await page.waitForTimeout(500);

    // Lead time selector should be hidden
    await expect(page.locator('.notif-pref-select')).not.toBeAttached();
  });

  test('notification preferences persist after page reload', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view', { timeout: 5000 });
    await expect(page.locator('.notif-pref-list')).toBeVisible({ timeout: 5000 });

    // Disable push notifications
    const patchRes = await page.request.patch('/api/notification-preferences', {
      data: { enabledPushTypes: [], pushEnabled: false },
      headers: { 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' },
    });
    expect(patchRes.ok()).toBe(true);

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view', { timeout: 5000 });

    // Per-type toggles should be hidden (push is off)
    await expect(page.locator('.notif-pref-list')).not.toBeVisible({ timeout: 5000 });

    // Push master toggle should be off
    const pushToggle = page.locator('.notif-pref-toggle input[data-action="toggle-push-enabled"]');
    await expect(pushToggle).not.toBeChecked();
  });

  test('lead time persists after page reload', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Set lead time via API
    const patchRes = await page.request.patch('/api/notification-preferences', {
      data: { defaultReminderLeadMinutes: 30 },
      headers: { 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' },
    });
    expect(patchRes.ok()).toBe(true);

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view', { timeout: 5000 });

    // Lead time selector should show 30
    await expect(page.locator('.notif-pref-select')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.notif-pref-select')).toHaveValue('30');
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

    // Verify defaultReminderLeadMinutes is present in preferences
    expect(data.preferences).toHaveProperty('defaultReminderLeadMinutes');
    expect(typeof data.preferences.defaultReminderLeadMinutes).toBe('number');
  });
});
