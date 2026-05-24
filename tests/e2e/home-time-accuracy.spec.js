// tests/e2e/home-time-accuracy.spec.js
// Regression test: home-tab direct tap must store completedAt as the current
// time, not noon UTC.  Before the fix, home-tap-chore sent only `date` + `hour`
// and the server fell back to noon UTC, making formatTimeAgo show "Xh ago"
// immediately after logging (the offset matched the user's UTC offset).

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-hta-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers a new user, creates a household, seeds default chores, and waits
 * for the home grid to be visible.  Returns { csrf, chores }.
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
    data: { name: `HTAcc Test ${Date.now()}` },
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

// ─── Tests ────────────────────────────────────────────────────────────────────

test.describe('Home tab: completedAt accuracy', () => {
  test('direct-tap stores completedAt at the current time, not noon UTC', async ({ page }) => {
    const { chores } = await setupWithChores(page);

    // Find a chore with no indicator labels — these log instantly on tap
    // without opening the home-log sheet.
    const noIndicatorChore = chores.find(
      c => !c.indicatorLabels || c.indicatorLabels.length === 0
    );
    expect(noIndicatorChore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${noIndicatorChore.id}"]`);
    const choreName = await card.locator('.home-card-name').innerText();

    // Record the time just before the tap.
    const beforeMs = Date.now();

    // Tap the chore card (logs instantly, no sheet).
    await card.click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    const afterMs = Date.now();

    // The home card should display "just now" after a fresh log.
    await expect(card.locator('.home-card-time')).toHaveText('just now');

    // Verify the stored completedAt via the latest-per-chore API.
    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    const log = latestLogs[noIndicatorChore.id];
    expect(log).toBeDefined();
    const completedAtMs = new Date(log.completedAt).getTime();

    // The stored timestamp must be within 10 seconds of the tap window.
    const tolerance = 10000;
    expect(completedAtMs).toBeGreaterThanOrEqual(beforeMs - tolerance);
    expect(completedAtMs).toBeLessThanOrEqual(afterMs + tolerance);
  });

  test('stored completedAt survives page reload and shows consistent time ago', async ({ page }) => {
    const { chores } = await setupWithChores(page);

    const noIndicatorChore = chores.find(
      c => !c.indicatorLabels || c.indicatorLabels.length === 0
    );
    expect(noIndicatorChore).toBeDefined();

    const card = page.locator(`.home-chore-card[data-home-chore-id="${noIndicatorChore.id}"]`);

    // Log the chore.
    await card.click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Record the displayed time text.
    const timeText = await card.locator('.home-card-time').innerText();
    expect(timeText).toBe('just now');

    // Reload the page.
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    // The time ago should still be very recent (within a minute) — it should
    // NOT jump to something like "7h ago".
    const reloadCard = page.locator(`.home-chore-card[data-home-chore-id="${noIndicatorChore.id}"]`);
    const reloadTimeText = await reloadCard.locator('.home-card-time').innerText();
    expect(reloadTimeText).toMatch(/^(just now|\d+m ago)$/);
  });
});
