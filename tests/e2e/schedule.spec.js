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

  test('renders all five period sections', async ({ page }) => {
    await setupWithChores(page);

    for (const period of ['morning', 'afternoon', 'evening', 'night', 'anytime']) {
      await expect(page.locator(`[data-period="${period}"]`)).toBeVisible();
    }
  });

  test('each period section has a heading with icon', async ({ page }) => {
    await setupWithChores(page);

    const headings = page.locator('.period-heading');
    const count = await headings.count();
    expect(count).toBe(5); // morning, afternoon, evening, night, anytime
    for (let i = 0; i < count; i++) {
      await expect(headings.nth(i).locator('.period-icon')).toBeVisible();
    }
  });

  test('non-anytime periods show an "Add chore" slot button', async ({ page }) => {
    await setupWithChores(page);

    for (const period of ['morning', 'afternoon', 'evening', 'night']) {
      const addBtn = page.locator(
        `[data-period="${period}"] button[data-action="open-pick-chore-sheet"]`
      );
      await expect(addBtn).toBeVisible();
      await expect(addBtn).toContainText('Add chore');
    }
  });

  test('anytime period does NOT have an "Add chore" slot button', async ({ page }) => {
    await setupWithChores(page);

    const addBtn = page.locator(
      '[data-period="anytime"] button[data-action="open-pick-chore-sheet"]'
    );
    await expect(addBtn).toHaveCount(0);
  });

  test('chore cards are draggable', async ({ page }) => {
    await setupWithChores(page);

    // All chores start in the anytime bucket (no schedule yet).
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
    await expect(page.locator('.period-sections')).toBeVisible();
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

  test('hour rows carry period band class names', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(800);

    for (const band of ['hour-row--morning', 'hour-row--afternoon', 'hour-row--evening', 'hour-row--night']) {
      await expect(page.locator(`.${band}`).first()).toBeVisible();
    }
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
  test('clicking Add chore in a period opens the sheet', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await expect(page.locator('.sheet-title')).toContainText('Morning');
  });

  test('sheet lists all household chores', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.sheet-chore-item')).toHaveCount(12);
    await expect(page.locator('.sheet-chore-item').first()).toContainText('Feed Cats (Morning)');
  });

  test('Cancel button closes the sheet', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);

    await page.locator('.bottom-sheet button[data-action="close-sheet"]').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  });

  test('clicking the backdrop closes the sheet', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);

    await page.locator('.sheet-backdrop').click({ force: true });
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  });

  test('sheet is rendered as a dialog with aria-modal', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);

    const sheet = page.locator('.bottom-sheet');
    await expect(sheet).toHaveAttribute('role', 'dialog');
    await expect(sheet).toHaveAttribute('aria-modal', 'true');
  });

  test('each sheet chore item has schedule-chore-here action with correct time-period', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-period="evening"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);

    const item = page.locator('.sheet-chore-item').first();
    await expect(item).toHaveAttribute('data-action', 'schedule-chore-here');
    await expect(item).toHaveAttribute('data-time-period', 'evening');
  });

  test('picking a chore from the sheet closes the sheet', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  });

  test('after scheduling, chore card appears in the target period section', async ({ page }) => {
    await setupWithChores(page);

    // Before: all chores start in anytime (no schedules yet)
    const initialCount = await page.locator('[data-period="anytime"] .chore-card').count();
    expect(initialCount).toBeGreaterThan(0);

    // Schedule first chore in morning via the sheet
    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    // After: chore should be in morning, anytime drops by one
    await expect(page.locator('[data-period="morning"] .chore-card')).toHaveCount(1);
    await expect(page.locator('[data-period="anytime"] .chore-card')).toHaveCount(initialCount - 1);
  });

  test('already-scheduled chores are excluded from the sheet list', async ({ page }) => {
    await setupWithChores(page);

    // Open morning sheet and capture the name of the first item before scheduling
    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);
    const firstName = await page.locator('.sheet-chore-item').first().locator('.chore-name').innerText();
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    // Open the morning sheet again — the scheduled chore should no longer appear
    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);

    const items = page.locator('.sheet-chore-item');
    const count = await items.count();
    // 11 of 12 chores remain (one was scheduled)
    expect(count).toBe(11);
    // The scheduled chore should not be in the list
    const names = await page.locator('.sheet-chore-item .chore-name').allInnerTexts();
    expect(names).not.toContain(firstName);

    // Clean up
    await page.locator('.bottom-sheet button[data-action="close-sheet"]').click();
  });

  test('shows empty message when all chores are already scheduled', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Schedule every chore in morning via the API so the sheet will be empty
    const { chores } = await (await page.request.get('/api/chores')).json();
    for (const chore of chores) {
      await page.request.post('/api/schedules', {
        data: { choreId: chore.id, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
        headers: { 'X-CSRF-Token': csrf },
      });
    }

    await page.reload();
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // Open the morning sheet — all chores are scheduled so it should show empty state
    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.sheet-empty')).toBeVisible();
    await expect(page.locator('.sheet-empty')).toContainText('already scheduled');

    await page.locator('.bottom-sheet button[data-action="close-sheet"]').click();
  });
});

