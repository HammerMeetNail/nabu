// tests/e2e/home-log-to-calendar.spec.js
// Regression tests: chores logged from the home tab must appear at the
// system time they were tapped, not in the catch-all "Anytime" row.
//
// Original bug 1: renderDayView / renderWeekView only rendered logs that had a
// slotHour or a matching timed schedule.  Home-tab logs (slotHour === null)
// were invisible in the calendar even though /api/logs/today returned them.
//
// Original bug 2: home-tap-chore (no-indicator path) passed slotHour=null so
// chores always landed in "Anytime" rather than the current hour row.
//
// Original bug 3: save-home-log (indicator sheet) also passed slotHour=null
// so the datetime-local "When" picker had no effect on calendar placement.
//
// Original bug 4: renderWeekView only showed schedules in hour rows; ad-hoc
// logs with a slotHour never appeared in the week grid.

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-htc-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers a new user, creates a household, seeds default chores, and waits
 * for the home grid to be visible.  Returns { email, csrf, chores }.
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
    data: { name: `HTCal Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  const chores = (await (await page.request.get('/api/chores')).json()).chores || [];

  return { email, csrf, chores };
}

// ─── Tests ────────────────────────────────────────────────────────────────────

test.describe('Home tab log → calendar visibility', () => {
  test('direct-tap chore appears in the current hour row of the day view, not Anytime', async ({ page }) => {
    // Regression: home-tap-chore (no indicator labels) always passed
    // slotHour=null so chores landed in "Anytime" regardless of the clock.
    await setupWithChores(page);

    // Record the current hour before tapping so we can find it in the grid.
    const expectedHour = new Date().getHours();

    // Tap the first no-indicator chore card (logs instantly, no sheet).
    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();
    await firstCard.click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Navigate to the calendar (day view).
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // The chore must appear as done in the current-hour row.
    const hourCard = page.locator(`[data-drop-hour="${expectedHour}"] .chore-card--done`);
    await expect(hourCard).toHaveCount(1);
    await expect(hourCard.first().locator('.chore-name')).toContainText(choreName);

    // The Anytime row must NOT contain a done card for this chore.
    await expect(page.locator('.day-anytime-row .chore-card--done')).toHaveCount(0);
  });

  test('direct-tap chore appears in the current hour row of the week view', async ({ page }) => {
    // Regression: week view hour rows only showed scheduled chores; ad-hoc
    // logs with slotHour never appeared in the week grid.
    await setupWithChores(page);

    const expectedHour = new Date().getHours();

    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();
    await firstCard.click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Navigate to the calendar, then switch to week view.
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });
    await page.click('[data-action="switch-view"][data-view="week"]');
    await page.waitForSelector('.week-view', { timeout: 5000 });

    // The chore must appear as done in today's cell of the current-hour row.
    const today = new Date();
    const pad = n => String(n).padStart(2, '0');
    const todayISO = `${today.getFullYear()}-${pad(today.getMonth() + 1)}-${pad(today.getDate())}`;

    const hourCell = page.locator(`.hour-row[data-hour="${expectedHour}"] [data-drop-date="${todayISO}"]`);
    await expect(hourCell.locator('.chore-card--done')).toHaveCount(1);
    await expect(hourCell.locator('.chore-card--done').first()).toHaveAttribute('aria-label', new RegExp(choreName));

    // The week-view Anytime row must NOT contain a done card.
    await expect(page.locator('.week-anytime-row .chore-card--done')).toHaveCount(0);
  });

  test('timed-schedule chore logged from home tab shows as done in the schedule hour row', async ({ page }) => {
    // Previously the home-tap log had slotHour=null and went to Anytime; the
    // timed schedule still showed the card as done at 8 AM via the logMap.
    // Now the home-tap log carries slotHour=currentHour.  The 8 AM schedule
    // must still show as done regardless of what hour the test runs.
    const { csrf, chores } = await setupWithChores(page);

    // Schedule the first chore at 08:00 daily.
    const choreId = chores[0].id;
    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Log the chore from the home tab (gets slotHour = current hour).
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();
    await firstCard.click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Navigate to day view.
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // The 8 AM hour row must show the chore as done (via the schedule + logMap).
    const hourRowCard = page.locator('[data-drop-hour="8"] .chore-card');
    await expect(hourRowCard).toHaveCount(1);
    await expect(hourRowCard.first()).toHaveClass(/chore-card--done/);
    await expect(hourRowCard.first().locator('.chore-name')).toContainText(choreName);

    // No log should appear in the Anytime row.
    await expect(page.locator('.day-anytime-row .chore-card--done')).toHaveCount(0);
  });

  test('chore logged from home sheet with explicit 2 PM time appears in hour-14 row, not Anytime', async ({ page }) => {
    // Regression: save-home-log always passed slotHour=null even when the user
    // had set a specific time via #home-log-when.  The calendar uses slotHour
    // (not completedAt) to place logs, so the chore landed in "Anytime".
    const { chores } = await setupWithChores(page);

    // Find a chore that has indicator labels — tapping it opens the home-log
    // sheet (with the datetime-local time picker) instead of logging instantly.
    // "Change Baby" is seeded with indicators ["💩 poo", "💛 pee"].
    const choreWithIndicators = chores.find(c => c.indicatorLabels && c.indicatorLabels.length > 0);
    expect(choreWithIndicators).toBeDefined();

    // Tap the chore card to open the home-log sheet.
    const card = page.locator(`.home-chore-card[data-home-chore-id="${choreWithIndicators.id}"]`);
    await expect(card).toBeVisible();
    await card.click();

    // The home-log sheet must open with the time picker.
    await expect(page.locator('#home-log-when')).toBeVisible({ timeout: 5000 });

    // Set the time to 2 PM (14:00) today via the datetime-local input.
    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    const dtLocal = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T14:00`;
    await page.fill('#home-log-when', dtLocal);

    // Submit the log.
    await page.locator('[data-action="save-home-log"]').click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Navigate to calendar day view.
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // The chore must appear as done in the 2 PM (hour 14) row.
    const slotCards = page.locator('[data-drop-hour="14"] .chore-card--done');
    await expect(slotCards).toHaveCount(1);
    await expect(slotCards.first().locator('.chore-name')).toContainText(choreWithIndicators.name);

    // The chore must NOT appear in the Anytime row.
    await expect(page.locator('.day-anytime-row .chore-card--done')).toHaveCount(0);
  });

  test('chore logged from home sheet with explicit 2 PM time appears in hour-14 row of week view', async ({ page }) => {
    // Regression: week view did not show ad-hoc logs in hour rows at all.
    const { chores } = await setupWithChores(page);

    const choreWithIndicators = chores.find(c => c.indicatorLabels && c.indicatorLabels.length > 0);
    expect(choreWithIndicators).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${choreWithIndicators.id}"]`);
    await card.click();
    await expect(page.locator('#home-log-when')).toBeVisible({ timeout: 5000 });

    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    const todayISO = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
    await page.fill('#home-log-when', `${todayISO}T14:00`);
    await page.locator('[data-action="save-home-log"]').click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Switch to week view.
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });
    await page.click('[data-action="switch-view"][data-view="week"]');
    await page.waitForSelector('.week-view', { timeout: 5000 });

    // The chore must appear as done in today's 2 PM cell of the week grid.
    const hourCell = page.locator(`.hour-row[data-hour="14"] [data-drop-date="${todayISO}"]`);
    await expect(hourCell.locator('.chore-card--done')).toHaveCount(1);
    await expect(hourCell.locator('.chore-card--done').first()).toHaveAttribute('aria-label', new RegExp(choreWithIndicators.name));

    // No done card in the Anytime row.
    await expect(page.locator('.week-anytime-row .chore-card--done')).toHaveCount(0);
  });
});
