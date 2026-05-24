// tests/e2e/schedule.spec.js
// End-to-end tests for scheduling, calendar views, and drag-and-drop.

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-sched-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers a new user, creates a household, seeds default chores, and waits
 * for the day view to be visible.  Returns { email, csrf }.
 */
async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  // Wait for registration to complete: the user avatar appears when logged in
  await page.waitForSelector('#user-avatar:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Sched Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  // Wait for the day view to appear (confirms app fully initialised with chores)
  await page.click('[data-nav="calendar"]');
  await page.waitForSelector('.cal-date', { timeout: 15000 });

  return { email, csrf };
}

/**
 * Programmatically fires HTML5 DragEvent pairs so the app's global listeners
 * pick them up.  source / target are Playwright Locators.
 */
async function htmlDragDrop(page, sourceLocator, targetLocator) {
  const [srcHandle, tgtHandle] = await Promise.all([
    sourceLocator.elementHandle(),
    targetLocator.elementHandle(),
  ]);
  await page.evaluate(([src, tgt]) => {
    const dt = new DataTransfer();
    src.dispatchEvent(new DragEvent('dragstart', { dataTransfer: dt, bubbles: true, cancelable: true }));
    tgt.dispatchEvent(new DragEvent('dragover',  { dataTransfer: dt, bubbles: true, cancelable: true }));
    tgt.dispatchEvent(new DragEvent('drop',      { dataTransfer: dt, bubbles: true, cancelable: true }));
    src.dispatchEvent(new DragEvent('dragend',   { dataTransfer: dt, bubbles: true }));
  }, [srcHandle, tgtHandle]);
}

/**
 * Extends setupWithChores by also scheduling the first chore at 08:00 daily,
 * then reloading so a chore card is visible in the day view.
 * Use this in tests that need an existing .chore-card in the day view.
 */
async function setupWithScheduledChore(page) {
  const { email, csrf } = await setupWithChores(page);
  const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;
  await page.request.post('/api/schedules', {
    data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.reload();
  await page.click('[data-nav=\"calendar\"]');
  await page.waitForSelector('.cal-date', { timeout: 15000 });
  return { email, csrf };
}

// ─── Day View: Structure ──────────────────────────────────────────────────────

test.describe('Day View: Structure', () => {
  test('renders cal-date header with navigation arrows', async ({ page }) => {
    await setupWithChores(page);

    await expect(page.locator('.cal-date')).toBeVisible();
    const arrowBtns = page.locator('button[data-action="navigate-day"]');
    await expect(arrowBtns).toHaveCount(2);
  });

  test('renders view-tabs with Day active and Week inactive', async ({ page }) => {
    await setupWithChores(page);

    const dayTab  = page.locator('.view-tab[data-view="day"]');
    const weekTab = page.locator('.view-tab[data-view="week"]');
    await expect(dayTab).toBeVisible();
    await expect(weekTab).toBeVisible();
    await expect(dayTab).toHaveClass(/view-tab--active/);
    await expect(weekTab).not.toHaveClass(/view-tab--active/);
  });

  test('renders progress bar and progress label', async ({ page }) => {
    await setupWithChores(page);

    await expect(page.locator('.progress-bar')).toBeVisible();
    const label = page.locator('.progress-label');
    await expect(label).toBeVisible();
    // e.g. "0 of 4 done"
    await expect(label).toContainText(/of \d+ done/);
  });

  test('renders 24 hourly rows', async ({ page }) => {
    await setupWithChores(page);

    const rows = page.locator('.day-hour-row');
    await expect(rows).toHaveCount(24);
  });

  test('each hourly row has a time label', async ({ page }) => {
    await setupWithChores(page);

    // Spot-check a few known labels
    const grid = page.locator('.day-hour-grid');
    await expect(grid).toContainText('12 AM');
    await expect(grid).toContainText('12 PM');
    await expect(grid).toContainText('8 AM');
  });

  test('each hourly row has a clickable drop zone', async ({ page }) => {
    await setupWithChores(page);

    const cells = page.locator('.day-hour-cell');
    await expect(cells).toHaveCount(24);
    // Each cell is a drop target
    const firstCell = cells.first();
    await expect(firstCell).toHaveAttribute('data-drop-hour');
    await expect(firstCell).toHaveAttribute('data-action', 'open-pick-chore-sheet');
  });

  test('chore cards are draggable', async ({ page }) => {
    await setupWithScheduledChore(page);

    const cards = page.locator('[data-drag-chore-id]');
    const count  = await cards.count();
    expect(count).toBeGreaterThan(0);
    for (let i = 0; i < Math.min(count, 3); i++) {
      await expect(cards.nth(i)).toHaveAttribute('draggable', 'true');
    }
  });
});

// ─── Day View: Navigation ─────────────────────────────────────────────────────

test.describe('Day View: Navigation', () => {
  test('right arrow advances to the next day', async ({ page }) => {
    await setupWithChores(page);

    const today = await page.locator('.cal-date').innerText();
    await page.locator('button[data-action="navigate-day"]').last().click();
    await page.waitForTimeout(800);
    const tomorrow = await page.locator('.cal-date').innerText();
    expect(tomorrow).not.toBe(today);
  });

  test('left arrow goes back to the previous day', async ({ page }) => {
    await setupWithChores(page);

    const today = await page.locator('.cal-date').innerText();
    await page.locator('button[data-action="navigate-day"]').last().click();
    await page.waitForTimeout(800);
    await page.locator('button[data-action="navigate-day"]').first().click();
    await page.waitForTimeout(800);
    const back = await page.locator('.cal-date').innerText();
    expect(back).toBe(today);
  });

  test('date in cal-date matches the data-date attribute on the next button', async ({ page }) => {
    await setupWithChores(page);

    const nextBtn  = page.locator('button[data-action="navigate-day"]').last();
    const nextDate = await nextBtn.getAttribute('data-date');

    await nextBtn.click();
    await page.waitForTimeout(800);

    const newLabel = await page.locator('.cal-date').innerText();
    // Verify the label contains the month/day of nextDate
    const d = new Date(nextDate + 'T00:00:00');
    const dayNum = String(d.getDate());
    expect(newLabel).toContain(dayNum);
  });
});

// ─── View Tab Switching ───────────────────────────────────────────────────────

test.describe('View Tabs: Day / Week switching', () => {
  test('clicking Week tab renders week-view', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    await expect(page.locator('.week-view')).toBeVisible();
    await expect(page.locator('.week-grid')).toBeVisible();
  });

  test('week view tab becomes active after switch', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    const weekTab = page.locator('.view-tab[data-view="week"]');
    await expect(weekTab).toHaveClass(/view-tab--active/);
  });

  test('clicking Day tab from week view restores day-view', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);
    await page.locator('.view-tab[data-view="day"]').click();
    await page.waitForTimeout(800);

    await expect(page.locator('.day-view')).toBeVisible();
    await expect(page.locator('.day-hour-grid')).toBeVisible();
  });

  test('day tab becomes active again after switching back', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);
    await page.locator('.view-tab[data-view="day"]').click();
    await page.waitForTimeout(800);

    await expect(page.locator('.view-tab[data-view="day"]')).toHaveClass(/view-tab--active/);
    await expect(page.locator('.view-tab[data-view="week"]')).not.toHaveClass(/view-tab--active/);
  });
});

// ─── Week View: Structure ─────────────────────────────────────────────────────

