// tests/e2e/log-sheet-manage-icon.spec.js
// Regression: the "manage chore" button in the upper right of the home-tab
// log sheet must use a gear (cog) icon, not a sun/asterisk shape, and must
// open the chore edit sheet when tapped.
//
// The original implementation (990ea13) shipped a Feather "sun" SVG (a circle
// with 8 rays) even though the commit message called it a gear. This test
// pins the gear path so a sun-shaped icon can never silently return.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-lsmi-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `LSManageIcon Test ${Date.now()}` },
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

test.describe('Home tab: log sheet manage (gear) icon', () => {
  test('manage button uses a gear icon and opens the chore edit sheet', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const chore = chores[0];
    expect(chore).toBeDefined();

    // Open the log sheet from the home grid.
    await page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`).click();
    const manageBtn = page.locator(`.sheet-manage-btn[data-chore-id="${chore.id}"]`);
    await expect(manageBtn).toBeVisible({ timeout: 5000 });

    // The icon must be the gear (settings) path, not the sun ray path.
    const svg = manageBtn.locator('svg');
    await expect(svg).toHaveAttribute('aria-hidden', 'true');
    const svgHTML = await svg.evaluate(el => el.innerHTML);
    expect(svgHTML).toContain('M19.4 15a1.65'); // gear body
    expect(svgHTML).not.toContain('M12 1v2M12 21v2'); // sun rays (the old icon)

    // Tapping it navigates to the chore edit sheet.
    await manageBtn.click();
    const editSheet = page.locator('.chore-edit-sheet');
    await expect(editSheet).toBeVisible({ timeout: 5000 });
    await expect(editSheet.locator('.sheet-title', { hasText: 'Edit Chore' })).toBeVisible();

    // The edit sheet carries the chore's name pre-filled.
    await expect(editSheet.locator('#chore-edit-name')).toHaveValue(chore.name);
  });
});
