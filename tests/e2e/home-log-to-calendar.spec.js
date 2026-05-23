// tests/e2e/home-log-to-calendar.spec.js
// Regression tests for: a chore logged from the home tab (no slotHour) must
// appear in the "Anytime" row of the calendar day view and week view.
//
// Bug: renderDayView / renderWeekView only rendered logs that had a slotHour
// or a matching timed schedule.  Home-tab logs (slotHour === null) were
// invisible in the calendar even though /api/logs/today returned them.
//
// Also covers: Bug where a chore logged from the home sheet with an explicit
// time set via #home-log-when appeared in "Anytime" instead of the correct
// hour row because save-home-log always passed slotHour=null.

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
  test('chore logged from home tab appears in day view Anytime row', async ({ page }) => {
    const { chores } = await setupWithChores(page);

    // Tap the first chore card (Feed Cats — no indicator labels, logs instantly).
    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();
    await firstCard.click();
    // Wait for the toast to confirm the log was created.
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Navigate to the calendar (day view).
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // The Anytime row must be present and contain the logged chore as done.
    const anytimeRow = page.locator('.day-anytime-row');
    await expect(anytimeRow).toBeVisible();

    const card = anytimeRow.locator('.chore-card');
    await expect(card).toHaveCount(1);
    await expect(card.first()).toHaveClass(/chore-card--done/);
    await expect(card.first().locator('.chore-name')).toContainText(choreName);

    // The card must NOT appear in any timed hour row (it has no slotHour).
    await expect(page.locator('.day-hour-row .chore-card')).toHaveCount(0);
  });

  test('chore logged from home tab appears in week view Anytime row for today', async ({ page }) => {
    await setupWithChores(page);

    // Log the first chore from the home tab.
    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();
    await firstCard.click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Navigate to the calendar, then switch to week view.
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });
    await page.click('[data-action="switch-view"][data-view="week"]');
    await page.waitForSelector('.week-view', { timeout: 5000 });

    // The week-view Anytime row must be visible.
    const anytimeRow = page.locator('.week-anytime-row');
    await expect(anytimeRow).toBeVisible();

    // At least one week-chore-card for the logged chore must be done.
    const doneCards = anytimeRow.locator('.week-chore-card.chore-card--done');
    await expect(doneCards).toHaveCount(1);

    // Verify the chore name is correct.
    await expect(doneCards.first()).toHaveAttribute('aria-label', new RegExp(choreName));
  });

  test('chore with timed schedule logged from home tab appears in BOTH Anytime row and its hour row', async ({ page }) => {
    const { csrf, chores } = await setupWithChores(page);

    // Schedule the first chore at 08:00 daily.
    const choreId = chores[0].id;
    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Log the chore from the home tab (no slotHour → anytime log).
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();
    await firstCard.click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Navigate to day view.
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // The Anytime row must contain the done card.
    const anytimeRow = page.locator('.day-anytime-row');
    await expect(anytimeRow).toBeVisible();
    const anytimeCard = anytimeRow.locator('.chore-card');
    await expect(anytimeCard).toHaveCount(1);
    await expect(anytimeCard.first()).toHaveClass(/chore-card--done/);
    await expect(anytimeCard.first().locator('.chore-name')).toContainText(choreName);

    // The 8 AM hour row must ALSO show the chore as done (via the schedule).
    const hourRowCard = page.locator('[data-drop-hour="8"] .chore-card');
    await expect(hourRowCard).toHaveCount(1);
    await expect(hourRowCard.first()).toHaveClass(/chore-card--done/);
    await expect(hourRowCard.first().locator('.chore-name')).toContainText(choreName);
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
});