test.describe('Week View: Structure', () => {
  test('shows 7 column headers', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    const headers = page.locator('.week-col-header');
    await expect(headers).toHaveCount(7);
  });

  test('renders cal-date showing week range (e.g. "Apr 27 – May 3")', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    const dateLabel = await page.locator('.cal-date').innerText();
    // Week range separator is an en-dash
    expect(dateLabel).toMatch(/–/);
  });

  test('week view has prev/next week navigation buttons', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    const arrowBtns = page.locator('button[data-action="navigate-week"]');
    await expect(arrowBtns).toHaveCount(2);
  });

  test('next-week arrow advances the week range', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    const before = await page.locator('.cal-date').innerText();
    await page.locator('button[data-action="navigate-week"]').last().click();
    await page.waitForTimeout(800);
    const after = await page.locator('.cal-date').innerText();
    expect(after).not.toBe(before);
  });

  test('prev-week arrow returns to original week', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    const orig = await page.locator('.cal-date').innerText();
    await page.locator('button[data-action="navigate-week"]').last().click();
    await page.waitForTimeout(800);
    await page.locator('button[data-action="navigate-week"]').first().click();
    await page.waitForTimeout(800);
    expect(await page.locator('.cal-date').innerText()).toBe(orig);
  });

  test('renders 24 hour rows in the week grid', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    const rows = page.locator('.hour-row');
    await expect(rows).toHaveCount(24);
  });

  test('week-grid scrollable wrapper is present', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    await expect(page.locator('.week-grid-wrapper')).toBeVisible();
  });
});

// ─── Pick-chore Bottom Sheet ──────────────────────────────────────────────────

test.describe('Pick-chore Bottom Sheet', () => {
  test('clicking an hour cell opens the sheet', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await expect(page.locator('.sheet-title')).toContainText('8 AM');
  });

  test('sheet lists all household chores', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.sheet-chore-item')).toHaveCount(17);
    await expect(page.locator('.sheet-chore-item').first()).toContainText('Feed Cats');
  });

  test('Cancel button closes the sheet', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);

    await page.locator('.bottom-sheet button[data-action="close-sheet"]').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  });

  test('clicking the backdrop closes the sheet', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);

    await page.locator('.sheet-backdrop').click({ force: true });
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  });

  test('sheet is rendered as a dialog with aria-modal', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);

    const sheet = page.locator('.bottom-sheet');
    await expect(sheet).toHaveAttribute('role', 'dialog');
    await expect(sheet).toHaveAttribute('aria-modal', 'true');
  });

  test('each sheet chore item has schedule-chore-here action, and sheet shows time input', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="18"]').click();
    await page.waitForTimeout(400);

    const item = page.locator('.sheet-chore-item').first();
    await expect(item).toHaveAttribute('data-action', 'schedule-chore-here');

    // Time input should be pre-filled with 18:00
    const timeInput = page.locator('#sheet-time');
    await expect(timeInput).toBeVisible();
    await expect(timeInput).toHaveValue('18:00');
  });

  test('picking a chore from the sheet closes the sheet', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  });

  test('after scheduling, chore card appears in the target hour row', async ({ page }) => {
    await setupWithChores(page);

    // Schedule first chore at 8 AM via the sheet
    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    // After: chore should be in the 8 AM row
    await expect(page.locator('[data-drop-hour="8"] .chore-card')).toHaveCount(1);
  });

  test('already-scheduled chores remain in the sheet list so they can be added again', async ({ page }) => {
    await setupWithChores(page);

    // Open hour-8 sheet and capture the name of the first item before scheduling
    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    const firstName = await page.locator('.sheet-chore-item').first().locator('.chore-name').innerText();
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    // Open hour-8 sheet again — the previously scheduled chore should still appear
    await page.locator('.day-hour-row[data-hour="8"] .hour-label').click();
    await page.waitForTimeout(400);

    const items = page.locator('.sheet-chore-item');
    const count = await items.count();
    // All 17 chores remain (repeatable chores are never removed)
    expect(count).toBe(17);
    // The scheduled chore is still present
    const names = await page.locator('.sheet-chore-item .chore-name').allInnerTexts();
    expect(names).toContain(firstName);

    // Clean up
    await page.locator('.bottom-sheet button[data-action="close-sheet"]').click();
  });

  test('sheet always shows all chores even when every chore has a schedule', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Schedule every chore at 8 AM via the API
    const { chores } = await (await page.request.get('/api/chores')).json();
    for (const chore of chores) {
      await page.request.post('/api/schedules', {
        data: { choreId: chore.id, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
        headers: { 'X-CSRF-Token': csrf },
      });
    }

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // Open the 8 AM sheet — all chores should still be visible
    await page.locator('.day-hour-row[data-hour="8"] .hour-label').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.sheet-chore-item')).toHaveCount(chores.length);
    // The old "all chores scheduled" empty-state message should not appear
    await expect(page.locator('.sheet-empty')).toHaveCount(0);

    await page.locator('.bottom-sheet button[data-action="close-sheet"]').click();
  });

  test('hour-label opens sheet even when the row already contains a chore card', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Pre-schedule a chore at 9 AM so the row is occupied
    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;
    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '09:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // The 9 AM row now has a chore card — clicking the cell centre would hit the
    // card's log-chore action instead.  The .hour-label button must still open
    // the pick-chore sheet regardless.
    await expect(page.locator('[data-drop-hour="9"] .chore-card')).toHaveCount(1);

    await page.locator('.day-hour-row[data-hour="9"] .hour-label').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await expect(page.locator('.sheet-title')).toContainText('9 AM');
  });
});

// ─── Chore Logging in Day View ────────────────────────────────────────────────

