// tests/e2e/settings-profile.spec.js
// Settings → Account sets the name and color the household sees (PATCH
// /api/me). The members list and profile menu update at once and after a
// reload; a blank name is refused and the saved name stays.

import { test, expect } from '@playwright/test';

async function setup(page) {
  const email = `e2e-profile-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', { data: { name: `Profile ${Date.now()}` }, headers: { 'X-CSRF-Token': csrf } });
  await page.goto('/settings');
  await expect(page.locator('#profile-form')).toBeVisible({ timeout: 15000 });
  const { user } = await (await page.request.get('/api/me')).json();
  return { user };
}

const selfRow = (page, user) => page.locator(`.member-row[data-user-id="${user.id}"]`);

test.describe('Settings profile', () => {
  test('a new name and color show everywhere and survive a reload', async ({ page }) => {
    const { user } = await setup(page);
    await expect(selfRow(page, user).locator('.member-name')).toHaveText(user.displayName);

    await page.fill('#profile-name', '  Dana O\'Connor  ');
    await page.locator('.profile-color', { hasText: 'Purple' }).click();
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await expect(page.locator('#toast-container .toast')).toContainText('Profile saved');

    const row = selfRow(page, user);
    await expect(row.locator('.member-name')).toHaveText("Dana O'Connor");
    await expect(row.locator('.avatar-circle-sm')).toHaveText('D');
    await expect(row.locator('.avatar-circle-sm')).toHaveCSS('background-color', 'rgb(123, 94, 167)');

    await page.reload();
    await expect(page.locator('#profile-form')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('#profile-name')).toHaveValue("Dana O'Connor");
    await expect(page.locator('.profile-color input[value="#7B5EA7"]')).toBeChecked();
    await expect(selfRow(page, user).locator('.member-name')).toHaveText("Dana O'Connor");
    const { user: saved } = await (await page.request.get('/api/me')).json();
    expect(saved).toMatchObject({ displayName: "Dana O'Connor", avatarColor: '#7B5EA7', email: user.email });

    // The profile menu leads with the name, in the chosen color.
    await page.click('#hh-indicator');
    await expect(page.locator('#profile-panel .profile-name')).toHaveText("Dana O'Connor");
    await expect(page.locator('#profile-panel .profile-avatar-lg')).toHaveCSS('background-color', 'rgb(123, 94, 167)');
  });

  test('a blank name is refused and the saved name stays', async ({ page }) => {
    const { user } = await setup(page);
    await page.fill('#profile-name', '   ');
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await expect(page.locator('#profile-error')).toHaveText('Enter your name');
    await expect(selfRow(page, user).locator('.member-name')).toHaveText(user.displayName);
    const { user: saved } = await (await page.request.get('/api/me')).json();
    expect(saved.displayName).toBe(user.displayName);

    // The server refuses names the form can't stop, such as control characters.
    const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
    const response = await page.request.patch('/api/me', { headers: { 'X-CSRF-Token': csrf }, data: { displayName: 'Dana\u0007' } });
    expect(response.status()).toBe(400);
  });
});
