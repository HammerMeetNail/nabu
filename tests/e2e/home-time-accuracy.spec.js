// tests/e2e/home-time-accuracy.spec.js
// Regression test: home-tab log must store completedAt as the current
// time, not noon UTC.  The sheet's datetime-local input is pre-filled with
// the current time, so saving immediately should produce an accurate
// completedAt.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-hta-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#user-avatar:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `HTAcc Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  const chores = (await (await page.request.get('/api/chores')).json()).chores || [];

  return { csrf, chores };
}

test.describe('Home tab: completedAt accuracy', () => {
  test('log via sheet stores completedAt at the current time, not noon UTC', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    expect(chore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);

    const beforeMs = Date.now();

    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const afterMs = Date.now();

    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    const log = latestLogs[chore.id];
    expect(log).toBeDefined();
    const completedAtMs = new Date(log.completedAt).getTime();

    const tolerance = 200000; // 200s covers 5-min rounding + test overhead
    expect(completedAtMs).toBeGreaterThanOrEqual(beforeMs - tolerance);
    expect(completedAtMs).toBeLessThanOrEqual(afterMs + tolerance);
  });

  test('stored completedAt survives page reload and shows consistent time ago', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    expect(chore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);

    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const reloadCard = page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    const reloadTimeText = await reloadCard.locator('.home-card-time').innerText();
    expect(reloadTimeText).toMatch(/^(just now|\d+m ago)$/);
  });
});
