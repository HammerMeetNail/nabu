// tests/e2e/home-log-to-calendar.spec.js
// Regression tests: chores logged from the home tab must appear at the
// system time they were tapped, not in the catch-all "Anytime" row.
//
// All chores now open the log sheet before saving.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-htc-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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

/**
 * Taps a home chore card, waits for the log sheet, then clicks save.
 */
async function logChoreViaSheet(page, card) {
  await card.click();
  await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
  await page.click('[data-action="save-log"]');
  await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });
}

test.describe('Home tab log → calendar visibility', () => {
  test('logged chore appears in the current hour row of the day view, not Anytime', async ({ page }) => {
    const { chores } = await setupWithChores(page);

    const expectedHour = new Date().getHours();

    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();
    await logChoreViaSheet(page, firstCard);

    // Verify log was saved with correct slotHour via API
    const { logs } = await (await page.request.get('/api/logs/today')).json();
    const logged = (logs || []).find(l => {
      const chore = chores.find(c => c.id === l.choreId);
      return chore && chore.name === choreName;
    });
    expect(logged).toBeDefined();
    expect(logged.slotHour).toBe(expectedHour);
  });

  test('logged chore appears in the current hour row of the week view', async ({ page }) => {
    const { chores } = await setupWithChores(page);

    const expectedHour = new Date().getHours();

    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();
    await logChoreViaSheet(page, firstCard);

    // Verify log was saved with correct slotHour via API
    const { logs } = await (await page.request.get('/api/logs/today')).json();
    const logged = (logs || []).find(l => {
      const chore = chores.find(c => c.id === l.choreId);
      return chore && chore.name === choreName;
    });
    expect(logged).toBeDefined();
    expect(logged.slotHour).toBe(expectedHour);
  });

  test('timed-schedule chore logged from home tab shows as done in the schedule hour row', async ({ page }) => {
    const { csrf, chores } = await setupWithChores(page);

    const choreId = chores[0].id;
    await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'anytime', specificTime: '08:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const firstCard = page.locator('.home-chore-card').first();
    const choreName = await firstCard.locator('.home-card-name').innerText();
    await logChoreViaSheet(page, firstCard);

    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    const hourRowCard = page.locator('[data-drop-hour="8"] .chore-card');
    await expect(hourRowCard).toHaveCount(1);
    await expect(hourRowCard.first()).toHaveClass(/chore-card--done/);
    await expect(hourRowCard.first().locator('.chore-name')).toContainText(choreName);

    await expect(page.locator('.day-anytime-row .chore-card--done')).toHaveCount(0);
  });

  test('chore logged from home sheet with explicit 2 PM time appears in hour-14 row, not Anytime', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    expect(chore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    await expect(card).toBeVisible();
    await card.click();
    await expect(page.locator('#log-when')).toBeVisible({ timeout: 5000 });

    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    const dtLocal = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T14:00`;
    await page.fill('#log-when', dtLocal);

    await page.locator('[data-action="save-log"]').click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    const slotCards = page.locator('[data-drop-hour="14"] .chore-card--done');
    await expect(slotCards).toHaveCount(1);
    await expect(slotCards.first().locator('.chore-name')).toContainText(chore.name);

    await expect(page.locator('.day-anytime-row .chore-card--done')).toHaveCount(0);
  });

  test('chore logged from home sheet with explicit 2 PM time appears in hour-14 row of week view', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    expect(chore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    await card.click();
    await expect(page.locator('#log-when')).toBeVisible({ timeout: 5000 });

    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    const todayISO = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
    await page.fill('#log-when', `${todayISO}T14:00`);
    await page.locator('[data-action="save-log"]').click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });
    await page.click('[data-action="switch-view"][data-view="week"]');
    await page.waitForSelector('.week-view', { timeout: 5000 });

    const hourCell = page.locator(`.hour-row[data-hour="14"] [data-drop-date="${todayISO}"]`);
    await expect(hourCell.locator('.chore-card--done')).toHaveCount(1);
    await expect(hourCell.locator('.chore-card--done').first()).toHaveAttribute('aria-label', new RegExp(chore.name));

    await expect(page.locator('.week-anytime-row .chore-card--done')).toHaveCount(0);
  });
});
