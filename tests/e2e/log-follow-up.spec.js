// tests/e2e/log-follow-up.spec.js
// Tests the follow-up scheduling feature: setting a follow-up time in
// the log sheet creates a one-off schedule, and logging the chore again
// (even from the home tab) clears the follow-up.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-followup-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `FollowUp Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  const chores = (await (await page.request.get('/api/chores')).json()).chores || [];

  return { csrf, chores, email };
}

async function enableFollowUp(page, choreId, choreName, csrf) {
  await page.request.patch(`/api/chores/${choreId}`, {
    data: { name: choreName, followUpEnabled: true },
    headers: { 'X-CSRF-Token': csrf },
  });
}

test.describe('Log follow-up scheduling', () => {
  test('log with follow-up creates a once schedule, re-log clears it', async ({ page }) => {
    const { csrf, chores } = await setupWithChores(page);

    // Use Feed Baby (predefined, has indicator labels)
    const feedBaby = chores.find(c => c.name === 'Feed Baby');
    expect(feedBaby).toBeDefined();

    // Enable follow-up for Feed Baby
    await enableFollowUp(page, feedBaby.id, feedBaby.name, csrf);

    // Reload to get the updated chore
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    // Open the log sheet by tapping Feed Baby
    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // The follow-up inputs should be visible
    await expect(page.locator('#followup-hours')).toBeVisible({ timeout: 2000 });

    // Set follow-up: 3 hours (click + three times)
    const incrHours = page.locator('.stepper-btn[data-action="followup-incr"][data-unit="hours"]');
    await incrHours.click();
    await incrHours.click();
    await incrHours.click();

    // Select formula indicator (required for Fast Feed Baby)
    const chip = page.locator('.log-chip[data-label="🍼 formula"]');
    const isChipOn = await chip.evaluate(el => el.classList.contains('log-chip--on'));
    if (!isChipOn) {
      await chip.click();
    }

    // Set volume
    const volumeSelect = page.locator('.indicator-volume-select').first();
    if (await volumeSelect.isVisible()) {
      await volumeSelect.selectOption({ index: 1 });
    }

    // Save the log
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Verify a follow-up schedule was created
    const schedulesResp = await page.request.get('/api/schedules');
    const { schedules } = await schedulesResp.json();
    const followUpSch = schedules.find(s => s.choreId === feedBaby.id && s.isFollowUp);
    expect(followUpSch).toBeDefined();
    expect(followUpSch.frequencyType).toBe('once');
    expect(followUpSch.isFollowUp).toBe(true);

    // Now log Feed Baby directly from the home tab (tap without sheet)
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    // Use direct API to log (simulates tapping the card which does a
    // direct log via the home-tap-chore handler, but we use the sheet
    // instead so we can set followUpMinutes=0)
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Verify the follow-up inputs are pre-filled with 3 hours (last used)
    const hoursVal = await page.locator('#followup-hours').textContent();
    expect(hoursVal).toBe('3');

    // Clear the follow-up hours (set to 0 — click − three times)
    const decrHours = page.locator('.stepper-btn[data-action="followup-decr"][data-unit="hours"]');
    await decrHours.click();
    await decrHours.click();
    await decrHours.click();

    // Select indicator again
    const chip2 = page.locator('.log-chip[data-label="🍼 formula"]');
    const isChipOn2 = await chip2.evaluate(el => el.classList.contains('log-chip--on'));
    if (!isChipOn2) {
      await chip2.click();
    }
    const volSel2 = page.locator('.indicator-volume-select').first();
    if (await volSel2.isVisible()) {
      await volSel2.selectOption({ index: 1 });
    }

    // Save with followUpMinutes = 0
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Verify the follow-up schedule was deleted
    const schedulesResp2 = await page.request.get('/api/schedules');
    const { schedules: schedules2 } = await schedulesResp2.json();
    const followUpSch2 = schedules2.find(s => s.choreId === feedBaby.id && s.isFollowUp);
    expect(followUpSch2).toBeUndefined();

    // Verify lastFollowUpMinutes is now 0 (pre-filled empty on next open)
    const chores2Resp = await page.request.get('/api/chores');
    const { chores: chores2 } = await chores2Resp.json();
    const updatedChore = chores2.find(c => c.id === feedBaby.id);
    expect(updatedChore.lastFollowUpMinutes).toBe(0);
  });

  test('follow-up schedule appears in schedule tab', async ({ page }) => {
    const { csrf, chores } = await setupWithChores(page);

    const feedBaby = chores.find(c => c.name === 'Feed Baby');
    expect(feedBaby).toBeDefined();

    await enableFollowUp(page, feedBaby.id, feedBaby.name, csrf);
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    // Open log sheet
    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#followup-hours')).toBeVisible({ timeout: 2000 });

    // Set follow-up: 4 hours
    const incrHours2 = page.locator('.stepper-btn[data-action="followup-incr"][data-unit="hours"]');
    await incrHours2.click();
    await incrHours2.click();
    await incrHours2.click();
    await incrHours2.click();

    // Select indicator and volume
    const chip = page.locator('.log-chip[data-label="🍼 formula"]');
    const isChipOn = await chip.evaluate(el => el.classList.contains('log-chip--on'));
    if (!isChipOn) await chip.click();
    const volSel = page.locator('.indicator-volume-select').first();
    if (await volSel.isVisible()) {
      await volSel.selectOption({ index: 1 });
    }

    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Navigate to schedule tab
    await page.click('[data-nav="schedule"]');
    await page.waitForSelector('.schedule-view', { timeout: 5000 });

    // The follow-up should appear in the upcoming list with the chore name
    await expect(page.locator('.sch-name').filter({ hasText: 'Feed Baby' })).toBeVisible({ timeout: 5000 });
  });

  test('follow-up toggle is visible in chore edit sheet', async ({ page }) => {
    const { csrf, chores } = await setupWithChores(page);

    const feedBaby = chores.find(c => c.name === 'Feed Baby');
    expect(feedBaby).toBeDefined();

    // Navigate to manage view
    await page.click('.home-header-tab[data-view="manage"]');
    await page.waitForSelector('.chores-view', { timeout: 5000 });

    // Open the edit sheet for Feed Baby
    const editBtn = page.locator(`.chore-row-edit[data-action="chore-edit"][data-chore-id="${feedBaby.id}"]`);
    await editBtn.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // The follow-up toggle should be present
    await expect(page.locator('[data-action="toggle-followup-enabled"]')).toBeVisible({ timeout: 2000 });

    // Initially unchecked
    const cb = page.locator('[data-action="toggle-followup-enabled"]');
    expect(await cb.isChecked()).toBe(false);

    // Check it and save
    await cb.check();
    await page.click('[data-action="save-chore"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Reload and verify it persisted
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const choresResp = await page.request.get('/api/chores');
    const { chores: updatedChores } = await choresResp.json();
    const updated = updatedChores.find(c => c.id === feedBaby.id);
    expect(updated.followUpEnabled).toBe(true);
  });
});
