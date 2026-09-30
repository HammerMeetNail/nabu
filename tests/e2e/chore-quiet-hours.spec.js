// tests/e2e/chore-quiet-hours.spec.js
// Per-task quiet hours, in three scopes:
//   1. Household-level (on the chore itself): silences the chore's reminders
//      for every member during the window.
//   2. Per-user, per-chore (chore_reminder_prefs): silences one member's
//      reminders for that chore.
//   3. Pre-existing global quiet hours (unchanged, regression-guarded).
//
// The behavioral test proves the end goal: a schedule that is due while the
// window is open does NOT produce a delivery record, while a control chore
// due at the same instant does — observable via GET /api/reminders/log.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-quiet-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
}

/**
 * Registers a new user, creates a household, seeds default chores, and waits
 * for the home grid. Returns { csrf }.
 */
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
    data: { name: `Quiet Hours ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { csrf };
}

async function openManageView(page) {
  await page.click('[data-action="switch-home-view"][data-view="manage"]');
  await page.waitForSelector('.chore-list', { timeout: 10000 });
}

async function createChore(page, csrf, name) {
  const res = await page.request.post('/api/chores', {
    data: { name, icon: '🍼', color: '#2E86AB', category: 'custom' },
    headers: { 'X-CSRF-Token': csrf },
  });
  expect(res.status()).toBe(201);
  return (await res.json()).chore;
}

/** "HH:MM" in UTC, offset by minutes (wraps midnight). */
function hhmmOffset(t, deltaMin) {
  return new Date(t.getTime() + deltaMin * 60000).toISOString().slice(11, 16);
}

test.describe('Chore quiet hours', () => {
  test('household quiet hours set in the chore edit sheet persist', async ({ page }) => {
    const { csrf } = await setupWithChores(page);
    // Unique name: the seeded defaults include a predefined "Feed baby",
    // which would otherwise match a substring row locator first.
    const name = `Feed baby ${Date.now()}`;
    const chore = await createChore(page, csrf, name);

    // The chore list renders from state loaded at page load; refresh so the
    // API-created chore is present in the Manage view.
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    await openManageView(page);
    const row = page.locator('.chore-row', { hasText: name });
    await row.locator('[data-action="chore-edit"]').click();
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#chore-quiet-start')).toBeVisible();
    await expect(page.locator('#chore-quiet-end')).toBeVisible();

    await page.fill('#chore-quiet-start', '22:00');
    await page.fill('#chore-quiet-end', '07:00');
    const patchDone = page.waitForResponse(
      r => r.url().includes('/api/chores/') && r.request().method() === 'PATCH'
    );
    await page.click('[data-action="save-chore"]');
    await patchDone;
    await expect(page.locator('.chore-edit-sheet')).toHaveCount(0, { timeout: 5000 });

    // The API reflects the values.
    const res = await page.request.get('/api/chores');
    const saved = (await res.json()).chores.find(c => c.id === chore.id);
    expect(saved.quietHoursStart).toBe('22:00');
    expect(saved.quietHoursEnd).toBe('07:00');

    // Persisted: the sheet is pre-filled after a reload.
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await openManageView(page);
    await page
      .locator('.chore-row', { hasText: name })
      .locator('[data-action="chore-edit"]')
      .click();
    await expect(page.locator('#chore-quiet-start')).toHaveValue('22:00', { timeout: 5000 });
    await expect(page.locator('#chore-quiet-end')).toHaveValue('07:00');
  });

  test('per-user, per-chore quiet hours persist and reject bad values', async ({ page }) => {
    const { csrf } = await setupWithChores(page);
    const chore = await createChore(page, csrf, 'Meds');
    const headers = { 'X-CSRF-Token': csrf };

    const patch = await page.request.patch(`/api/chore-reminder-prefs/${chore.id}`, {
      data: { enabled: true, leadMinutes: 10, quietHoursStart: '23:00', quietHoursEnd: '05:00' },
      headers,
    });
    expect(patch.status()).toBe(200);

    const list = await page.request.get('/api/chore-reminder-prefs', { headers });
    const pref = (await list.json()).prefs.find(p => p.choreId === chore.id);
    expect(pref.quietHoursStart).toBe('23:00');
    expect(pref.quietHoursEnd).toBe('05:00');

    // Malformed times are rejected.
    const bad = await page.request.patch(`/api/chore-reminder-prefs/${chore.id}`, {
      data: { quietHoursStart: '9am' },
      headers,
    });
    expect(bad.status()).toBe(400);

    // A partial update preserves the other bound.
    const partial = await page.request.patch(`/api/chore-reminder-prefs/${chore.id}`, {
      data: { quietHoursStart: '22:00' },
      headers,
    });
    expect(partial.status()).toBe(200);
    const afterPartial = (await partial.json()).pref;
    expect(afterPartial.quietHoursStart).toBe('22:00');
    expect(afterPartial.quietHoursEnd).toBe('05:00');

    // Explicit empty strings clear the window.
    const clear = await page.request.patch(`/api/chore-reminder-prefs/${chore.id}`, {
      data: { quietHoursStart: '', quietHoursEnd: '' },
      headers,
    });
    expect(clear.status()).toBe(200);
    const afterClear = (await clear.json()).pref;
    expect(afterClear.quietHoursStart ?? '').toBe('');
    expect(afterClear.quietHoursEnd ?? '').toBe('');
  });

  test('invalid household quiet hours are rejected on create and update', async ({ page }) => {
    const { csrf } = await setupWithChores(page);
    const headers = { 'X-CSRF-Token': csrf };

    const badCreate = await page.request.post('/api/chores', {
      data: { name: 'Nap', icon: '😴', color: '#2E86AB', quietHoursStart: '25:99', quietHoursEnd: '07:00' },
      headers,
    });
    expect(badCreate.status()).toBe(400);

    const chore = await createChore(page, csrf, 'Nap');
    const badUpdate = await page.request.patch(`/api/chores/${chore.id}`, {
      data: { quietHoursStart: '25:99', quietHoursEnd: '07:00' },
      headers,
    });
    expect(badUpdate.status()).toBe(400);

    // A partial window (start without end) is rejected on create.
    const partial = await page.request.post('/api/chores', {
      data: { name: 'Snack', icon: '🍌', color: '#2E86AB', quietHoursStart: '22:00' },
      headers,
    });
    expect(partial.status()).toBe(400);
  });

  test('a due reminder inside a chore quiet window is suppressed; a control is delivered', async ({ page }) => {
    // The scheduler ticks every 30s; allow up to 4 minutes for one tick cycle.
    test.setTimeout(300_000);
    const { csrf } = await setupWithChores(page);
    const headers = { 'X-CSRF-Token': csrf };

    // The PWA saves the browser's IANA zone to user_preferences.timezone at
    // load, and the scheduler computes due dates in that zone when
    // reminder_preferences.timezone is the 'UTC' default. Clear the user
    // zone so the effective zone is UTC, matching the container clock and
    // the quiet-window evaluation (which uses reminder_preferences.timezone).
    const tzRes = await page.request.patch('/api/preferences', {
      data: { timezone: '' },
      headers,
    });
    expect(tzRes.status()).toBe(200);

    const t = new Date();
    const nowHM = t.toISOString().slice(11, 16);
    const winStart = hhmmOffset(t, -60);
    const winEnd = hhmmOffset(t, 60);

    const control = await createChore(page, csrf, 'Control feed');
    const householdQuiet = await createChore(page, csrf, 'Household quiet feed');
    const userQuiet = await createChore(page, csrf, 'My quiet feed');

    // Household-level window on one chore, per-user window on another, both
    // covering "now". The control has no quiet hours anywhere.
    const hhRes = await page.request.patch(`/api/chores/${householdQuiet.id}`, {
      data: { quietHoursStart: winStart, quietHoursEnd: winEnd },
      headers,
    });
    expect(hhRes.status()).toBe(200);
    // Unassigned schedules are only candidate reminders when the per-chore
    // pref is enabled. Without this, householdQuiet would never reach the
    // quiet-hours check and its assertion would pass vacuously.
    const hhPref = await page.request.patch(`/api/chore-reminder-prefs/${householdQuiet.id}`, {
      data: { enabled: true, leadMinutes: 10 },
      headers,
    });
    expect(hhPref.status()).toBe(200);
    const uqRes = await page.request.patch(`/api/chore-reminder-prefs/${userQuiet.id}`, {
      data: { enabled: true, leadMinutes: 10, quietHoursStart: winStart, quietHoursEnd: winEnd },
      headers,
    });
    expect(uqRes.status()).toBe(200);
    // Unassigned schedules are only candidate reminders when the per-chore
    // pref is enabled — enable it for the control too.
    const ctlRes = await page.request.patch(`/api/chore-reminder-prefs/${control.id}`, {
      data: { enabled: true, leadMinutes: 10 },
      headers,
    });
    expect(ctlRes.status()).toBe(200);

    // Three daily schedules due at "now" (lead window: now-10m..now+15m).
    for (const c of [control, householdQuiet, userQuiet]) {
      const res = await page.request.post('/api/schedules', {
        data: { choreId: c.id, frequencyType: 'daily', specificTime: nowHM, timePeriod: 'anytime' },
        headers,
      });
      expect(res.status()).toBe(201);
    }

    // Wait for the control's delivery record (push is a no-op without a
    // subscription, but RecordReminder still runs).
    await expect
      .poll(
        async () => {
          const res = await page.request.get('/api/reminders/log', { headers });
          const { reminders } = await res.json();
          return reminders.filter(r => r.choreId === control.id).length;
        },
        { timeout: 180_000, intervals: [5000] }
      )
      .toBe(1);

    // The suppressed chores have no delivery record.
    const res = await page.request.get('/api/reminders/log', { headers });
    const { reminders } = await res.json();
    expect(reminders.some(r => r.choreId === householdQuiet.id)).toBe(false);
    expect(reminders.some(r => r.choreId === userQuiet.id)).toBe(false);
  });
});
