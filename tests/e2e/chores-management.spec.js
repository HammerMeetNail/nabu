// tests/e2e/chores-management.spec.js
// End-to-end tests for the new Chores tab management UI.
//
// Covers:
//   - Manage Chores view displays all chores (including hidden ones)
//   - Add custom chore: happy path, empty-name sad path, cancel
//   - Edit chore: name/icon/color changes persist
//   - Delete custom chore: confirmed deletion removes from list
//   - Eye toggle: hide/show a chore from Home log grid
//   - Restore-to-default for a predefined chore
//   - Persistence across full page reload

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-chores-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

/**
 * Registers a user, creates a household, seeds default chores, and navigates
 * to the Manage Chores view (Home tab → Manage toggle).
 * Returns { csrf, page }.
 */
async function setupManageChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Chores Mgmt Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  // Switch to Manage view from the Home tab.
  await page.click('[data-action="switch-home-view"][data-view="manage"]');
  await page.waitForSelector('.chore-list', { timeout: 10000 });

  return { csrf };
}

// ─── Chores tab display ───────────────────────────────────────────────────────

test.describe('Manage Chores: display', () => {
  test('shows all seeded chores in the list', async ({ page }) => {
    await setupManageChores(page);
    const rows = page.locator('.chore-row');
    // Default seed = 13 chores
    await expect(rows).toHaveCount(13);
  });

  test('shows drag handle, eye toggle, and edit button on each row', async ({ page }) => {
    await setupManageChores(page);
    const firstRow = page.locator('.chore-row').first();
    await expect(firstRow.locator('.chore-row-drag-handle')).toBeVisible();
    await expect(firstRow.locator('[data-action="chore-toggle-home"]')).toBeVisible();
    await expect(firstRow.locator('[data-action="chore-edit"]')).toBeVisible();
  });

  test('shows Default badge on predefined chores and Custom badge on user-added chores', async ({ page }) => {
    const { csrf } = await setupManageChores(page);

    // First row is a seeded default chore — must show "Default"
    const firstBadge = page.locator('.chore-row').first().locator('.chore-row-badge');
    await expect(firstBadge).toHaveClass(/chore-row-badge--default/);
    await expect(firstBadge).toContainText('Default');

    // Add a custom chore via API
    await page.request.post('/api/chores', {
      data: { name: 'My Custom Chore' },
      headers: { 'X-CSRF-Token': csrf },
    });
    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-list', { timeout: 10000 });

    // Find the custom chore row — it should show "Custom"
    const customRow = page.locator('.chore-row', { hasText: 'My Custom Chore' });
    await expect(customRow.locator('.chore-row-badge')).toHaveClass(/chore-row-badge--custom/);
    await expect(customRow.locator('.chore-row-badge')).toContainText('Custom');
  });

  test('FAB + button is visible', async ({ page }) => {
    await setupManageChores(page);
    await expect(page.locator('.fab[data-action="chore-add"]')).toBeVisible();
  });
});

// ─── Add custom chore ────────────────────────────────────────────────────────

test.describe('Manage Chores: add custom chore', () => {
  test('tapping + opens the add-chore sheet', async ({ page }) => {
    await setupManageChores(page);
    await page.click('.fab[data-action="chore-add"]');
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('.sheet-title')).toContainText('Add Chore');
  });

  test('Cancel closes the sheet without adding a chore', async ({ page }) => {
    await setupManageChores(page);
    const countBefore = await page.locator('.chore-row').count();

    await page.click('.fab[data-action="chore-add"]');
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });
    await page.click('.chore-edit-sheet button[data-action="close-sheet"]');
    await page.waitForTimeout(300);

    await expect(page.locator('.chore-edit-sheet')).toHaveCount(0);
    await expect(page.locator('.chore-row')).toHaveCount(countBefore);
  });

  test('saving with empty name does not create a chore', async ({ page }) => {
    await setupManageChores(page);
    const countBefore = await page.locator('.chore-row').count();

    await page.click('.fab[data-action="chore-add"]');
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });
    // Leave name empty — click Save
    await page.click('[data-action="save-chore"]');
    await page.waitForTimeout(400);

    // Sheet should still be open (focus returned to name input)
    await expect(page.locator('.chore-edit-sheet')).toBeVisible();
    await expect(page.locator('.chore-row')).toHaveCount(countBefore);
  });

  test('adding a new chore appears in the list', async ({ page }) => {
    await setupManageChores(page);
    const countBefore = await page.locator('.chore-row').count();

    await page.click('.fab[data-action="chore-add"]');
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });
    await page.fill('#chore-edit-name', 'Walk the dog');
    await page.click('[data-action="save-chore"]');

    // Wait for sheet to close and new row to appear
    await expect(page.locator('.chore-edit-sheet')).toHaveCount(0, { timeout: 5000 });
    await expect(page.locator('.chore-row')).toHaveCount(countBefore + 1);
    await expect(page.locator('.chore-list')).toContainText('Walk the dog');
  });

  test('new chore also appears on the Home tab grid', async ({ page }) => {
    await setupManageChores(page);

    await page.click('.fab[data-action="chore-add"]');
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });
    await page.fill('#chore-edit-name', 'Feed the fish');
    await page.click('[data-action="save-chore"]');
    await expect(page.locator('.chore-edit-sheet')).toHaveCount(0, { timeout: 5000 });

    // Switch to Log view and verify the new chore appears
    await page.click('[data-action="switch-home-view"][data-view="log"]');
    await page.waitForSelector('.home-grid', { timeout: 10000 });
    await expect(page.locator('.home-grid')).toContainText('Feed the fish');
  });

  test('new chore persists after page reload', async ({ page }) => {
    await setupManageChores(page);

    await page.click('.fab[data-action="chore-add"]');
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });
    await page.fill('#chore-edit-name', 'Water plants');

    const saveDone = page.waitForResponse(
      r => r.url().includes('/api/chores') && r.request().method() === 'POST'
    );
    await page.click('[data-action="save-chore"]');
    await saveDone;
    await expect(page.locator('.chore-edit-sheet')).toHaveCount(0, { timeout: 5000 });

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-list', { timeout: 10000 });

    await expect(page.locator('.chore-list')).toContainText('Water plants');
  });

  test('picking a quick-emoji updates the preview', async ({ page }) => {
    await setupManageChores(page);

    await page.click('.fab[data-action="chore-add"]');
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });

    // Click the first quick-pick emoji button
    const firstEmoji = page.locator('.emoji-quick').first();
    const emojiChar = await firstEmoji.textContent();
    await firstEmoji.click();

    // Preview should now show that emoji
    await expect(page.locator('#chore-icon-preview')).toContainText(emojiChar.trim());
    // Input should also reflect it
    await expect(page.locator('#chore-icon-input')).toHaveValue(emojiChar.trim());
  });
});

