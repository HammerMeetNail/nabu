// tests/e2e/schedule-reminders.spec.js
// Verifies schedule reminder notification preferences via API.

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

  test('notification preferences include defaultReminderLeadMinutes', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const res = await page.request.get('/api/notification-preferences', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const data = await res.json();
    expect(data.preferences).toHaveProperty('defaultReminderLeadMinutes');
    expect(data.preferences.defaultReminderLeadMinutes).toBe(10);
  });

  test('can update defaultReminderLeadMinutes', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const patchRes = await page.request.patch('/api/notification-preferences', {
      data: { defaultReminderLeadMinutes: 30 },
      headers: { 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' },
    });
    expect(patchRes.ok()).toBe(true);

    const getRes = await page.request.get('/api/notification-preferences', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const data = await getRes.json();
    expect(data.preferences.defaultReminderLeadMinutes).toBe(30);
  });

  test('can enable schedule_reminder notification type', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const patchRes = await page.request.patch('/api/notification-preferences', {
      data: { enabledPushTypes: ['schedule_reminder'] },
      headers: { 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' },
    });
    expect(patchRes.ok()).toBe(true);

    const getRes = await page.request.get('/api/notification-preferences', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const data = await getRes.json();
    expect(data.preferences.pushEnabled).toBe(true);
    expect(data.preferences.enabledPushTypes).toContain('schedule_reminder');
  });

  test('chore reminder prefs list is empty by default', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const res = await page.request.get('/api/chore-reminder-prefs', {
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(res.ok()).toBe(true);
    const data = await res.json();
    expect(data.prefs).toEqual([]);
  });

  test('can create and update a per-chore reminder pref', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const choresData = await choresRes.json();
    const firstChore = choresData.chores[0];
    expect(firstChore).toBeTruthy();

    const patchRes = await page.request.patch(`/api/chore-reminder-prefs/${firstChore.id}`, {
      data: { enabled: true, leadMinutes: 15 },
      headers: { 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' },
    });
    expect(patchRes.ok()).toBe(true);
    const pref = (await patchRes.json()).pref;
    expect(pref.enabled).toBe(true);
    expect(pref.leadMinutes).toBe(15);
    expect(pref.choreId).toBe(firstChore.id);

    const listRes = await page.request.get('/api/chore-reminder-prefs', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const listData = await listRes.json();
    expect(listData.prefs.length).toBe(1);
    expect(listData.prefs[0].enabled).toBe(true);
  });

  test('per-chore reminder pref persists after reload', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choresRes = await page.request.get('/api/chores', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const firstChore = (await choresRes.json()).chores[0];

    await page.request.patch(`/api/chore-reminder-prefs/${firstChore.id}`, {
      data: { enabled: true, leadMinutes: 15 },
      headers: { 'X-CSRF-Token': csrf, 'Content-Type': 'application/json' },
    });

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const listRes = await page.request.get('/api/chore-reminder-prefs', {
      headers: { 'X-CSRF-Token': csrf },
    });
    const data = await listRes.json();
    const pref = data.prefs.find(p => p.choreId === firstChore.id);
    expect(pref).toBeTruthy();
    expect(pref.enabled).toBe(true);
    expect(pref.leadMinutes).toBe(15);
  });

  test('unauthorized request returns 401', async ({ page }) => {
    const res = await page.request.get('/api/chore-reminder-prefs');
    expect(res.status()).toBe(401);
  });
});
