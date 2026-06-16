// tests/e2e/schedule-edit-bug.spec.js
// Regression test: editing a schedule preserves all fields including daysOfWeek.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-schedbug-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Sched Bug Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.click('[data-nav="activity"]');
  await page.click('[data-action="switch-view"][data-view="day"]');
  await page.waitForSelector('.cal-date', { timeout: 15000 });

  return { email, csrf };
}

/**
 * Navigate day view forward until nextBtn's data-date matches target, or max clicks.
 */
async function navigateToDate(page, targetISO) {
  for (let i = 0; i < 14; i++) {
    const nextBtn = page.locator('button[data-action="navigate-day"]').last();
    const nextDate = await nextBtn.getAttribute('data-date');
    if (nextDate > targetISO) break; // overshot
    await nextBtn.click();
    await page.waitForTimeout(400);
    // The previous-day button's data-date is the current date (the one we just navigated to)
    const prevDate = await page.locator('button[data-action="navigate-day"]').first().getAttribute('data-date');
    if (prevDate >= targetISO) {
      // prevDate is the date that would be returned to; current is one day after that
      // Actually prev button navigates back, so its data-date IS the previous date
      // We need to compare against what we just landed on
      // The next button's original data-date (from before we clicked) was our target
      // After clicking, we're now showing that date. Let's check next button's new data-date:
      const newNext = await page.locator('button[data-action="navigate-day"]').last().getAttribute('data-date');
      if (newNext > targetISO) break;
    }
  }
}

test.describe('Schedule Edit: preserves fields', () => {

  test('editing a weekly schedule preserves all daysOfWeek', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Get Feed Baby chore
    const choreResp = await page.request.get('/api/chores');
    const chores = (await choreResp.json()).chores;
    const feedBaby = chores.find(c => c.name === 'Feed Baby');
    expect(feedBaby).toBeDefined();

    // Use today's weekday plus Wed and Fri.
    const today = new Date();
    const todayWD = today.getDay();
    const days = [...new Set([todayWD, 3, 5])].sort(); // include today's weekday

    // Create a weekly schedule at 08:00
    const createResp = await page.request.post('/api/schedules', {
      data: {
        choreId: feedBaby.id,
        timePeriod: 'anytime',
        specificTime: '08:00',
        frequencyType: 'weekly',
        daysOfWeek: days,
        isActive: true,
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(createResp.status()).toBe(201);
    const created = (await createResp.json()).schedule;
    expect(created.daysOfWeek).toEqual(days);

    // Reload so the card appears in day view
    await page.reload();
    await page.click('[data-nav="activity"]');
    await page.click('[data-action="switch-view"][data-view="day"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // The card should be visible in the 8 AM row (weekly includes today's weekday)
    const card = page.locator('.day-hour-row[data-hour="8"] .chore-card').filter({ hasText: 'Feed Baby' });
    await expect(card.first()).toBeVisible({ timeout: 10000 });

    // Open the edit sheet via the pencil button
    const wrap = page.locator('.day-hour-row[data-hour="8"] .chore-card-wrap').first();
    await wrap.hover();
    await wrap.locator('.chore-card-edit-btn').click();
    await expect(page.locator('#edit-sheet-time')).toBeVisible();

    // Verify all day pills matching the schedule are shown as on
    const onPills = page.locator('.day-pill--on');
    await expect(onPills).toHaveCount(days.length);

    // Click Save without making any changes
    await page.locator('[data-action="save-schedule-edit"]').click();
    await page.waitForTimeout(1500);

    // Sheet should close
    await expect(page.locator('[data-action="save-schedule-edit"]')).not.toBeVisible();

    // Verify via API that daysOfWeek is preserved
    const getResp = await page.request.get('/api/schedules');
    const schedules = (await getResp.json()).schedules;
    const feedBabySch = schedules.find(s => s.choreId === feedBaby.id);
    expect(feedBabySch).toBeDefined();
    expect(feedBabySch.daysOfWeek).toEqual(days);
  });

  test('editing a daily schedule preserves the schedule and card stays visible', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreResp = await page.request.get('/api/chores');
    const chores = (await choreResp.json()).chores;
    const feedBaby = chores.find(c => c.name === 'Feed Baby');

    // Create a daily schedule
    await page.request.post('/api/schedules', {
      data: {
        choreId: feedBaby.id,
        timePeriod: 'anytime',
        specificTime: '09:00',
        frequencyType: 'daily',
        isActive: true,
      },
      headers: { 'X-CSRF-Token': csrf },
    });

    await page.reload();
    await page.click('[data-nav="activity"]');
    await page.click('[data-action="switch-view"][data-view="day"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // Card should be visible in the 9 AM row
    const card = page.locator('.day-hour-row[data-hour="9"] .chore-card').filter({ hasText: 'Feed Baby' });
    await expect(card.first()).toBeVisible({ timeout: 5000 });

    // Open edit sheet via pencil
    const wrap = page.locator('.day-hour-row[data-hour="9"] .chore-card-wrap').first();
    await wrap.hover();
    await wrap.locator('.chore-card-edit-btn').click();
    await expect(page.locator('#edit-sheet-time')).toBeVisible();

    // Save without changes
    await page.locator('[data-action="save-schedule-edit"]').click();
    await page.waitForTimeout(1500);

    // Card should still be visible in the 9 AM row
    await expect(page.locator('.day-hour-row[data-hour="9"] .chore-card')
      .filter({ hasText: 'Feed Baby' }).first()).toBeVisible({ timeout: 5000 });
  });

  test('creating a once schedule via pick-chore sheet shows the card', async ({ page }) => {
    await setupWithChores(page);

    // Tap the 10 AM hour cell to open pick-chore sheet
    await page.locator('[data-drop-hour="10"]').click();
    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await expect(page.locator('.sheet-title')).toContainText('10 AM');

    // Find and click the Feed Baby chore item
    const feedBabyItem = page.locator('.sheet-chore-item').filter({ has: page.locator('.chore-name', { hasText: 'Feed Baby' }) });
    await expect(feedBabyItem).toBeVisible();
    await feedBabyItem.click();

    // Wait for the schedule to be created and the view to re-render
    await page.waitForTimeout(2000);

    // Card should appear in the 10 AM row
    const card = page.locator('.day-hour-row[data-hour="10"] .chore-card').filter({ hasText: 'Feed Baby' });
    await expect(card.first()).toBeVisible({ timeout: 5000 });
  });
});
