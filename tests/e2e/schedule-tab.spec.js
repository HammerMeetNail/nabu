// tests/e2e/schedule-tab.spec.js
// Tests for the new Schedule tab showing upcoming scheduled chores.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-sch-tab-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithSchedules(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Schedule Tab ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  const chores = (await (await page.request.get('/api/chores')).json()).chores;
  const feedCats = chores.find(c => c.name === 'Feed Cats');

  await page.request.post('/api/schedules', {
    data: {
      choreId: feedCats.id,
      timePeriod: 'anytime',
      specificTime: '08:00',
      frequencyType: 'daily',
      isActive: true,
    },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  return { csrf, feedCats };
}

test.describe('Schedule Tab', () => {
  test('Schedule tab is present in bottom nav between Activity and Home', async ({ page }) => {
    await setupWithSchedules(page);
    await page.waitForSelector('#bottom-tabs', { timeout: 5000 });

    const tabs = page.locator('#bottom-tabs .tab-item');
    await expect(tabs).toHaveCount(5);

    const labels = await tabs.locator('span').allInnerTexts();
    expect(labels).toEqual(['Stats', 'Activity', 'Home', 'Schedule', 'Settings']);
  });

  test('navigating to Schedule tab shows upcoming list with heading', async ({ page }) => {
    await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.schedule-view h2', { timeout: 5000 });
    await expect(page.locator('.schedule-view h2')).toHaveText('Upcoming');
  });

  test('Schedule tab shows scheduled chores with recurrence info', async ({ page }) => {
    await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.sch-row', { timeout: 5000 });

    await expect(page.locator('.sch-row')).not.toHaveCount(0);
    await expect(page.locator('.sch-name').first()).toContainText('Feed Cats');
    await expect(page.locator('.sch-meta').first()).toContainText('Every day');
    await expect(page.locator('.sch-time').first()).toContainText('8:00 AM');
  });

  test('Schedule tab shows day headers for next 2 weeks', async ({ page }) => {
    await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.sch-day-header', { timeout: 5000 });

    await expect(page.locator('.sch-day-header').first()).toHaveText('Today');
    await expect(page.locator('.sch-day-header')).not.toHaveCount(0);
  });

  test('today rows show log button and edit button', async ({ page }) => {
    await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.sch-row', { timeout: 5000 });

    const todayGroup = page.locator('.sch-day-header').filter({ hasText: 'Today' }).first();
    await todayGroup.scrollIntoViewIfNeeded();

    const logs = page.locator('.sch-log-btn').first();
    await expect(logs).toBeVisible();

    const edits = page.locator('.sch-edit-btn').first();
    await expect(edits).toBeVisible();
  });

  test('tapping log button logs the chore for today', async ({ page }) => {
    await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.sch-log-btn', { timeout: 5000 });

    const logBtn = page.locator('.sch-log-btn').first();
    await logBtn.click();
    await page.waitForTimeout(1000);

    await expect(page.locator('.sch-row--done')).not.toHaveCount(0);
  });

  test('empty state shows when no schedules exist', async ({ page }) => {
    const email = uniqueEmail();
    await page.goto('/register');
    await page.waitForSelector('#register-form');
    await page.fill('#reg-email', email);
    await page.fill('#reg-password', 'test123456');
    await page.fill('#reg-confirm', 'test123456');
    await page.click('button[type="submit"]');
    await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

    const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';
    await page.request.post('/api/household', {
      data: { name: `No Schedules ${Date.now()}` },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.request.post('/api/chores/seed-defaults', {
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.reload();

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.schedule-view', { timeout: 5000 });
    await expect(page.locator('.empty-state-title')).toContainText('No scheduled chores');
  });

  test('edit button opens the edit-schedule sheet', async ({ page }) => {
    await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.sch-edit-btn', { timeout: 5000 });

    const editBtn = page.locator('.sch-edit-btn').first();
    await editBtn.click();
    await page.waitForTimeout(500);

    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await expect(page.locator('.sheet-title')).toContainText('Feed Cats');
  });

  test('edit-schedule sheet shows recurrence end date field', async ({ page }) => {
    await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.sch-edit-btn', { timeout: 5000 });

    await page.locator('.sch-edit-btn').first().click();
    await page.waitForTimeout(500);

    await expect(page.locator('#edit-sheet-end-date')).toBeVisible();
  });

  test('setting recurrenceEnd and saving updates the schedule', async ({ page }) => {
    const { csrf, feedCats } = await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.sch-edit-btn', { timeout: 5000 });

    await page.locator('.sch-edit-btn').first().click();
    await page.waitForTimeout(500);

    const tomorrow = new Date();
    tomorrow.setDate(tomorrow.getDate() + 1);
    const endDate = tomorrow.toISOString().slice(0, 10);
    await page.fill('#edit-sheet-end-date', endDate);

    await page.locator('[data-action="save-schedule-edit"]').click();
    await page.waitForTimeout(1000);

    const schedules = (await (await page.request.get('/api/schedules')).json()).schedules;
    const updated = schedules.find(s => s.choreId === feedCats.id);
    expect(updated).toBeDefined();
    expect(updated.recurrenceEnd).toBeTruthy();
    expect(String(updated.recurrenceEnd).slice(0, 10)).toBe(endDate);
  });

  test('FAB button opens pick-chore sheet to schedule a chore', async ({ page }) => {
    const { feedCats } = await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.schedule-view', { timeout: 5000 });

    await expect(page.locator('.fab')).toBeVisible();
    await page.locator('.fab').click();
    await page.waitForTimeout(500);

    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await expect(page.locator('.sheet-title')).toContainText('Add Chore');
  });

  test('scheduling a chore from the schedule tab FAB updates the list', async ({ page }) => {
    await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.fab', { timeout: 5000 });

    const countBefore = await page.locator('.sch-row').count();

    await page.locator('.fab').click();
    await page.waitForTimeout(500);
    await expect(page.locator('.bottom-sheet')).toBeVisible();

    const choreItem = page.locator('.sheet-chore-item').first();
    await choreItem.click();
    await page.waitForTimeout(1000);

    await expect(page.locator('.bottom-sheet')).not.toBeVisible();
    const countAfter = await page.locator('.sch-row').count();
    expect(countAfter).toBeGreaterThan(countBefore);
  });

  test('tapping a schedule row opens the log sheet', async ({ page }) => {
    await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.sch-row-main', { timeout: 5000 });

    const row = page.locator('.sch-row-main').first();
    await row.click();
    await page.waitForTimeout(500);

    await expect(page.locator('.bottom-sheet')).toBeVisible();
    await expect(page.locator('.sheet-title')).toContainText('Feed Cats');
    await expect(page.locator('#log-note')).toBeVisible();
    await expect(page.locator('[data-action="save-log"]')).toBeVisible();
  });

  test('logging via the schedule row sheet creates a log for that date', async ({ page }) => {
    await setupWithSchedules(page);

    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.sch-row-main', { timeout: 5000 });

    const row = page.locator('.sch-row-main').first();
    await row.click();
    await page.waitForTimeout(500);
    await expect(page.locator('.bottom-sheet')).toBeVisible();

    await page.locator('[data-action="save-log"]').click();
    await page.waitForTimeout(1000);

    await expect(page.locator('.bottom-sheet')).not.toBeVisible();
    await expect(page.locator('.sch-row--done')).not.toHaveCount(0);
  });
});