// ─── Edit chore ──────────────────────────────────────────────────────────────

test.describe('Manage Chores: edit chore', () => {
  test('tapping edit opens the edit sheet with existing name', async ({ page }) => {
    await setupManageChores(page);

    const firstName = await page.locator('.chore-row-name').first().innerText();
    await page.locator('.chore-row').first().locator('[data-action="chore-edit"]').click();

    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('.sheet-title')).toContainText('Edit Chore');
    await expect(page.locator('#chore-edit-name')).toHaveValue(firstName);
  });

  test('renaming a chore updates the list', async ({ page }) => {
    await setupManageChores(page);

    await page.locator('.chore-row').first().locator('[data-action="chore-edit"]').click();
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });

    await page.fill('#chore-edit-name', 'Renamed Chore XYZ');
    await page.click('[data-action="save-chore"]');
    await expect(page.locator('.chore-edit-sheet')).toHaveCount(0, { timeout: 5000 });

    await expect(page.locator('.chore-list')).toContainText('Renamed Chore XYZ');
  });

  test('edit persists after page reload', async ({ page }) => {
    await setupManageChores(page);

    await page.locator('.chore-row').first().locator('[data-action="chore-edit"]').click();
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });

    await page.fill('#chore-edit-name', 'PersistMe');
    const patchDone = page.waitForResponse(
      r => r.url().includes('/api/chores/') && r.request().method() === 'PATCH'
    );
    await page.click('[data-action="save-chore"]');
    await patchDone;
    await expect(page.locator('.chore-edit-sheet')).toHaveCount(0, { timeout: 5000 });

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-list', { timeout: 10000 });
    await expect(page.locator('.chore-list')).toContainText('PersistMe');
  });
});

// ─── Delete custom chore ─────────────────────────────────────────────────────

test.describe('Manage Chores: delete custom chore', () => {
  /**
   * Creates a custom chore via API and returns its id and name.
   */
  async function createCustomChore(page, csrf, name = 'Delete Me') {
    const resp = await page.request.post('/api/chores', {
      data: { name, icon: '🗑️', color: '#6B7280', category: 'custom' },
      headers: { 'X-CSRF-Token': csrf },
    });
    const { chore } = await resp.json();
    return chore;
  }

  test('editing a custom chore shows Delete button (not Restore)', async ({ page }) => {
    const { csrf } = await setupManageChores(page);
    await createCustomChore(page, csrf, 'Custom Deletable');

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-list', { timeout: 10000 });

    // Find the custom chore row and click edit
    const row = page.locator('.chore-row', { hasText: 'Custom Deletable' });
    await row.locator('[data-action="chore-edit"]').click();
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });

    await expect(page.locator('[data-action="delete-chore"]')).toBeVisible();
    await expect(page.locator('[data-action="restore-chore-default"]')).toHaveCount(0);
  });

  test('confirming delete removes the chore from the list', async ({ page }) => {
    const { csrf } = await setupManageChores(page);
    await createCustomChore(page, csrf, 'Gone Soon');

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await page.click('[data-action="switch-home-view"][data-view="manage"]');
    await page.waitForSelector('.chore-list', { timeout: 10000 });

    const countBefore = await page.locator('.chore-row').count();
    const row = page.locator('.chore-row', { hasText: 'Gone Soon' });
    await row.locator('[data-action="chore-edit"]').click();
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });

    // Accept the confirm() dialog that fires on delete
    page.once('dialog', d => d.accept());
    await page.click('[data-action="delete-chore"]');
    await expect(page.locator('.chore-edit-sheet')).toHaveCount(0, { timeout: 5000 });
    await expect(page.locator('.chore-row')).toHaveCount(countBefore - 1);
    await expect(page.locator('.chore-list')).not.toContainText('Gone Soon');
  });
});

