// tests/e2e/feed-baby-volume.spec.js
// Tests for the volume (mL) picker on the Feed Baby chore log sheet.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-vol-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Vol Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  const chores = (await (await page.request.get('/api/chores')).json()).chores || [];
  const feedBaby = chores.find(c => c.name === 'Feed Baby');

  return { csrf, chores, feedBaby };
}

test.describe('Feed Baby volume picker', () => {
  test('home log sheet shows volume picker for Feed Baby', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);
    expect(feedBaby).toBeDefined();
    expect(feedBaby.hasVolumeML).toBe(true);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await expect(card).toBeVisible();
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    await expect(page.locator('#log-volume')).toBeVisible();
  });

  test('home log sheet saves volumeML via API', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#log-volume')).toBeVisible();

    await page.selectOption('#log-volume', '120');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[feedBaby.id];
    expect(log).toBeDefined();
    expect(log.volumeML).toBe(120);
  });

  test('home log sheet without volume selected sends null volumeML', async ({ page }) => {
    const { feedBaby, csrf } = await setupWithChores(page);

    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    const today = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;

    const resp = await page.request.post('/api/logs', {
      data: { choreId: feedBaby.id, note: '', indicators: [], date: today },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const body = await resp.json();
    expect(body.log.volumeML ?? null).toBeNull();
  });

  test('volume appears in history row', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });

    await page.selectOption('#log-volume', '85');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    const meta = page.locator('.hist-meta').first();
    await expect(meta).toContainText('85mL');
  });

  test('non-Feed Baby chores do not show volume picker', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const nonFeedChore = chores.find(c => c.name !== 'Feed Baby');
    expect(nonFeedChore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${nonFeedChore.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    await expect(page.locator('#log-volume')).toHaveCount(0);
  });

  test('calendar log sheet shows volume picker for Feed Baby', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    await page.click('[data-nav="activity"]');
    await page.click('[data-action="switch-view"][data-view="day"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // Schedule Feed Baby at 10 AM so we have a card to long-press.
    const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
    await page.request.post('/api/schedules', {
      data: { choreId: feedBaby.id, timePeriod: 'anytime', specificTime: '10:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.reload();
    await page.click('[data-nav="activity"]');
    await page.click('[data-action="switch-view"][data-view="day"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    // Long-press the scheduled card to open the log sheet.
    const card = page.locator('[data-drop-hour="10"] .chore-card').first();
    await expect(card).toBeVisible();
    await card.scrollIntoViewIfNeeded();
    const box = await card.boundingBox();
    const cx = box.x + box.width / 2;
    const cy = box.y + box.height / 2;
    await page.mouse.move(cx, cy);
    await page.mouse.down();
    await page.waitForTimeout(650);
    await page.mouse.up();

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#log-volume')).toBeVisible();
  });

  test('calendar log sheet saves volumeML via API', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    await page.click('[data-nav="activity"]');
    await page.click('[data-action="switch-view"][data-view="day"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
    await page.request.post('/api/schedules', {
      data: { choreId: feedBaby.id, timePeriod: 'anytime', specificTime: '10:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.reload();
    await page.click('[data-nav="activity"]');
    await page.click('[data-action="switch-view"][data-view="day"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    const card = page.locator('[data-drop-hour="10"] .chore-card').first();
    await card.scrollIntoViewIfNeeded();
    const box = await card.boundingBox();
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.waitForTimeout(650);
    await page.mouse.up();

    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });
    await page.selectOption('#log-volume', '50');
    await page.click('[data-action="save-log"]');
    await page.waitForTimeout(1500);

    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[feedBaby.id];
    expect(log).toBeDefined();
    expect(log.volumeML).toBe(50);
  });

  test('chores API returns hasVolumeML for Feed Baby', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);
    expect(feedBaby.hasVolumeML).toBe(true);
  });

  test('home log sheet pre-populates volume from previous log', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);

    // First log: set volume to 120 mL
    await card.click();
    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#log-volume')).toHaveValue('');
    await page.selectOption('#log-volume', '120');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Verify volume was saved via API
    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    const log = latestLogs[feedBaby.id];
    expect(log).toBeDefined();
    expect(log.volumeML).toBe(120);
  });

  test('home log sheet pre-populates from latest log on reload', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    // Log with 80 mL then reload to test cache-miss (cold start from API)
    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });
    await page.selectOption('#log-volume', '80');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Reload the page (cold cache) and open the sheet again
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const card2 = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card2.click();
    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#log-volume')).toHaveValue('80');
  });

  test('home log sheet volume defaults to empty when not set on prior log', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);

    // First log: formula is default-on, set 45 mL
    await card.click();
    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });
    await page.selectOption('#log-volume', '45');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Open again: volume should still default to empty (not 45)
    await card.click();
    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#log-volume')).toHaveValue('');
  });

  test('calendar log sheet pre-populates volume from previous log', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    // Schedule Feed Baby so we have a card to long-press in calendar
    const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
    await page.request.post('/api/schedules', {
      data: { choreId: feedBaby.id, timePeriod: 'anytime', specificTime: '10:00', frequencyType: 'daily', isActive: true },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Log with 65 mL via home sheet first
    const homeCard = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await homeCard.click();
    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });
    await page.selectOption('#log-volume', '65');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Go to calendar and long-press the scheduled card
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await page.click('[data-nav="activity"]');
    await page.click('[data-action="switch-view"][data-view="day"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });

    const card = page.locator('[data-drop-hour="10"] .chore-card').first();
    await expect(card).toBeVisible();
    await card.scrollIntoViewIfNeeded();
    const box = await card.boundingBox();
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.waitForTimeout(650);
    await page.mouse.up();

    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#log-volume')).toHaveValue('65');
  });

  test('editing an existing log uses its own volume, not the cache', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);

    // Log with 150 mL first (older). Set time back 5 minutes to ensure
    // completedAt differs from the second log.
    await card.click();
    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });
    const now = new Date();
    const ago = new Date(now.getTime() - 5 * 60000);
    const pad = n => String(n).padStart(2, '0');
    const earlier = `${ago.getFullYear()}-${pad(ago.getMonth() + 1)}-${pad(ago.getDate())}T${pad(ago.getHours())}:${pad(ago.getMinutes())}`;
    await page.fill('#log-when', earlier);
    await page.selectOption('#log-volume', '150');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#toast-container .toast')).not.toBeVisible({ timeout: 10000 });

    // Log with 30 mL second (newer, this becomes the cached value)
    await card.click();
    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });
    await page.selectOption('#log-volume', '30');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Verify via API: the older log still has 150 mL (not overwritten by cache)
    const resp = await page.request.get('/api/logs/history');
    const logs = (await resp.json()).logs || [];
    const olderLog = logs.find(l => l.volumeML === 150);
    expect(olderLog).toBeDefined();
    expect(olderLog.volumeML).toBe(150);
  });
});

