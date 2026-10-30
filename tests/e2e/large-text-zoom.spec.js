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

// What doesn't fit in the sheet: a label or field that leaves it or overlaps
// another, or a follow-up choice too narrow for its longest value.
function sheetProblems(sheet) {
  return sheet.evaluate(sheet => {
    const style = getComputedStyle(sheet);
    const box = sheet.getBoundingClientRect();
    const left = box.left + parseFloat(style.paddingLeft);
    const right = box.right - parseFloat(style.paddingRight);
    const shown = [...sheet.querySelectorAll('.field-label, input, select, textarea, .log-chip, .star-rating, .star-clear-btn')]
      .filter(el => el.getClientRects().length > 0 && getComputedStyle(el).opacity !== '0');
    const name = el => el.id || el.getAttribute('aria-label') || el.textContent.trim();
    const problems = [];
    shown.forEach((el, i) => {
      const a = el.getBoundingClientRect();
      if (a.left < left - 1 || a.right > right + 1) problems.push(`${name(el)} leaves the sheet`);
      for (const other of shown.slice(i + 1)) {
        if (el.contains(other) || other.contains(el)) continue;
        const b = other.getBoundingClientRect();
        const x = Math.min(a.right, b.right) - Math.max(a.left, b.left);
        const y = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
        if (x > 1 && y > 1) problems.push(`${name(el)} overlaps ${name(other)}`);
      }
    });
    const measure = document.createElement('canvas').getContext('2d');
    for (const select of sheet.querySelectorAll('.followup-select')) {
      const s = getComputedStyle(select);
      measure.font = `${s.fontWeight} ${s.fontSize} ${s.fontFamily}`;
      const room = select.clientWidth - parseFloat(s.paddingLeft) - parseFloat(s.paddingRight);
      const widest = Math.max(...[...select.options].map(option => measure.measureText(option.text).width));
      if (widest > room) problems.push(`${select.id} cuts its values`);
    }
    return problems;
  });
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

test('each kind of log sheet fits at 200% zoom', async ({ page }) => {
  const { headers } = await fixture(page);
  expect((await page.request.post('/api/chores/seed-defaults', { headers })).ok()).toBe(true);
  await page.reload();
  await expect(page.locator('.home-grid')).toBeVisible();
  const sheet = page.locator('.bottom-sheet');
  const open = async name => {
    const card = page.locator('.home-chore-card', { hasText: name });
    await card.click();
    await expect(sheet).toBeVisible();
    // Every default activity offers a follow-up.
    await expect(sheet.locator('.followup-select')).toHaveCount(3);
    return Number(await card.getAttribute('data-home-chore-id'));
  };

  // A type with its own amount, both types on, and one unit for them, at
  // 100%, 125%, 150% and 200% zoom on a 390 px phone.
  const feedBaby = await open('Feed Baby');
  await sheet.getByRole('button', { name: '🤱 breast' }).click();
  await sheet.getByRole('spinbutton', { name: '🍼 formula amount' }).fill('90');
  await sheet.getByRole('spinbutton', { name: '🤱 breast amount' }).fill('30');
  for (const width of [390, 312, 260, 195]) {
    await page.setViewportSize({ width, height: 422 });
    expect(await sheetProblems(sheet), `Feed Baby at ${width} px`).toEqual([]);
  }
  await sheet.getByRole('button', { name: 'Log', exact: true }).click();
  await expect(sheet).toHaveCount(0);
  await expect.poll(async () => {
    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    return latestLogs[feedBaby]?.indicatorVolumes;
  }).toEqual({ '🍼 formula': 90, '🤱 breast': 30 });

  // One amount and its unit.
  await open('Cat Meds');
  expect(await sheetProblems(sheet), 'Cat Meds').toEqual([]);
  await sheet.getByRole('button', { name: 'Cancel' }).click();
  await expect(sheet).toHaveCount(0);

  // A rating, whose fifth star is in reach.
  await open('Read Book');
  const stars = sheet.locator('.star-rating');
  const box = await stars.boundingBox();
  await stars.click({ position: { x: box.width - 4, y: box.height / 2 } });
  await expect(stars).toHaveAttribute('aria-valuenow', '50');
  expect(await sheetProblems(sheet), 'Read Book').toEqual([]);
  expect(await overflow(page)).toBe(0);
});
