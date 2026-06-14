// tests/e2e/log-rating.spec.js
// Tests for star rating on chore logs (Read Book, Watch Movie).

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-rate-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Rating Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  const chores = (await (await page.request.get('/api/chores')).json()).chores || [];
  const readBook = chores.find(c => c.name === 'Read Book');
  const watchMovie = chores.find(c => c.name === 'Watch Movie');
  const feedCats = chores.find(c => c.name === 'Feed Cats');

  return { csrf, chores, readBook, watchMovie, feedCats };
}

async function tapChoreCard(page, chore) {
  const card = page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
  await expect(card).toBeVisible();
  await card.click();
  await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
}

test.describe('Star rating on chore logs', () => {
  test('Read Book and Watch Movie are predefined defaults with hasRating', async ({ page }) => {
    const { readBook, watchMovie } = await setupWithChores(page);
    expect(readBook).toBeDefined();
    expect(readBook.hasRating).toBe(true);
    expect(readBook.icon).toBe('📖');
    expect(watchMovie).toBeDefined();
    expect(watchMovie.hasRating).toBe(true);
    expect(watchMovie.icon).toBe('🎬');
  });

  test('log sheet for Read Book shows star rating widget', async ({ page }) => {
    const { readBook } = await setupWithChores(page);
    await tapChoreCard(page, readBook);

    await expect(page.locator('.star-rating')).toBeVisible();
    await expect(page.locator('.star-rating-bg')).toBeVisible();
    await expect(page.locator('.star-rating-fg')).toBeVisible();
  });

  test('log sheet for non-rating chore does not show star rating widget', async ({ page }) => {
    const { feedCats } = await setupWithChores(page);
    await tapChoreCard(page, feedCats);

    await expect(page.locator('.star-rating')).toHaveCount(0);
  });

  test('saves rating via API when logging a chore', async ({ page }) => {
    const { readBook } = await setupWithChores(page);
    await tapChoreCard(page, readBook);

    await expect(page.locator('.star-rating')).toBeVisible();

    const starRating = page.locator('.star-rating');
    const box = await starRating.boundingBox();
    expect(box).toBeDefined();

    // Click at 60% of the star widget = 3 stars (rating 30)
    const clickX = box.x + box.width * 0.6;
    const clickY = box.y + box.height / 2;
    await page.mouse.click(clickX, clickY);

    await page.fill('#log-note', 'The Hobbit');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[readBook.id];
    expect(log).toBeDefined();
    expect(log.rating).toBe(30);
    expect(log.note).toBe('The Hobbit');
  });

  test('rating appears in history view', async ({ page }) => {
    const { readBook } = await setupWithChores(page);
    await tapChoreCard(page, readBook);

    // Click at 70% = 3.5 stars (rating 35)
    const starRating = page.locator('.star-rating');
    const box = await starRating.boundingBox();
    await page.mouse.click(box.x + box.width * 0.7, box.y + box.height / 2);

    await page.fill('#log-note', 'Dune');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    const meta = page.locator('.hist-meta').first();
    await expect(meta).toContainText('3.5');
    await expect(meta).toContainText('⭐');
  });

  test('rating persists on page reload', async ({ page }) => {
    const { readBook } = await setupWithChores(page);
    await tapChoreCard(page, readBook);

    const starRating = page.locator('.star-rating');
    const box = await starRating.boundingBox();
    await page.mouse.click(box.x + box.width * 0.8, box.y + box.height / 2);

    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[readBook.id];
    expect(log).toBeDefined();
    expect(log.rating).toBe(40);
  });

  test('can edit an existing log and change the rating', async ({ page }) => {
    const { readBook, csrf } = await setupWithChores(page);

    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    const when = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T${pad(now.getHours())}:${pad(now.getMinutes())}`;

    // Create a log with rating 10 (1 star) via API
    const createResp = await page.request.post('/api/logs', {
      data: {
        choreId: readBook.id,
        note: 'Initial',
        indicators: [],
        rating: 10,
        completedAt: new Date().toISOString(),
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(createResp.status()).toBe(201);

    // Open history and find the log
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await page.click('[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    const histRow = page.locator('.hist-row').first();
    await expect(histRow).toBeVisible();
    await histRow.click();

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('.star-rating-fg')).toBeVisible();

    // Change rating to 25 (2.5 stars) by clicking at 50%
    const starRating = page.locator('.star-rating');
    const box = await starRating.boundingBox();
    await page.mouse.click(box.x + box.width * 0.5, box.y + box.height / 2);

    await page.fill('#log-note', 'Updated');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[readBook.id];
    expect(log).toBeDefined();
    expect(log.rating).toBe(25);
    expect(log.note).toBe('Updated');
  });

  test('clear button removes rating', async ({ page }) => {
    const { readBook } = await setupWithChores(page);
    await tapChoreCard(page, readBook);

    const starRating = page.locator('.star-rating');
    const box = await starRating.boundingBox();
    await page.mouse.click(box.x + box.width * 0.9, box.y + box.height / 2);

    // Clear the rating
    await page.locator('.star-clear-btn').click();

    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[readBook.id];
    expect(log).toBeDefined();
    expect(log.rating).toBeFalsy();
  });

  test('Watch Movie chore also supports rating', async ({ page }) => {
    const { watchMovie } = await setupWithChores(page);
    await tapChoreCard(page, watchMovie);

    await expect(page.locator('.star-rating')).toBeVisible();

    const starRating = page.locator('.star-rating');
    const box = await starRating.boundingBox();
    await page.mouse.click(box.x + box.width * 0.4, box.y + box.height / 2);

    await page.fill('#log-note', 'Inception');
    await page.click('[data-action="save-log"]');
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const resp = await page.request.get('/api/logs/latest-per-chore');
    const body = await resp.json();
    const log = body.latestLogs[watchMovie.id];
    expect(log).toBeDefined();
    expect(log.rating).toBe(20);
    expect(log.note).toBe('Inception');
  });
});
