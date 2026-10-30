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
// another, a type's name split mid-word, or a follow-up choice too narrow for
// its longest value.
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
    for (const chip of sheet.querySelectorAll('.log-chip')) {
      const text = chip.firstChild;
      if (text?.nodeType !== Node.TEXT_NODE) continue;
      for (const word of text.data.matchAll(/\S+/g)) {
        const range = document.createRange();
        range.setStart(text, word.index);
        range.setEnd(text, word.index + word[0].length);
        if (new Set([...range.getClientRects()].map(r => Math.round(r.top))).size > 1) problems.push(`${name(chip)} splits "${word[0]}"`);
      }
    }
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

// What doesn't fit in a Stats card: anything that leaves the card, a Baby
// column or the explainer, text that runs past its own box, or a date field
// narrower than its date. Where a column is narrower still (Chromium's date
// field, with its calendar button, needs 152 px; WebKit's 109), the field must
// have the column's whole width. Charts scale to their column.
function cardProblems(card) {
  return card.evaluate(card => {
    const inner = box => {
      const r = box.getBoundingClientRect(), s = getComputedStyle(box);
      return [r.left + parseFloat(s.paddingLeft) + parseFloat(s.borderLeftWidth),
        r.right - parseFloat(s.paddingRight) - parseFloat(s.borderRightWidth)];
    };
    const name = el => el.getAttribute('aria-label') || el.textContent.trim().slice(0, 30) || el.className;
    const problems = [];
    for (const el of card.querySelectorAll('*')) {
      if (!el.getClientRects().length || (el.closest('svg') && el.tagName.toLowerCase() !== 'svg')) continue;
      const b = el.getBoundingClientRect();
      if (!b.width) continue;
      for (const box of [card, el.parentElement.closest('.baby-care-column'), el.parentElement.closest('.feeding-gaps-explainer')]) {
        if (!box) continue;
        const [left, right] = inner(box);
        if (b.left < left - 1 || b.right > right + 1) problems.push(`${name(el)} leaves ${box.className.split(' ')[0]}`);
      }
    }
    for (const el of card.querySelectorAll('h3, h4, button, td, p, .baby-member-name')) {
      if (getComputedStyle(el).textOverflow !== 'ellipsis' && el.scrollWidth > el.clientWidth + 1) problems.push(`${name(el)} runs past its box`);
    }
    for (const input of card.querySelectorAll('input[type="date"]')) {
      const probe = input.cloneNode();
      probe.style.cssText = 'position:absolute;visibility:hidden;width:auto;min-width:0;flex:none';
      card.append(probe);
      const needs = probe.getBoundingClientRect().width;
      probe.remove();
      const [left, right] = inner(input.closest('.baby-care-column') || card);
      if (input.getBoundingClientRect().width < Math.min(needs, right - left) - 1) problems.push(`${input.getAttribute('aria-label')} cuts its date`);
    }
    return [...new Set(problems)];
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
  // 100%, 110%, 118%, 125%, 150% and 200% zoom on a 390 px phone. At 125%
  // each amount goes under its type, whose name had no room beside it.
  const feedBaby = await open('Feed Baby');
  await sheet.getByRole('button', { name: '🤱 breast' }).click();
  await sheet.getByRole('spinbutton', { name: '🍼 formula amount' }).fill('90');
  await sheet.getByRole('spinbutton', { name: '🤱 breast amount' }).fill('30');
  for (const width of [390, 355, 330, 312, 260, 195]) {
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

test('the Baby card fits at 200% zoom', async ({ page }) => {
  const { headers } = await fixture(page);
  expect((await page.request.post('/api/chores/seed-defaults', { headers })).ok()).toBe(true);
  const { chores } = await (await page.request.get('/api/chores')).json();
  const feed = chores.find(c => c.name === 'Feed Baby'), change = chores.find(c => c.name === 'Change Baby');
  // Two feeds an hour apart make one cluster-feeding gap; the member row
  // shows the fixture's long name.
  for (const [chore, minutes, data] of [[feed, 150, { volumeML: 120 }], [feed, 90, { volumeML: 60 }], [change, 60, { indicators: ['💛 pee'] }]]) {
    const at = new Date(Date.now() - minutes * 60000);
    const date = `${at.getFullYear()}-${String(at.getMonth() + 1).padStart(2, '0')}-${String(at.getDate()).padStart(2, '0')}`;
    const logged = await page.request.post('/api/logs', { headers, data: { choreId: chore.id, completedAt: at.toISOString(), date, hour: at.getHours(), ...data } });
    expect(logged.ok()).toBe(true);
  }
  await page.reload();
  await expect(page.locator('.home-grid')).toBeVisible();
  await page.locator('[data-nav="stats"]').click();
  const card = page.locator('[data-stats-section="baby"] .card');
  await expect(card.locator('[data-action="scatter-tap"]')).toHaveCount(1);
  await expect(card.locator('.baby-member-row')).toHaveCount(2);
  await card.getByRole('button', { name: 'How to read this chart' }).click();
  await expect(card.locator('.feeding-gaps-explainer--visible')).toBeVisible();

  // 200%, 150%, 125% and 100% zoom on a 390 px phone, and a tablet's narrow
  // columns, three to a row. Busy Hours has dates side by side too.
  const busyHours = page.locator('[data-stats-section="busy-hours"] .card');
  for (const width of [195, 260, 312, 390, 600]) {
    await page.setViewportSize({ width, height: 844 });
    expect(await cardProblems(card), `the Baby card at ${width} px`).toEqual([]);
    expect(await cardProblems(busyHours), `Busy Hours at ${width} px`).toEqual([]);
  }
  expect(await overflow(page)).toBe(0);

  // Its controls still work when wrapped.
  await page.setViewportSize({ width: 195, height: 844 });
  await card.locator(`[data-action="stats-baby-period"][data-chore-id="${feed.id}"][data-period="monthly"]`).click();
  await expect(card.locator(`[data-chore-id="${feed.id}"][data-period="monthly"]`)).toHaveAttribute('aria-pressed', 'true');
  await card.locator('[data-action="stats-feeding-gaps-quick"][data-days="14"]').click();
  await expect(card.locator('[data-action="stats-feeding-gaps-quick"][data-days="14"]')).toHaveClass(/period-toggle--active/);
  expect(await cardProblems(card), 'the Baby card after its controls').toEqual([]);
});
