// tests/e2e/chores.spec.js
// End-to-end tests for:
//   • Creating new chores inline from the pick-chore bottom sheet
//   • Chore repeatability — already-scheduled chores stay in the sheet list
//   • Drag-and-drop in the week view

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-chores-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers, creates household, seeds default chores, waits for day view.
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
    data: { name: `Chores Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.click('[data-nav=\"calendar\"]');
  await page.waitForSelector('.cal-date', { timeout: 15000 });

  return { email, csrf };
}

/**
 * Fires HTML5 DragEvent pairs so the app's global listeners pick them up.
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

// ─── Default chores: no time-of-day labels ────────────────────────────────────

test.describe('Default chores: action-specific names', () => {
  test('default chores do not contain "(Morning)" or "(Evening)" labels', async ({ page }) => {
    await setupWithChores(page);

    const resp = await page.request.get('/api/chores');
    expect(resp.status()).toBe(200);
    const { chores } = await resp.json();

    for (const chore of chores) {
      expect(chore.name).not.toMatch(/\(morning\)/i);
      expect(chore.name).not.toMatch(/\(evening\)/i);
      expect(chore.name).not.toMatch(/\(night\)/i);
      expect(chore.name).not.toMatch(/\(afternoon\)/i);
    }
  });

  test('seeded default chores include "Feed Cats" (single entry, no duplicates)', async ({ page }) => {
    await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    const feedCats = chores.filter(c => c.name === 'Feed Cats');
    expect(feedCats).toHaveLength(1);
  });

  test('seeded default chores do not include the cat-specific custom chores', async ({ page }) => {
    await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    const names = chores.map(c => c.name);
    expect(names).not.toContain('Feed Orange Cat');
    expect(names).not.toContain('Feed Black Cat');
    expect(names).not.toContain('Feed Mongo');
    expect(names).not.toContain('Feed Roger');
  });

  test('seeded default chores list has 15 items', async ({ page }) => {
    await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    expect(chores).toHaveLength(15);
  });

  test('Feed Mongo and Feed Roger can be added as custom chores', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    for (const name of ['Feed Mongo', 'Feed Roger']) {
      const resp = await page.request.post('/api/chores', {
        data: { name },
        headers: { 'X-CSRF-Token': csrf },
      });
      expect(resp.status()).toBe(201);
      const { chore } = await resp.json();
      expect(chore.name).toBe(name);
      expect(chore.isPredefined).toBe(false);
    }
  });
});

// ─── Pick-chore sheet: chores always available ────────────────────────────────

test.describe('Pick-chore sheet: repeatable chores', () => {
  test('sheet shows all chores even after one has been scheduled', async ({ page }) => {
    await setupWithChores(page);

    // Schedule the first chore
    await page.locator('.day-hour-row[data-hour="8"] .hour-label').click();
    await page.waitForTimeout(400);
    const totalBefore = await page.locator('.sheet-chore-item').count();
    await page.locator('.sheet-chore-item').first().click();
    await page.waitForTimeout(1500);

    // Open the sheet again — all chores must still be available
    await page.locator('.day-hour-row[data-hour="8"] .hour-label').click();
    await page.waitForTimeout(400);
    const totalAfter = await page.locator('.sheet-chore-item').count();

    expect(totalAfter).toBe(totalBefore);
    await page.locator('.bottom-sheet button[data-action="close-sheet"]').click();
  });

  test('same chore can be added to a period more than once', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    // Add the same chore to morning twice
    const r1 = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(r1.status()).toBe(201);

    const r2 = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(r2.status()).toBe(201);

    const schedules = (await (await page.request.get('/api/schedules')).json()).schedules;
    const morningOccurrences = schedules.filter(s => s.choreId === choreId && s.timePeriod === 'morning');
    expect(morningOccurrences.length).toBe(2);
  });

  test('sheet "Create & add chore" form is visible', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.day-hour-row[data-hour="8"] .hour-label').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.sheet-new-chore-form')).toBeVisible();
    await expect(page.locator('.sheet-new-chore-input')).toBeVisible();
    await expect(page.locator('.sheet-new-chore-form button[type="submit"]')).toBeVisible();
  });

  test('creating a new chore from the sheet adds it and closes the sheet', async ({ page }) => {
    await setupWithChores(page);

    const countBefore = (await (await page.request.get('/api/chores')).json()).chores.length;

    await page.locator('.day-hour-row[data-hour="8"] .hour-label').click();
    await page.waitForTimeout(400);

    const choreName = `Test Chore ${Date.now()}`;
    await page.locator('.sheet-new-chore-input').fill(choreName);
    await page.locator('.sheet-new-chore-form button[type="submit"]').click();
    await page.waitForTimeout(2000);

    // Sheet should be closed
    await expect(page.locator('.bottom-sheet')).toHaveCount(0);

    // New chore should exist in the API
    const { chores } = await (await page.request.get('/api/chores')).json();
    expect(chores.length).toBe(countBefore + 1);
    expect(chores.some(c => c.name === choreName)).toBe(true);
  });

  test('new chore created from sheet is scheduled for the correct period', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.day-hour-row[data-hour="18"] .hour-label').click();
    await page.waitForTimeout(400);

    const choreName = `Evening Chore ${Date.now()}`;
    await page.locator('.sheet-new-chore-input').fill(choreName);
    await page.locator('.sheet-new-chore-form button[type="submit"]').click();
    await page.waitForTimeout(2000);

    // The new chore's schedule should have specificTime 18:00
    const schedules = (await (await page.request.get('/api/schedules')).json()).schedules;
    const { chores } = await (await page.request.get('/api/chores')).json();
    const newChore = chores.find(c => c.name === choreName);
    expect(newChore).toBeDefined();

    const sch = schedules.find(s => s.choreId === newChore.id);
    expect(sch).toBeDefined();
    expect(sch.specificTime).toBe('18:00');
    expect(sch.timePeriod).toBe('anytime');
  });

  test('new chore created from sheet appears in the day view', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('.day-hour-row[data-hour="8"] .hour-label').click();
    await page.waitForTimeout(400);

    const choreName = `Custom Morning Chore ${Date.now()}`;
    await page.locator('.sheet-new-chore-input').fill(choreName);
    await page.locator('.sheet-new-chore-form button[type="submit"]').click();
    await page.waitForTimeout(2000);

    // Chore card should appear in the 8 AM row
    const hourCards = page.locator('[data-drop-hour="8"] .chore-card');
    const names = await hourCards.locator('.chore-name').allInnerTexts();
    expect(names).toContain(choreName);
  });

  test('submitting empty chore name from sheet does nothing', async ({ page }) => {
    await setupWithChores(page);
    const countBefore = (await (await page.request.get('/api/chores')).json()).chores.length;

    await page.locator('.day-hour-row[data-hour="8"] .hour-label').click();
    await page.waitForTimeout(400);

    // Leave input empty and submit
    await page.locator('.sheet-new-chore-form button[type="submit"]').click();
    await page.waitForTimeout(500);

    // Sheet should still be open (form validation prevented submission)
    // or if it closed nothing was created
    const countAfter = (await (await page.request.get('/api/chores')).json()).chores.length;
    expect(countAfter).toBe(countBefore);
  });
});

// ─── Drag and Drop: Week View ─────────────────────────────────────────────────

test.describe('Drag and Drop: Week View', () => {
  /**
   * Schedules a chore at a specific hour via the API and switches to week view.
   * Returns { choreId, scheduleId }.
   */
  async function setupWeekViewWithSchedule(page, csrf, hour = 8) {
    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const createResp = await page.request.post('/api/schedules', {
      data: {
        choreId,
        timePeriod:    'morning',
        specificTime:  `${String(hour).padStart(2, '0')}:00`,
        frequencyType: 'daily',
        isActive:      true,
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    const scheduleId = (await createResp.json()).schedule.id;

    await page.reload();
    await page.click('[data-nav=\"calendar\"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    await page.locator('.view-tab[data-view="week"]').click();
    await page.waitForTimeout(1200);

    return { choreId, scheduleId };
  }

  test('dragging a week-view chore card to another hour cell moves it there', async ({ page }) => {
    const { csrf } = await setupWithChores(page);
    await setupWeekViewWithSchedule(page, csrf, 8);

    // Find the chore card in hour row 8
    const srcRow  = page.locator('.hour-row[data-hour="8"]');
    const card    = srcRow.locator('.week-chore-card').first();
    const destRow = page.locator('.hour-row[data-hour="10"]');
    // Drop onto the first week-cell in that row
    const destCell = destRow.locator('.week-cell').first();

    await htmlDragDrop(page, card, destCell);
    await page.waitForTimeout(1500);

    // Chore should now appear in hour 10, not hour 8
    await expect(page.locator('.hour-row[data-hour="10"] .week-chore-card')).toHaveCount(7);
    await expect(page.locator('.hour-row[data-hour="8"] .week-chore-card')).toHaveCount(0);
  });

  test('chore does not disappear after week-view drag-and-drop', async ({ page }) => {
    const { csrf } = await setupWithChores(page);
    await setupWeekViewWithSchedule(page, csrf, 9);

    const srcRow   = page.locator('.hour-row[data-hour="9"]');
    const card     = srcRow.locator('.week-chore-card').first();
    const destCell = page.locator('.hour-row[data-hour="11"]').locator('.week-cell').first();

    await htmlDragDrop(page, card, destCell);
    await page.waitForTimeout(1500);

    // After drop the total number of chore cards across all hour rows should
    // be the same as before (one card per day = 7).
    const totalCards = await page.locator('.hour-row .week-chore-card').count();
    expect(totalCards).toBe(7);
  });

  test('week-view drag preserves chore name after move', async ({ page }) => {
    const { csrf } = await setupWithChores(page);
    await setupWeekViewWithSchedule(page, csrf, 7);

    const srcRow  = page.locator('.hour-row[data-hour="7"]');
    const card    = srcRow.locator('.week-chore-card').first();
    const choreName = await card.locator('.chore-name').innerText();
    const destCell = page.locator('.hour-row[data-hour="13"]').locator('.week-cell').first();

    await htmlDragDrop(page, card, destCell);
    await page.waitForTimeout(1500);

    const movedCard = page.locator('.hour-row[data-hour="13"] .week-chore-card').first();
    await expect(movedCard.locator('.chore-name')).toContainText(choreName);
  });

  test('week-view drag updates the schedule specificTime in the API', async ({ page }) => {
    const { csrf } = await setupWithChores(page);
    const { scheduleId } = await setupWeekViewWithSchedule(page, csrf, 6);

    const srcRow   = page.locator('.hour-row[data-hour="6"]');
    const card     = srcRow.locator('.week-chore-card').first();
    const destCell = page.locator('.hour-row[data-hour="14"]').locator('.week-cell').first();

    await htmlDragDrop(page, card, destCell);
    await page.waitForTimeout(1500);

    // Verify via API that specificTime was updated and isActive is still true
    const schedules = (await (await page.request.get('/api/schedules')).json()).schedules;
    const updated = schedules.find(s => s.id === scheduleId);
    expect(updated).toBeDefined();
    expect(updated.specificTime).toBe('14:00');
    expect(updated.isActive).toBe(true);  // must not have been reset to false
  });

  test('week-view drag does not reset isActive to false', async ({ page }) => {
    // This is the regression test for the original bug: PATCH /api/schedules/:id
    // was overwriting isActive with false (Go zero-value for bool) whenever
    // only timePeriod / specificTime were sent in the patch body.
    const { csrf } = await setupWithChores(page);
    const choreId = (await (await page.request.get('/api/chores')).json()).chores[0].id;

    const createResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'morning', specificTime: '05:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    const scheduleId = (await createResp.json()).schedule.id;

    // PATCH with only timePeriod + specificTime (no isActive field)
    const patchResp = await page.request.patch(`/api/schedules/${scheduleId}`, {
      data: { timePeriod: 'afternoon', specificTime: '14:00' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(patchResp.status()).toBe(200);
    const patched = (await patchResp.json()).schedule;
    expect(patched.isActive).toBe(true);   // must remain true
    expect(patched.timePeriod).toBe('afternoon');
    expect(patched.specificTime).toBe('14:00');
  });

  test('week-view drag between periods preserves chore visibility', async ({ page }) => {
    const { csrf } = await setupWithChores(page);
    // Schedule at evening hour (17)
    await setupWeekViewWithSchedule(page, csrf, 17);

    const srcRow   = page.locator('.hour-row[data-hour="17"]');
    const card     = srcRow.locator('.week-chore-card').first();
    // Drop to a morning cell (hour 8)
    const destCell = page.locator('.hour-row[data-hour="8"]').locator('.week-cell').first();

    await htmlDragDrop(page, card, destCell);
    await page.waitForTimeout(1500);

    // Chore must be visible in the new row
    await expect(page.locator('.hour-row[data-hour="8"] .week-chore-card')).toHaveCount(7);
    await expect(page.locator('.hour-row[data-hour="17"] .week-chore-card')).toHaveCount(0);
  });
});

// ─── Chore API ────────────────────────────────────────────────────────────────

test.describe('Chore API', () => {
  test('POST /api/chores creates a custom chore', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.post('/api/chores', {
      data: { name: 'Test Custom Chore', icon: '🧪', color: '#FF0000', category: 'custom' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const body = await resp.json();
    expect(body.chore).toBeDefined();
    expect(body.chore.name).toBe('Test Custom Chore');
    expect(body.chore.isPredefined).toBe(false);
  });

  test('POST /api/chores uses default icon and color when omitted', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.post('/api/chores', {
      data: { name: 'Minimal Chore' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const { chore } = await resp.json();
    expect(chore.icon).toBeTruthy();
    expect(chore.color).toBeTruthy();
  });

  test('GET /api/chores lists custom chore after creation', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.request.post('/api/chores', {
      data: { name: 'Listed Chore' },
      headers: { 'X-CSRF-Token': csrf },
    });

    const { chores } = await (await page.request.get('/api/chores')).json();
    expect(chores.some(c => c.name === 'Listed Chore')).toBe(true);
  });

  test('POST /api/chores then POST /api/schedules wires them together', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const createResp = await page.request.post('/api/chores', {
      data: { name: 'Wired Chore' },
      headers: { 'X-CSRF-Token': csrf },
    });
    const choreId = (await createResp.json()).chore.id;

    const schedResp = await page.request.post('/api/schedules', {
      data: { choreId, timePeriod: 'morning', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(schedResp.status()).toBe(201);
    const sch = (await schedResp.json()).schedule;
    expect(sch.choreId).toBe(choreId);
    expect(sch.isActive).toBe(true);
  });
});