test.describe('Chore Logging: Day View', () => {
  test('clicking an unlogged chore card marks it as done', async ({ page }) => {
    await setupWithScheduledChore(page);

    const card = page.locator('.chore-card').first();
    await expect(card).toHaveAttribute('data-action', 'log-chore');
    await card.click();
    await page.waitForTimeout(1000);

    await expect(card).toHaveClass(/chore-card--done/);
    await expect(card).toHaveAttribute('data-action', 'view-log');
  });

  test('progress label updates after logging a chore', async ({ page }) => {
    await setupWithScheduledChore(page);

    const label = page.locator('.progress-label');
    await expect(label).toContainText(/0 of \d+ done/);

    await page.locator('.chore-card').first().click();
    await page.waitForTimeout(1000);

    await expect(label).toContainText(/1 of \d+ done/);
  });

  test('clicking a done chore undoes it', async ({ page }) => {
    await setupWithScheduledChore(page);

    const card = page.locator('.chore-card').first();
    await card.click();
    await page.waitForTimeout(1000);
    await expect(card).toHaveClass(/chore-card--done/);

    // Done card now opens a log sheet; undo lives inside the sheet.
    await card.click();
    await page.waitForTimeout(500);
    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await page.locator('[data-action="undo-chore"]').click();
    await page.waitForTimeout(1000);
    await expect(card).not.toHaveClass(/chore-card--done/);
    await expect(card).toHaveAttribute('data-action', 'log-chore');
  });

  test('progress label decrements after undoing a chore', async ({ page }) => {
    await setupWithScheduledChore(page);

    const card  = page.locator('.chore-card').first();
    const label = page.locator('.progress-label');

    await card.click();
    await page.waitForTimeout(1000);
    await expect(label).toContainText(/1 of \d+ done/);

    // Open log sheet, then undo from inside it.
    await card.click();
    await page.waitForTimeout(500);
    await page.locator('[data-action="undo-chore"]').click();
    await page.waitForTimeout(1000);
    await expect(label).toContainText(/0 of \d+ done/);
  });

  test('long-pressing a done chore opens log sheet and Remove log updates day view', async ({ page }) => {
    await setupWithScheduledChore(page);

    const card = page.locator('.chore-card').first();

    // Log the chore via a normal click first.
    await card.click();
    await page.waitForTimeout(1000);
    await expect(card).toHaveClass(/chore-card--done/);

    // Now long-press the done card (≥500 ms) to open the log sheet via the
    // long-press path (not the view-log click path).
    await card.scrollIntoViewIfNeeded();
    const box = await card.boundingBox();
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    await page.mouse.move(x, y);
    await page.mouse.down();
    await page.waitForTimeout(650);
    await page.mouse.up();

    // Log sheet must be visible with the "Remove log" button.
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-action="undo-chore"]')).toBeVisible();

    // Click "Remove log".
    await page.locator('[data-action="undo-chore"]').click();
    await page.waitForTimeout(1500);

    // The card should no longer be done.
    await expect(card).not.toHaveClass(/chore-card--done/);
    await expect(card).toHaveAttribute('data-action', 'log-chore');
  });

  test('touch long-press on done chore then tap Remove log updates day view', async ({ page }) => {
    await setupWithScheduledChore(page);

    const card = page.locator('.chore-card').first();

    // Log the chore via a click.
    await card.click();
    await page.waitForTimeout(1000);
    await expect(card).toHaveClass(/chore-card--done/);

    // Simulate a touch long-press on the done card using raw touch events.
    // This exercises the touchstart/touchend code path in app.js, including
    // e.preventDefault() and the longPressJustFired guard.
    await card.scrollIntoViewIfNeeded();
    const box = await card.boundingBox();
    const cx = box.x + box.width / 2;
    const cy = box.y + box.height / 2;

    await page.evaluate(([x, y]) => {
      const el = document.elementFromPoint(x, y);
      const touch = new Touch({ identifier: 1, target: el, clientX: x, clientY: y, pageX: x, pageY: y });
      el.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, cancelable: true, touches: [touch], changedTouches: [touch] }));
    }, [cx, cy]);

    await page.waitForTimeout(650); // exceed 500 ms long-press threshold

    await page.evaluate(([x, y]) => {
      const el = document.elementFromPoint(x, y) || document.body;
      const touch = new Touch({ identifier: 1, target: document.body, clientX: x, clientY: y, pageX: x, pageY: y });
      document.dispatchEvent(new TouchEvent('touchend', { bubbles: true, cancelable: true, touches: [], changedTouches: [touch] }));
    }, [cx, cy]);

    // Log sheet must open with "Remove log" button.
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-action="undo-chore"]')).toBeVisible();

    // Simulate a touch tap on the "Remove log" button (touchstart + touchend,
    // no e.preventDefault() for this element, so click should be dispatched
    // either by iOS or by our touchend handler).
    const undoBtn = page.locator('[data-action="undo-chore"]');
    const undoBox = await undoBtn.boundingBox();
    const ux = undoBox.x + undoBox.width / 2;
    const uy = undoBox.y + undoBox.height / 2;

    await page.evaluate(([x, y]) => {
      const el = document.elementFromPoint(x, y);
      const touch = new Touch({ identifier: 2, target: el, clientX: x, clientY: y, pageX: x, pageY: y });
      el.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, cancelable: true, touches: [touch], changedTouches: [touch] }));
      // Simulate the click that the browser would synthesize after touchend
      // (since preventDefault was not called for this touchstart).
      el.dispatchEvent(new TouchEvent('touchend', { bubbles: true, cancelable: true, touches: [], changedTouches: [touch] }));
      el.click();
    }, [ux, uy]);

    await page.waitForTimeout(1500);

    // The card should no longer be done.
    await expect(card).not.toHaveClass(/chore-card--done/);
    await expect(card).toHaveAttribute('data-action', 'log-chore');
  });

  test('done chore shows check-overlay in day view', async ({ page }) => {
    await setupWithScheduledChore(page);

    const card = page.locator('.chore-card').first();
    await card.click();
    await page.waitForTimeout(1000);

    await expect(card.locator('.check-overlay')).toBeVisible();
  });
});

// ─── Chore Logging: Week View ──────────────────────────────────────────────────

test.describe('Chore Logging: Week View', () => {
  test('clicking an unlogged week-view card marks it as done', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Create a daily schedule so it appears in any week.
    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;
    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // Switch to week view.
    await page.locator('[data-action="switch-view"][data-view="week"]').click();
    await page.waitForTimeout(500);

    const card = page.locator('.week-chore-card').first();
    await expect(card).toBeVisible();
    await expect(card).toHaveAttribute('data-action', 'log-chore');

    await card.click();
    await page.waitForTimeout(1000);

    // Card must now show as done without requiring a view switch.
    await expect(card).toHaveClass(/chore-card--done/);
    await expect(card).toHaveAttribute('data-action', 'view-log');
  });

  test('clicking a done week-view card undoes it', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;
    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    await page.locator('[data-action="switch-view"][data-view="week"]').click();
    await page.waitForTimeout(500);

    const card = page.locator('.week-chore-card').first();
    await card.click();
    await page.waitForTimeout(1000);
    await expect(card).toHaveClass(/chore-card--done/);

    // Done card opens log sheet; undo lives inside it.
    await card.click();
    await page.waitForTimeout(500);
    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await page.locator('[data-action="undo-chore"]').click();
    await page.waitForTimeout(1000);
    await expect(card).not.toHaveClass(/chore-card--done/);
    await expect(card).toHaveAttribute('data-action', 'log-chore');
  });
});

// ─── Drag and Drop: Day View ──────────────────────────────────────────────────

test.describe('Drag and Drop: Day View', () => {
  test('dragging a chore to an hour row moves it there', async ({ page }) => {
    await setupWithChores(page);

    // Schedule the first chore at 8 AM so it appears as a draggable card
    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    await expect(page.locator('[data-drop-hour="8"] .chore-card')).toHaveCount(1);

    const card = page.locator('[data-drop-hour="8"] [data-drag-chore-id]').first();
    const hourCell = page.locator('[data-drop-hour="14"]');

    await htmlDragDrop(page, card, hourCell);
    await page.waitForTimeout(1500);

    // Chore should now be in the 2 PM row, not 8 AM
    await expect(page.locator('[data-drop-hour="14"] .chore-card')).toHaveCount(1);
    await expect(page.locator('[data-drop-hour="8"] .chore-card')).toHaveCount(0);
  });

  test('dragging preserves the chore name after move', async ({ page }) => {
    await setupWithChores(page);

    // Schedule a chore at 8 AM so it appears as a draggable card
    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    const choreItem = page.locator('.sheet-chore-item').first();
    const choreName = await choreItem.locator('.chore-name').innerText();
    await choreItem.click();
    await page.waitForTimeout(1500);

    const card = page.locator('[data-drop-hour="8"] [data-drag-chore-id]').first();
    const hourCell = page.locator('[data-drop-hour="14"]');

    await htmlDragDrop(page, card, hourCell);
    await page.waitForTimeout(1500);

    await expect(page.locator('[data-drop-hour="14"] .chore-name').first()).toContainText(choreName);
  });

  test('dragging a scheduled chore between hour rows updates the schedule', async ({ page }) => {
    await setupWithChores(page);

    // First schedule the chore at 8 AM via the sheet
    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    await expect(page.locator('[data-drop-hour="8"] .chore-card')).toHaveCount(1);

    // Drag from 8 AM to 6 PM
    const card     = page.locator('[data-drop-hour="8"] [data-drag-chore-id]').first();
    const target18 = page.locator('[data-drop-hour="18"]');

    await htmlDragDrop(page, card, target18);
    await page.waitForTimeout(1500);

    await expect(page.locator('[data-drop-hour="18"] .chore-card')).toHaveCount(1);
    await expect(page.locator('[data-drop-hour="8"] .chore-card')).toHaveCount(0);
  });

});

