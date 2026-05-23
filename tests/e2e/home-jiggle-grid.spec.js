// tests/e2e/home-jiggle-grid.spec.js
// End-to-end tests for jiggle-mode grid structure and drag-to-reorder.
//
// Root cause this covers: the previous implementation nested the X badge
// <button> inside the card <button>.  Nesting interactive elements is invalid
// HTML; browsers auto-close the outer button, ejecting the card's content
// (icon, name, time) as siblings of the empty card.  The fix wraps each card
// in a <div class="home-card-wrapper"> so the badge and card button are
// siblings, not nested.

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-jiggle-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#user-avatar:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Jiggle Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { csrf };
}

async function longPress(page, locator) {
  await locator.scrollIntoViewIfNeeded();
  const box = await locator.boundingBox();
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.waitForTimeout(650);
  await page.mouse.up();
}

async function enterJiggleMode(page) {
  const firstCard = page.locator('.home-chore-card').first();
  await longPress(page, firstCard);
  await page.waitForTimeout(100);
  await expect(page.locator('[data-action="exit-jiggle-mode"]')).toBeVisible({ timeout: 3000 });
}

// ─── DOM structure ────────────────────────────────────────────────────────────

test.describe('Home Jiggle: card DOM structure', () => {
  test('jiggle cards contain icon and name (not empty buttons)', async ({ page }) => {
    await setupWithChores(page);
    await enterJiggleMode(page);

    const cards = page.locator('.home-chore-card');
    const count = await cards.count();
    expect(count).toBeGreaterThan(0);

    // Spot-check the first three cards — each must contain the name and icon spans.
    for (let i = 0; i < Math.min(count, 3); i++) {
      await expect(cards.nth(i).locator('.home-card-name')).toBeVisible();
      await expect(cards.nth(i).locator('.home-card-icon')).toBeVisible();
    }
  });

  test('X badge is NOT nested inside .home-chore-card (no invalid button-in-button)', async ({ page }) => {
    await setupWithChores(page);
    await enterJiggleMode(page);

    // With the fix the badge is a sibling of the card inside .home-card-wrapper.
    // A nested badge (.home-chore-card .home-card-remove) indicates the bug is back.
    await expect(page.locator('.home-chore-card .home-card-remove')).toHaveCount(0);
  });

  test('X badge is inside .home-card-wrapper (correct sibling structure)', async ({ page }) => {
    await setupWithChores(page);
    await enterJiggleMode(page);

    const cardCount = await page.locator('.home-chore-card').count();
    // Every jiggle card must have a wrapper with the badge as a direct child.
    await expect(page.locator('.home-card-wrapper .home-card-remove')).toHaveCount(cardCount);
  });

  test('.home-card-wrapper carries draggable and reorder data attribute', async ({ page }) => {
    await setupWithChores(page);
    await enterJiggleMode(page);

    const wrappers = page.locator('.home-card-wrapper[data-home-reorder-chore-id]');
    const cardCount = await page.locator('.home-chore-card').count();
    await expect(wrappers).toHaveCount(cardCount);
  });

  test('normal mode cards are plain buttons with no wrapper', async ({ page }) => {
    await setupWithChores(page);

    // No wrappers in normal mode
    await expect(page.locator('.home-card-wrapper')).toHaveCount(0);
    // Cards are present and contain names
    const cards = page.locator('.home-chore-card');
    await expect(cards.first().locator('.home-card-name')).toBeVisible();
  });
});

// ─── Drag-to-reorder ──────────────────────────────────────────────────────────

/**
 * Fires HTML5 DragEvent pairs so the app's global listeners pick them up.
 * DataTransfer must be constructed inside page.evaluate — it cannot be passed
 * as a plain object via dispatchEvent arguments.
 */
async function htmlDragDrop(page, sourceLocator, targetLocator) {
  const [srcHandle, tgtHandle] = await Promise.all([
    sourceLocator.elementHandle(),
    targetLocator.elementHandle(),
  ]);
  await page.evaluate(([src, tgt]) => {
    const dt = new DataTransfer();
    src.dispatchEvent(new DragEvent('dragstart', { dataTransfer: dt, bubbles: true, cancelable: true }));
    tgt.dispatchEvent(new DragEvent('dragover',  { dataTransfer: dt, bubbles: true, cancelable: true }));
    tgt.dispatchEvent(new DragEvent('drop',      { dataTransfer: dt, bubbles: true, cancelable: true }));
    src.dispatchEvent(new DragEvent('dragend',   { dataTransfer: dt, bubbles: true }));
  }, [srcHandle, tgtHandle]);
}

test.describe('Home Jiggle: drag-to-reorder', () => {
  test('dragging a card changes the order and persists after Done', async ({ page }) => {
    await setupWithChores(page);

    // Capture names in normal mode (cards intact)
    const allCards = page.locator('.home-chore-card');
    const nameBefore0 = await allCards.nth(0).locator('.home-card-name').innerText();
    const nameBefore2 = await allCards.nth(2).locator('.home-card-name').innerText();

    await enterJiggleMode(page);

    // Drag wrapper[0] onto wrapper[2].
    // Set up the waitForResponse BEFORE dispatching the drag so we cannot
    // miss a fast async PATCH that resolves before the await below.
    const wrappers = page.locator('.home-card-wrapper');
    const patchPromise = page.waitForResponse(
      r => r.url().includes('/api/preferences') && r.request().method() === 'PATCH',
    );
    await htmlDragDrop(page, wrappers.nth(0), wrappers.nth(2));

    // Wait for the saveChoreOrder API call and re-render
    await patchPromise;
    await page.waitForTimeout(200);

    // Exit jiggle mode
    await page.locator('[data-action="exit-jiggle-mode"]').click();
    await page.waitForTimeout(200);

    // The card that was first should no longer be first
    const nameAfter0 = await page.locator('.home-chore-card').nth(0).locator('.home-card-name').innerText();
    expect(nameAfter0).not.toBe(nameBefore0);
    // All original chores should still be present
    const allNames = await page.locator('.home-chore-card .home-card-name').allInnerTexts();
    expect(allNames).toContain(nameBefore0);
    expect(allNames).toContain(nameBefore2);

    // The order persists after reload
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    const nameAfterReload0 = await page.locator('.home-chore-card').nth(0).locator('.home-card-name').innerText();
    expect(nameAfterReload0).toBe(nameAfter0);
  });
});
