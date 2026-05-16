// tests/e2e/log-from-slot.spec.js
// Regression tests for: logging a chore from a specific time slot should
// display the completed chore in that hour row.
//
// Bug: when a user opened the 2 PM pick-chore sheet, long-pressed a chore, and
// chose "Log", the chore appeared in the wrong place because:
//   1. slotHour was never forwarded to the API
//   2. The calendar only placed chores via schedules, ignoring slotHour on logs

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

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
  await page.waitForSelector('#user-avatar:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Slot Log Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.cal-date', { timeout: 15000 });

  return { email, csrf };
}

/**
 * Simulates a long-press (≥500 ms) on a locator by holding mousedown, then
 * releasing.  Matches the 500 ms threshold in app.js.  Also dispatches a
 * synthetic click to consume the `longPressJustFired` guard so subsequent
 * clicks on sheet buttons are not accidentally suppressed.
 */
async function longPress(page, locator) {
  await locator.scrollIntoViewIfNeeded();
  const box = await locator.boundingBox();
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.waitForTimeout(650); // just over the 500 ms threshold
  await page.mouse.up();
  // Consume the longPressJustFired guard so the next real click works.
  await page.evaluate(([cx, cy]) => {
    const el = document.elementFromPoint(cx, cy);
    if (el && el.dataset.action !== 'close-sheet') {
      el.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
    }
  }, [x, y]);
  await page.waitForTimeout(50);
}

// ─── Tests ────────────────────────────────────────────────────────────────────

test.describe('Log from time slot', () => {
  test('long-pressing a chore in the 2 PM sheet and logging it places the card in the 2 PM row', async ({ page }) => {
    await setupWithChores(page);

    // Open the 2 PM (hour 14) pick-chore sheet.
    await page.locator('[data-drop-hour="14"]').click();
    await page.waitForTimeout(400);
    await expect(page.locator('.bottom-sheet')).toBeVisible();

    // Long-press the first chore item to get the log sheet with notes.
    const choreItem = page.locator('.sheet-chore-item').first();
    const choreName = await choreItem.locator('.chore-name').innerText();
    await longPress(page, choreItem);

    // Log sheet must be visible (not edit-schedule sheet).
    await expect(page.locator('[data-action="save-log"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#log-note')).toBeVisible();

    // Tap Log to save.
    await page.locator('[data-action="save-log"]').click();
    await page.waitForTimeout(1500);

    // The completed chore must appear as a done card inside the 2 PM hour row.
    const slotCards = page.locator('[data-drop-hour="14"] .chore-card');
    await expect(slotCards).toHaveCount(1);
    await expect(slotCards.first()).toHaveClass(/chore-card--done/);
    await expect(slotCards.first().locator('.chore-name')).toContainText(choreName);
  });

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
    // Regression: logging without a slot hour must not set slotHour, so the
    // chore does not end up pinned to a phantom hour row in the calendar.
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const resp = await page.request.post('/api/logs', {
      data: { choreId, note: '', indicators: [] }, // no hour field
      headers: { 'X-CSRF-Token': csrf },
    });

    expect(resp.status()).toBe(201);
    const body = await resp.json();
    expect(body.log).toBeDefined();
    // slotHour is omitted from the response (Go omitempty) when null,
    // so accept either null or undefined (both mean "no slot set").
    expect(body.log.slotHour ?? null).toBeNull();

    // Reload and confirm the chore does not appear in any hour row
    // (it has no schedule and no slotHour, so the calendar does not show it).
    await page.reload();
    await page.waitForSelector('.cal-date', { timeout: 15000 });
    await expect(page.locator('.day-hour-grid .chore-card')).toHaveCount(0);
  });

  test('logging from hour 9 slot places the card in the 9 AM row, not 2 PM', async ({ page }) => {
    await setupWithChores(page);

    // Open the 9 AM pick-chore sheet and long-press → log the first item.
    await page.locator('[data-drop-hour="9"]').click();
    await page.waitForTimeout(400);
    await expect(page.locator('.bottom-sheet')).toBeVisible();

    const choreItem = page.locator('.sheet-chore-item').first();
    const choreName = await choreItem.locator('.chore-name').innerText();
    await longPress(page, choreItem);

    await expect(page.locator('[data-action="save-log"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-action="save-log"]').click();
    await page.waitForTimeout(1500);

    // Card should appear in hour 9 only.
    await expect(page.locator('[data-drop-hour="9"] .chore-card--done')).toHaveCount(1);
    await expect(page.locator('[data-drop-hour="14"] .chore-card')).toHaveCount(0);

    // The chore name should be visible in hour 9.
    await expect(page.locator('[data-drop-hour="9"] .chore-name').first()).toContainText(choreName);
  });
});
