// The everyday path at 200% zoom (plan §8): the PWA sizes text in pixels, so
// a larger text setting reaches it as page zoom, which halves a 390 px
// phone's viewport to 195 CSS px. Nothing may scroll sideways, the log sheet's
// fields keep apart, and logging, undo and Stats still work.
import { test, expect } from '@playwright/test';
import { fixture } from './review-fixtures.js';

test.use({ serviceWorkers: 'block', viewport: { width: 195, height: 422 } });

const overflow = page => page.evaluate(() => document.scrollingElement.scrollWidth - window.innerWidth);

// Resizes the window and waits for the frame its resize event runs in, where
// an open log sheet lays its types out again.
async function resize(page, width, height) {
  await page.setViewportSize({ width, height });
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
}

function overlaps(a, b) {
  return a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;
}

// What doesn't fit in the sheet: a label or field that leaves it or overlaps
// another, a type's name split mid-word, a date or time narrower than its
// value, or a follow-up choice too narrow for its longest value.
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
    // A date or time narrower than its value: a copy beside it, so it has the
    // field's styles, at its natural width.
    for (const field of sheet.querySelectorAll('input[type="date"], input[type="time"], input[type="datetime-local"]')) {
      if (!field.getClientRects().length) continue;
      const probe = field.cloneNode(false);
      probe.removeAttribute('id');
      probe.style.cssText = 'position:absolute;visibility:hidden;width:auto;min-width:0;max-width:none;flex:none';
      field.after(probe);
      const needs = probe.getBoundingClientRect().width;
      probe.remove();
      if (field.getBoundingClientRect().width < needs - 1) problems.push(`${name(field)} cuts its value`);
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
// column or the explainer, text that runs past its own box, a date field
// narrower than its date (Chromium's, with its calendar button, is the
// widest), or a choice narrower than what's chosen ("All members"). Charts
// scale to their column; what scrolls sideways (the heatmap) may be wider.
function cardProblems(card) {
  return card.evaluate(card => {
    const inner = box => {
      const r = box.getBoundingClientRect(), s = getComputedStyle(box);
      return [r.left + parseFloat(s.paddingLeft) + parseFloat(s.borderLeftWidth),
        r.right - parseFloat(s.paddingRight) - parseFloat(s.borderRightWidth)];
    };
    const name = el => el.getAttribute('aria-label') || el.textContent.trim().slice(0, 30) || el.getAttribute('class');
    const problems = [];
    const scrolls = el => {
      for (let up = el.parentElement; up && up !== card; up = up.parentElement) {
        if (['auto', 'scroll'].includes(getComputedStyle(up).overflowX)) return true;
      }
      return false;
    };
    for (const el of card.querySelectorAll('*')) {
      if (!el.getClientRects().length || getComputedStyle(el).visibility === 'hidden'
        || (el.closest('svg') && el.tagName.toLowerCase() !== 'svg') || scrolls(el)) continue;
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
      // Beside the field, so it has the field's styles.
      const probe = input.cloneNode();
      probe.style.cssText = 'position:absolute;visibility:hidden;width:auto;min-width:0;flex:none';
      input.after(probe);
      const needs = probe.getBoundingClientRect().width;
      probe.remove();
      if (input.getBoundingClientRect().width < needs - 1) problems.push(`${input.getAttribute('aria-label') || `${input.dataset.filter} date`} cuts its date`);
    }
    for (const select of card.querySelectorAll('select')) {
      if (!select.getClientRects().length) continue;
      // Beside the choice, holding only the chosen option, which is all it shows.
      const probe = select.cloneNode(false);
      probe.append(select.selectedOptions[0].cloneNode(true));
      probe.style.cssText = 'position:absolute;visibility:hidden;width:auto;min-width:0;max-width:none;flex:none';
      select.after(probe);
      const needs = probe.getBoundingClientRect().width;
      probe.remove();
      if (select.getBoundingClientRect().width < needs - 1) problems.push(`${select.dataset.filter || select.getAttribute('aria-label')} cuts "${select.selectedOptions[0].text}"`);
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
    await resize(page, width, 422);
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

test('every Stats card fits at 200% zoom', async ({ page }) => {
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
  // A widget, whose header holds a toggle and a remove button.
  const widgets = [{ id: 'w1', type: 'total', choreIds: [feed.id], metric: 'count', agg: 'sum', period: 'week', grain: '', title: 'Feeds this week' }];
  expect((await page.request.patch('/api/preferences', { headers, data: { statsWidgets: widgets } })).ok()).toBe(true);
  await page.reload();
  await expect(page.locator('.home-grid')).toBeVisible();
  await page.locator('[data-nav="stats"]').click();
  const card = page.locator('[data-stats-section="baby"] .card');
  await expect(card.locator('[data-action="scatter-tap"]')).toHaveCount(1);
  await expect(card.locator('.baby-member-row')).toHaveCount(2);
  await card.getByRole('button', { name: 'How to read this chart' }).click();
  await expect(card.locator('.feeding-gaps-explainer--visible')).toBeVisible();

  await expect(page.locator('[data-stats-section="widget:w1"] .card')).toBeVisible();
  // 200%, 150%, 125% and 100% zoom on a 390 px phone, and a tablet's narrow
  // columns, three to a row.
  for (const width of [195, 260, 312, 390, 600]) {
    await resize(page, width, 844);
    for (const each of await page.locator('.stats-page .card').all()) {
      const section = await each.evaluate(c => c.closest('[data-stats-section]')?.dataset.statsSection || 'Stats');
      expect(await cardProblems(each), `${section} at ${width} px`).toEqual([]);
    }
  }
  expect(await overflow(page)).toBe(0);

  // The Baby card's controls still work when wrapped.
  await resize(page, 195, 844);
  await card.locator(`[data-action="stats-baby-period"][data-chore-id="${feed.id}"][data-period="monthly"]`).click();
  await expect(card.locator(`[data-chore-id="${feed.id}"][data-period="monthly"]`)).toHaveAttribute('aria-pressed', 'true');
  await card.locator('[data-action="stats-feeding-gaps-quick"][data-days="14"]').click();
  await expect(card.locator('[data-action="stats-feeding-gaps-quick"][data-days="14"]')).toHaveClass(/period-toggle--active/);
  expect(await cardProblems(card), 'the Baby card after its controls').toEqual([]);
});

test('a type with a long name keeps it whole, beside or over its amount', async ({ page }) => {
  // A member's own types: one long word, and two words.
  const { headers } = await fixture(page, {
    name: 'Pump', metricUnit: 'mL',
    indicatorLabels: ['🧴 supplementation', '🥛 expressed milk'],
    indicatorDefaults: ['🧴 supplementation', '🥛 expressed milk'],
  });
  expect(headers).toBeTruthy();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.locator('.home-chore-card', { hasText: 'Pump' }).click();
  const sheet = page.locator('.bottom-sheet');
  await expect(sheet).toBeVisible();
  await expect(sheet.locator('.indicator-row')).toHaveCount(2);

  // At 100%, 110%, 118%, 125%, 150% and 200% zoom on a 390 px phone, and at
  // 360 px, with the sheet open as the window changes.
  for (const width of [390, 360, 355, 330, 312, 260, 195]) {
    await resize(page, width, 844);
    expect(await sheetProblems(sheet), `Pump at ${width} px`).toEqual([]);
    // Every type sits the same way: all beside their amounts, or all over them.
    const under = await sheet.locator('.indicator-row').evaluateAll(rows => rows.map(row => {
      const chip = row.querySelector('.log-chip').getBoundingClientRect();
      const amount = row.querySelector('input').getBoundingClientRect();
      return amount.top >= chip.bottom - 1;
    }));
    expect(new Set(under).size, `Pump's types at ${width} px all sit the same way`).toBe(1);
  }
  // Its amounts still save.
  await page.setViewportSize({ width: 312, height: 844 });
  await sheet.getByRole('spinbutton', { name: '🧴 supplementation amount' }).fill('20');
  await sheet.getByRole('spinbutton', { name: '🥛 expressed milk amount' }).fill('60');
  await sheet.getByRole('button', { name: 'Log', exact: true }).click();
  await expect(sheet).toHaveCount(0);
});

test('the When field shows its whole date and time, and saves what is chosen there', async ({ page }) => {
  const { chore, headers } = await fixture(page);
  const local = d => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const yesterday = local(new Date(Date.now() - 86400000));
  const logs = async () => (await (await page.request.get('/api/logs/history')).json()).logs.filter(log => log.choreId === chore.id);
  const sheet = page.locator('.bottom-sheet');
  const when = sheet.locator('#log-when'), date = sheet.locator('#log-when-date'), time = sheet.locator('#log-when-time');

  await resize(page, 390, 844);
  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
  await expect(when).toBeVisible();
  await expect(date).toBeHidden();
  // 100%, 110%, 118%, 125%, 150%, 170% and 200% zoom on a 390 px phone.
  for (const width of [390, 360, 330, 312, 260, 230, 195]) {
    await resize(page, width, 844);
    expect(await sheetProblems(sheet), `the sheet at ${width} px`).toEqual([]);
  }
  // Where the date and time don't fit in one field, each has its own, which
  // sets the one a log saves.
  await expect(when).toBeHidden();
  const [today, now] = (await when.inputValue()).split('T');
  await expect(date).toHaveValue(today);
  await expect(time).toHaveValue(now);
  await date.fill(yesterday);
  await time.fill('03:07');
  await expect(when).toHaveValue(`${yesterday}T03:07`);
  await resize(page, 390, 844);
  await expect(when).toBeVisible();
  await expect(when).toHaveValue(`${yesterday}T03:07`);
  await resize(page, 195, 844);
  await expect(time).toHaveValue('03:07');
  await sheet.locator('#log-volume').fill('15');
  await sheet.locator('#log-note').fill('chosen at 200%');
  await sheet.getByRole('button', { name: 'Log', exact: true }).click();
  await expect(sheet).toHaveCount(0);
  await expect.poll(async () => (await logs()).length).toBe(1);
  let [log] = await logs();
  const saved = new Date(log.completedAt);
  expect([local(saved), saved.getHours(), saved.getMinutes(), log.slotHour]).toEqual([yesterday, 3, 7, 3]);

  // Editing keeps a time nobody changed, seconds and all, and saves one
  // changed in the time field.
  const at = new Date(Date.now() - 3 * 3600000);
  at.setSeconds(42, 0);
  const posted = await page.request.post('/api/logs', { headers, data: { choreId: chore.id, completedAt: at.toISOString(), date: local(at), hour: at.getHours(), note: 'edited at 200%' } });
  expect(posted.ok()).toBe(true);
  const id = (await posted.json()).log.id;
  await page.goto('/activity');
  await page.locator('.hist-row').filter({ hasText: 'edited at 200%' }).click();
  await expect(time).toBeVisible();
  await sheet.locator('#log-note').fill('edited at 200%, time kept');
  await sheet.getByRole('button', { name: 'Update' }).click();
  await expect(sheet).toHaveCount(0);
  await expect.poll(async () => (await logs()).find(l => l.id === id)?.note).toBe('edited at 200%, time kept');
  log = (await logs()).find(l => l.id === id);
  expect(new Date(log.completedAt).getTime()).toBe(at.getTime());
  await page.locator('.hist-row').filter({ hasText: 'time kept' }).click();
  await time.fill('04:15');
  await sheet.getByRole('button', { name: 'Update' }).click();
  await expect(sheet).toHaveCount(0);
  await expect.poll(async () => {
    const changed = new Date((await logs()).find(l => l.id === id).completedAt);
    return [local(changed), changed.getHours(), changed.getMinutes()];
  }).toEqual([local(at), 4, 15]);
});
