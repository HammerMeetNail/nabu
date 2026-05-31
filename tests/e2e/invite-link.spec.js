// tests/e2e/invite-link.spec.js
// Verifies that clicking "New tracked link" in settings shows the full invite
// URL in the UI and that the "Copy" button on the permanent invite link is present.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-invite-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithHousehold(page) {
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
    data: { name: `Invite Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  // Navigate to settings to trigger household load
  await page.reload();
  await page.waitForSelector('[data-nav="settings"]', { timeout: 10000 });
  await page.click('[data-nav="settings"]');
  await page.waitForSelector('.invite-link-url', { timeout: 10000 });

  return { email, csrf };
}

test.describe('Invite link UI', () => {
  test('permanent invite link shows full URL in settings', async ({ page }) => {
    await setupWithHousehold(page);

    const inviteUrl = await page.locator('.invite-link-url').innerText();
    expect(inviteUrl).toMatch(/^https?:\/\/.+\/join\?code=[A-Z0-9]{6}$/);
  });

  test('"New tracked link" button creates invite and shows full URL in invite list', async ({ page }) => {
    await setupWithHousehold(page);

    // Grant clipboard permissions so we can inspect what was written
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);

    await page.click('button[data-action="create-invite"]');

    // Wait specifically for an invite list item (has delete-invite action, unlike member items)
    await page.waitForSelector('[data-action="delete-invite"]', { timeout: 5000 });

    // The invite list item should show the full URL (not just a bare code)
    const inviteItem = page.locator('li:has([data-action="delete-invite"])').first();
    const listItemText = await inviteItem.innerText();
    expect(listItemText).toMatch(/https?:\/\/.+\/join\?code=[A-Z0-9]{6}/);
  });

  test('"New tracked link" copies full URL to clipboard', async ({ page }) => {
    await setupWithHousehold(page);
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);

    await page.click('button[data-action="create-invite"]');
    await page.waitForTimeout(500);

    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toMatch(/^https?:\/\/.+\/join\?code=[A-Z0-9]{6}$/);
  });
});