// ─── Schedule API ─────────────────────────────────────────────────────────────

test.describe('Schedule API', () => {
  test('GET /api/schedules returns empty list on fresh account', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.get('/api/schedules');
    expect(resp.status()).toBe(200);
    const data = await resp.json();
    expect(Array.isArray(data.schedules)).toBe(true);
    // No schedules created yet
    expect(data.schedules.length).toBe(0);
  });

  test('POST /api/schedules creates a schedule and returns it', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    const resp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const data = await resp.json();
    expect(data.schedule).toBeDefined();
    expect(data.schedule.choreId).toBe(choreId);
    expect(data.schedule.specificTime).toBe('08:00');
    expect(data.schedule.frequencyType).toBe('daily');
  });

  test('GET /api/schedules lists the newly created schedule', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '18:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    const listResp = await page.request.get('/api/schedules');
    const list     = (await listResp.json()).schedules;
    expect(list.length).toBe(1);
    expect(list[0].choreId).toBe(choreId);
    expect(list[0].specificTime).toBe('18:00');
  });

  test('PATCH /api/schedules/:id updates specificTime', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    const createResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    const scheduleId = (await createResp.json()).schedule.id;

    const patchResp = await page.request.patch(`/api/schedules/${scheduleId}`, {
      data: { specificTime: '21:00' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(patchResp.status()).toBe(200);
    const updated = (await patchResp.json()).schedule;
    expect(updated.specificTime).toBe('21:00');
  });

  test('PATCH /api/schedules/:id updates timePeriod', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    const createResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    const scheduleId = (await createResp.json()).schedule.id;

    const patchResp = await page.request.patch(`/api/schedules/${scheduleId}`, {
      data: { specificTime: '08:30' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(patchResp.status()).toBe(200);
    const updated = (await patchResp.json()).schedule;
    expect(updated.specificTime).toBe('08:30');
  });

  test('DELETE /api/schedules/:id removes the schedule', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    const createResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    const scheduleId = (await createResp.json()).schedule.id;

    const delResp = await page.request.delete(`/api/schedules/${scheduleId}`, {
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(delResp.status()).toBe(200);
    const body = await delResp.json();
    expect(body.status).toBe('deleted');

    // Verify it's gone from the list
    const listResp = await page.request.get('/api/schedules');
    expect((await listResp.json()).schedules.length).toBe(0);
  });

  test('GET /api/schedules/for-date returns active daily schedule', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Get today's date in YYYY-MM-DD
    const today = new Date().toISOString().split('T')[0];
    const resp  = await page.request.get(`/api/schedules/for-date?date=${today}`);
    expect(resp.status()).toBe(200);
    const data = await resp.json();
    expect(Array.isArray(data.schedules)).toBe(true);
    expect(data.schedules.length).toBe(1);
    expect(data.schedules[0].choreId).toBe(choreId);
  });

  test('POST /api/schedules requires choreId', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.post('/api/schedules', {
      data: { timePeriod: 'anytime', frequencyType: 'daily' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(400);
  });

  test('PATCH /api/schedules/:id returns 404 for non-existent id', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.patch('/api/schedules/999999', {
      data: { specificTime: '09:00' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(404);
  });

  test('DELETE /api/schedules/:id returns 404 for non-existent id', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.delete('/api/schedules/999999', {
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(404);
  });
});

// ─── Schedule: Week frequency type ───────────────────────────────────────────

test.describe('Schedule API: recurrence types', () => {
  test('POST /api/schedules with weekly frequency and daysOfWeek', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const resp = await page.request.post('/api/schedules', {
      data: {
        choreId,
        timePeriod: 'anytime',
        frequencyType: 'weekly',
        daysOfWeek: [1, 3, 5], // Mon, Wed, Fri
        isActive: true,
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const sch = (await resp.json()).schedule;
    expect(sch.frequencyType).toBe('weekly');
  });

  test('POST /api/schedules with every_n_days frequency', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const resp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'every_n_days', intervalDays: 3, isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const sch = (await resp.json()).schedule;
    expect(sch.frequencyType).toBe('every_n_days');
  });

  test('POST /api/schedules with monthly_by_date frequency', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const resp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'monthly_by_date', dayOfMonth: 15, isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const sch = (await resp.json()).schedule;
    expect(sch.frequencyType).toBe('monthly_by_date');
  });
});

// ─── Week View: Scheduled Chores Appear in Correct Cells ─────────────────────

test.describe('Week View: Scheduled chores appear in grid cells', () => {
  test('a daily schedule with specificTime 05:00 renders a card in the 5 AM hour row', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    // Specifically schedule at 05:00
    await page.request.post('/api/schedules', {
      data: {
        choreId,
        timePeriod: 'anytime',
        specificTime: '05:00',
        frequencyType: 'daily',
        isActive: true,
      },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Switch to week view
    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(1200);

    // The 5 AM row should contain at least one week-chore-card
    const row = page.locator('.hour-row[data-hour="5"]');
    await expect(row).toBeVisible();
    await expect(row.locator('.week-chore-card')).toHaveCount(7); // one per day of week (daily)
  });
});

// ─── Long-press edit/delete for scheduled chores ─────────────────────────────

test.describe('Long-press edit sheet', () => {
  /**
   * Simulate a long-press (500 ms hold) on a chore card by dispatching
   * mousedown, waiting, then mouseup.  This matches the 500 ms threshold in
   * app.js.
   *
   * Playwright's low-level mouse API does NOT auto-fire a `click` event after
   * mouse.up() (unlike locator.click()).  The app uses a `longPressJustFired`
   * guard to suppress the residual browser-native click that follows a real
   * long-press.  We therefore dispatch one synthetic click explicitly after
   * mouse.up() so the guard is consumed before we interact with the sheet.
   */
  async function longPress(page, locator) {
    await locator.scrollIntoViewIfNeeded();
    const box = await locator.boundingBox();
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    await page.mouse.move(x, y);
    await page.mouse.down();
    await page.waitForTimeout(650); // slightly over the 500 ms threshold
    await page.mouse.up();
    // Consume the longPressJustFired guard with a synthetic click so subsequent
    // clicks on sheet buttons are not suppressed.  Skip if the element at that
    // position is the sheet backdrop — that would close the just-opened sheet.
    await page.evaluate(([cx, cy]) => {
      const el = document.elementFromPoint(cx, cy);
      if (el && el.dataset.action !== 'close-sheet') {
        el.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
      }
    }, [x, y]);
    await page.waitForTimeout(50);
  }

  test('long-pressing a scheduled chore card opens the log sheet', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    // Create a schedule with a known time
    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '09:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // The card should be in the 9 AM row
    const card = page.locator('.day-hour-row[data-hour="9"] .chore-card').first();
    await expect(card).toBeVisible();

    await longPress(page, card);

    // Log sheet should appear (not edit sheet)
    await expect(page.locator('[data-action="save-log"]')).toBeVisible();
    await expect(page.locator('#log-note')).toBeVisible();
  });

  test('saving a new time from the edit sheet updates the card position', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    const wrap = page.locator('.day-hour-row[data-hour="8"] .chore-card-wrap').first();
    await expect(wrap).toBeVisible();

    // Hover to reveal the pencil button, then click it to open the edit sheet.
    await wrap.hover();
    await wrap.locator('.chore-card-edit-btn').click();
    await expect(page.locator('#edit-sheet-time')).toBeVisible();

    // Change the time to 14:00
    await page.fill('#edit-sheet-time', '14:00');
    await page.locator('[data-action="save-schedule-edit"]').click();

    // Sheet should close
    await expect(page.locator('[data-action="save-schedule-edit"]')).not.toBeVisible();

    // Card should now appear in the 14:00 row, not the 8:00 row
    await expect(page.locator('.day-hour-row[data-hour="14"] .chore-card')).toBeVisible();
    await expect(page.locator('.day-hour-row[data-hour="8"] .chore-card')).not.toBeVisible();
  });

  test('deleting a schedule from the edit sheet removes the card from the view', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '10:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    const wrap = page.locator('.day-hour-row[data-hour="10"] .chore-card-wrap').first();
    await expect(wrap).toBeVisible();

    // Open edit sheet via pencil button.
    await wrap.hover();
    await wrap.locator('.chore-card-edit-btn').click();
    await expect(page.locator('[data-action="delete-schedule"]')).toBeVisible();

    await page.locator('[data-action="delete-schedule"]').click();

    // Sheet should close and card should disappear from the 10 AM row
    await expect(page.locator('[data-action="delete-schedule"]')).not.toBeVisible();
    await expect(page.locator('.day-hour-row[data-hour="10"] .chore-card')).not.toBeVisible();
  });
});

// ─── Day View: Multiple chores per hour row ───────────────────────────────────

test.describe('Day View: Multiple chores per hour row', () => {
  test('two chores scheduled at the same hour both render in that row', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const chores    = (await choreResp.json()).chores;
    // Need at least two chores; setupWithChores creates 3.
    const [choreA, choreB] = chores;

    await Promise.all([
      page.request.post('/api/schedules', {
        data: { choreId: choreA.id, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
        headers: { 'X-CSRF-Token': csrf },
      }),
      page.request.post('/api/schedules', {
        data: { choreId: choreB.id, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
        headers: { 'X-CSRF-Token': csrf },
      }),
    ]);

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // Both compact chip cards should appear inside the 8 AM cell.
    await expect(page.locator('[data-drop-hour="8"] .chore-card')).toHaveCount(2);
    // They should use the compact chip style.
    await expect(page.locator('[data-drop-hour="8"] .chore-card--compact')).toHaveCount(2);
  });

  test('dragging a second chore into an occupied hour row adds it alongside the first', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const chores    = (await choreResp.json()).chores;
    const [choreA, choreB] = chores;

    // Pre-schedule choreA at 8 AM and choreB at 9 AM.
    await Promise.all([
      page.request.post('/api/schedules', {
        data: { choreId: choreA.id, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
        headers: { 'X-CSRF-Token': csrf },
      }),
      page.request.post('/api/schedules', {
        data: { choreId: choreB.id, timePeriod: 'anytime', specificTime: '09:00', frequencyType: 'daily', isActive: true },
        headers: { 'X-CSRF-Token': csrf },
      }),
    ]);

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    await expect(page.locator('[data-drop-hour="8"] .chore-card')).toHaveCount(1);
    await expect(page.locator('[data-drop-hour="9"] .chore-card')).toHaveCount(1);

    // Drag choreB from the 9 AM row into the occupied 8 AM row.
    const sourceCard = page.locator('[data-drop-hour="9"] [data-drag-chore-id]').first();
    const hourCell   = page.locator('[data-drop-hour="8"]');

    await htmlDragDrop(page, sourceCard, hourCell);
    await page.waitForTimeout(1500);

    // Both cards should now be in the 8 AM row.
    await expect(page.locator('[data-drop-hour="8"] .chore-card')).toHaveCount(2);
    // They should use the compact chip style.
    await expect(page.locator('[data-drop-hour="8"] .chore-card--compact')).toHaveCount(2);
  });

  test('hour-label button is always accessible even with multiple cards in the row', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const chores    = (await choreResp.json()).chores;
    const [choreA, choreB] = chores;

    await Promise.all([
      page.request.post('/api/schedules', {
        data: { choreId: choreA.id, timePeriod: 'anytime', specificTime: '09:00', frequencyType: 'daily', isActive: true },
        headers: { 'X-CSRF-Token': csrf },
      }),
      page.request.post('/api/schedules', {
        data: { choreId: choreB.id, timePeriod: 'anytime', specificTime: '09:00', frequencyType: 'daily', isActive: true },
        headers: { 'X-CSRF-Token': csrf },
      }),
    ]);

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    await expect(page.locator('[data-drop-hour="9"] .chore-card')).toHaveCount(2);

    // The hour-label button should still open the pick sheet regardless of how
    // many cards fill the cell.
    await page.locator('.day-hour-row[data-hour="9"] .hour-label').click();
    await page.waitForTimeout(400);
    await expect(page.locator('.bottom-sheet')).toBeVisible();
  });
});

// ─── Week View: Clicking a cell opens the pick-chore sheet ───────────────────

test.describe('Week View: Clicking a cell opens pick-chore sheet', () => {
  test('clicking an empty week cell opens the pick-chore sheet', async ({ page }) => {
    await setupWithChores(page);

    // Switch to week view.
    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    // Click the first week cell at hour 8 (empty — no schedules created).
    await page.locator('.week-cell[data-drop-hour="8"]').first().click();
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible();
    // Sheet should be pre-targeted at hour 8.
    await expect(page.locator('#sheet-time')).toHaveValue('08:00');
  });

  test('clicking a week cell that already contains a card still opens the sheet via background', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '10:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(1000);

    // Verify the card is there in at least one of the week cells.
    await expect(page.locator('.week-cell[data-drop-hour="10"] .week-chore-card').first()).toBeVisible();

    // Click the cell itself (not the card) using the hour-label column as a reference.
    // We target the cell by its data attributes; Playwright clicks the centre, which
    // may land on the card.  Instead use the first week cell at an adjacent empty hour
    // on the same row to confirm the mechanism works without card interference.
    await page.locator('.week-cell[data-drop-hour="11"]').first().click();
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await expect(page.locator('#sheet-time')).toHaveValue('11:00');
  });
});

// ─── Frequency selector in bottom sheets ─────────────────────────────────────

test.describe('Frequency selector: pick-chore sheet', () => {
  test('sheet contains a frequency select defaulting to "once"', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);

    // The freq <select> must be visible with "once" pre-selected.
    const sel = page.locator('#sheet-freq');
    await expect(sel).toBeVisible();
    await expect(sel).toHaveValue('once');
  });

  test('weekday pill row is hidden when "once" is selected', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);

    // Row must be present in the DOM but hidden via the "hidden" attribute.
    const wkRow = page.locator('#sheet-weekday-row');
    await expect(wkRow).toBeHidden();
  });

  test('selecting "weekly" reveals the weekday pill row', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);

    await page.locator('#sheet-freq').selectOption('weekly');

    const wkRow = page.locator('#sheet-weekday-row');
    await expect(wkRow).toBeVisible();
    // Seven day pills should be present.
    await expect(page.locator('#sheet-weekday-row .day-pill')).toHaveCount(7);
  });

  test('switching back from "weekly" to another option hides the pill row', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);

    await page.locator('#sheet-freq').selectOption('weekly');
    await expect(page.locator('#sheet-weekday-row')).toBeVisible();

    await page.locator('#sheet-freq').selectOption('daily');
    await expect(page.locator('#sheet-weekday-row')).toBeHidden();
  });

  test('scheduling via UI creates a "once" schedule with startDate = today', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Get today's ISO date from the server perspective (use the cal-date heading).
    const calDateText = await page.locator('.cal-date').innerText();
    // The heading is like "Thursday, April 30" — derive today's ISO date from the
    // data-date on the next-day navigation button instead (reliable machine-readable).
    const nextBtn = page.locator('[data-action="navigate-day"]').last();
    const nextDate = await nextBtn.getAttribute('data-date'); // "YYYY-MM-DD" of tomorrow
    const tomorrow = nextDate;
    const [y, m, d] = tomorrow.split('-').map(Number);
    const todayDate = new Date(y, m - 1, d - 1);
    const todayISO = `${todayDate.getFullYear()}-${String(todayDate.getMonth() + 1).padStart(2, '0')}-${String(todayDate.getDate()).padStart(2, '0')}`;

    // Open hour-8 sheet and pick the first chore (freq defaults to "once").
    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    // Fetch the created schedule from the API.
    const { schedules } = await (await page.request.get('/api/schedules')).json();
    const sch = schedules.find(s => s.specificTime === '08:00');
    expect(sch).toBeDefined();
    expect(sch.frequencyType).toBe('once');
    expect(sch.startDate).toBe(todayISO);
  });

  test('"once" schedule only appears on its startDate, not adjacent days', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Derive today's ISO date from the navigation button.
    const nextBtn = page.locator('[data-action="navigate-day"]').last();
    const tomorrow = await nextBtn.getAttribute('data-date');
    const [y, m, d] = tomorrow.split('-').map(Number);
    const todayDate = new Date(y, m - 1, d - 1);
    const todayISO = `${todayDate.getFullYear()}-${String(todayDate.getMonth() + 1).padStart(2, '0')}-${String(todayDate.getDate()).padStart(2, '0')}`;

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    // Create a "once" schedule for today at 10:00 via the API.
    await page.request.post('/api/schedules', {
      data: {
        choreId,
        timePeriod: 'anytime',
        specificTime: '10:00',
        frequencyType: 'once',
        startDate: todayISO,
        isActive: true,
      },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // Card must appear in the 10 AM row today.
    await expect(page.locator('.day-hour-row[data-hour="10"] .chore-card')).toHaveCount(1);

    // Navigate to tomorrow — card must NOT appear.
    await page.locator('[data-action="navigate-day"]').last().click();
    await page.waitForTimeout(800);
    await expect(page.locator('.day-hour-row[data-hour="10"] .chore-card')).toHaveCount(0);

    // Navigate back to today — card must reappear.
    await page.locator('[data-action="navigate-day"]').first().click();
    await page.waitForTimeout(800);
    await expect(page.locator('.day-hour-row[data-hour="10"] .chore-card')).toHaveCount(1);
  });

  test('scheduling with "weekly" frequency creates a weekly schedule', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.locator('[data-drop-hour="9"]').click();
    await page.waitForTimeout(400);

    // Switch freq to "weekly".
    await page.locator('#sheet-freq').selectOption('weekly');
    await expect(page.locator('#sheet-weekday-row')).toBeVisible();

    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    const { schedules } = await (await page.request.get('/api/schedules')).json();
    const sch = schedules.find(s => s.specificTime === '09:00');
    expect(sch).toBeDefined();
    expect(sch.frequencyType).toBe('weekly');
    expect(Array.isArray(sch.daysOfWeek)).toBe(true);
    expect(sch.daysOfWeek.length).toBeGreaterThan(0);
  });
});

