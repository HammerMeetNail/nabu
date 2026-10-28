// tests/e2e/desktop-layout.spec.js
// The wide layout (900 px and up: desktops, tablets in landscape). The tab bar
// becomes a sidebar with Home first, Home shows the household's latest logs
// beside its grid, sheets open as dialogs, and keys 1–9, u and / log, undo
// and search. Phones keep the bottom tab bar.

import { test, expect } from '@playwright/test';

const WIDE = { width: 1280, height: 800 };

function uniqueEmail() {
  return `e2e-desk-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers through the API, creates a household with the default chores,
 * and opens Home. Returns the request headers and the chores.
 */
async function setup(page) {
  await page.goto('/register');
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf').value;
  const headers = { 'X-CSRF-Token': csrf };
  const registered = await page.request.post('/api/auth/register', {
    data: { email: uniqueEmail(), password: 'test123456' }, headers,
  });
  expect(registered.status()).toBe(201);
  expect((await page.request.post('/api/household', { data: { name: 'Desk House' }, headers })).status()).toBe(201);
  expect((await page.request.post('/api/chores/seed-defaults', { headers })).ok()).toBeTruthy();
  const { chores } = await (await page.request.get('/api/chores')).json();
  await page.goto('/today');
  await expect(page.locator('.home-grid')).toBeVisible();
  return { headers, chores };
}

// Logs a chore now through the API, as another device would.
async function logViaAPI(page, headers, chore, note = '') {
  const now = new Date();
  const pad = n => String(n).padStart(2, '0');
  const response = await page.request.post('/api/logs', {
    data: {
      choreId: chore.id, note, indicators: [], completedAt: now.toISOString(), hour: now.getHours(),
      date: `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`,
      idempotencyKey: `${Date.now()}-${Math.random()}`,
    },
    headers,
  });
  expect(response.ok()).toBeTruthy();
}

const activityColumn = page => page.getByRole('complementary', { name: 'Recent activity' });

test.describe('wide layout', () => {
  // A desktop: a mouse, so the shortcut hints show.
  test.use({ viewport: WIDE, hasTouch: false });

  test('puts the tabs in a sidebar, Home first, with the latest logs beside the grid', async ({ page }) => {
    const { headers, chores } = await setup(page);
    await logViaAPI(page, headers, chores[1], 'Took the long way');
    await page.reload();
    await expect(page.locator('.home-grid')).toBeVisible();

    const nav = page.locator('#bottom-tabs');
    const navBox = await nav.boundingBox();
    expect(navBox.x).toBe(0);
    expect(navBox.y).toBe(0);
    expect(navBox.height).toBe(WIDE.height);
    expect(navBox.width).toBeLessThan(300);
    expect(await nav.locator('.tab-item').evaluateAll(tabs => tabs.map(tab => tab.dataset.nav)))
      .toEqual(['today', 'stats', 'activity', 'schedule', 'settings']);
    await expect(page.locator('.sidebar-brand')).toBeVisible();
    await expect(page.locator('.sidebar-shortcuts')).toBeVisible();
    // Content keeps a readable width instead of stretching.
    expect((await page.locator('#app').boundingBox()).width).toBeLessThanOrEqual(1120);

    const column = activityColumn(page);
    const rows = column.locator('button[data-log-id]');
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toContainText(chores[1].name);
    await expect(rows.first()).toContainText('Took the long way');
    await expect(column.locator('h3')).toHaveText(['Today']);
    const grid = await page.locator('.home-grid').boundingBox();
    expect((await column.boundingBox()).x).toBeGreaterThanOrEqual(grid.x + grid.width);

    // A row opens its log, as on the Activity tab.
    await rows.first().click();
    const sheet = page.locator('.bottom-sheet');
    await expect(sheet.locator('[data-action="save-log"]')).toHaveText('Update');
    await page.keyboard.press('Escape');
    await expect(sheet).toHaveCount(0);

    await column.getByRole('link', { name: 'See all' }).click();
    await expect(page).toHaveURL(/\/activity$/);
    await expect(page.locator('.tab-item[data-nav="activity"]')).toHaveClass(/active/);
    await expect(page.locator('.hist-row')).toHaveCount(1);
  });

  test('1 opens the first tile as a dialog, the log joins the column, and u undoes it', async ({ page }) => {
    await setup(page);
    const column = activityColumn(page);
    await expect(column).toContainText('Nothing logged in the last 7 days.');
    const first = page.locator('.home-chore-card').first();
    await expect(first.locator('.home-card-key')).toHaveText('1');
    await expect(first).toHaveAttribute('aria-keyshortcuts', '1');
    const name = (await first.locator('.home-card-name').textContent()).trim();

    await page.keyboard.press('1');
    const sheet = page.locator('.bottom-sheet');
    await expect(sheet).toBeVisible();
    // A dialog in the middle of the window, not a sheet along its bottom.
    const box = await sheet.boundingBox();
    expect(Math.abs(box.x + box.width / 2 - WIDE.width / 2)).toBeLessThan(2);
    expect(Math.abs(box.y + box.height / 2 - WIDE.height / 2)).toBeLessThan(2);
    expect(box.width).toBeLessThanOrEqual(520);
    await sheet.locator('[data-action="save-log"]').click();
    await expect(sheet).toHaveCount(0);
    await expect(column.locator('button[data-log-id]')).toHaveCount(1);
    await expect(column.locator('button[data-log-id]').first()).toContainText(name);
    // Closing the sheet returned focus to the tile the key opened.
    await expect(first).toBeFocused();

    await expect(page.locator('#toast-container [data-undo-toast]')).toBeVisible();
    await page.keyboard.press('u');
    await expect(column.locator('button[data-log-id]')).toHaveCount(0);
    await expect(column).toContainText('Nothing logged in the last 7 days.');
    await page.reload();
    await expect(page.locator('.home-grid')).toBeVisible();
    await expect(column).toContainText('Nothing logged in the last 7 days.');
  });

  test('keys do nothing in a text field or over a sheet, and / searches Activity', async ({ page }) => {
    await setup(page);
    await page.keyboard.press('2');
    const sheet = page.locator('.bottom-sheet');
    await expect(sheet).toBeVisible();
    const title = await sheet.locator('.sheet-title').textContent();
    // Typing a note is not a shortcut, nor is a key over the open sheet.
    await sheet.locator('textarea').fill('');
    await sheet.locator('textarea').pressSequentially('3 u/');
    await expect(sheet.locator('textarea')).toHaveValue('3 u/');
    await sheet.locator('[data-action="save-log"]').focus();
    await page.keyboard.press('4');
    await expect(sheet.locator('.sheet-title')).toHaveText(title);
    await page.keyboard.press('Escape');
    await expect(sheet).toHaveCount(0);

    await page.keyboard.press('/');
    await expect(page).toHaveURL(/\/activity$/);
    await expect(page.locator('#history-search-input')).toBeFocused();
    // Typing into the search is not a shortcut either.
    await page.keyboard.type('1u');
    await expect(page.locator('#history-search-input')).toHaveValue('1u');
    await expect(page).toHaveURL(/\/activity$/);
  });

  test('another member’s log reaches the column within a minute', async ({ page }) => {
    await page.clock.install();
    const { headers, chores } = await setup(page);
    const column = activityColumn(page);
    await expect(column).toContainText('Nothing logged in the last 7 days.');
    await logViaAPI(page, headers, chores[2]);
    await page.clock.runFor(61000);
    await expect(column.locator('button[data-log-id]')).toHaveCount(1);
    await expect(column).toContainText(chores[2].name);
  });

  test('Stats sections fill two columns', async ({ page }) => {
    await setup(page);
    await page.locator('.tab-item[data-nav="stats"]').click();
    const lastDone = page.locator('[data-stats-section="last-done"]');
    const overview = page.locator('[data-stats-section="overview"]');
    await expect(lastDone).toBeVisible();
    const [last, over] = [await lastDone.boundingBox(), await overview.boundingBox()];
    // The overview spans both columns; a section takes about half of that.
    expect(last.width).toBeLessThan(over.width * 0.6);
  });

  test('leaving the wide layout brings back the tab bar and drops the column', async ({ page }) => {
    await setup(page);
    await expect(activityColumn(page)).toBeVisible();
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(activityColumn(page)).toHaveCount(0);
    await expect(page.locator('.home-card-key')).toHaveCount(0);
    const tabs = page.locator('#bottom-tabs');
    expect((await tabs.boundingBox()).y).toBeGreaterThan(700);
    expect(await tabs.locator('.tab-item').evaluateAll(items => items.map(item => item.dataset.nav)))
      .toEqual(['stats', 'activity', 'today', 'schedule', 'settings']);
    await page.setViewportSize(WIDE);
    await expect(activityColumn(page)).toBeVisible();
    expect(await tabs.locator('.tab-item').first().getAttribute('data-nav')).toBe('today');
  });
});

test.describe('a tablet in landscape', () => {
  test.use({ viewport: { width: 1180, height: 820 }, hasTouch: true });

  test('gets the sidebar and the column, without keyboard hints', async ({ page }) => {
    await setup(page);
    expect((await page.locator('#bottom-tabs').boundingBox()).x).toBe(0);
    await expect(activityColumn(page)).toBeVisible();
    await expect(page.locator('.sidebar-shortcuts')).toBeHidden();
    await expect(page.locator('.home-card-key').first()).toBeHidden();
  });
});

test('a phone in landscape keeps the compact bottom tab bar', async ({ page }) => {
  await page.setViewportSize({ width: 844, height: 390 });
  await setup(page);
  const tabs = await page.locator('#bottom-tabs').boundingBox();
  expect(tabs.y + tabs.height).toBe(390);
  expect(tabs.width).toBe(844);
  await expect(activityColumn(page)).toHaveCount(0);
  await expect(page.locator('.sidebar-brand')).toBeHidden();
});
