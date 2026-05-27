// tests/e2e/home-remove-chore.spec.js
// End-to-end tests for removing a chore from the home grid via long-press + X.
//
// Feature: long-pressing a home card enters jiggle mode; each card then shows
// a red X badge.  Tapping X opens a confirmation sheet.  Confirming hides the
// chore from the home grid for that user (stored in user_preferences).  The
// chore is NOT deleted — it still appears in the Chores tab.
//
// DOM structure in jiggle mode (post-fix):
//   <div class="home-card-wrapper" draggable="true" data-home-reorder-chore-id="N">
//     <button class="home-card-remove" …>✕</button>          ← sibling of card
//     <button class="home-chore-card home-chore-card--jiggle" …>
//       <span class="home-card-icon">…</span>
//       <span class="home-card-name">…</span>                ← inside the card
//     </button>
//   </div>
//
// The previous implementation nested the X badge <button> inside the card
// <button>, which is invalid HTML.  Browsers auto-closed the outer button,
// emptying the cards.  That bug is now fixed.  Selectors that query
// .home-chore-card .home-card-name work correctly in jiggle mode.

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-remove-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers a new user, creates a household, seeds default chores, and waits
 * for the home grid to be visible.  Returns { csrf }.
 */
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
    data: { name: `Remove Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { csrf };
}

/**
 * Simulates a long-press (≥500 ms) on a locator by holding mousedown.
 * Matches the 500 ms threshold in app.js.
 */
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

/**
 * Enters jiggle mode by long-pressing the first card and waits for the Done
 * button to appear.
 *
 * Includes a 100 ms wait after mouseup so the longPressJustFired flag (reset
 * after 50 ms) is cleared before any subsequent click is dispatched.
 */
async function enterJiggleMode(page) {
  const firstCard = page.locator('.home-chore-card').first();
  await longPress(page, firstCard);
  // Allow the 50 ms longPressJustFired reset to fire before any subsequent
  // click, otherwise the click handler swallows the next intentional tap.
  await page.waitForTimeout(100);
  await expect(page.locator('[data-action="exit-jiggle-mode"]')).toBeVisible({ timeout: 3000 });
}

/**
 * Dispatches a click event directly on the nth X button.
 *
 * We use dispatchEvent instead of locator.click() because the sticky top-bar
 * (z-index 100) visually covers the X badges after Playwright scrolls them
 * into view, causing coordinate-based clicks to land on the header instead.
 */
async function clickRemoveButton(page, index) {
  await page.locator('.home-card-remove').nth(index).dispatchEvent('click');
}

// ─── X button visibility ──────────────────────────────────────────────────────

test.describe('Home Remove: X button in jiggle mode', () => {
  test('X remove buttons are visible on every card in jiggle mode', async ({ page }) => {
    await setupWithChores(page);
    await enterJiggleMode(page);

    const removeBtns = page.locator('.home-card-remove');
    // 14 default chores → 14 X buttons
    await expect(removeBtns).toHaveCount(14);
    await expect(removeBtns.first()).toBeVisible();
  });

  test('X buttons are absent in normal (non-jiggle) mode', async ({ page }) => {
    await setupWithChores(page);

    // No jiggle mode — X buttons must not exist at all
    await expect(page.locator('.home-card-remove')).toHaveCount(0);
  });

  test('exiting jiggle mode removes the X buttons', async ({ page }) => {
    await setupWithChores(page);
    await enterJiggleMode(page);

    await expect(page.locator('.home-card-remove')).toHaveCount(14);

    await page.locator('[data-action="exit-jiggle-mode"]').click();
    await page.waitForTimeout(300);

    await expect(page.locator('.home-card-remove')).toHaveCount(0);
  });
});

// ─── Confirmation sheet ───────────────────────────────────────────────────────

test.describe('Home Remove: confirmation sheet', () => {
  test('tapping X opens the confirmation bottom sheet', async ({ page }) => {
    await setupWithChores(page);
    await enterJiggleMode(page);

    await clickRemoveButton(page, 4);
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-action="confirm-remove-home-chore"]')).toBeVisible();
    await expect(page.locator('button[data-action="close-sheet"]')).toBeVisible();
  });

  test('confirmation sheet contains the chore name', async ({ page }) => {
    await setupWithChores(page);

    const cardName = await page.locator('.home-chore-card').nth(4).locator('.home-card-name').innerText();

    await enterJiggleMode(page);
    await clickRemoveButton(page, 4);
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('.bottom-sheet')).toContainText(cardName);
  });

  test('Cancel closes the sheet without removing the chore', async ({ page }) => {
    await setupWithChores(page);
    const initialCount = await page.locator('.home-chore-card').count();

    await enterJiggleMode(page);
    await clickRemoveButton(page, 4);
    await page.waitForTimeout(400);

    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await page.locator('button[data-action="close-sheet"]').click();
    await page.waitForTimeout(300);

    // Sheet gone, card count unchanged
    await expect(page.locator('.bottom-sheet')).toHaveCount(0);
    await expect(page.locator('.home-chore-card')).toHaveCount(initialCount);
  });
});

// ─── Removing a chore ─────────────────────────────────────────────────────────

test.describe('Home Remove: removing a chore', () => {
  test('confirming removes the chore card from the home grid', async ({ page }) => {
    await setupWithChores(page);
    const initialCount = await page.locator('.home-chore-card').count();

    const removedName = await page.locator('.home-chore-card').nth(4).locator('.home-card-name').innerText();

    await enterJiggleMode(page);
    await clickRemoveButton(page, 4);
    await page.waitForTimeout(400);
    await page.locator('[data-action="confirm-remove-home-chore"]').click();
    await page.waitForTimeout(600);

    // One fewer card
    await expect(page.locator('.home-chore-card')).toHaveCount(initialCount - 1);

    // The removed chore's name no longer appears in the grid.
    const names = await page.locator('.home-chore-card .home-card-name').allInnerTexts();
    expect(names).not.toContain(removedName);
  });

  test('removed chore still appears in the Chores tab', async ({ page }) => {
    await setupWithChores(page);

    const removedName = await page.locator('.home-chore-card').nth(4).locator('.home-card-name').innerText();

    await enterJiggleMode(page);
    await clickRemoveButton(page, 4);
    await page.waitForTimeout(400);
    await page.locator('[data-action="confirm-remove-home-chore"]').click();
    await page.waitForTimeout(600);

    // Navigate to Chores tab
    await page.click('[data-nav="chores"]');
    await page.waitForTimeout(500);

    // Chore should still be listed there
    await expect(page.locator('.chores-view')).toContainText(removedName);
  });

  test('hidden chore persists across a full page reload', async ({ page }) => {
    await setupWithChores(page);

    const removedName = await page.locator('.home-chore-card').nth(4).locator('.home-card-name').innerText();

    await enterJiggleMode(page);
    await clickRemoveButton(page, 4);
    await page.waitForTimeout(400);

    // Gate the reload on the PATCH to /api/preferences completing, so the
    // hidden-chore preference is persisted before we reload the page.
    const patchDone = page.waitForResponse(
      r => r.url().includes('/api/preferences') && r.request().method() === 'PATCH'
    );
    await page.locator('[data-action="confirm-remove-home-chore"]').click();
    await patchDone;
    // Brief wait for DOM optimistic update to settle
    await page.waitForTimeout(200);

    const countAfterRemove = await page.locator('.home-chore-card').count();

    // Full reload — hidden list must be loaded from server
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    await expect(page.locator('.home-chore-card')).toHaveCount(countAfterRemove);
    // After reload in normal mode, .home-card-name is inside .home-chore-card
    const names = await page.locator('.home-chore-card .home-card-name').allInnerTexts();
    expect(names).not.toContain(removedName);
  });

  test('PATCH /api/preferences with hiddenHomeChoreIds updates preferences', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    const choreId = chores[0].id;

    const patchResp = await page.request.patch('/api/preferences', {
      data: { hiddenHomeChoreIds: [choreId] },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(patchResp.status()).toBe(200);
    const { preferences } = await patchResp.json();
    expect(preferences.hiddenHomeChoreIds).toContain(choreId);
  });

  test('hiddenHomeChoreIds and choreOrder are preserved independently', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const { chores } = await (await page.request.get('/api/chores')).json();
    const ids = chores.map(c => c.id);

    // Set a chore order
    await page.request.patch('/api/preferences', {
      data: { choreOrder: [ids[2], ids[1], ids[0]] },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Hide a chore — must not wipe out choreOrder
    await page.request.patch('/api/preferences', {
      data: { hiddenHomeChoreIds: [ids[0]] },
      headers: { 'X-CSRF-Token': csrf },
    });

    const prefsResp = await page.request.get('/api/preferences');
    const { preferences } = await prefsResp.json();
    expect(preferences.choreOrder).toEqual([ids[2], ids[1], ids[0]]);
    expect(preferences.hiddenHomeChoreIds).toContain(ids[0]);
  });
});
