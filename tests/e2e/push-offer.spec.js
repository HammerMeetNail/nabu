// tests/e2e/push-offer.spec.js
// Notifications are offered on Home after someone's first log, never at
// sign-up. The offer needs Web Push, so this spec runs its own servers: one
// with VAPID keys (as production has) and one without.

import { test, expect } from '@playwright/test';
import { execFile, spawn } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, rm } from 'node:fs/promises';
import { openSync, closeSync } from 'node:fs';
import { generateKeyPairSync } from 'node:crypto';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createServer } from 'node:net';

test.describe.configure({ mode: 'serial' });
let artifacts, binary;
test.beforeAll(async () => {
  test.setTimeout(180000);
  artifacts = await mkdtemp(join(tmpdir(), 'nabu-push-offer-e2e-'));
  binary = join(artifacts, 'server');
  await promisify(execFile)('go', ['build', '-o', binary, './cmd/server'], { timeout: 150000 });
});
test.afterAll(async () => { if (artifacts) await rm(artifacts, { recursive: true, force: true }); });

// A P-256 pair in the server's format: the raw private scalar and the
// uncompressed public point, both base64url.
function vapidKeys() {
  const jwk = generateKeyPairSync('ec', { namedCurve: 'P-256' }).privateKey.export({ format: 'jwk' });
  const point = Buffer.concat([Buffer.from([4]), Buffer.from(jwk.x, 'base64url'), Buffer.from(jwk.y, 'base64url')]);
  return { VAPID_PRIVATE_KEY: jwk.d, VAPID_PUBLIC_KEY: point.toString('base64url'), VAPID_SUBJECT: 'mailto:e2e@nabu.local' };
}

async function startServer(request, env) {
  const reservation = createServer();
  await new Promise(resolve => reservation.listen(0, '127.0.0.1', resolve));
  const port = reservation.address().port;
  await new Promise(resolve => reservation.close(resolve));
  const url = `http://127.0.0.1:${port}`;
  const fd = openSync(join(artifacts, `server-${port}.log`), 'w', 0o600);
  const child = spawn(binary, [], { stdio: ['ignore', fd, fd], env: { ...process.env,
    PORT: String(port), APP_ENV: 'development', APP_BASE_URL: url, SERVER_SECURE: 'false', DATABASE_URL: '',
    SMTP_HOST: '', SMTP_USER: '', SMTP_PASS: '', VAPID_PRIVATE_KEY: '', VAPID_PUBLIC_KEY: '', APNS_AUTH_KEY_P8: '',
    RATE_LIMIT_AUTH_MAX: '1000', RATE_LIMIT_JOIN_MAX: '1000', ...env } });
  closeSync(fd);
  await expect.poll(async () => {
    if (child.exitCode !== null) return -1;
    try { return (await request.get(`${url}/ready`, { timeout: 1000 })).status(); } catch { return 0; }
  }).toBe(200);
  return { url, stop: () => child.kill() };
}

// A browser that hasn't decided on notifications, as on a first visit
// (headless Chromium denies them outright). Requests are counted here, across
// page loads, and not answered.
async function undecidedBrowser(page) {
  const requests = { count: 0 };
  await page.exposeFunction('__permissionRequested', () => { requests.count += 1; });
  await page.addInitScript(() => {
    if (typeof Notification !== 'undefined') {
      Object.defineProperty(Notification, 'permission', { configurable: true, get: () => 'default' });
      Notification.requestPermission = () => { window.__permissionRequested(); return Promise.resolve('default'); };
    }
  });
  return requests;
}

// A new account with a Water Plants household, on Home.
async function signUpWithPlants(page, url) {
  const email = `e2e-push-offer-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
  await page.goto(`${url}/register`);
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await expect(page.getByRole('heading', { name: 'Welcome to Nabu' })).toBeVisible({ timeout: 10000 });
  const csrf = (await page.context().cookies(url)).find(c => c.name === 'nabu_csrf')?.value || '';
  const headers = { 'X-CSRF-Token': csrf };
  expect((await page.request.post(`${url}/api/household`, { data: { name: 'Home' }, headers })).ok()).toBeTruthy();
  expect((await page.request.post(`${url}/api/chores/packs`, { data: { packs: [{ key: 'plants' }] }, headers })).ok()).toBeTruthy();
  await page.goto(`${url}/`);
  await expect(page.locator('.home-chore-card', { hasText: 'Water Plants' })).toBeVisible({ timeout: 10000 });
}

async function logWaterPlants(page) {
  await page.locator('.home-chore-card', { hasText: 'Water Plants' }).click();
  await page.locator('[data-action="save-log"]').click();
  await expect(page.locator('#toast-container .toast').first()).toBeVisible();
}

test.describe('Notifications offer', () => {
  test('signing up asks nothing; the first log offers it, and Not now keeps it away', async ({ page, request }) => {
    const server = await startServer(request, vapidKeys());
    try {
      const requests = await undecidedBrowser(page);
      await signUpWithPlants(page, server.url);
      expect(requests.count, 'signing up asks nothing').toBe(0);
      await expect(page.locator('.push-offer')).toHaveCount(0);

      await logWaterPlants(page);
      const offer = page.getByRole('region', { name: 'Get reminders on this device?' });
      await expect(offer).toBeVisible();
      // The undo toast still offers Undo alongside it.
      await expect(page.locator('#toast-container [data-undo-toast]')).toBeVisible();
      await offer.getByRole('button', { name: 'Not now' }).click();
      await expect(page.locator('.push-offer')).toHaveCount(0);

      await page.reload();
      await expect(page.locator('.home-chore-card', { hasText: 'Water Plants' })).toBeVisible();
      await logWaterPlants(page);
      await expect(page.locator('.push-offer')).toHaveCount(0);
      expect(requests.count).toBe(0);
    } finally {
      server.stop();
    }
  });

  test('Turn on asks the browser once, from the tap', async ({ page, request }) => {
    const server = await startServer(request, vapidKeys());
    try {
      const requests = await undecidedBrowser(page);
      await signUpWithPlants(page, server.url);
      await logWaterPlants(page);
      await page.getByRole('button', { name: 'Turn on' }).click();
      await expect(page.locator('.push-offer')).toHaveCount(0);
      await expect.poll(() => requests.count).toBe(1);
      await page.reload();
      await expect(page.locator('.home-chore-card', { hasText: 'Water Plants' })).toBeVisible();
      await expect(page.locator('.push-offer')).toHaveCount(0);
    } finally {
      server.stop();
    }
  });

  test('nothing is offered where the server cannot send Web Push', async ({ page, request }) => {
    const server = await startServer(request, {});
    try {
      await undecidedBrowser(page);
      await signUpWithPlants(page, server.url);
      await logWaterPlants(page);
      await expect(page.locator('#toast-container [data-undo-toast]')).toBeVisible();
      await expect(page.locator('.push-offer')).toHaveCount(0);
    } finally {
      server.stop();
    }
  });
});
