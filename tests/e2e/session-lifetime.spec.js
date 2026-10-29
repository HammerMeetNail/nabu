// tests/e2e/session-lifetime.spec.js
// Signing in keeps you signed in for a year: the browser holds the session
// cookie until the server's session would end, not for 30 days. Signing in
// again issues a new one-year cookie, and signing out clears it.

import { test, expect } from '@playwright/test';

const YEAR_SECONDS = 365 * 24 * 60 * 60;

async function sessionCookie(page) {
  return (await page.context().cookies()).find(c => c.name === 'nabu_session');
}

function expectLastsAYear(cookie, before) {
  expect(cookie, 'session cookie').toBeTruthy();
  // `expires` is in seconds. A 30-day cookie would be 335 days short.
  expect(cookie.expires).toBeGreaterThanOrEqual(before + YEAR_SECONDS - 5);
  expect(cookie.expires).toBeLessThanOrEqual(Date.now() / 1000 + YEAR_SECONDS + 5);
}

test.describe('Session lifetime', () => {
  test('sign-up and sign-in keep you signed in for a year, and sign-out ends it', async ({ page }) => {
    const email = `e2e-session-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
    await page.goto('/register');
    await page.waitForSelector('#register-form');
    await page.fill('#reg-email', email);
    await page.fill('#reg-password', 'test123456');
    let before = Math.floor(Date.now() / 1000);
    await page.click('button[type="submit"]');
    await page.waitForSelector('#hh-indicator:not([hidden]), .onboarding', { timeout: 10000 });
    expectLastsAYear(await sessionCookie(page), before);

    const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
    const logout = await page.request.post('/api/auth/logout', { headers: { 'X-CSRF-Token': csrf } });
    expect(logout.ok()).toBe(true);
    expect(await sessionCookie(page)).toBeUndefined();

    await page.goto('/login');
    await page.waitForSelector('#login-form');
    await page.fill('#login-email', email);
    await page.fill('#login-password', 'test123456');
    before = Math.floor(Date.now() / 1000);
    await page.click('#login-form button[type="submit"]');
    await page.waitForSelector('#hh-indicator:not([hidden]), .onboarding', { timeout: 10000 });
    expectLastsAYear(await sessionCookie(page), before);
  });
});
