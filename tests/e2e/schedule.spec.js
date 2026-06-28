// tests/e2e/schedule.spec.js
// End-to-end tests for the schedule API.

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-sched-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers a new user, creates a household, seeds default chores, and waits
 * for the home grid to be visible.  Returns { email, csrf }.
 */
async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  // Wait for registration to complete: the user avatar appears when logged in
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Sched Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { email, csrf };
}

// ─── Schedule API ─────────────────────────────────────────────────────────────

test.describe('Schedule API', () => {
  test('GET /api/schedules returns empty list on fresh account', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.get('/api/schedules');
    expect(resp.status()).toBe(200);
    const data = await resp.json();
    expect(Array.isArray(data.schedules)).toBe(true);
    // No schedules created yet
    expect(data.schedules.length).toBe(0);
  });

  test('POST /api/schedules creates a schedule and returns it', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    const resp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const data = await resp.json();
    expect(data.schedule).toBeDefined();
    expect(data.schedule.choreId).toBe(choreId);
    expect(data.schedule.specificTime).toBe('08:00');
    expect(data.schedule.frequencyType).toBe('daily');
  });

  test('GET /api/schedules lists the newly created schedule', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '18:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    const listResp = await page.request.get('/api/schedules');
    const list     = (await listResp.json()).schedules;
    expect(list.length).toBe(1);
    expect(list[0].choreId).toBe(choreId);
    expect(list[0].specificTime).toBe('18:00');
  });

  test('PATCH /api/schedules/:id updates specificTime', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    const createResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    const scheduleId = (await createResp.json()).schedule.id;

    const patchResp = await page.request.patch(`/api/schedules/${scheduleId}`, {
      data: { specificTime: '21:00' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(patchResp.status()).toBe(200);
    const updated = (await patchResp.json()).schedule;
    expect(updated.specificTime).toBe('21:00');
  });

  test('PATCH /api/schedules/:id updates timePeriod', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    const createResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    const scheduleId = (await createResp.json()).schedule.id;

    const patchResp = await page.request.patch(`/api/schedules/${scheduleId}`, {
      data: { specificTime: '08:30' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(patchResp.status()).toBe(200);
    const updated = (await patchResp.json()).schedule;
    expect(updated.specificTime).toBe('08:30');
  });

  test('DELETE /api/schedules/:id removes the schedule', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    const createResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    const scheduleId = (await createResp.json()).schedule.id;

    const delResp = await page.request.delete(`/api/schedules/${scheduleId}`, {
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(delResp.status()).toBe(200);
    const body = await delResp.json();
    expect(body.status).toBe('deleted');

    // Verify it's gone from the list
    const listResp = await page.request.get('/api/schedules');
    expect((await listResp.json()).schedules.length).toBe(0);
  });

  test('GET /api/schedules/for-date returns active daily schedule', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Get today's date in YYYY-MM-DD
    const today = new Date().toISOString().split('T')[0];
    const resp  = await page.request.get(`/api/schedules/for-date?date=${today}`);
    expect(resp.status()).toBe(200);
    const data = await resp.json();
    expect(Array.isArray(data.schedules)).toBe(true);
    expect(data.schedules.length).toBe(1);
    expect(data.schedules[0].choreId).toBe(choreId);
  });

  test('POST /api/schedules requires choreId', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.post('/api/schedules', {
      data: { timePeriod: 'anytime', frequencyType: 'daily' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(400);
  });

  test('PATCH /api/schedules/:id returns 404 for non-existent id', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.patch('/api/schedules/999999', {
      data: { specificTime: '09:00' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(404);
  });

  test('DELETE /api/schedules/:id returns 404 for non-existent id', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.delete('/api/schedules/999999', {
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(404);
  });
});

// ─── Schedule: Week frequency type ───────────────────────────────────────────

test.describe('Schedule API: recurrence types', () => {
  test('POST /api/schedules with weekly frequency and daysOfWeek', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const resp = await page.request.post('/api/schedules', {
      data: {
        choreId,
        timePeriod: 'anytime',
        frequencyType: 'weekly',
        daysOfWeek: [1, 3, 5], // Mon, Wed, Fri
        isActive: true,
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const sch = (await resp.json()).schedule;
    expect(sch.frequencyType).toBe('weekly');
  });

  test('POST /api/schedules with every_n_days frequency', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const resp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'every_n_days', intervalDays: 3, isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const sch = (await resp.json()).schedule;
    expect(sch.frequencyType).toBe('every_n_days');
  });

  test('POST /api/schedules with monthly_by_date frequency', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const resp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'monthly_by_date', dayOfMonth: 15, isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const sch = (await resp.json()).schedule;
    expect(sch.frequencyType).toBe('monthly_by_date');
  });
});
