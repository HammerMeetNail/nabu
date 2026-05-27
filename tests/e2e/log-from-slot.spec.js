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
  await page.click('[data-nav=\"calendar\"]');
  await page.waitForSelector('.cal-date', { timeout: 15000 });

  return { email, csrf };
}

/**
 * Simulates a long-press (≥500 ms) on a locator by holding mousedown, then
 * releasing.  Matches the 500 ms threshold in app.js.
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
}

// ─── Tests ────────────────────────────────────────────────────────────────────

test.describe('Log from time slot', () => {
  const localTodayISO = () => {
    const now = new Date();
    const pad = (n) => String(n).padStart(2, '0');
    return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
  };

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
      data: { choreId, note: '', indicators: [], date: localTodayISO() }, // no hour field
      headers: { 'X-CSRF-Token': csrf },
    });

    expect(resp.status()).toBe(201);
    const body = await resp.json();
    expect(body.log).toBeDefined();
    // slotHour is omitted from the response (Go omitempty) when null,
    // so accept either null or undefined (both mean "no slot set").
    expect(body.log.slotHour ?? null).toBeNull();

    // Reload and confirm the chore appears in the Anytime row (not in any
    // timed hour row), since it has no slotHour and no schedule.
    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });
    await expect(page.locator('.day-hour-row .chore-card')).toHaveCount(0);
    await expect(page.locator('.day-anytime-row .chore-card')).toHaveCount(1);
  });

  test('touch long-press on sheet chore item then tap Log logs the chore in that slot', async ({ page }) => {
    // Regression: touch long-press on a .sheet-chore-item must open the log
    // sheet and tapping "Log" inside .bottom-sheet must work even when
    // longPressJustFired is true (the guard must let .bottom-sheet clicks
    // through).
    await setupWithChores(page);

    // Open the 2 PM pick-chore sheet.
    await page.locator('[data-drop-hour="14"]').click();
    await page.waitForTimeout(400);
    await expect(page.locator('.bottom-sheet')).toBeVisible();

    // Get position of first sheet chore item.
    const choreItem = page.locator('.sheet-chore-item').first();
    const choreName = await choreItem.locator('.chore-name').innerText();
    await choreItem.scrollIntoViewIfNeeded();
    const box = await choreItem.boundingBox();
    const cx = box.x + box.width / 2;
    const cy = box.y + box.height / 2;

    // Dispatch a raw touch long-press (touchstart → wait 650 ms → touchend).
    // This exercises the touchstart e.preventDefault() path and the
    // longPressJustFired guard in app.js for .sheet-chore-item targets.
    await page.evaluate(([x, y]) => {
      const el = document.elementFromPoint(x, y);
      const touch = new Touch({ identifier: 1, target: el, clientX: x, clientY: y, pageX: x, pageY: y });
      el.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, cancelable: true, touches: [touch], changedTouches: [touch] }));
    }, [cx, cy]);

    await page.waitForTimeout(650); // exceed the 500 ms long-press threshold

    await page.evaluate(([x, y]) => {
      const touch = new Touch({ identifier: 1, target: document.body, clientX: x, clientY: y, pageX: x, pageY: y });
      document.dispatchEvent(new TouchEvent('touchend', { bubbles: true, cancelable: true, touches: [], changedTouches: [touch] }));
    }, [cx, cy]);

    // Log sheet must open with "Log" (save-log) button and notes field.
    await expect(page.locator('[data-action="save-log"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#log-note')).toBeVisible();

    // Simulate a touch tap + browser-synthesised click on the "Log" button.
    // The button is inside .bottom-sheet so the longPressJustFired guard must
    // let it through.
    const saveBtn = page.locator('[data-action="save-log"]');
    const saveBox = await saveBtn.boundingBox();
    const sx = saveBox.x + saveBox.width / 2;
    const sy = saveBox.y + saveBox.height / 2;
    await page.evaluate(([x, y]) => {
      const el = document.elementFromPoint(x, y);
      const touch = new Touch({ identifier: 2, target: el, clientX: x, clientY: y, pageX: x, pageY: y });
      el.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, cancelable: true, touches: [touch], changedTouches: [touch] }));
      el.dispatchEvent(new TouchEvent('touchend', { bubbles: true, cancelable: true, touches: [], changedTouches: [touch] }));
      el.click();
    }, [sx, sy]);

    await page.waitForTimeout(1500);

    // The completed chore must appear as a done card in the 2 PM hour row.
    const slotCards = page.locator('[data-drop-hour="14"] .chore-card');
    await expect(slotCards).toHaveCount(1);
    await expect(slotCards.first()).toHaveClass(/chore-card--done/);
    await expect(slotCards.first().locator('.chore-name')).toContainText(choreName);
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

  test('tapping a scheduled chore card in the day view logs it in the correct hour row, not Anytime', async ({ page }) => {
    // Regression: clicking a chore card directly in the calendar (without going
    // through any sheet) was passing slotHour=null to the API, placing the log
    // in the catch-all Anytime row instead of the hour where the card was tapped.
    await setupWithChores(page);

    // Create a schedule at the current hour via API to get a card in the grid.
    const now = new Date();
    const testHour = String(now.getHours() % 12 + 8).padStart(2, '0') + ':00';
    const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';
    const chores = (await (await page.request.get('/api/chores')).json()).chores;
    const choreId = chores[0].id;
    const choreName = chores[0].name;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: testHour, frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    const testHourNum = parseInt(testHour.split(':')[0], 10);

    // The scheduled chore card should appear unlogged in its hour row.
    const card = page.locator(`[data-drop-hour="${testHourNum}"] .chore-card`).first();
    await expect(card).toHaveAttribute('data-action', 'log-chore');
    await expect(card.locator('.chore-name')).toContainText(choreName);

    // Tap the card directly to log it.
    await card.click();
    await page.waitForTimeout(1000);

    // The card must now be marked done and still be in its hour row.
    const doneCard = page.locator(`[data-drop-hour="${testHourNum}"] .chore-card--done`).first();
    await expect(doneCard).toBeVisible();
    await expect(doneCard.locator('.chore-name')).toContainText(choreName);

    // The Anytime row must NOT contain this chore (regression guard).
    const anytimeCards = page.locator('.day-anytime-row .chore-card');
    await expect(anytimeCards).toHaveCount(0);
  });

  test('tapping a scheduled chore card in the week view logs it in the correct hour row', async ({ page }) => {
    // Same regression as the day-view test but for the week view grid.
    await setupWithChores(page);

    const now = new Date();
    const testHour = String(now.getHours() % 12 + 9).padStart(2, '0') + ':00';
    const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';
    const chores = (await (await page.request.get('/api/chores')).json()).chores;
    const choreId = chores[0].id;
    const choreName = chores[0].name;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: testHour, frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // Switch to week view.
    await page.locator('[data-view="week"]').click();
    await page.waitForSelector('.week-view', { timeout: 5000 });

    const testHourNum = parseInt(testHour.split(':')[0], 10);

    // Find the card for today's column in the correct hour row.
    const todayISO = localTodayISO();
    const card = page.locator(`.week-cell[data-drop-date="${todayISO}"][data-drop-hour="${testHourNum}"] .week-chore-card`).first();
    await expect(card).toHaveAttribute('data-action', 'log-chore');
    await expect(card).toContainText(choreName);

    // Tap the card directly.
    await card.scrollIntoViewIfNeeded();
    await card.click();
    await page.waitForTimeout(1000);

    // Card must be done and still in the correct cell.
    const doneCard = page.locator(`.week-cell[data-drop-date="${todayISO}"][data-drop-hour="${testHourNum}"] .week-chore-card.chore-card--done`).first();
    await expect(doneCard).toContainText(choreName);

    // Verify via API that the log has the correct slotHour.
    const logsResp = await page.request.get(`/api/logs/today?date=${todayISO}`);
    const logsBody = await logsResp.json();
    const ourLog = (logsBody.logs || []).find(l => l.choreId === choreId);
    expect(ourLog).toBeDefined();
    expect(ourLog.slotHour).toBe(testHourNum);
  });
});
