// tests/e2e/marketing-home.spec.js
// End-to-end tests for the marketing homepage at "/".
//
// "/" serves the marketing page (home.html) to anonymous visitors and the
// SPA app shell to authenticated users, so crawlers and share links land on
// real SEO-visible HTML instead of an empty shell. All server-rendered HTML
// is served with Cache-Control: no-store so every visit reflects the latest
// session state. CTAs on the marketing page point at /login, and /home (the
// legacy marketing URL) declares the canonical URL at the root.

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-marketing-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers a new user, creates a household, seeds default chores, and waits
 * for the home grid to be visible. Returns { email, csrf }.
 */
async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Marketing Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { email, csrf };
}

// ─── Tests ────────────────────────────────────────────────────────────────────

test.describe('Marketing homepage at "/"', () => {
  test('anonymous visitors get server-rendered marketing HTML, not the app shell', async ({ page }) => {
    const resp = await page.goto('/');
    await page.waitForSelector('.hm-hero');

    // Real SEO-visible content, rendered without any client JS state.
    await expect(page.locator('.hm-hero h1')).toContainText('Household activity tracking');
    await expect(page.locator('#app')).toHaveCount(0);

    // Cache policy: server-rendered HTML is never cached, because the root
    // alternates between marketing (anonymous) and the app shell (signed in).
    expect(resp.headers()['cache-control']).toContain('no-store');
    expect((resp.headers()['content-type'] || '').startsWith('text/html')).toBeTruthy();

    // SEO consolidation: canonical + og:url both point at the root.
    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', /\/$/);
    await expect(page.locator('meta[property="og:url"]')).toHaveAttribute('content', /\/$/);

    // Every CTA leads to the login view, not back into the marketing loop.
    await expect(page.locator('.hm-nav-cta')).toHaveAttribute('href', '/login');
    await expect(page.locator('.hm-hero-ctas a.btn-primary')).toHaveAttribute('href', '/login');
    await expect(page.locator('.hm-cta a.btn-primary')).toHaveAttribute('href', '/login');
  });

  test('Open Nabu CTA leads to the login view', async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('.hm-nav-cta');
    await page.click('.hm-nav-cta');
    await page.waitForSelector('#login-form');
  });

  test('authenticated users get the app at "/"', async ({ page }) => {
    await setupWithChores(page);

    // Full page navigation to / with a valid session → app shell, not marketing.
    const resp = await page.goto('/');
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await expect(page.locator('.hm-hero')).toHaveCount(0);
    expect(resp.headers()['cache-control']).toContain('no-store');
  });

  test('after logout, "/" serves the marketing page again', async ({ page }) => {
    await setupWithChores(page);

    // Log out via the profile sheet.
    await page.locator('#hh-indicator').click();
    await expect(page.locator('.profile-panel')).toBeVisible({ timeout: 5000 });
    await page.locator('button[data-action="logout"]').click();
    await expect(page.locator('#login-form')).toBeVisible({ timeout: 5000 });

    // Hard navigation to / with no session → marketing page.
    await page.goto('/');
    await page.waitForSelector('.hm-hero');
    await expect(page.locator('#app')).toHaveCount(0);
  });

  test('legacy /home still serves marketing but declares the canonical URL at the root', async ({ page }) => {
    await page.goto('/home');
    await page.waitForSelector('.hm-hero');
    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', /\/$/);
    await expect(page.locator('meta[property="og:url"]')).toHaveAttribute('content', /\/$/);
  });
});