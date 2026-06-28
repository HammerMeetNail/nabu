// tests/e2e/chores.spec.js
// End-to-end tests for:
//   • Default seeded chore validation
//   • Chore repeatability — same chore can be scheduled multiple times
//   • Chore CRUD via API

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-chores-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers, creates household, seeds default chores, waits for history view.
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

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Chores Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.click('[data-nav="activity"]');
  await page.waitForSelector('.history-view', { timeout: 15000 });

  return { email, csrf };
}

// ─── Default chores: no time-of-day labels ────────────────────────────────────

test.describe('Default chores: action-specific names', () => {
  test('default chores do not contain "(Morning)" or "(Evening)" labels', async ({ page }) => {
    await setupWithChores(page);

    const resp = await page.request.get('/api/chores');
    expect(resp.status()).toBe(200);
    const { chores } = await resp.json();

    for (const chore of chores) {
      expect(chore.name).not.toMatch(/\(morning\)/i);
      expect(chore.name).not.toMatch(/\(evening\)/i);
      expect(chore.name).not.toMatch(/\(night\)/i);
      expect(chore.name).not.toMatch(/\(afternoon\)/i);
    }
  });

  test('seeded default chores include "Feed Cats" (single entry, no duplicates)', async ({ page }) => {
    await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    const feedCats = chores.filter(c => c.name === 'Feed Cats');
    expect(feedCats).toHaveLength(1);
  });

  test('seeded default chores do not include the cat-specific custom chores', async ({ page }) => {
    await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    const names = chores.map(c => c.name);
    expect(names).not.toContain('Feed Orange Cat');
    expect(names).not.toContain('Feed Black Cat');
    expect(names).not.toContain('Feed Mongo');
    expect(names).not.toContain('Feed Roger');
    expect(names).not.toContain('Cat Wipe');
    expect(names).not.toContain('Cat Pumpkin');
  });

  test('seeded default chores list has 15 items', async ({ page }) => {
    await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    expect(chores).toHaveLength(15);
  });

  test('Feed Mongo, Feed Roger, Cat Wipe, and Cat Pumpkin can be added as custom chores', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    for (const name of ['Feed Mongo', 'Feed Roger', 'Cat Wipe', 'Cat Pumpkin']) {
      const resp = await page.request.post('/api/chores', {
        data: { name },
        headers: { 'X-CSRF-Token': csrf },
      });
      expect(resp.status()).toBe(201);
      const { chore } = await resp.json();
      expect(chore.name).toBe(name);
      expect(chore.isPredefined).toBe(false);
    }
  });

  test('same chore can be added to a period more than once', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    // Add the same chore to morning twice
    const r1 = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(r1.status()).toBe(201);

    const r2 = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(r2.status()).toBe(201);

    const schedules = (await (await page.request.get('/api/schedules')).json()).schedules;
    const morningOccurrences = schedules.filter(s => s.choreId === choreId && s.timePeriod === 'morning');
    expect(morningOccurrences.length).toBe(2);
  });
});

// ─── Chore API ────────────────────────────────────────────────────────────────

test.describe('Chore API', () => {
  test('POST /api/chores creates a custom chore', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.post('/api/chores', {
      data: { name: 'Test Custom Chore', icon: '🧪', color: '#FF0000', category: 'custom' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const body = await resp.json();
    expect(body.chore).toBeDefined();
    expect(body.chore.name).toBe('Test Custom Chore');
    expect(body.chore.isPredefined).toBe(false);
  });

  test('POST /api/chores uses default icon and color when omitted', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.post('/api/chores', {
      data: { name: 'Minimal Chore' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const { chore } = await resp.json();
    expect(chore.icon).toBeTruthy();
    expect(chore.color).toBeTruthy();
  });

  test('GET /api/chores lists custom chore after creation', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.request.post('/api/chores', {
      data: { name: 'Listed Chore' },
      headers: { 'X-CSRF-Token': csrf },
    });

    const { chores } = await (await page.request.get('/api/chores')).json();
    expect(chores.some(c => c.name === 'Listed Chore')).toBe(true);
  });

  test('POST /api/chores then POST /api/schedules wires them together', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const createResp = await page.request.post('/api/chores', {
      data: { name: 'Wired Chore' },
      headers: { 'X-CSRF-Token': csrf },
    });
    const choreId = (await createResp.json()).chore.id;

    const schedResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(schedResp.status()).toBe(201);
    const sch = (await schedResp.json()).schedule;
    expect(sch.choreId).toBe(choreId);
    expect(sch.isActive).toBe(true);
  });
});