// ─── Eye toggle (hide/show from Home) ────────────────────────────────────────

test.describe('Manage Chores: hide/show from Home', () => {
  test('toggling eye hides chore from Home grid', async ({ page }) => {
    await setupManageChores(page);

    const firstName = await page.locator('.chore-row-name').first().innerText();
    await page.locator('.chore-row').first().locator('[data-action="chore-toggle-home"]').click();
    await page.waitForTimeout(400);

    // Row should gain the --hidden modifier
    await expect(page.locator('.chore-row').first()).toHaveClass(/chore-row--hidden/);

    // Home log grid should no longer show that chore
    await page.click('[data-action="switch-home-view"][data-view="log"]');
    await page.waitForSelector('.home-grid', { timeout: 10000 });
    const names = await page.locator('.home-card-name').allInnerTexts();
    expect(names).not.toContain(firstName);
  });

  test('toggling eye twice restores chore to Home grid', async ({ page }) => {
    await setupManageChores(page);
    const firstName = await page.locator('.chore-row-name').first().innerText();

    // Hide
    await page.locator('.chore-row').first().locator('[data-action="chore-toggle-home"]').click();
    await page.waitForTimeout(400);

    // Show again
    await page.locator('.chore-row').first().locator('[data-action="chore-toggle-home"]').click();
    await page.waitForTimeout(400);

    await expect(page.locator('.chore-row').first()).not.toHaveClass(/chore-row--hidden/);

    // Chore should be back on Home log grid
    await page.click('[data-action="switch-home-view"][data-view="log"]');
    await page.waitForSelector('.home-grid', { timeout: 10000 });
    const names = await page.locator('.home-card-name').allInnerTexts();
    expect(names).toContain(firstName);
  });

  test('eye toggle persists after page reload', async ({ page }) => {
    await setupManageChores(page);
    const firstName = await page.locator('.chore-row-name').first().innerText();

    const patchDone = page.waitForResponse(
      r => r.url().includes('/api/preferences') && r.request().method() === 'PATCH'
    );
    await page.locator('.chore-row').first().locator('[data-action="chore-toggle-home"]').click();
    await patchDone;

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    // Chore should be hidden from Home after reload
    const names = await page.locator('.home-card-name').allInnerTexts();
    expect(names).not.toContain(firstName);
  });

  test('hidden chore is still visible in Manage Chores', async ({ page }) => {
    await setupManageChores(page);
    const firstName = await page.locator('.chore-row-name').first().innerText();

    await page.locator('.chore-row').first().locator('[data-action="chore-toggle-home"]').click();
    await page.waitForTimeout(400);

    // Still in Manage view — hidden row should be present (just dimmed)
    await expect(page.locator('.chore-list')).toContainText(firstName);
  });
});

// ─── Restore to default (predefined chore) ───────────────────────────────────

test.describe('Manage Chores: restore predefined chore to default', () => {
  test('editing a predefined chore shows Restore button (not Delete)', async ({ page }) => {
    await setupManageChores(page);

    // All seeded chores are predefined — open the first one
    await page.locator('.chore-row').first().locator('[data-action="chore-edit"]').click();
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });

    await expect(page.locator('[data-action="restore-chore-default"]')).toBeVisible();
    await expect(page.locator('[data-action="delete-chore"]')).toHaveCount(0);
  });

  test('restoring default reverts a renamed predefined chore', async ({ page }) => {
    await setupManageChores(page);

    // Get original name
    const originalName = await page.locator('.chore-row-name').first().innerText();

    // Rename it
    await page.locator('.chore-row').first().locator('[data-action="chore-edit"]').click();
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });
    await page.fill('#chore-edit-name', 'Temporarily Renamed');
    await page.click('[data-action="save-chore"]');
    await expect(page.locator('.chore-edit-sheet')).toHaveCount(0, { timeout: 5000 });
    await expect(page.locator('.chore-list')).toContainText('Temporarily Renamed');

    // Now restore to default
    const renamedRow = page.locator('.chore-row', { hasText: 'Temporarily Renamed' });
    await renamedRow.locator('[data-action="chore-edit"]').click();
    await expect(page.locator('.chore-edit-sheet')).toBeVisible({ timeout: 3000 });

    page.once('dialog', d => d.accept());
    const restoreDone = page.waitForResponse(
      r => r.url().includes('/restore-default') && r.request().method() === 'POST'
    );
    await page.click('[data-action="restore-chore-default"]');
    await restoreDone;
    await expect(page.locator('.chore-edit-sheet')).toHaveCount(0, { timeout: 5000 });

    // Original name should be back
    await expect(page.locator('.chore-list')).toContainText(originalName);
    await expect(page.locator('.chore-list')).not.toContainText('Temporarily Renamed');
  });
});
