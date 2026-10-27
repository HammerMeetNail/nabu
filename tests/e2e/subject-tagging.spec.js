// tests/e2e/subject-tagging.spec.js
// Phase 5.5 — subject tagging: chore-level subjects, the log-sheet single-select
// picker, and the tag shown in history.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-subject-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithSubjectChore(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', {
    data: { name: `Subjects ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  const res = await page.request.post('/api/chores', {
    data: { name: 'Feed', icon: '🍼', color: '#EC4899', subjects: ['Alice', 'Bob'] },
    headers: { 'X-CSRF-Token': csrf },
  });
  const chore = (await res.json()).chore;
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { csrf, chore };
}

test.describe('Subject tagging (Phase 5.5)', () => {
  test('single-select subject picker, persisted and shown in history', async ({ page }) => {
    const { chore } = await setupWithSubjectChore(page);

    await page.click(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    await page.waitForSelector('.subject-chip');

    // Single-select: picking Alice turns it on; picking Bob deselects Alice.
    await page.click('.subject-chip:has-text("Alice")');
    await expect(page.locator('.subject-chip:has-text("Alice")')).toHaveClass(/subject-chip--on/);
    await page.click('.subject-chip:has-text("Bob")');
    await expect(page.locator('.subject-chip:has-text("Alice")')).not.toHaveClass(/subject-chip--on/);
    await expect(page.locator('.subject-chip:has-text("Bob")')).toHaveClass(/subject-chip--on/);

    await page.click('[data-action="save-log"]');
    await page.waitForSelector('#toast-container .toast');

    await page.click('a[data-nav="activity"]');
    await page.waitForSelector('.history-view');
    await expect(page.locator('.hist-subject')).toContainText('Bob');
  });

  test('a subject with a quote cannot inject script (attribute XSS is inert)', async ({ page }) => {
    const { csrf } = await setupWithSubjectChore(page);
    // A chore whose subject tries to break out of the chip's data-subject attr.
    const res = await page.request.post('/api/chores', {
      data: { name: 'Twins', icon: '👶', color: '#8B5CF6', subjects: [`x" onmouseover="window.__xss=1`] },
      headers: { 'X-CSRF-Token': csrf },
    });
    const evilChore = (await res.json()).chore;
    await page.reload();
    await page.click(`.home-chore-card[data-home-chore-id="${evilChore.id}"]`);
    await page.waitForSelector('.subject-chip');
    // Hover + click the chip to trigger any injected handler.
    await page.locator('.subject-chip').first().hover();
    await page.locator('.subject-chip').first().click();
    await page.waitForTimeout(200);
    const xss = await page.evaluate(() => window.__xss);
    expect(xss).toBeUndefined();
    // The subject round-trips correctly as data (browser-decoded), proving the
    // value is stored/escaped rather than parsed as markup.
    const subj = await page.locator('.subject-chip').first().getAttribute('data-subject');
    expect(subj).toBe(`x" onmouseover="window.__xss=1`);
  });
});
