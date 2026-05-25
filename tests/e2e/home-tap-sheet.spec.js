// tests/e2e/home-tap-sheet.spec.js
// End-to-end tests: tapping any chore on the home grid opens the log sheet
// (with time picker, note, and Cancel button) instead of auto-logging.
//
// Before this change, only chores with indicator labels opened a sheet;
// no-indicator chores logged instantly.  Now all chores open the sheet.

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-hts-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Tap Sheet Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { email, csrf };
}

// ─── Tap Opens Sheet — All Chores ────────────────────────────────────────────

test.describe('Home Tap: Sheet opens for all chores', () => {
  test('tapping a no-indicator chore opens the log sheet', async ({ page }) => {
    await setupWithChores(page);

    // "Feed Cats" has no indicator labels — it should now open a sheet too.
    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();

    await firstCard.click();

    // The home-log sheet should open.
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('.sheet-title')).toContainText(choreName);
  });

  test('no-indicator chore sheet has time picker and note fields', async ({ page }) => {
    await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    await firstCard.click();

    await expect(page.locator('#home-log-note')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#home-log-when')).toBeVisible();

    // Time picker should be pre-filled.
    const value = await page.locator('#home-log-when').inputValue();
    expect(value).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/);
  });

  test('no-indicator chore sheet has no indicator chips', async ({ page }) => {
    await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    await firstCard.click();

    await expect(page.locator('.log-chip')).toHaveCount(0);
  });

  test('indicator chore sheet still shows chips', async ({ page }) => {
    await setupWithChores(page);

    // Find and click Change Baby
    const cards = page.locator('.home-chore-card');
    const count = await cards.count();
    let clicked = false;
    for (let i = 0; i < count; i++) {
      const name = await cards.nth(i).locator('.home-card-name').innerText();
      if (name === 'Change Baby') {
        await cards.nth(i).click();
        clicked = true;
        break;
      }
    }
    expect(clicked).toBe(true);

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('.log-chip')).toHaveCount(2);
  });

  test('Cancel button closes the sheet without logging', async ({ page }) => {
    await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    await firstCard.click();

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Click Cancel button
    await page.locator('button[data-action="close-sheet"]').click();
    await page.waitForTimeout(300);

    // Sheet should be gone
    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
    // No toast should appear
    await expect(page.locator('#toast-container .toast')).toHaveCount(0);
  });
});

// ─── Log via Sheet — No-Indicator Chore ──────────────────────────────────────

test.describe('Home Tap: Log via sheet', () => {
  test('logging a no-indicator chore via sheet shows toast and updates time-ago', async ({ page }) => {
    await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();

    // Before: "never"
    await expect(firstCard.locator('.home-card-time--never')).toBeVisible();

    await firstCard.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Save without any note or time change
    await page.locator('[data-action="save-home-log"]').click();
    await page.waitForTimeout(1500);

    // Sheet gone, toast visible
    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#toast-container .toast')).toContainText(choreName);
    await expect(page.locator('#toast-container .toast button')).toContainText('Undo');

    // Time-ago updated
    await expect(firstCard.locator('.home-card-time--never')).toHaveCount(0);
    await expect(firstCard.locator('.home-card-time')).toContainText(/ago|just now/);
  });

  test('logging with a note and custom time', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    await firstCard.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Fill note
    await page.fill('#home-log-note', 'E2E custom note');

    // Set time to 3 PM today
    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    const dtLocal = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T15:00`;
    await page.fill('#home-log-when', dtLocal);

    await page.locator('[data-action="save-home-log"]').click();
    await page.waitForTimeout(1500);

    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Check that the log was saved with the note
    const latest = (await (await page.request.get('/api/logs/latest-per-chore')).json()).latestLogs;
    const choreIds = Object.keys(latest);
    expect(choreIds.length).toBeGreaterThan(0);
    const log = latest[choreIds[0]];
    expect(log.note).toBe('E2E custom note');
  });

  test('undo button removes the log created via sheet', async ({ page }) => {
    await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    await firstCard.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    await page.locator('[data-action="save-home-log"]').click();
    await page.waitForTimeout(1500);

    // Verify a log was created
    let latest = (await (await page.request.get('/api/logs/latest-per-chore')).json()).latestLogs;
    const choreIds = Object.keys(latest);
    expect(choreIds.length).toBeGreaterThan(0);

    // Click Undo
    const undoBtn = page.locator('#toast-container .toast button');
    await expect(undoBtn).toBeVisible({ timeout: 5000 });
    await undoBtn.click();
    await page.waitForTimeout(1500);

    // Log should be removed
    latest = (await (await page.request.get('/api/logs/latest-per-chore')).json()).latestLogs;
    expect(Object.keys(latest)).toHaveLength(0);
  });
});
