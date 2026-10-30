// The everyday path at 200% zoom (plan §8): the PWA sizes text in pixels, so
// a larger text setting reaches it as page zoom, which halves a 390 px
// phone's viewport to 195 CSS px. Nothing may scroll sideways, the log sheet's
// fields keep apart, and logging, undo and Stats still work.
import { test, expect } from '@playwright/test';
import { fixture } from './review-fixtures.js';

test.use({ serviceWorkers: 'block', viewport: { width: 195, height: 422 } });

const overflow = page => page.evaluate(() => document.scrollingElement.scrollWidth - window.innerWidth);

function overlaps(a, b) {
  return a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;
}

test('logging, undo and Stats work at 200% zoom', async ({ page }) => {
  const { chore, headers } = await fixture(page);
  expect((await page.request.post('/api/chores/seed-defaults', { headers })).ok()).toBe(true);
  await page.reload();
  await expect(page.locator('.home-grid')).toBeVisible();
  expect(await overflow(page)).toBe(0);

  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
  const sheet = page.locator('.bottom-sheet');
  await expect(sheet).toBeVisible();
  expect(await overflow(page)).toBe(0);
  // The amount and its unit stack instead of overlapping.
  const amount = await sheet.locator('label[for="log-volume"]').boundingBox();
  const unit = await sheet.locator('label[for="log-metric-unit"]').boundingBox();
  const amountInput = await sheet.locator('#log-volume').boundingBox();
  const unitInput = await sheet.locator('#log-metric-unit').boundingBox();
  expect(overlaps(amount, unit), 'the Amount and Unit labels overlap').toBe(false);
  expect(overlaps(amountInput, unitInput), 'the amount and unit inputs overlap').toBe(false);

  await sheet.locator('#log-volume').fill('120');
  await sheet.getByRole('button', { name: 'Log', exact: true }).click();
  await expect(sheet).toHaveCount(0);
  const undo = page.locator('#toast-container [data-undo-toast] button');
  await expect(undo).toBeVisible();
  await undo.click();
  await expect(undo).toHaveCount(0);
  await expect(page.locator(`[data-home-chore-id="${chore.id}"] .home-card-time`)).toHaveText('never');

  await page.locator('[data-nav="stats"]').click();
  await expect(page.locator('.stats-page')).toBeVisible();
  await expect(page.locator('.stats-page')).not.toContainText('Loading charts');
  expect(await overflow(page)).toBe(0);
});
