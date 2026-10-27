// tests/e2e/home-log-latest.spec.js
// A save from Home updates its tile from the save's own answer (the chore's
// latest log) instead of reloading every chore's latest log and today's logs.
// A backdated save keeps the newer time, undo restores the previous state, and
// an answer without "latest" (an older server) still reloads.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-hll-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', { data: { name: `HLL ${Date.now()}` }, headers: { 'X-CSRF-Token': csrf } });
  await page.request.post('/api/chores/seed-defaults', { headers: { 'X-CSRF-Token': csrf } });
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  const chores = (await (await page.request.get('/api/chores')).json()).chores || [];
  return { chores };
}

// GET reads of the log collections a save used to trigger.
function recordLogReloads(page) {
  const reads = [];
  page.on('request', request => {
    const path = new URL(request.url()).pathname;
    if (request.method() === 'GET' && (path === '/api/logs/latest-per-chore' || path === '/api/logs/today')) reads.push(path);
  });
  return reads;
}

const card = (page, chore) => page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
const recent = /^(just now|\d+m ago)$/;

async function saveFromHome(page, chore, when = null) {
  await card(page, chore).click();
  await expect(page.locator('.bottom-sheet')).toBeVisible();
  if (when) {
    const local = await page.evaluate(ms => {
      const d = new Date(ms), pad = n => String(n).padStart(2, '0');
      return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    }, when.getTime());
    await page.fill('#log-when', local);
  }
  const saved = page.waitForResponse(r => new URL(r.url()).pathname === '/api/logs' && r.request().method() === 'POST');
  await page.click('[data-action="save-log"]');
  const response = await saved;
  await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  return response.json();
}

test.describe('Home saves use their own answer', () => {
  test('a save updates its tile without reloading the logs', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    const reloads = recordLogReloads(page);
    const body = await saveFromHome(page, chore);
    expect(body.latest.id).toBe(body.log.id);
    await expect(card(page, chore).locator('.home-card-time')).toHaveText(recent);
    await expect(page.locator('#toast-container .toast')).toBeVisible();
    expect(reloads).toEqual([]);
  });

  test('a backdated save keeps the newer time on the tile', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    await saveFromHome(page, chore);
    await expect(card(page, chore).locator('.home-card-time')).toHaveText(recent);
    const body = await saveFromHome(page, chore, new Date(Date.now() - 3 * 3600_000));
    expect(body.latest.id).not.toBe(body.log.id);
    await expect(card(page, chore).locator('.home-card-time')).toHaveText(recent);
    await page.reload();
    await page.waitForSelector('.home-grid');
    await expect(card(page, chore).locator('.home-card-time')).toHaveText(recent);
  });

  test('undo restores the tile from the delete answer', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    await saveFromHome(page, chore);
    await expect(card(page, chore).locator('.home-card-time')).toHaveText(recent);
    const reloads = recordLogReloads(page);
    const deleted = page.waitForResponse(r => r.request().method() === 'DELETE');
    await page.locator('#toast-container .toast').getByRole('button', { name: 'Undo' }).click();
    expect((await (await deleted).json()).latest).toBeNull();
    // Never done again.
    await expect(card(page, chore).locator('.home-card-time')).toHaveText('never');
    expect(reloads).toEqual([]);
  });

  test('an answer without the latest log still reloads', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    // An older server answers with the log only.
    await page.route('**/api/logs', async route => {
      if (route.request().method() !== 'POST') return route.continue();
      const response = await route.fetch();
      const { log } = await response.json();
      await route.fulfill({ response, json: { log } });
    });
    const reloads = recordLogReloads(page);
    await saveFromHome(page, chore);
    await expect(card(page, chore).locator('.home-card-time')).toHaveText(recent);
    expect(reloads).toContain('/api/logs/latest-per-chore');
  });
});
