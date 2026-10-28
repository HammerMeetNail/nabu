// tests/e2e/invite-landing.spec.js
// An invite link opened signed out says who sent it and for which household,
// and joins it after sign-up. A dead link says so and leads to setup instead.
// Setup's invite step and Settings show the invite link as a QR code.

import { test, expect } from '@playwright/test';

const uniqueEmail = tag => `e2e-invite-${tag}-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;

async function registerHere(page, email = uniqueEmail('user')) {
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  return email;
}

// An owner named "Dana O'Connor" (purple) with a household, set up through the API.
async function ownerWithHousehold(browser, name = 'Shared home') {
  const context = await browser.newContext({ baseURL: process.env.BASE_URL || 'http://localhost:8080' });
  const page = await context.newPage();
  await page.goto('/register');
  await registerHere(page, uniqueEmail('owner'));
  await expect(page.getByRole('heading', { name: 'Welcome to Nabu' })).toBeVisible({ timeout: 10000 });
  const csrf = (await context.cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  const headers = { 'X-CSRF-Token': csrf };
  expect((await page.request.patch('/api/me', { data: { displayName: "Dana O'Connor", avatarColor: '#7B5EA7' }, headers })).ok()).toBeTruthy();
  expect((await page.request.post('/api/household', { data: { name }, headers })).ok()).toBeTruthy();
  expect((await page.request.post('/api/chores/packs', { data: { packs: [{ key: 'plants' }] }, headers })).ok()).toBeTruthy();
  const { household } = await (await page.request.get('/api/household')).json();
  return { context, page, headers, inviteCode: household.inviteCode };
}

// The path of the QR code the page shows, and of one drawn for `text`.
async function qrPaths(page, scope, text) {
  const shown = await scope.locator('.invite-qr svg path').getAttribute('d');
  const expected = await page.evaluate(async t => {
    const { renderQRCode } = await import('/static/js/qr.js');
    return new DOMParser().parseFromString(renderQRCode(t, ''), 'image/svg+xml').querySelector('path').getAttribute('d');
  }, text);
  return { shown, expected };
}

test.describe('Invite links', () => {
  test('say who sent them and join the household after sign-up', async ({ page, browser }) => {
    const owner = await ownerWithHousehold(browser);
    await page.goto(`/join?code=${owner.inviteCode}`);
    await expect(page.locator('.invite-lead')).toHaveText("Dana O'Connor invited you to join");
    await expect(page.getByRole('heading', { name: 'Shared home' })).toBeVisible();
    await expect(page.locator('.invite-avatar')).toHaveText('D');
    await expect(page.locator('.invite-avatar')).toHaveAttribute('style', 'background:#7B5EA7');

    await page.getByRole('button', { name: 'Create Account' }).click();
    await registerHere(page);
    // Joined: the household's activities, and no setup.
    await expect(page.locator('.home-grid .home-card-name')).toHaveText(['Water Plants'], { timeout: 15000 });
    await expect(page.getByRole('heading', { name: 'Welcome to Nabu' })).toHaveCount(0);
    const { household } = await (await page.request.get('/api/household')).json();
    expect(household.name).toBe('Shared home');
    await owner.context.close();
  });

  test('a single-use link names its maker; once used, or unknown, it says so and leads to setup', async ({ page, browser }) => {
    const owner = await ownerWithHousehold(browser, 'Flat 4');
    const created = await owner.page.request.post('/api/household/invites', { headers: owner.headers });
    expect(created.ok()).toBeTruthy();
    const { invite } = await created.json();

    await page.goto(`/join?code=${invite.code}`);
    await expect(page.locator('.invite-lead')).toHaveText("Dana O'Connor invited you to join");
    await expect(page.getByRole('heading', { name: 'Flat 4' })).toBeVisible();
    await page.getByRole('button', { name: 'Create Account' }).click();
    await registerHere(page);
    await expect(page.locator('.home-grid .home-card-name')).toHaveText(['Water Plants'], { timeout: 15000 });

    // The same link, now used up, in a fresh browser.
    const late = await browser.newContext({ baseURL: process.env.BASE_URL || 'http://localhost:8080' });
    const latePage = await late.newPage();
    await latePage.goto(`/join?code=${invite.code}`);
    await expect(latePage.getByRole('heading', { name: "This invite link isn't valid any more" })).toBeVisible();
    // Signing up anyway leads to setup without trying the dead code.
    const joins = [];
    latePage.on('request', request => {
      if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/household/join') joins.push(request);
    });
    await latePage.getByRole('button', { name: 'Create Account' }).click();
    await registerHere(latePage);
    await expect(latePage.getByRole('heading', { name: 'Welcome to Nabu' })).toBeVisible({ timeout: 10000 });
    await expect(latePage.locator('#onboarding-name')).toBeVisible();
    expect((await (await latePage.request.get('/api/me')).json()).user.householdId).toBeNull();
    expect(joins).toHaveLength(0);
    await late.close();

    const stranger = await browser.newContext({ baseURL: process.env.BASE_URL || 'http://localhost:8080' });
    const strangerPage = await stranger.newPage();
    await strangerPage.goto('/join?code=NOPE234567');
    await expect(strangerPage.getByRole('heading', { name: "This invite link isn't valid any more" })).toBeVisible();
    await stranger.close();
    await owner.context.close();
  });

  test('setup and Settings show the invite link as a QR code', async ({ page }) => {
    await page.goto('/register');
    await registerHere(page);
    await expect(page.getByRole('heading', { name: 'Welcome to Nabu' })).toBeVisible({ timeout: 10000 });
    await page.fill('#onboarding-name', 'Lee');
    await page.getByRole('button', { name: 'Continue' }).click();
    await page.getByRole('button', { name: 'Continue' }).click();
    await page.locator('[data-onboarding-pack="plants"]').check();
    await page.getByRole('button', { name: 'Create household' }).click();
    await expect(page.getByRole('heading', { name: 'Invite your household' })).toBeVisible({ timeout: 15000 });

    const link = await page.locator('.invite-link-url').innerText();
    await expect(page.getByRole('img', { name: 'QR code for the invite link' })).toBeVisible();
    const setup = await qrPaths(page, page.locator('.onboarding'), link);
    expect(setup.shown).toBe(setup.expected);

    await page.getByRole('button', { name: 'Go to Home' }).click();
    await page.click('a[data-nav="settings"]');
    const settings = page.locator('.settings-view');
    await expect(settings.locator('.invite-qr')).toHaveCount(0);
    await settings.getByRole('button', { name: 'QR code' }).click();
    await expect(settings.getByRole('img', { name: 'QR code for the invite link' })).toBeVisible();
    const shown = await qrPaths(page, settings, await settings.locator('.invite-link-row .invite-link-url').first().innerText());
    expect(shown.shown).toBe(shown.expected);
    expect(shown.shown).toBe(setup.shown);
    await settings.getByRole('button', { name: 'Hide QR code' }).click();
    await expect(settings.locator('.invite-qr')).toHaveCount(0);
  });
});
