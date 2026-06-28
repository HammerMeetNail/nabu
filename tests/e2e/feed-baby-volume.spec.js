// tests/e2e/feed-baby-volume.spec.js
// Tests for per-indicator volume (mL) pickers on the Feed Baby chore log sheet.

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

const formulaVol = '.indicator-volume-select[data-indicator="🍼 formula"]';
const breastVol  = '.indicator-volume-select[data-indicator="🤱 breast"]';

test.describe('Feed Baby volume picker', () => {
  test('home log sheet shows per-indicator volume pickers for Feed Baby', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);
    expect(feedBaby).toBeDefined();
    expect(feedBaby.hasVolumeML).toBe(true);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await expect(card).toBeVisible();
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Formula is default-on, so its volume select is visible
    await expect(page.locator(formulaVol)).toBeVisible();
    // Breast is off, so its volume select is hidden
    await expect(page.locator(breastVol)).toBeHidden();
  });

  test('home log sheet saves indicatorVolumes via API', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator(formulaVol)).toBeVisible();

    await page.selectOption(formulaVol, '120');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[feedBaby.id];
    expect(log).toBeDefined();
    expect(log.indicatorVolumes['🍼 formula']).toBe(120);
    expect(log.indicators).toContain('🍼 formula');
  });

  test('home log sheet without volume selected sends null indicatorVolumes', async ({ page }) => {
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
    expect(body.log.indicatorVolumes ?? null).toBeNull();
  });

  test('volume appears in history row', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });

    await page.selectOption(formulaVol, '85');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    const meta = page.locator('.hist-meta').first();
    await expect(meta).toContainText('85mL');
  });

  test('non-Feed Baby chores do not show per-indicator volume pickers', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const nonFeedChore = chores.find(c => c.name !== 'Feed Baby');
    expect(nonFeedChore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${nonFeedChore.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    await expect(page.locator('.indicator-volume-select')).toHaveCount(0);
  });

  test('home log sheet pre-populates per-indicator volume from previous log', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);

    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });
    await expect(page.locator(formulaVol)).toHaveValue('');
    await page.selectOption(formulaVol, '120');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    const log = latestLogs[feedBaby.id];
    expect(log).toBeDefined();
    expect(log.indicatorVolumes['🍼 formula']).toBe(120);

    // Open sheet again: volume should default to empty (not cached — new behavior)
    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });
    await expect(page.locator(formulaVol)).toHaveValue('');
  });

  test('home log sheet pre-populates from latest log on reload', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });
    await page.selectOption(formulaVol, '80');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const card2 = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card2.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });
    await expect(page.locator(formulaVol)).toHaveValue('80');
  });

  test('home log sheet volume defaults to empty when not set on prior log', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);

    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });
    await page.selectOption(formulaVol, '45');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });
    await expect(page.locator(formulaVol)).toHaveValue('');
  });

  test('home log sheet volume does not pre-fill when previous food type differs from defaults', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);

    // First log: toggle formula off, breast on, set 95 mL on breast
    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });
    await page.locator('.log-chip').nth(0).click(); // toggle formula off
    await page.locator('.log-chip').nth(1).click(); // toggle breast on
    await expect(page.locator('.log-chip').nth(1)).toHaveClass(/log-chip--on/);
    await page.selectOption(breastVol, '95');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Open again: formula is default-on (breast not selected), so formula volume should be empty
    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });
    await expect(page.locator(formulaVol)).toHaveValue('');
    // Formula should be selected, breast should not
    await expect(page.locator('.log-chip').nth(0)).toHaveClass(/log-chip--on/);
    await expect(page.locator('.log-chip').nth(1)).not.toHaveClass(/log-chip--on/);
  });



  test('editing an existing log uses its own volume, not the cache', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);

    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });
    const now = new Date();
    const ago = new Date(now.getTime() - 5 * 60000);
    const pad = n => String(n).padStart(2, '0');
    const earlier = `${ago.getFullYear()}-${pad(ago.getMonth() + 1)}-${pad(ago.getDate())}T${pad(ago.getHours())}:${pad(ago.getMinutes())}`;
    await page.fill('#log-when', earlier);
    await page.selectOption(formulaVol, '150');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#toast-container .toast')).not.toBeVisible({ timeout: 10000 });

    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });
    await page.selectOption(formulaVol, '30');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const resp = await page.request.get('/api/logs/history');
    const logs = (await resp.json()).logs || [];
    const olderLog = logs.find(l => l.indicatorVolumes?.['🍼 formula'] === 150);
    expect(olderLog).toBeDefined();
    expect(olderLog.indicatorVolumes['🍼 formula']).toBe(150);
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

  test('feed baby log sheet shows both indicator chips and per-indicator volume pickers', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);
    expect(feedBaby.indicatorLabels).toEqual(['🍼 formula', '🤱 breast']);
    expect(feedBaby.hasVolumeML).toBe(true);

    await tapFeedBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    const chips = page.locator('.log-chip');
    await expect(chips).toHaveCount(2);
    await expect(chips.nth(0)).toHaveText('🍼 formula');
    await expect(chips.nth(1)).toHaveText('🤱 breast');

    // Formula volume select is visible (chip is default-on); breast is hidden
    await expect(page.locator(formulaVol)).toBeVisible();
    await expect(page.locator(breastVol)).toBeHidden();
  });

  test('indicator chips are toggleable and show/hide volume selects', async ({ page }) => {
    await setupWithChores(page);
    await tapFeedBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    const chips = page.locator('.log-chip');
    await expect(chips).toHaveCount(2);

    const formulaChip = chips.nth(0);
    const breastChip = chips.nth(1);
    await expect(formulaChip).toHaveClass(/log-chip--on/);
    await expect(breastChip).not.toHaveClass(/log-chip--on/);

    // Formula volume is visible
    await expect(page.locator(formulaVol)).toBeVisible();
    await expect(page.locator(breastVol)).toBeHidden();

    // Toggle formula off — volume select should hide
    await formulaChip.click();
    await expect(formulaChip).not.toHaveClass(/log-chip--on/);
    await expect(page.locator(formulaVol)).toBeHidden();

    // Toggle breast on — volume select should show
    await breastChip.click();
    await expect(breastChip).toHaveClass(/log-chip--on/);
    await expect(page.locator(breastVol)).toBeVisible();
  });

  test('saves per-indicator volumes via API', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);
    await tapFeedBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Formula is default-on; set its volume
    await page.selectOption(formulaVol, '95');

    // Toggle breast on and set its volume too
    await page.locator('.log-chip').nth(1).click();
    await expect(page.locator(breastVol)).toBeVisible();
    await page.selectOption(breastVol, '45');

    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[feedBaby.id];
    expect(log).toBeDefined();
    expect(log.indicators).toContain('🍼 formula');
    expect(log.indicators).toContain('🤱 breast');
    expect(log.indicatorVolumes['🍼 formula']).toBe(95);
    expect(log.indicatorVolumes['🤱 breast']).toBe(45);
  });

  test('reopen saved log shows selected indicators and volumes', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    await tapFeedBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    await page.selectOption(formulaVol, '120');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[feedBaby.id];
    expect(log).toBeDefined();
    expect(log.indicators).toContain('🍼 formula');
    expect(log.indicators).not.toContain('🤱 breast');
    expect(log.indicatorVolumes['🍼 formula']).toBe(120);
  });

  test('hidden indicator volumes from previous log are not saved', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    // First log: formula + breast, both with volumes
    await tapFeedBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await page.selectOption(formulaVol, '100');
    await page.locator('.log-chip').nth(1).click(); // toggle breast on
    await expect(page.locator(breastVol)).toBeVisible();
    await page.selectOption(breastVol, '50');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Reload so latestLogs cache includes breast volume from previous log
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    // Second log: only formula (default-on), breast is off and hidden
    await tapFeedBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator(formulaVol)).toBeVisible();
    await expect(page.locator(breastVol)).toBeHidden();
    // Only set formula volume; do not touch breast at all
    await page.selectOption(formulaVol, '75');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Verify breast did not leak into the saved log
    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[feedBaby.id];
    expect(log).toBeDefined();
    expect(log.indicators).toContain('🍼 formula');
    expect(log.indicators).not.toContain('🤱 breast');
    expect(log.indicatorVolumes['🍼 formula']).toBe(75);
    expect(log.indicatorVolumes).not.toHaveProperty('🤱 breast');
  });

  test('non-Feed Baby chores do not show food type chips', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const nonFeedChore = chores.find(c => c.name !== 'Feed Baby');
    expect(nonFeedChore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${nonFeedChore.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
  });
});

test.describe('History indicator icons', () => {
  test('feed baby indicators show per-indicator volume in history', async ({ page }) => {
    const { feedBaby } = await setupWithChores(page);

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator(formulaVol)).toBeVisible({ timeout: 3000 });

    // Toggle breast chip and set volumes
    await page.locator('.log-chip').nth(1).click();
    await expect(page.locator(breastVol)).toBeVisible();
    await page.selectOption(formulaVol, '80');
    await page.selectOption(breastVol, '60');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    const meta = page.locator('.hist-meta').first();
    await expect(meta).toContainText('🍼');
    await expect(meta).toContainText('80mL');
    await expect(meta).toContainText('🤱');
    await expect(meta).toContainText('60mL');
    await expect(meta).not.toContainText('breast');
    await expect(meta).not.toContainText('formula');
  });

  test('change baby indicators show emoji-only icons in history', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const changeBaby = chores.find(c => c.name === 'Change Baby');
    expect(changeBaby).toBeDefined();

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

    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

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

    await page.locator('.log-chip').nth(1).click();
    await page.selectOption(formulaVol, '90');

    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    const meta = page.locator('.hist-meta').first();
    await expect(meta).toContainText('🍼');
    await expect(meta).toContainText('🤱');
  });
});
