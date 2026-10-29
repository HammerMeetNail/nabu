// tests/e2e/settings-my-data-export.spec.js
// Settings → Account → "Download my data" gives the signed-in user one JSON
// file of everything Nabu keeps about them, without starting a deletion. It
// holds no session or CSRF token, and works for someone in no household.

import { test, expect } from '@playwright/test';
import fs from 'node:fs';
import { deferred } from './review-fixtures.js';

test.use({ serviceWorkers: 'block' });

function uniqueEmail() {
  return `e2e-md-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function register(page) {
  const email = uniqueEmail();
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  return { email, csrf };
}

async function openSettings(page) {
  await page.click('a[data-nav="settings"]');
  await page.waitForSelector('.settings-view');
}

async function download(page, testId) {
  const [file] = await Promise.all([
    page.waitForEvent('download'),
    page.getByTestId(testId).click(),
  ]);
  return { name: file.suggestedFilename(), text: fs.readFileSync(await file.path(), 'utf8') };
}

test('Settings downloads everything about the user as JSON', async ({ page }) => {
  const { email, csrf } = await register(page);
  const household = `My data ${Date.now()}`;
  await page.request.post('/api/household', { data: { name: household }, headers: { 'X-CSRF-Token': csrf } });
  await page.request.post('/api/chores/seed-defaults', { headers: { 'X-CSRF-Token': csrf } });
  const { chores } = await (await page.request.get('/api/chores')).json();
  const note = `md-${Date.now()}`;
  const log = await page.request.post('/api/logs', {
    data: { choreId: chores[0].id, note, completedAt: new Date().toISOString(), hour: new Date().getHours() },
    headers: { 'X-CSRF-Token': csrf },
  });
  expect(log.ok()).toBe(true);
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  await openSettings(page);
  const { name, text } = await download(page, 'account-download-my-data');
  expect(name).toBe('nabu-my-data.json');
  const data = JSON.parse(text);
  expect(data.account.email).toBe(email);
  expect(data.households).toEqual([expect.objectContaining({ name: household, role: 'owner', suspended: false })]);
  expect(data.logs).toEqual([expect.objectContaining({ note, household, activity: chores[0].name })]);
  expect(data.signIns.filter(s => s.current)).toHaveLength(1);
  for (const cookie of await page.context().cookies()) {
    if (cookie.name.startsWith('nabu_')) expect(text).not.toContain(cookie.value);
  }
});

test('someone in no household can still download their data', async ({ page }) => {
  const { email } = await register(page);
  await openSettings(page);
  const data = JSON.parse((await download(page, 'account-download-my-data')).text);
  expect(data.account.email).toBe(email);
  expect(data.households).toEqual([]);
  expect(data.logs).toEqual([]);

  // The logs CSV used to answer 401 "no household" here.
  await page.click('[data-action="open-delete-account"]');
  await expect(page.getByTestId('delete-account-confirm')).toBeVisible();
  const csv = await download(page, 'download-my-logs');
  expect(csv.name).toBe('nabu-my-logs.csv');
  expect(csv.text).toMatch(/^household,date,time,chore,/);
});

test('a failed download says why and saves nothing; Cancel stops one', async ({ page }) => {
  await register(page);
  await openSettings(page);
  const downloads = [];
  page.on('download', file => downloads.push(file));
  const button = page.getByTestId('account-download-my-data');

  const message = 'an export is already running; try again shortly';
  await page.route('**/api/me/export', route => route.fulfill({ status: 429, contentType: 'application/json', body: JSON.stringify({ error: message }) }), { times: 1 });
  await button.click();
  await expect(page.getByTestId('my-data-error')).toHaveText(message);
  await expect(button).toBeEnabled();

  // Keep the route until its callback finishes, as export-recovery.spec.js does.
  const entered = deferred(), release = deferred(), finished = deferred();
  await page.route('**/api/me/export', async route => {
    entered.resolve(); await release.promise;
    try { await route.fulfill({ status: 200, contentType: 'application/json', body: '{"late":true}' }); } catch { /* aborted */ } finally { finished.resolve(); }
  });
  await button.click(); await entered.promise;
  await expect(button).toBeDisabled();
  await expect(page.getByTestId('my-data-error')).toHaveCount(0);
  await page.click('[data-action="cancel-my-data"]');
  await expect(button).toBeEnabled();
  release.resolve(); await finished.promise;
  await page.waitForTimeout(300); // a late body has no DOM signal to wait on
  expect(downloads).toHaveLength(0);
});
