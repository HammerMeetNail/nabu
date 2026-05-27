// tests/e2e/home-grid.spec.js
// End-to-end tests for the home grid view (the default "/" route).
//
// After the redesign the home grid is the landing page for authenticated users.
// The calendar/day view is reached via the Calendar tab.  Seeded default chores:
//   • "Feed Cats" (SortOrder 0) — no indicator labels → instant log on tap
//   • "Change Baby" (SortOrder 2) — indicators ["💩 poo", "💛 pee"] → opens sheet

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-home-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers a new user, creates a household, seeds default chores, and waits
 * for the home grid to be visible.  Returns { email, csrf }.
 */
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
    data: { name: `Home Grid Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  // Home grid is the default authenticated view — wait for the grid, not .cal-date.
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { email, csrf };
}

/**
 * Simulates a long-press (≥500 ms) on a locator by holding mousedown.
 * Matches the 500 ms threshold in app.js.
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

// ─── Home Grid: Rendering ─────────────────────────────────────────────────────

test.describe('Home Grid: Rendering', () => {
  test('home grid renders chore cards after login', async ({ page }) => {
    await setupWithChores(page);

    await expect(page.locator('.home-grid')).toBeVisible();
    // 14 default seeded chores
    await expect(page.locator('.home-chore-card')).toHaveCount(14);

    // Each card has icon, name, and a time label
    const first = page.locator('.home-chore-card').first();
    await expect(first.locator('.home-card-icon')).toBeVisible();
    await expect(first.locator('.home-card-name')).toBeVisible();
    await expect(first.locator('.home-card-time')).toBeVisible();
  });

  test('home grid is the default tab and marks "today" tab active', async ({ page }) => {
    await setupWithChores(page);

    const todayTab = page.locator('.tab-item[data-nav="today"]');
    await expect(todayTab).toHaveClass(/active/);

    const calTab = page.locator('.tab-item[data-nav="calendar"]');
    await expect(calTab).not.toHaveClass(/active/);
  });

  test('unlogged chore cards show "never" time label', async ({ page }) => {
    await setupWithChores(page);

    // Every card should start as "never" before any logging
    const neverCount = await page.locator('.home-card-time--never').count();
    expect(neverCount).toBe(14);
    await expect(page.locator('.home-card-time--never').first()).toContainText('never');
  });
});

// ─── Home Grid: Log Sheet ────────────────────────────────────────────────────

test.describe('Home Grid: Log Sheet', () => {
  test('tapping any card opens the log sheet and saving logs it', async ({ page }) => {
    await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();

    await firstCard.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-action="save-home-log"]')).toBeVisible();

    await page.click('[data-action="save-home-log"]');

    const toast = page.locator('#toast-container .toast');
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toContainText(choreName);
    await expect(toast.locator('button')).toContainText('Undo');
  });

  test('after saving a log the time-ago label updates from "never"', async ({ page }) => {
    await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    await expect(firstCard.locator('.home-card-time--never')).toBeVisible();

    await firstCard.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await page.click('[data-action="save-home-log"]');

    // After: "never" class gone; label shows "just now" or a relative time
    await expect(firstCard.locator('.home-card-time--never')).toHaveCount(0);
    await expect(firstCard.locator('.home-card-time')).toContainText(/ago|just now/);
  });

  test('undo button in toast removes the log entry', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    const choreId = chores[0].id;

    // Tap first card, save log via sheet
    await page.locator('.home-chore-card').first().click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await page.click('[data-action="save-home-log"]');

    // Verify a log was created via latest-per-chore
    let latest = (await (await page.request.get('/api/logs/latest-per-chore')).json()).latestLogs;
    expect(latest[String(choreId)]).toBeDefined();

    // Click Undo
    const undoBtn = page.locator('#toast-container .toast button');
    await expect(undoBtn).toBeVisible({ timeout: 5000 });
    await undoBtn.click();
    await page.waitForTimeout(1500);

    // Log should be removed from latest-per-chore
    latest = (await (await page.request.get('/api/logs/latest-per-chore')).json()).latestLogs;
    expect(latest[String(choreId)]).toBeUndefined();
  });
});

// ─── Home Grid: Log Sheet Details ───────────────────────────────────────────

test.describe('Home Grid: Log Sheet Details', () => {
  async function tapChangeBaby(page) {
    const cards = page.locator('.home-chore-card');
    const count = await cards.count();
    for (let i = 0; i < count; i++) {
      const name = await cards.nth(i).locator('.home-card-name').innerText();
      if (name === 'Change Baby') {
        await cards.nth(i).click();
        return;
      }
    }
    throw new Error('Change Baby chore card not found');
  }

  test('every card opens the log sheet', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.home-chore-card').first().click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-action="save-home-log"]')).toBeVisible();
  });

  test('indicator chips are visible and toggleable in the sheet', async ({ page }) => {
    await setupWithChores(page);

    await tapChangeBaby(page);
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Chips should match Change Baby's labels
    const chips = page.locator('.log-chip');
    await expect(chips).toHaveCount(2);

    const firstChip = chips.first();
    await expect(firstChip).toBeVisible();
    // Toggle on
    await firstChip.click();
    await expect(firstChip).toHaveClass(/log-chip--on/);
    // Toggle off
    await firstChip.click();
    await expect(firstChip).not.toHaveClass(/log-chip--on/);
  });

  test('log sheet datetime-local input is pre-filled with current time', async ({ page }) => {
    await setupWithChores(page);

    await tapChangeBaby(page);
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    const whenInput = page.locator('#home-log-when');
    await expect(whenInput).toBeVisible();
    const value = await whenInput.inputValue();
    // Should be a valid datetime-local format: "YYYY-MM-DDTHH:MM"
    expect(value).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/);
  });

  test('log sheet saves with a note and closes', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await tapChangeBaby(page);
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Fill note
    await page.fill('#home-log-note', 'E2E test note');

    // Select a chip
    await page.locator('.log-chip').first().click();

    // Save
    await page.locator('[data-action="save-home-log"]').click();
    await page.waitForTimeout(1500);

    // Sheet should be gone
    await expect(page.locator('.bottom-sheet')).toHaveCount(0);

    // Toast should appear confirming the log
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Log should be in latest-per-chore
    const { chores } = await (await page.request.get('/api/chores')).json();
    const baby = chores.find(c => c.name === 'Change Baby');
    const latest = (await (await page.request.get('/api/logs/latest-per-chore')).json()).latestLogs;
    expect(latest[String(baby.id)]).toBeDefined();
  });

  test('log sheet saves with a backdated completedAt', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await tapChangeBaby(page);
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Backdate to 2 hours ago
    const twoHoursAgo = new Date(Date.now() - 2 * 3600 * 1000);
    const pad = n => String(n).padStart(2, '0');
    const dtValue = `${twoHoursAgo.getFullYear()}-${pad(twoHoursAgo.getMonth()+1)}-${pad(twoHoursAgo.getDate())}T${pad(twoHoursAgo.getHours())}:${pad(twoHoursAgo.getMinutes())}`;
    await page.fill('#home-log-when', dtValue);

    await page.locator('[data-action="save-home-log"]').click();
    await page.waitForTimeout(1500);

    await expect(page.locator('.bottom-sheet')).toHaveCount(0);

    // Check the stored completedAt is close to what we set
    const { chores } = await (await page.request.get('/api/chores')).json();
    const baby = chores.find(c => c.name === 'Change Baby');
    const latest = (await (await page.request.get('/api/logs/latest-per-chore')).json()).latestLogs;
    const storedLog = latest[String(baby.id)];
    expect(storedLog).toBeDefined();
    const diff = Math.abs(new Date(storedLog.completedAt).getTime() - twoHoursAgo.getTime());
    // Allow up to 2 minutes tolerance (datetime-local drops seconds)
    expect(diff).toBeLessThan(2 * 60 * 1000);
  });
});

// ─── Home Grid: Jiggle Mode ───────────────────────────────────────────────────

test.describe('Home Grid: Jiggle Mode', () => {
  test('long-pressing a card enters jiggle mode', async ({ page }) => {
    await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    await longPress(page, firstCard);
    await page.waitForTimeout(200);

    // Cards should have the jiggle class
    await expect(page.locator('.home-chore-card--jiggle').first()).toBeVisible({ timeout: 3000 });
    // "Done" button should appear
    await expect(page.locator('[data-action="exit-jiggle-mode"]')).toBeVisible({ timeout: 3000 });
  });

  test('"Done" button exits jiggle mode', async ({ page }) => {
    await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    await longPress(page, firstCard);
    await page.waitForTimeout(200);

    await expect(page.locator('[data-action="exit-jiggle-mode"]')).toBeVisible({ timeout: 3000 });

    await page.locator('[data-action="exit-jiggle-mode"]').click();
    await page.waitForTimeout(300);

    // Jiggle class and Done button should be gone
    await expect(page.locator('.home-chore-card--jiggle')).toHaveCount(0);
    await expect(page.locator('[data-action="exit-jiggle-mode"]')).toHaveCount(0);
  });

  test('in jiggle mode cards have the reorder drag attribute', async ({ page }) => {
    await setupWithChores(page);

    const firstCard = page.locator('.home-chore-card').first();
    await longPress(page, firstCard);
    await page.waitForTimeout(200);

    await expect(page.locator('[data-action="exit-jiggle-mode"]')).toBeVisible({ timeout: 3000 });

    // Cards in jiggle mode get data-home-reorder-chore-id
    const reorderCards = page.locator('[data-home-reorder-chore-id]');
    await expect(reorderCards).toHaveCount(14);
  });
});

// ─── Home Grid: Tab Navigation ────────────────────────────────────────────────

test.describe('Home Grid: Tab Navigation', () => {
  test('Calendar tab navigates to the calendar view (.cal-date visible)', async ({ page }) => {
    await setupWithChores(page);

    await page.click('[data-nav="calendar"]');
    await expect(page.locator('.cal-date')).toBeVisible({ timeout: 10000 });
  });

  test('Home tab returns to the home grid from calendar', async ({ page }) => {
    await setupWithChores(page);

    // Navigate to calendar
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 10000 });

    // Navigate back
    await page.click('[data-nav="today"]');
    await expect(page.locator('.home-grid')).toBeVisible({ timeout: 5000 });
  });
});

// ─── Home Grid: API ───────────────────────────────────────────────────────────

test.describe('Home Grid: API', () => {
  test('GET /api/logs/latest-per-chore returns a map with the logged choreId', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    const choreId = chores[0].id;

    // Create a log via the API
    const logResp = await page.request.post('/api/logs', {
      data: { choreId, note: '', indicators: [] },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(logResp.status()).toBe(201);

    // Fetch latest-per-chore
    const resp = await page.request.get('/api/logs/latest-per-chore');
    expect(resp.status()).toBe(200);
    const { latestLogs } = await resp.json();
    expect(latestLogs).toBeDefined();
    expect(latestLogs[String(choreId)]).toBeDefined();
    expect(latestLogs[String(choreId)].choreId).toBe(choreId);
  });

  test('GET /api/logs/latest-per-chore returns empty map when no logs exist', async ({ page }) => {
    await setupWithChores(page);

    const resp = await page.request.get('/api/logs/latest-per-chore');
    expect(resp.status()).toBe(200);
    const { latestLogs } = await resp.json();
    expect(latestLogs).toBeDefined();
    expect(Object.keys(latestLogs)).toHaveLength(0);
  });

  test('POST /api/logs with completedAt stores the provided timestamp', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    const choreId = chores[0].id;

    const past = new Date(Date.now() - 2 * 3600 * 1000); // 2 hours ago
    const completedAt = past.toISOString();

    const resp = await page.request.post('/api/logs', {
      data: { choreId, note: '', indicators: [], completedAt },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const { log } = await resp.json();
    expect(log).toBeDefined();

    // completedAt should be within 1 minute of what we sent
    const diff = Math.abs(new Date(log.completedAt).getTime() - past.getTime());
    expect(diff).toBeLessThan(60 * 1000);
  });

  test('GET /api/logs/latest-per-chore returns only the most recent log per chore', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    const choreId = chores[0].id;

    // Log the same chore twice on different days (dedup is per-day).
    // "older" is yesterday, "newer" is today — latest-per-chore must return today's.
    const older = new Date(Date.now() - 25 * 3600 * 1000).toISOString(); // ~yesterday
    const newer = new Date(Date.now() - 300 * 1000).toISOString();       // 5 minutes ago

    await page.request.post('/api/logs', {
      data: { choreId, note: 'older', indicators: [], completedAt: older },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.request.post('/api/logs', {
      data: { choreId, note: 'newer', indicators: [], completedAt: newer },
      headers: { 'X-CSRF-Token': csrf },
    });

    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    const latest = latestLogs[String(choreId)];
    expect(latest).toBeDefined();
    // Should be the newer log
    expect(latest.note).toBe('newer');
  });
});