// ─── Frequency selector: edit-schedule sheet ─────────────────────────────────

test.describe('Frequency selector: edit-schedule sheet', () => {
  test('edit sheet shows #edit-sheet-freq pre-populated from the schedule', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    // Create a weekly schedule so we can verify the edit sheet reflects it.
    await page.request.post('/api/schedules', {
      data: {
        choreId,
        timePeriod: 'anytime',
        specificTime: '11:00',
        frequencyType: 'weekly',
        daysOfWeek: [1, 3],
        isActive: true,
      },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // The card is in the 11 AM row only on Mon/Wed.  We may or may not be on one
    // of those days.  Use the API directly to find the current displayed date and
    // navigate to the next Monday if needed — or simply look for the card at all.
    // For simplicity just check if the card is visible; if not, skip the long-press
    // part since the schedule might not be active today.
    // Instead, we'll just check the API schedule record to confirm frequencyType,
    // and verify the edit sheet select is populated correctly by opening it via API
    // then using the long-press path only when the card exists.

    // Easier: navigate to a Monday if today is not one.
    // We check the current date and advance until we hit day-of-week 1 (Monday).
    const nextBtn = page.locator('[data-action="navigate-day"]').last();
    let attempts = 0;
    while (attempts < 7) {
      const nextDate = await nextBtn.getAttribute('data-date');
      const dayOfWeek = new Date(nextDate + 'T00:00:00').getDay(); // 0=Sun
      // If today is Monday (day before tomorrow is Sunday... wait let me recalculate)
      // tomorrow's day-1 = today's day of week
      const todayDOW = (dayOfWeek + 6) % 7; // convert Sun=0→6, Mon=1→0 is wrong...
      // Actually: if nextDate is tomorrow, today is new Date(nextDate)-1day
      const todayDOWRaw = new Date(nextDate + 'T00:00:00').getDay();
      // todayDOWRaw is tomorrow's DOW; today = (todayDOWRaw - 1 + 7) % 7
      const todayActual = (todayDOWRaw - 1 + 7) % 7; // 0=Sun, 1=Mon...
      if (todayActual === 1 || todayActual === 3) break; // Mon or Wed
      await page.locator('[data-action="navigate-day"]').last().click();
      await page.waitForTimeout(500);
      attempts++;
    }

    const card = page.locator('.day-hour-row[data-hour="11"] .chore-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    await card.scrollIntoViewIfNeeded();

    // Open edit sheet via pencil button.
    const wrap11 = page.locator('.day-hour-row[data-hour="11"] .chore-card-wrap').first();
    await wrap11.hover();
    await wrap11.locator('.chore-card-edit-btn').click();
    await page.waitForTimeout(300);

    await expect(page.locator('#edit-sheet-freq')).toBeVisible();
    await expect(page.locator('#edit-sheet-freq')).toHaveValue('weekly');
    await expect(page.locator('#edit-sheet-weekday-row')).toBeVisible();
  });

  test('changing frequency in edit sheet and saving persists the new type', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    // Create a daily schedule so it's always visible regardless of today's DOW.
    const { schedule: created } = await (await page.request.post('/api/schedules', {
      data: {
        choreId,
        timePeriod: 'anytime',
        specificTime: '12:00',
        frequencyType: 'daily',
        isActive: true,
      },
      headers: { 'X-CSRF-Token': csrf },
    })).json();

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    const card = page.locator('.day-hour-row[data-hour="12"] .chore-card').first();
    await expect(card).toBeVisible();
    await card.scrollIntoViewIfNeeded();

    // Open edit sheet via pencil button.
    const wrap12 = page.locator('.day-hour-row[data-hour="12"] .chore-card-wrap').first();
    await wrap12.hover();
    await wrap12.locator('.chore-card-edit-btn').click();
    await page.waitForTimeout(300);

    await expect(page.locator('#edit-sheet-freq')).toBeVisible();
    // Edit sheet should show the existing "daily" frequency.
    await expect(page.locator('#edit-sheet-freq')).toHaveValue('daily');

    // Change to "once".
    await page.locator('#edit-sheet-freq').selectOption('once');
    await page.locator('[data-action="save-schedule-edit"]').click();
    await page.waitForTimeout(1500);

    // Verify via API that the schedule was updated.
    const { schedules } = await (await page.request.get('/api/schedules')).json();
    const updated = schedules.find(s => s.id === created.id);
    expect(updated).toBeDefined();
    expect(updated.frequencyType).toBe('once');
  });
});

// ─── Drag-and-drop creates "once" schedule ────────────────────────────────────

test.describe('Drag-and-drop: default "once" frequency', () => {
  test('dragging a "once" scheduled card between hour rows preserves the "once" frequency', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Derive today's ISO date.
    const nextBtn = page.locator('[data-action="navigate-day"]').last();
    const tomorrow = await nextBtn.getAttribute('data-date');
    const [y, m, d] = tomorrow.split('-').map(Number);
    const todayDate = new Date(y, m - 1, d - 1);
    const todayISO = `${todayDate.getFullYear()}-${String(todayDate.getMonth() + 1).padStart(2, '0')}-${String(todayDate.getDate()).padStart(2, '0')}`;

    // Schedule a chore at 8 AM via pick-chore sheet (defaults to "once" frequency).
    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    // Verify the schedule is "once" with startDate = today.
    const { schedules: before } = await (await page.request.get('/api/schedules')).json();
    const sch = before.find(s => s.specificTime === '08:00');
    expect(sch).toBeDefined();
    expect(sch.frequencyType).toBe('once');
    expect(sch.startDate).toBe(todayISO);

    // Now drag the card from 8 AM to 7 AM.
    const sourceCard = page.locator('[data-drop-hour="8"] [data-drag-chore-id]').first();
    const dropTarget = page.locator('[data-drop-hour="7"]');
    await htmlDragDrop(page, sourceCard, dropTarget);
    await page.waitForTimeout(1500);

    // Card should now be in the 7 AM row.
    await expect(page.locator('.day-hour-row[data-hour="7"] .chore-card')).toHaveCount(1);

    // The schedule must still be "once" with the new specificTime.
    const { schedules: after } = await (await page.request.get('/api/schedules')).json();
    const updated = after.find(s => s.specificTime === '07:00');
    expect(updated).toBeDefined();
    expect(updated.frequencyType).toBe('once');
    expect(updated.startDate).toBe(todayISO);
  });
});

