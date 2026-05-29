// tests/e2e/home-when-picker.spec.js
// Regression: when logging a chore from the home tab, the time selected in
// the when picker must be the time submitted — not the current time.
//
// This covers two scenarios:
//  1. The picker is pre-filled with the current time (with minutes); saving
//     immediately must use that pre-filled time.
//  2. The user changes the picker to a different hour; the submitted
//     completedAt and slotHour must reflect the chosen time.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-hwp-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `HWPick Test ${Date.now()}` },
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

test.describe('Home tab: when picker time accuracy', () => {
  test('pre-filled when picker time matches submitted completedAt', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    expect(chore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    const beforeMs = Date.now();

    await card.click();
    await expect(page.locator('#log-when')).toBeVisible({ timeout: 5000 });

    // Read the pre-filled value from the when input.
    const whenVal = await page.locator('#log-when').inputValue();

    await page.locator('[data-action="save-log"]').click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const afterMs = Date.now();

    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    const log = latestLogs[chore.id];
    expect(log).toBeDefined();

    const completedAtMs = new Date(log.completedAt).getTime();

    // The completedAt should match the when input exactly, which was
    // pre-filled with the current local time (truncated to minutes).
    const expectedMs = new Date(whenVal).getTime();
    const tolerance = 200000; // 200s covers 5-min rounding + test overhead
    expect(completedAtMs).toBeGreaterThanOrEqual(expectedMs - tolerance);
    expect(completedAtMs).toBeLessThanOrEqual(expectedMs + tolerance);

    // It should also be close to now (the picker was pre-filled with current time).
    expect(completedAtMs).toBeGreaterThanOrEqual(beforeMs - tolerance);
    expect(completedAtMs).toBeLessThanOrEqual(afterMs + tolerance);
  });

  test('changing the when picker submits the selected time, not current time', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    expect(chore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    await card.click();
    await expect(page.locator('#log-when')).toBeVisible({ timeout: 5000 });

    // Pick a time that is different from the current hour so the comparison
    // exposes any fallback-to-current-time bug.
    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    // Choose 8 AM on today's date — reliably different from the current hour
    // during normal working hours.
    const chosenVal = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T08:00`;
    await page.fill('#log-when', chosenVal);

    await page.locator('[data-action="save-log"]').click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    const log = latestLogs[chore.id];
    expect(log).toBeDefined();

    // completedAt should match 8 AM today, not the current time.
    const expectedMs = new Date(chosenVal).getTime();
    const completedAtMs = new Date(log.completedAt).getTime();
    expect(completedAtMs).toBe(expectedMs);

    // slotHour should be 8.
    expect(log.slotHour).toBe(8);
  });

  test('changing the when picker minutes within same hour submits the chosen time', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    expect(chore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    await card.click();
    await expect(page.locator('#log-when')).toBeVisible({ timeout: 5000 });

    // Pick the current hour but at :30 — the old comparison logic only checked
    // the hour, so minute-only changes were silently ignored.
    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    const chosenVal = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T${pad(now.getHours())}:30`;
    await page.fill('#log-when', chosenVal);

    await page.locator('[data-action="save-log"]').click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    const log = latestLogs[chore.id];
    expect(log).toBeDefined();

    const expectedMs = new Date(chosenVal).getTime();
    const completedAtMs = new Date(log.completedAt).getTime();
    expect(completedAtMs).toBe(expectedMs);
  });
});
