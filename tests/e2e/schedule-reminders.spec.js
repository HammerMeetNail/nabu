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
    expect(data).toHaveProperty('availableTypes');

    const scheduleReminder = data.availableTypes.find(t => t.type === 'schedule_reminder');
    expect(scheduleReminder).toBeTruthy();
    expect(scheduleReminder.label).toBe('Schedule Reminder');
  });

  test('settings toggle for schedule_reminder appears and works', async ({ page }) => {
    await setupWithChores(page);

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view', { timeout: 5000 });
    await page.waitForSelector('.notif-pref-list', { timeout: 5000 });

    const checkbox = page.locator('.notif-pref-toggle input[data-notif-type="schedule_reminder"]');
    await expect(checkbox).toBeAttached();
    await expect(checkbox).toBeChecked();

    const slider = checkbox.locator('+ .toggle-slider');
    await slider.scrollIntoViewIfNeeded();
    await slider.click();
    await page.waitForTimeout(500);

    await expect(checkbox).not.toBeChecked();
  });

  test('default lead time selector shows when schedule_reminder is enabled', async ({ page }) => {
    await setupWithChores(page);

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view', { timeout: 5000 });
    await page.waitForSelector('.notif-pref-list', { timeout: 5000 });

    const leadSelect = page.locator('select[data-action="change-default-reminder-lead"]');
    await expect(leadSelect).toBeVisible({ timeout: 3000 });
    await expect(leadSelect).toHaveValue('10');
  });

  test('can update default lead time', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.click('a[data-nav="settings"]');
    await page.waitForSelector('.settings-view', { timeout: 5000 });
    await page.waitForSelector('select[data-action="change-default-reminder-lead"]', { timeout: 5000 });

    await page.selectOption('select[data-action="change-default-reminder-lead"]', '30');
    await page.waitForTimeout(500);

    await expect.poll(async () => {
      const res = await page.request.get('/api/notification-preferences', {
        headers: { 'X-CSRF-Token': csrf },
      });
      if (!res.ok()) return null;
      const data = await res.json();
      return data.preferences.defaultReminderLeadMinutes;
    }, { timeout: 5000, intervals: [300] }).toBe(30);
  });

  test('chore edit sheet shows remind me section when schedule_reminder is enabled', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];
    expect(firstChore).toBeTruthy();

    // Navigate to Manage view
    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-row', { timeout: 10000 });

    const editBtn = page.locator(`[data-action="chore-edit"][data-chore-id="${firstChore.id}"]`);
    await editBtn.scrollIntoViewIfNeeded();
    await editBtn.click();
    await page.waitForSelector('.chore-edit-sheet', { timeout: 5000 });

    const reminderToggle = page.locator('[data-action="toggle-chore-reminder"]');
    await expect(reminderToggle).toBeVisible({ timeout: 3000 });
    await expect(reminderToggle).not.toBeChecked();
  });

  test('can toggle per-chore reminder on and select lead time', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];

    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-row', { timeout: 10000 });

    const editBtn = page.locator(`[data-action="chore-edit"][data-chore-id="${firstChore.id}"]`);
    await editBtn.scrollIntoViewIfNeeded();
    await editBtn.click();
    await page.waitForSelector('.chore-edit-sheet', { timeout: 5000 });

    const reminderToggle = page.locator('[data-action="toggle-chore-reminder"]');
    await reminderToggle.scrollIntoViewIfNeeded();

    const toggleSlider = reminderToggle.locator('..');
    await toggleSlider.click();
    await page.waitForTimeout(500);

    await expect(reminderToggle).toBeChecked();

    const leadSelect = page.locator('select[data-action="change-chore-reminder-lead"]');
    await expect(leadSelect).toBeVisible({ timeout: 3000 });
    await expect(leadSelect).toHaveValue('10');

    await leadSelect.selectOption('30');
    await page.waitForTimeout(500);

    await expect.poll(async () => {
      const res = await page.request.get('/api/chore-reminder-prefs', {
        headers: { 'X-CSRF-Token': csrf },
      });
      if (!res.ok()) return [];
      const data = await res.json();
      const pref = data.prefs.find(p => p.choreId === firstChore.id);
      return pref?.enabled && pref?.leadMinutes === 30;
    }, { timeout: 5000, intervals: [300] }).toBe(true);
  });

  test('per-chore reminder prefs persist after page reload', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];

    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-row', { timeout: 10000 });

    const editBtn = page.locator(`[data-action="chore-edit"][data-chore-id="${firstChore.id}"]`);
    await editBtn.scrollIntoViewIfNeeded();
    await editBtn.click();
    await page.waitForSelector('.chore-edit-sheet', { timeout: 5000 });

    const reminderToggle = page.locator('[data-action="toggle-chore-reminder"]');
    const toggleSlider = reminderToggle.locator('..');
    await toggleSlider.click();
    await page.waitForTimeout(300);

    await page.locator('select[data-action="change-chore-reminder-lead"]').selectOption('15');
    await page.waitForTimeout(500);

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-row', { timeout: 10000 });

    const editBtn2 = page.locator(`[data-action="chore-edit"][data-chore-id="${firstChore.id}"]`);
    await editBtn2.scrollIntoViewIfNeeded();
    await editBtn2.click();
    await page.waitForSelector('.chore-edit-sheet', { timeout: 5000 });

    const reminderToggleReload = page.locator('[data-action="toggle-chore-reminder"]');
    await expect(reminderToggleReload).toBeChecked({ timeout: 3000 });

    const leadSelectReload = page.locator('select[data-action="change-chore-reminder-lead"]');
    await expect(leadSelectReload).toHaveValue('15', { timeout: 3000 });
  });

  test('new chore edit sheet does not show remind me section', async ({ page }) => {
    await setupWithChores(page);

    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-row', { timeout: 10000 });

    await page.click('.fab[data-action="chore-add"]');
    await page.waitForSelector('.chore-edit-sheet', { timeout: 5000 });

    const reminderToggle = page.locator('[data-action="toggle-chore-reminder"]');
    await expect(reminderToggle).toHaveCount(0, { timeout: 3000 });
  });
});