// ─── Frequency selector: every_n_days ─────────────────────────────────────────

test.describe('Frequency selector: every_n_days', () => {
  const localTodayISO = () => {
    const now = new Date();
    const pad = (n) => String(n).padStart(2, '0');
    return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
  };

  test('pick-chore sheet contains "Every N days" option', async ({ page }) => {
    await setupWithChores(page);

    // Click an hour slot to open the pick-chore sheet.
    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await expect(page.locator('#sheet-freq')).toBeVisible();

    // The "every_n_days" option must exist.
    const options = await page.locator('#sheet-freq option').allTextContents();
    expect(options.some(t => /every.*days/i.test(t))).toBe(true);
  });

  test('selecting "every_n_days" reveals the interval input row', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await expect(page.locator('#sheet-freq')).toBeVisible();

    // Interval row should be hidden initially (default is "once").
    await expect(page.locator('#sheet-interval-row')).toBeHidden();

    // Selecting every_n_days should reveal it.
    await page.locator('#sheet-freq').selectOption('every_n_days');
    await expect(page.locator('#sheet-interval-row')).toBeVisible();

    // Switching back to "once" should hide it again.
    await page.locator('#sheet-freq').selectOption('once');
    await expect(page.locator('#sheet-interval-row')).toBeHidden();
  });

  test('scheduling via UI with every_n_days creates schedule with correct intervalDays', async ({ page }) => {
    const { csrf } = await setupWithChores(page);
    const chores = (await (await page.request.get('/api/chores')).json()).chores;
    const chore = chores[0];

    // Open pick-chore sheet for hour 8.
    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await expect(page.locator('#sheet-freq')).toBeVisible();

    // Select "every_n_days" and set interval to 3.
    await page.locator('#sheet-freq').selectOption('every_n_days');
    await expect(page.locator('#sheet-interval')).toBeVisible();
    await page.locator('#sheet-interval').fill('3');

    // Pick the first chore.
    await page.locator(`[data-action="schedule-chore-here"][data-chore-id="${chore.id}"]`).click();
    await page.waitForTimeout(1500);

    // Verify via API.
    const { schedules } = await (await page.request.get('/api/schedules')).json();
    const sch = schedules.find(s => s.choreId === chore.id && s.specificTime === '08:00');
    expect(sch).toBeDefined();
    expect(sch.frequencyType).toBe('every_n_days');
    expect(sch.intervalDays).toBe(3);
  });

  test('interval input label updates as user types', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await expect(page.locator('#sheet-freq')).toBeVisible();

    await page.locator('#sheet-freq').selectOption('every_n_days');
    await page.locator('#sheet-interval').fill('7');

    // The selected option text should update to "Every 7 days".
    const selectedText = await page.locator('#sheet-freq').evaluate(
      sel => sel.options[sel.selectedIndex].textContent.trim()
    );
    expect(selectedText).toBe('Every 7 days');
  });

  test('edit sheet shows every_n_days pre-populated and can be saved', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    // Create an every_n_days=3 schedule at noon so it's always visible.
    const { schedule: created } = await (await page.request.post('/api/schedules', {
      data: {
        choreId,
        timePeriod: 'anytime',
        specificTime: '12:00',
        frequencyType: 'every_n_days',
        intervalDays: 3,
        startDate: localTodayISO(),
        isActive: true,
      },
      headers: { 'X-CSRF-Token': csrf },
    })).json();

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    const card = page.locator('.day-hour-row[data-hour="12"] .chore-card').first();
    await expect(card).toBeVisible();
    await card.scrollIntoViewIfNeeded();

    // Open edit sheet via pencil button (long-press now opens the log sheet).
    const wrap12b = page.locator('.day-hour-row[data-hour="12"] .chore-card-wrap').first();
    await wrap12b.hover();
    await wrap12b.locator('.chore-card-edit-btn').click();
    await page.waitForTimeout(300);

    await expect(page.locator('#edit-sheet-freq')).toBeVisible();
    await expect(page.locator('#edit-sheet-freq')).toHaveValue('every_n_days');
    await expect(page.locator('#edit-sheet-interval-row')).toBeVisible();
    await expect(page.locator('#edit-sheet-interval')).toHaveValue('3');

    // Change interval to 5 and save.
    await page.locator('#edit-sheet-interval').fill('5');
    await page.locator('[data-action="save-schedule-edit"]').click();
    await page.waitForTimeout(1500);

    const { schedules } = await (await page.request.get('/api/schedules')).json();
    const updated = schedules.find(s => s.id === created.id);
    expect(updated).toBeDefined();
    expect(updated.frequencyType).toBe('every_n_days');
    expect(updated.intervalDays).toBe(5);
  });
});