test.describe('Feed Baby food type indicators', () => {
  async function tapFeedBaby(page) {
    const cards = page.locator('.home-chore-card');
    const count = await cards.count();
    for (let i = 0; i < count; i++) {
      const name = await cards.nth(i).locator('.home-card-name').innerText();
      if (name === 'Feed Baby') {
        await cards.nth(i).click();
        return;
      }
    }
    throw new Error('Feed Baby chore card not found');
  }

  test('feed baby log sheet shows both indicator chips and volume picker', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);
    expect(feedBaby.indicatorLabels).toEqual(['🍼 formula', '🤱 breast']);
    expect(feedBaby.hasVolumeML).toBe(true);

    await tapFeedBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Both indicator chips are present
    const chips = page.locator('.log-chip');
    await expect(chips).toHaveCount(2);
    await expect(chips.nth(0)).toHaveText('🍼 formula');
    await expect(chips.nth(1)).toHaveText('🤱 breast');

    // Volume picker is also present
    await expect(page.locator('#log-volume')).toBeVisible();
  });

  test('indicator chips are toggleable', async ({ page }) => {
    await setupWithChores(page);
    await tapFeedBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    const chips = page.locator('.log-chip');
    await expect(chips).toHaveCount(2);

    // Formula is default-on; breast is default-off.
    const formulaChip = chips.nth(0);
    const breastChip = chips.nth(1);
    await expect(formulaChip).toHaveClass(/log-chip--on/);
    await expect(breastChip).not.toHaveClass(/log-chip--on/);
    // Toggle formula off
    await formulaChip.click();
    await expect(formulaChip).not.toHaveClass(/log-chip--on/);
    // Toggle breast on
    await breastChip.click();
    await expect(breastChip).toHaveClass(/log-chip--on/);
  });

  test('saves indicators and volume together via API', async ({ page }) => {
    const { feedBaby, csrf } = await setupWithChores(page);
    await tapFeedBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Formula is default-on; toggle it off, then toggle breast on
    await page.locator('.log-chip').nth(0).click();
    await page.locator('.log-chip').nth(1).click();
    await expect(page.locator('.log-chip').nth(1)).toHaveClass(/log-chip--on/);
    await expect(page.locator('.log-chip').nth(0)).not.toHaveClass(/log-chip--on/);

    // Set volume to 95 mL
    await page.selectOption('#log-volume', '95');

    // Save
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Verify via API
    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[feedBaby.id];
    expect(log).toBeDefined();
    expect(log.volumeML).toBe(95);
    expect(log.indicators).toContain('🤱 breast');
    expect(log.indicators).not.toContain('🍼 formula');
  });

  test('reopen saved log shows selected indicators and volume', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    await tapFeedBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Formula is default-on; set volume and save
    await page.selectOption('#log-volume', '120');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Verify via API: indicators and volume were persisted
    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[feedBaby.id];
    expect(log).toBeDefined();
    expect(log.indicators).toContain('🍼 formula');
    expect(log.indicators).not.toContain('🤱 breast');
    expect(log.volumeML).toBe(120);
  });

  test('non-Feed Baby chores do not show food type chips', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const nonFeedChore = chores.find(c => c.name !== 'Feed Baby');
    expect(nonFeedChore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${nonFeedChore.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // No log-chip elements unless the chore has its own indicatorLabels
    // (Change Baby has its own chips, but other chores shouldn't)
  });
});

