// tests/e2e/settings-member-row.spec.js
// A member row opened in Settings stays open when Settings re-renders, as it
// does whenever data arrives. It used to snap shut, hiding the row's controls
// (household-lifecycle.spec.js failed now and then on exactly this).

import { test, expect } from '@playwright/test';

test('an opened member row survives a re-render', async ({ page }) => {
  const email = `e2e-member-row-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
  await page.goto('/register');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await expect(page.getByRole('heading', { name: 'Welcome to Nabu' })).toBeVisible({ timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  expect((await page.request.post('/api/household', { data: { name: 'Home' }, headers: { 'X-CSRF-Token': csrf } })).ok()).toBeTruthy();

  await page.goto('/settings');
  const row = page.locator('.member-row').first();
  await row.locator('summary').click();
  await expect(row).toHaveAttribute('open', '');

  // Re-render Settings, and know that it happened: morph removes an
  // attribute the markup doesn't have.
  const settings = page.locator('.settings-view');
  await settings.evaluate(el => el.setAttribute('data-e2e-before-render', ''));
  await page.evaluate(() => window.dispatchEvent(new Event('nabu-journal-change')));
  await expect(settings).not.toHaveAttribute('data-e2e-before-render', '');
  await expect(row).toHaveAttribute('open', '');
});

// Registers through the API in this page's browser context.
async function register(page) {
  await page.goto('/register');
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf').value;
  const headers = { 'X-CSRF-Token': csrf };
  const email = `e2e-member-row-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
  const response = await page.request.post('/api/auth/register', { data: { email, password: 'test123456' }, headers });
  expect(response.status()).toBe(201);
  return { user: (await response.json()).user, headers };
}

// Rows are matched by their key. When the member above an opened row
// leaves, the opened row used to take the leaver's closed row's place.
test('an opened member row stays open when a member above it leaves', async ({ page, browser }) => {
  const owner = await register(page);
  const created = await page.request.post('/api/household', { data: { name: 'Home' }, headers: owner.headers });
  expect(created.status()).toBe(201);
  const { inviteCode } = (await created.json()).household;
  const contexts = [await browser.newContext(), await browser.newContext()];
  try {
    const members = [];
    for (const context of contexts) {
      const memberPage = await context.newPage();
      const member = await register(memberPage);
      expect((await memberPage.request.post('/api/household/join', { data: { inviteCode }, headers: member.headers })).ok()).toBeTruthy();
      members.push({ page: memberPage, ...member });
    }
    await page.goto('/settings');
    const rows = page.locator('.member-row');
    await expect(rows).toHaveCount(3);
    const order = await rows.evaluateAll(els => els.map(el => Number(el.dataset.userId)));
    const [leaving, staying] = members.sort((a, b) => order.indexOf(a.user.id) - order.indexOf(b.user.id));
    const row = page.locator(`.member-row[data-user-id="${staying.user.id}"]`);
    await row.locator('summary').click();
    await expect(row).toHaveAttribute('open', '');

    expect((await leaving.page.request.post('/api/household/leave', { headers: leaving.headers })).ok()).toBeTruthy();
    // Coming back to the page reloads the household.
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
    await expect(rows).toHaveCount(2);
    await expect(row).toHaveAttribute('open', '');
  } finally {
    for (const context of contexts) await context.close();
  }
});
