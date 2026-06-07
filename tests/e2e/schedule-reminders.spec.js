// tests/e2e/schedule-reminders.spec.js
// Verifies schedule reminder notification preferences: type availability,
// settings toggle, default lead time selector, and per-chore reminder toggles.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-sr-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `SR Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { email, csrf };
}

test.describe('Schedule reminder notifications', () => {
  test('schedule_reminder type appears in available types', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const res = await page.request.get('/api/notification-preferences', {
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(res.ok()).toBe(true);

    const data = await res.json();
    const scheduleReminder = data.availableTypes.find(t => t.type === 'schedule_reminder');
    expect(scheduleReminder).toBeTruthy();
    expect(scheduleReminder.label).toBe('Schedule Reminder');
  });

  test('settings toggle for schedule_reminder appears and can be toggled', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.notif-pref-list', { timeout: 8000 });

    const checkbox = page.locator('.notif-pref-toggle input[data-notif-type="schedule_reminder"]');
    await expect(checkbox).toBeAttached();
    await expect(checkbox).toBeChecked();

    const slider = page.locator('.notif-pref-toggle input[data-notif-type="schedule_reminder"] + .toggle-slider');
    await slider.click();
    await page.waitForTimeout(800);

    const res = await page.request.get('/api/notification-preferences', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const data = await res.json();
    expect(data.preferences.enabledPushTypes).not.toContain('schedule_reminder');
  });

  test('default lead time selector shows and updates', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.notif-pref-list', { timeout: 8000 });

    const leadSelect = page.locator('select[data-action="change-default-reminder-lead"]');
    await expect(leadSelect).toBeVisible({ timeout: 5000 });
    await expect(leadSelect).toHaveValue('10');

    await page.selectOption('select[data-action="change-default-reminder-lead"]', '30');
    await page.waitForTimeout(800);

    await expect.poll(async () => {
      const res = await page.request.get('/api/notification-preferences', {
        headers: { 'X-CSRF-Token': csrf },
      });
      if (!res.ok()) return null;
      return (await res.json()).preferences.defaultReminderLeadMinutes;
    }, { timeout: 8000, intervals: [500] }).toBe(30);
  });

  test('chore edit sheet shows remind me section', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];
    expect(firstChore).toBeTruthy();

    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-row', { timeout: 10000 });

    const editBtn = page.locator(`[data-action="chore-edit"][data-chore-id="${firstChore.id}"]`);
    await editBtn.click();
    await page.waitForSelector('.chore-edit-sheet', { timeout: 8000 });

    const reminderToggle = page.locator('[data-action="toggle-chore-reminder"]');
    await expect(reminderToggle).toBeVisible({ timeout: 5000 });
    await expect(reminderToggle).not.toBeChecked();
  });

  test('can enable per-chore reminder and set lead time', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];

    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-row', { timeout: 10000 });

    const editBtn = page.locator(`[data-action="chore-edit"][data-chore-id="${firstChore.id}"]`);
    await editBtn.click();
    await page.waitForSelector('.chore-edit-sheet', { timeout: 8000 });

    const reminderToggle = page.locator('[data-action="toggle-chore-reminder"]');
    const toggleLabel = reminderToggle.locator('..');
    await toggleLabel.click();
    await page.waitForTimeout(800);

    const leadSelect = page.locator('select[data-action="change-chore-reminder-lead"]');
    await expect(leadSelect).toBeVisible({ timeout: 5000 });
    await leadSelect.selectOption('30');
    await page.waitForTimeout(800);

    await expect.poll(async () => {
      const res = await page.request.get('/api/chore-reminder-prefs', {
        headers: { 'X-CSRF-Token': csrf },
      });
      if (!res.ok()) return [];
      const prefs = (await res.json()).prefs;
      const pref = prefs.find(p => p.choreId === firstChore.id);
      return pref?.enabled && pref?.leadMinutes === 30;
    }, { timeout: 8000, intervals: [500] }).toBe(true);
  });

  test('per-chore reminder prefs persist after page reload', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];

    // Set the pref via API first for reliability
    await page.request.patch(`/api/chore-reminder-prefs/${firstChore.id}`, {
      data: { enabled: true, leadMinutes: 15 },
      headers: { 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' },
    });

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    // Verify via API
    const res = await page.request.get('/api/chore-reminder-prefs', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const data = await res.json();
    const pref = data.prefs.find(p => p.choreId === firstChore.id);
    expect(pref).toBeTruthy();
    expect(pref.enabled).toBe(true);
    expect(pref.leadMinutes).toBe(15);
  });

  test('new chore sheet does not show remind me section', async ({ page }) => {
    await setupWithChores(page);

    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-row', { timeout: 10000 });

    await page.click('.fab[data-action="chore-add"]');
    await page.waitForSelector('.chore-edit-sheet', { timeout: 5000 });

    const reminderToggle = page.locator('[data-action="toggle-chore-reminder"]');
    await expect(reminderToggle).toHaveCount(0);
  });
});