// ─── Chore Logging in Day View ────────────────────────────────────────────────

test.describe('Chore Logging: Day View', () => {
  test('clicking an unlogged chore card marks it as done', async ({ page }) => {
    await setupWithChores(page);

    const card = page.locator('.chore-card').first();
    await expect(card).toHaveAttribute('data-action', 'log-chore');
    await card.click();
    await page.waitForTimeout(1000);

    await expect(card).toHaveClass(/chore-card--done/);
    await expect(card).toHaveAttribute('data-action', 'undo-chore');
  });

  test('progress label updates after logging a chore', async ({ page }) => {
    await setupWithChores(page);

    const label = page.locator('.progress-label');
    await expect(label).toContainText(/0 of \d+ done/);

    await page.locator('.chore-card').first().click();
    await page.waitForTimeout(1000);

    await expect(label).toContainText(/1 of \d+ done/);
  });

  test('clicking a done chore undoes it', async ({ page }) => {
    await setupWithChores(page);

    const card = page.locator('.chore-card').first();
    await card.click();
    await page.waitForTimeout(1000);
    await expect(card).toHaveClass(/chore-card--done/);

    await card.click();
    await page.waitForTimeout(1000);
    await expect(card).not.toHaveClass(/chore-card--done/);
    await expect(card).toHaveAttribute('data-action', 'log-chore');
  });

  test('progress label decrements after undoing a chore', async ({ page }) => {
    await setupWithChores(page);

    const card  = page.locator('.chore-card').first();
    const label = page.locator('.progress-label');

    await card.click();
    await page.waitForTimeout(1000);
    await expect(label).toContainText(/1 of \d+ done/);

    await card.click();
    await page.waitForTimeout(1000);
    await expect(label).toContainText(/0 of \d+ done/);
  });

  test('done chore shows check-overlay in day view', async ({ page }) => {
    await setupWithChores(page);

    const card = page.locator('.chore-card').first();
    await card.click();
    await page.waitForTimeout(1000);

    await expect(card.locator('.check-overlay')).toBeVisible();
  });
});

// ─── Drag and Drop: Day View ──────────────────────────────────────────────────