// ─── Long-press on sheet chore items ─────────────────────────────────────────

test.describe('Long-press on sheet chore items', () => {
  /**
   * Simulates a 500 ms long-press on any element, then consumes the
   * longPressJustFired guard with a synthetic click so subsequent interactions
   * with the opened sheet are not blocked.
   */
  async function longPressItem(page, locator) {
    await locator.scrollIntoViewIfNeeded();
    const box = await locator.boundingBox();
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    await page.mouse.move(x, y);
    await page.mouse.down();
    await page.waitForTimeout(650);
    await page.mouse.up();
    await page.evaluate(([cx, cy]) => {
      const el = document.elementFromPoint(cx, cy);
      if (el && el.dataset.action !== 'close-sheet') {
        el.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
      }
    }, [x, y]);
    await page.waitForTimeout(50);
  }

  test('long-pressing a chore item in the quick-log sheet opens the log detail sheet', async ({ page }) => {
    await setupWithChores(page);

    // Open quick-log sheet via the FAB.
    await page.locator('.fab').click();
    await page.waitForTimeout(400);
    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await expect(page.locator('.sheet-title')).toContainText('Log a chore');

    // Long-press the first chore item.
    const item = page.locator('.sheet-chore-item').first();
    await longPressItem(page, item);

    // The log detail sheet should replace the quick-log sheet.
    await expect(page.locator('[data-action="save-log"]')).toBeVisible();
    await expect(page.locator('#log-note')).toBeVisible();
  });

  test('long-pressing a chore item in the pick-chore sheet opens the log detail sheet', async ({ page }) => {
    await setupWithChores(page);

    // Open pick-chore sheet by clicking an hour cell.
    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);
    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await expect(page.locator('.sheet-title')).toContainText('8 AM');

    // Long-press the first chore item.
    const item = page.locator('.sheet-chore-item').first();
    await longPressItem(page, item);

    // The log detail sheet should appear (not the pick-chore sheet).
    await expect(page.locator('[data-action="save-log"]')).toBeVisible();
    await expect(page.locator('#log-note')).toBeVisible();
    // The schedule-chore-here action must no longer be visible.
    await expect(page.locator('[data-action="schedule-chore-here"]')).toHaveCount(0);
  });

  test('single tap on a quick-log item still logs instantly without opening log sheet', async ({ page }) => {
    await setupWithChores(page);

    const label = page.locator('.progress-label');
    await expect(label).toContainText(/0 of \d+ done/);

    // Open quick-log sheet via the FAB.
    await page.locator('.fab').click();
    await page.waitForTimeout(400);

    // Short tap on the first chore item — should log instantly.
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1200);

    // Sheet closes and progress counter increments by one.
    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
    await expect(label).toContainText(/1 of \d+ done/);
  });

  test('log sheet opened from quick-log item shows save-log button and closes on save', async ({ page }) => {
    await setupWithChores(page);

    const label = page.locator('.progress-label');
    await expect(label).toContainText(/0 of \d+ done/);

    // Open quick-log sheet via FAB.
    await page.locator('.fab').click();
    await page.waitForTimeout(400);

    // Long-press first item to open log detail sheet.
    const item = page.locator('.sheet-chore-item').first();
    await longPressItem(page, item);
    await expect(page.locator('[data-action="save-log"]')).toBeVisible();

    // Optionally fill in a note then save.
    await page.fill('#log-note', 'test note from long-press');
    await page.locator('[data-action="save-log"]').click();
    await page.waitForTimeout(1200);

    // Sheet closes and progress counter increments.
    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
    await expect(label).toContainText(/1 of \d+ done/);
  });

  test('pick-chore sheet shows the discoverability hint text', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="8"]').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.sheet-hint')).toContainText('Hold to log');
  });

  test('quick-log sheet shows the discoverability hint text', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.fab').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.sheet-hint')).toContainText('Hold to add notes');
  });
});