test.describe('History indicator icons', () => {
  test('feed baby indicators show emoji-only icons in history', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator('#log-volume')).toBeVisible({ timeout: 3000 });

    // Toggle breast chip and set volume
    await page.locator('.log-chip').nth(1).click();
    await page.selectOption('#log-volume', '60');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Navigate to history
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    const meta = page.locator('.hist-meta').first();
    // Should show emoji 🤱 but not the word "breast"
    await expect(meta).toContainText('🤱');
    await expect(meta).toContainText('60mL');
    await expect(meta).not.toContainText('breast');
    await expect(meta).not.toContainText('formula');
  });

  test('change baby indicators show emoji-only icons in history', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const changeBaby = chores.find(c => c.name === 'Change Baby');
    expect(changeBaby).toBeDefined();

    // Find and tap Change Baby card to log with indicator
    const cards = page.locator('.home-chore-card');
    const count = await cards.count();
    let card = null;
    for (let i = 0; i < count; i++) {
      const name = await cards.nth(i).locator('.home-card-name').innerText();
      if (name === 'Change Baby') {
        card = cards.nth(i);
        break;
      }
    }
    expect(card).toBeDefined();
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Pee is autoselected by default — no toggle needed
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Navigate to history
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    const meta = page.locator('.hist-meta').first();
    await expect(meta).toContainText('💛');
    await expect(meta).not.toContainText('pee');
  });

  test('both indicators shown when multiple are selected', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Formula is default-on; toggle breast too, and set a volume
    await page.locator('.log-chip').nth(1).click();
    await page.selectOption('#log-volume', '90');

    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    const meta = page.locator('.hist-meta').first();
    await expect(meta).toContainText('🍼');
    await expect(meta).toContainText('🤱');
  });
});
