// tests/e2e/meds-indicator-deselect.spec.js
// Regression: volume+indicator chores (Cat Meds / Baby Meds with meds as
// indicator labels) must let the user deselect every previously submitted
// or available indicator and save a log with none of them selected.
// Previously the save path blocked with "Select a volume and food type",
// forcing users to keep a med selected and enter 0 as the quantity.

import { test, expect } from '@playwright/test';
import { rerender } from './review-fixtures.js';

function uniqueEmail() {
  return `e2e-medsclear-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithMedsChore(page, choreName = 'Cat Meds', labels = ['am', 'pm']) {
  const email = uniqueEmail();
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  const headers = { 'X-CSRF-Token': csrf };

  const hh = await page.request.post('/api/household', { data: { name: `Meds Clear ${Date.now()}` }, headers });
  expect(hh.ok()).toBeTruthy();
  await page.request.post('/api/chores/seed-defaults', { headers });

  const { chores } = await (await page.request.get('/api/chores')).json();
  const chore = chores.find(c => c.name === choreName);
  expect(chore).toBeDefined();
  expect(chore.hasVolumeML).toBe(true);

  // Give the meds chore its medication indicator labels.
  const patch = await page.request.patch(`/api/chores/${chore.id}`, { headers, data: { indicatorLabels: labels, indicatorDefaults: [] } });
  expect(patch.ok()).toBeTruthy();

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { email, headers, chore };
}

const amChip = '.log-chip[data-label="am"]';
const pmChip = '.log-chip[data-label="pm"]';

test('new log sheet lets every echoed med be deselected and saved with none selected', async ({ page }) => {
  const { headers, chore } = await setupWithMedsChore(page);

  // Seed a previous log with the "am" med so the next sheet echoes it.
  const seed = await page.request.post('/api/logs', {
    headers,
    data: { choreId: chore.id, indicators: ['am'], indicatorVolumes: { am: 20 }, volumeML: 20, hour: 12, completedAt: new Date().toISOString() },
  });
  expect(seed.ok()).toBeTruthy();

  // Refresh so state.latestLogs picks up the seed log (drives the echo).
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  // Open a fresh log sheet from the home tab.
  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
  await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

  // The previously submitted med is echoed on.
  await expect(page.locator(amChip)).toHaveClass(/log-chip--on/);
  // Deselect it — the quantity field must hide.
  await page.locator(amChip).click();
  await expect(page.locator(amChip)).not.toHaveClass(/log-chip--on/);
  await expect(page.locator('.indicator-volume-select[data-indicator="am"]')).toBeHidden();
  // Force a full re-render of the open sheet while the med is deselected:
  // the in-progress selection must survive, and the previously submitted
  // med must not be re-echoed from the latest log.
  await rerender(page);
  await expect(page.locator(amChip)).not.toHaveClass(/log-chip--on/);

  // Save with no med selected: must succeed, not block with a validation toast.
  await page.locator('[data-action="save-log"]').click();
  await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });
  await expect(page.locator('.bottom-sheet')).toHaveCount(0);

  // A log with no meds must have been saved (the most recent one we created).
  const { logs } = await (await page.request.get('/api/logs/history')).json();
  const choreLogs = (logs || []).filter(l => l.choreId === chore.id);
  const cleared = choreLogs.find(l => Array.isArray(l.indicators) && l.indicators.length === 0);
  expect(cleared).toBeDefined();
  expect(cleared.indicatorVolumes ?? {}).toEqual({});
});

test('editing a saved log can clear its med indicators', async ({ page }) => {
  const { headers, chore } = await setupWithMedsChore(page);

  // Seed a log carrying both meds.
  const seed = await page.request.post('/api/logs', {
    headers,
    data: { choreId: chore.id, indicators: ['am', 'pm'], indicatorVolumes: { am: 20, pm: 10 }, volumeML: 30, hour: 12, completedAt: new Date().toISOString() },
  });
  expect(seed.ok()).toBeTruthy();
  const log = (await seed.json()).log;

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  await page.locator('[data-nav="activity"]').click();
  await page.locator(`[data-action="view-log"][data-log-id="${log.id}"]`).click();
  await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

  // Both meds start selected; deselect them all.
  await expect(page.locator(amChip)).toHaveClass(/log-chip--on/);
  await expect(page.locator(pmChip)).toHaveClass(/log-chip--on/);
  await page.locator(amChip).click();
  await page.locator(pmChip).click();
  await expect(page.locator(amChip)).not.toHaveClass(/log-chip--on/);
  await expect(page.locator(pmChip)).not.toHaveClass(/log-chip--on/);

  await page.locator('[data-action="save-log"]').click();
  // The update path closes the sheet (no success toast on update).
  await expect(page.locator('.bottom-sheet')).toHaveCount(0);

  const { logs } = await (await page.request.get('/api/logs/history')).json();
  const updated = logs.find(l => l.id === log.id);
  expect(updated.indicators).toEqual([]);
  expect(updated.indicatorVolumes ?? {}).toEqual({});
});
