// tests/e2e/schedule-edit-bug.spec.js
// Reproduction test: editing a schedule should preserve all fields including daysOfWeek.

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

test.describe('Schedule Edit: preserves fields', () => {

  test('editing a weekly schedule preserves all daysOfWeek', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    // Get Feed Baby chore
    const choreResp = await page.request.get('/api/chores');
    const chores = (await choreResp.json()).chores;
    const feedBaby = chores.find(c => c.name === 'Feed Baby');
    expect(feedBaby).toBeDefined();

    // Create a weekly schedule on Mon, Wed, Fri at 08:00
    const createResp = await page.request.post('/api/schedules', {
      data: {
        choreId: feedBaby.id,
        timePeriod: 'anytime',
        specificTime: '08:00',
        frequencyType: 'weekly',
        daysOfWeek: [1, 3, 5],
        isActive: true,
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(createResp.status()).toBe(201);
    const created = (await createResp.json()).schedule;

    // Verify the schedule has all 3 days
    expect(created.daysOfWeek).toEqual([1, 3, 5]);

    // Reload and navigate to a Monday so the card is visible
    await page.reload();
    await page.click('[data-nav="activity"]');
    await page.click('[data-action="switch-view"][data-view="day"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // Navigate to next Monday if needed
    const today = new Date();
    const targetMonday = new Date();
    const daysUntilMonday = (8 - today.getDay()) % 7; // days until next Monday
    targetMonday.setDate(today.getDate() + daysUntilMonday);
    const mondayIso = targetMonday.toISOString().split('T')[0];

    // Navigate to that Monday
    for (let i = 0; i < 7; i++) {
      const currentDate = await page.locator('.cal-date').getAttribute('data-date');
      if (currentDate === mondayIso) break;
      await page.locator('button[data-action="navigate-day"]').last().click();
      await page.waitForTimeout(500);
    }

    // The chore card for Feed Baby should be visible in the 8 AM row
    await expect(page.locator('[data-drop-hour="8"] .chore-card', { hasText: 'Feed Baby' })).toBeVisible({ timeout: 5000 });

    // Open the edit sheet via the pencil button
    const wrap = page.locator('[data-drop-hour="8"] .chore-card-wrap').filter({ has: page.locator('.chore-name', { hasText: 'Feed Baby' }) }).first();
    await wrap.hover();
    await wrap.locator('.chore-card-edit-btn').click();
    await expect(page.locator('#edit-sheet-time')).toBeVisible();

    // Verify all 3 day pills are shown as on (Mon, Wed, Fri)
    const onPills = page.locator('.day-pill--on');
    await expect(onPills).toHaveCount(3);

    const pillTexts = await onPills.allInnerTexts();
    expect(pillTexts).toEqual(expect.arrayContaining(['Mon', 'Wed', 'Fri']));

    // Click Save without making any changes
    await page.locator('[data-action="save-schedule-edit"]').click();
    await page.waitForTimeout(1500);

    // Sheet should close
    await expect(page.locator('[data-action="save-schedule-edit"]')).not.toBeVisible();

    // Verify via API that daysOfWeek is still [1, 3, 5]
    const getResp = await page.request.get('/api/schedules');
    const schedules = (await getResp.json()).schedules;
    const feedBabySch = schedules.find(s => s.choreId === feedBaby.id);
    expect(feedBabySch).toBeDefined();
    expect(feedBabySch.daysOfWeek).toEqual([1, 3, 5]);
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
    const card = page.locator('[data-drop-hour="9"] .chore-card', { hasText: 'Feed Baby' });
    await expect(card).toBeVisible({ timeout: 5000 });

    // Open edit sheet via pencil
    const wrap = page.locator('[data-drop-hour="9"] .chore-card-wrap').filter({ has: page.locator('.chore-name', { hasText: 'Feed Baby' }) }).first();
    await wrap.hover();
    await wrap.locator('.chore-card-edit-btn').click();
    await expect(page.locator('#edit-sheet-time')).toBeVisible();

    // Save without changes
    await page.locator('[data-action="save-schedule-edit"]').click();
    await page.waitForTimeout(1500);

    // Card should still be visible in the 9 AM row
    await expect(page.locator('[data-drop-hour="9"] .chore-card', { hasText: 'Feed Baby' })).toBeVisible({ timeout: 5000 });
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
    await expect(page.locator('[data-drop-hour="10"] .chore-card', { hasText: 'Feed Baby' })).toBeVisible({ timeout: 5000 });
  });
});
