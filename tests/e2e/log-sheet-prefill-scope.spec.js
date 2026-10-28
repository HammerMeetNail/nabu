// tests/e2e/log-sheet-prefill-scope.spec.js
// Regression: the Feed Baby sheet prefill echo (latest log's type+volume)
// must not bleed into plain chip chores like a custom Laundry task. A new
// sheet for a chip-only chore always starts from the chore's defaults.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-prefill-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Prefill Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  const chores = (await (await page.request.get('/api/chores')).json()).chores || [];

  return { csrf, chores };
}

test.describe('Log sheet prefill scope', () => {
  test('plain chip chores start from defaults, not the previous log', async ({ page }) => {
    const { chores, csrf } = await setupWithChores(page);

    // The user's Laundry task carries indicator chips (washed/folded) but
    // no volume — the exact shape that the Feed Baby echo must not touch.
    const laundry = chores.find(c => c.name === 'Laundry');
    expect(laundry).toBeDefined();
    const res = await page.request.patch(`/api/chores/${laundry.id}`, {
      data: {
        name: 'Laundry',
        icon: '👕',
        color: '#F97316',
        indicatorLabels: ['🧺 washed', '👖 folded'],
        indicatorDefaults: ['🧺 washed'],
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(res.status()).toBe(200);

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const card = page.locator(`.home-chore-card[data-home-chore-id="${laundry.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    // Log with the non-default chip so a prior log would echo it if the
    // bleed existed.
    await page.locator('.log-chip[data-label="🧺 washed"]').click();
    await page.locator('.log-chip[data-label="👖 folded"]').click();
    await expect(page.locator('.log-chip[data-label="👖 folded"]')).toHaveClass(/log-chip--on/);
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Reload so the latest log drives any prefill.
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    // The sheet must start from the chore's defaults — the previous
    // log's folded selection must NOT be echoed.
    const card2 = page.locator(`.home-chore-card[data-home-chore-id="${laundry.id}"]`);
    await card2.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('.log-chip[data-label="🧺 washed"]')).toHaveClass(/log-chip--on/);
    await expect(page.locator('.log-chip[data-label="👖 folded"]')).not.toHaveClass(/log-chip--on/);
  });

  test('Feed Baby still echoes its previous type after reload', async ({ page }) => {
    const { chores, csrf } = await setupWithChores(page);
    const feedBaby = chores.find(c => c.name === 'Feed Baby');
    expect(feedBaby).toBeDefined();

    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    const today = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;

    // Breast-only feed, 95 mL — the echo must preserve this on reopen.
    const resp = await page.request.post('/api/logs', {
      data: {
        choreId: feedBaby.id,
        note: '',
        indicators: ['🤱 breast'],
        indicatorVolumes: { '🤱 breast': 95 },
        date: today,
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const card = page.locator(`.home-chore-card[data-home-chore-id="${feedBaby.id}"]`);
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('.log-chip[data-label="🤱 breast"]')).toHaveClass(/log-chip--on/);
    await expect(page.locator('.log-chip[data-label="🍼 formula"]')).not.toHaveClass(/log-chip--on/);
    await expect(page.locator('.indicator-volume-select[data-indicator="🤱 breast"]')).toHaveValue('95');
  });

  test('a medication starts from its last dose', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const meds = chores.find(c => c.name === 'Cat Meds');
    expect(meds).toBeDefined();
    const card = page.locator(`.home-chore-card[data-home-chore-id="${meds.id}"]`);

    // No previous dose: the amount starts empty.
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#log-volume')).toHaveValue('');
    await page.fill('#log-volume', '5');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('.bottom-sheet')).toHaveCount(0, { timeout: 5000 });

    // The next dose starts from 5, also after a reload.
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await card.click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#log-volume')).toHaveValue('5');
  });
});