test.describe('Drag and Drop: Day View', () => {
  test('dragging a chore to a period drop-zone moves it there', async ({ page }) => {
    await setupWithChores(page);

    // Count how many chores start in anytime before drag
    const initialCount = await page.locator('[data-period="anytime"] .chore-card').count();
    expect(initialCount).toBeGreaterThan(0);

    const card = page.locator('[data-drag-chore-id]').first();
    const afternoonZone = page.locator('[data-drop-period="afternoon"]');

    await htmlDragDrop(page, card, afternoonZone);
    await page.waitForTimeout(1500);

    // Chore should now be in afternoon; anytime count drops by one
    await expect(page.locator('[data-period="afternoon"] .chore-card')).toHaveCount(1);
    await expect(page.locator('[data-period="anytime"] .chore-card')).toHaveCount(initialCount - 1);
  });

  test('dragging preserves the chore name after move', async ({ page }) => {
    await setupWithChores(page);

    const card = page.locator('[data-drag-chore-id]').first();
    // Capture the name before dragging
    const choreName = await card.locator('.chore-name').innerText();
    const morningZone = page.locator('[data-drop-period="morning"]');

    await htmlDragDrop(page, card, morningZone);
    await page.waitForTimeout(1500);

    await expect(page.locator('[data-period="morning"] .chore-name').first()).toContainText(choreName);
  });

  test('dragging a scheduled chore between periods updates the schedule', async ({ page }) => {
    await setupWithChores(page);

    // First schedule the chore in morning via the sheet
    await page.locator('[data-period="morning"] button[data-action="open-pick-chore-sheet"]').click();
    await page.waitForTimeout(400);
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    await expect(page.locator('[data-period="morning"] .chore-card')).toHaveCount(1);

    // Drag from morning to evening
    const card       = page.locator('[data-period="morning"] [data-drag-chore-id]').first();
    const eveningZone = page.locator('[data-drop-period="evening"]');

    await htmlDragDrop(page, card, eveningZone);
    await page.waitForTimeout(1500);

    await expect(page.locator('[data-period="evening"] .chore-card')).toHaveCount(1);
    await expect(page.locator('[data-period="morning"] .chore-card')).toHaveCount(0);
  });

  test('dragging a chore to night period works', async ({ page }) => {
    await setupWithChores(page);

    const card      = page.locator('[data-drag-chore-id]').first();
    const nightZone = page.locator('[data-drop-period="night"]');

    await htmlDragDrop(page, card, nightZone);
    await page.waitForTimeout(1500);

    await expect(page.locator('[data-period="night"] .chore-card')).toHaveCount(1);
  });
});

// ─── Week View: Anytime Section ───────────────────────────────────────────────

test.describe('Week View: Anytime Section', () => {
  test('anytime section appears when a schedule has timePeriod=anytime', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // The chore is already in anytime bucket (no schedule).
    // Explicitly create an anytime schedule via the API.
    const choreResp = await page.request.get('/api/chores');
    const choreData = await choreResp.json();
    const choreId   = choreData.chores[0].id;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Switch to week view
    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(1000);

    await expect(page.locator('.anytime-week-section')).toBeVisible();
    await expect(page.locator('.anytime-week-section .period-heading')).toContainText('Anytime');
  });

  test('anytime section is absent when no anytime schedules exist', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Schedule the chore specifically in morning (not anytime)
    const choreResp = await page.request.get('/api/chores');
    const choreData = await choreResp.json();
    const choreId   = choreData.chores[0].id;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(1000);

    await expect(page.locator('.anytime-week-section')).toHaveCount(0);
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
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const data = await resp.json();
    expect(data.schedule).toBeDefined();
    expect(data.schedule.choreId).toBe(choreId);
    expect(data.schedule.timePeriod).toBe('morning');
    expect(data.schedule.frequencyType).toBe('daily');
  });

  test('GET /api/schedules lists the newly created schedule', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'evening', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    const listResp = await page.request.get('/api/schedules');
    const list     = (await listResp.json()).schedules;
    expect(list.length).toBe(1);
    expect(list[0].choreId).toBe(choreId);
    expect(list[0].timePeriod).toBe('evening');
  });

  test('PATCH /api/schedules/:id updates timePeriod', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    const createResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    const scheduleId = (await createResp.json()).schedule.id;

    const patchResp = await page.request.patch(`/api/schedules/${scheduleId}`, {
      data: { timePeriod: 'night' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(patchResp.status()).toBe(200);
    const updated = (await patchResp.json()).schedule;
    expect(updated.timePeriod).toBe('night');
  });

  test('PATCH /api/schedules/:id updates specificTime', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const choreId   = (await choreResp.json()).chores[0].id;

    const createResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    const scheduleId = (await createResp.json()).schedule.id;

    const patchResp = await page.request.patch(`/api/schedules/${scheduleId}`, {
      data: { timePeriod: 'morning', specificTime: '08:30' },
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
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
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
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
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
      data: { timePeriod: 'morning', frequencyType: 'daily' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(400);
  });

  test('PATCH /api/schedules/:id returns 404 for non-existent id', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.patch('/api/schedules/999999', {
      data: { timePeriod: 'night' },
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
        timePeriod: 'morning',
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
  test('a daily morning schedule renders a card in the 5 AM hour row', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    // Specifically schedule at 05:00 (first morning hour shown in grid)
    await page.request.post('/api/schedules', {
      data: {
        choreId,
        timePeriod: 'morning',
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
