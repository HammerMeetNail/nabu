import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-calhist-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Calendar History Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.click('[data-nav="calendar"]');
  await page.waitForSelector('.cal-date', { timeout: 15000 });
}

test.describe('Calendar add writes history', () => {
  test('adding a chore from a calendar hour slot creates a done log and appears in history', async ({ page }) => {
    await setupWithChores(page);

    await page.locator('[data-drop-hour="14"]').click();
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

    const firstItem = page.locator('.sheet-chore-item').first();
    const choreName = (await firstItem.locator('.chore-name').innerText()).trim();
    await firstItem.click();

    await expect(page.locator('.bottom-sheet')).toHaveCount(0, { timeout: 5000 });

    const doneCards = page.locator('[data-drop-hour="14"] .chore-card--done');
    await expect(doneCards).toHaveCount(1);
    await expect(doneCards.first().locator('.chore-name')).toContainText(choreName);

    await page.click('[data-nav="history"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });
    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(page.locator('.hist-row .hist-name').first()).toContainText(choreName);
  });
});
