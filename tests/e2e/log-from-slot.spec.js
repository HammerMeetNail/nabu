// tests/e2e/log-from-slot.spec.js
// Regression tests for: logging a chore from a specific time slot should
// display the completed chore in that hour row.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-slot-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Slot Log Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { email, csrf };
}

test.describe('Log from time slot', () => {
  test('POST /api/logs with hour field stores slotHour in the response', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const resp = await page.request.post('/api/logs', {
      data: { choreId, note: '', indicators: [], hour: 14 },
      headers: { 'X-CSRF-Token': csrf },
    });

    expect(resp.status()).toBe(201);
    const body = await resp.json();
    expect(body.log).toBeDefined();
    expect(body.log.slotHour).toBe(14);
  });

  test('POST /api/logs without hour field returns slotHour as null', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const resp = await page.request.post('/api/logs', {
      data: { choreId, note: '', indicators: [] },
      headers: { 'X-CSRF-Token': csrf },
    });

    expect(resp.status()).toBe(201);
    const body = await resp.json();
    expect(body.log).toBeDefined();
    expect(body.log.slotHour ?? null).toBeNull();
  });
});