// ─── Calendar scroll position preservation ────────────────────────────────────

test.describe('Calendar scroll position: preserved across sheet open/close', () => {
  /**
   * Returns the scrollTop of the day-hour-grid-wrapper, or -1 if not found.
   */
  function getWrapperScroll(page) {
    return page.evaluate(() => {
      const w = document.querySelector('.day-hour-grid-wrapper');
      return w ? w.scrollTop : -1;
    });
  }

  /**
   * Sets the day-hour-grid-wrapper scrollTop directly.
   */
  function setWrapperScroll(page, px) {
    return page.evaluate((px) => {
      const w = document.querySelector('.day-hour-grid-wrapper');
      if (w) w.scrollTop = px;
    }, px);
  }

  test('scroll position is preserved when sheet opens and closes (non-zero)', async ({ page }) => {
    await setupWithChores(page);

    // Scroll to 10 AM (10 rows × 48 px = 480) — safely within bounds for all viewport sizes.
    await setWrapperScroll(page, 480);

    // Open pick-chore sheet by clicking the 10 AM slot.
    await page.getByRole('button', { name: '10 AM' }).click();
    await page.waitForSelector('.bottom-sheet');

    const scrollAfterOpen = await getWrapperScroll(page);
    expect(scrollAfterOpen).toBe(480);

    // Close the sheet via the backdrop.
    await page.locator('.sheet-backdrop').click({ force: true });
    await page.waitForFunction(() => !document.querySelector('.bottom-sheet'));

    const scrollAfterClose = await getWrapperScroll(page);
    expect(scrollAfterClose).toBe(480);
  });

  test('scroll position stays at 0 (midnight) when sheet opens — no spurious auto-scroll', async ({ page }) => {
    await setupWithChores(page);

    // Scroll to midnight (top).
    await setWrapperScroll(page, 0);

    // Open pick-chore sheet from the midnight slot.
    await page.getByRole('button', { name: '12 AM' }).click();
    await page.waitForSelector('.bottom-sheet');

    const scrollAfterOpen = await getWrapperScroll(page);
    expect(scrollAfterOpen).toBe(0);

    // Close the sheet via the backdrop.
    await page.locator('.sheet-backdrop').click({ force: true });
    await page.waitForFunction(() => !document.querySelector('.bottom-sheet'));

    const scrollAfterClose = await getWrapperScroll(page);
    expect(scrollAfterClose).toBe(0);
  });

  test('scroll position is preserved after scheduling a chore from the sheet', async ({ page }) => {
    await setupWithChores(page);

    // Scroll to 10 AM (within bounds for all viewport sizes).
    await setWrapperScroll(page, 480);

    // Open pick-chore sheet and schedule the first chore.
    await page.getByRole('button', { name: '10 AM' }).click();
    await page.waitForSelector('.bottom-sheet');
    await page.locator('[data-action="schedule-chore-here"]').first().click();
    await page.waitForFunction(() => !document.querySelector('.bottom-sheet'), { timeout: 8000 });

    const scrollAfterSchedule = await getWrapperScroll(page);
    expect(scrollAfterSchedule).toBe(480);
  });

  test('scroll stays at 0 after scheduling a chore from the midnight slot', async ({ page }) => {
    await setupWithChores(page);

    // Scroll to midnight.
    await setWrapperScroll(page, 0);

    // Open pick-chore sheet from midnight and schedule.
    await page.getByRole('button', { name: '12 AM' }).click();
    await page.waitForSelector('.bottom-sheet');
    await page.locator('[data-action="schedule-chore-here"]').first().click();
    await page.waitForFunction(() => !document.querySelector('.bottom-sheet'), { timeout: 8000 });

    const scrollAfterSchedule = await getWrapperScroll(page);
    expect(scrollAfterSchedule).toBe(0);
  });
});
