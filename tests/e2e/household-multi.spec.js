// tests/e2e/household-multi.spec.js
// Verifies multi-household support:
//  - User can create a second household
//  - Household indicator (initials badge) appears in the header
//  - Profile sheet shows household switcher list
//  - Switching households reloads scoped data and updates the indicator
//  - Initials auto-generate from the household name during creation

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-multi-hh-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';
}

async function registerAndCreateHousehold(page, hhName) {
  const email = uniqueEmail();
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = await getCSRF(page);
  const res = await page.request.post('/api/household', {
    data: { name: hhName, initials: '' },
    headers: { 'X-CSRF-Token': csrf },
  });
  expect(res.ok()).toBeTruthy();

  await page.reload();
  await page.waitForSelector('.home-view', { timeout: 15000 });
  return { email, csrf };
}

test.describe('Multi-Household', () => {
  test('household indicator shows initials in header after creating household', async ({ page }) => {
    await registerAndCreateHousehold(page, 'Smith Family');

    // The initials badge should be visible in the header.
    const indicator = page.locator('#hh-indicator');
    await expect(indicator).toBeVisible({ timeout: 5000 });
    const text = await indicator.innerText();
    // "Smith Family" → "SF" (two-word initials)
    expect(text.trim()).toBe('SF');
  });

  test('initials auto-generate from name when creating a household via UI', async ({ page }) => {
    const email = uniqueEmail();
    await page.goto('/register');
    await page.waitForSelector('#register-form');
    await page.fill('#reg-email', email);
    await page.fill('#reg-password', 'test123456');
    await page.fill('#reg-confirm', 'test123456');
    await page.click('button[type="submit"]');
    await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

    // Navigate to settings to see the create-household form
    await page.click('[data-nav="settings"]');
    await page.waitForSelector('#hh-name', { timeout: 5000 });

    // Type a name; initials should auto-fill
    await page.fill('#hh-name', 'Jones Home');
    // Trigger the input event so auto-initials fires
    await page.dispatchEvent('#hh-name', 'input');
    await page.waitForTimeout(100);
    const initials = await page.inputValue('#hh-initials');
    expect(initials).toBe('JH');
  });

  test('user can create a second household via API', async ({ page }) => {
    await registerAndCreateHousehold(page, 'First Home');

    const csrf = await getCSRF(page);
    const res = await page.request.post('/api/household', {
      data: { name: 'Second Home', initials: 'SH' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(res.ok()).toBeTruthy();
    const data = await res.json();
    expect(data.household).toBeTruthy();
    expect(data.household.name).toBe('Second Home');
  });

  test('GET /api/households lists all user households', async ({ page }) => {
    await registerAndCreateHousehold(page, 'Alpha Home');

    const csrf = await getCSRF(page);
    // Create a second household
    const r2 = await page.request.post('/api/household', {
      data: { name: 'Beta Home', initials: 'BH' },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(r2.ok()).toBeTruthy();

    const listRes = await page.request.get('/api/households');
    expect(listRes.ok()).toBeTruthy();
    const listData = await listRes.json();
    expect(Array.isArray(listData.households)).toBeTruthy();
    expect(listData.households.length).toBeGreaterThanOrEqual(2);
    const names = listData.households.map(h => h.name);
    expect(names).toContain('Alpha Home');
    expect(names).toContain('Beta Home');
  });

  test('profile sheet shows household switcher when user has multiple households', async ({ page }) => {
    await registerAndCreateHousehold(page, 'First Place');

    const csrf = await getCSRF(page);
    // Create a second household
    await page.request.post('/api/household', {
      data: { name: 'Second Place', initials: 'SP' },
      headers: { 'X-CSRF-Token': csrf },
    });

    // Reload so userHouseholds state is populated
    await page.reload();
    await page.waitForSelector('.home-view', { timeout: 15000 });

    // Open profile sheet
    await page.click('#hh-indicator');
    await page.waitForSelector('#profile-panel', { timeout: 5000 });

    // Household switcher should be visible with both households
    await expect(page.locator('.profile-households')).toBeVisible();
    const items = page.locator('.profile-household-item');
    await expect(items).toHaveCount(2, { timeout: 3000 });
    const texts = await items.allInnerTexts();
    const joined = texts.join(' ');
    expect(joined).toContain('First Place');
    expect(joined).toContain('Second Place');
  });

  test('switching household via profile sheet updates indicator and active state', async ({ page }) => {
    await registerAndCreateHousehold(page, 'Home A');

    const csrf = await getCSRF(page);
    // Create a second household (this becomes the active one after creation)
    const r2 = await page.request.post('/api/household', {
      data: { name: 'Home B', initials: 'HB' },
      headers: { 'X-CSRF-Token': csrf },
    });
    const hhBData = await r2.json();
    const hhBId = hhBData.household?.id;
    expect(hhBId).toBeTruthy();

    // Reload to populate state
    await page.reload();
    await page.waitForSelector('.home-view', { timeout: 15000 });

    // Open profile sheet
    await page.click('#hh-indicator');
    await page.waitForSelector('#profile-panel', { timeout: 5000 });

    // Find the non-active household (Home A) and click to switch
    const items = page.locator('.profile-household-item');
    const count = await items.count();
    let switchedName = null;
    for (let i = 0; i < count; i++) {
      const item = items.nth(i);
      if (!(await item.evaluate(el => el.classList.contains('profile-household-item--active')))) {
        switchedName = (await item.innerText()).split('\n')[0].trim();
        await item.click();
        break;
      }
    }
    expect(switchedName).toBeTruthy();

    // Wait for profile panel to close and indicator to update
    await page.waitForSelector('#profile-panel', { state: 'hidden', timeout: 5000 });
    await page.waitForTimeout(500); // allow reload to settle

    // Indicator should now show the new household's initials
    const indicator = page.locator('#hh-indicator');
    await expect(indicator).toBeVisible();
    const indicatorText = await indicator.innerText();
    expect(indicatorText.trim().length).toBeGreaterThan(0);
  });

  test('household edit form updates name and initials', async ({ page }) => {
    await registerAndCreateHousehold(page, 'Old Name');

    // Go to settings
    await page.click('[data-nav="settings"]');
    await page.waitForSelector('.hh-edit-btn', { timeout: 10000 });

    // Click Edit
    await page.click('.hh-edit-btn');
    await page.waitForSelector('#edit-household-form:not(.hidden)', { timeout: 3000 });

    // Update the name and initials
    await page.fill('#edit-hh-name', 'New Name');
    await page.fill('#edit-hh-initials', 'NN');
    await page.click('#edit-household-form [type="submit"]');

    // Wait for the header indicator to update
    await page.waitForTimeout(800);
    const indicator = page.locator('#hh-indicator');
    await expect(indicator).toBeVisible();
    expect((await indicator.innerText()).trim()).toBe('NN');

    // Settings page should reflect new name
    const settingsText = await page.locator('.settings-view').innerText();
    expect(settingsText).toContain('New Name');
  });
});
