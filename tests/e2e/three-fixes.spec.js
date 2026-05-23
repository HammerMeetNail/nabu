// tests/e2e/three-fixes.spec.js
// Regression tests for three fixes:
//   1. Dedup bug: logging "Change Baby" (or any chore with indicators) twice in
//      the same day silently returned the existing log — both logs must now be
//      created and appear in /api/logs/today.
//   2. Scroll fix: #app no longer reserves 80px padding-bottom for the old
//      fixed bottom nav, so the home grid never forces a spurious scroll.
//   3. Nav-in-header: the nav tabs (#bottom-tabs) live inside the top-bar
//      header; text labels are hidden; icon buttons are visible.

import { test, expect } from '@playwright/test';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function uniqueEmail() {
  return `e2e-3fix-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
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
    data: { name: `Three-Fixes Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  const chores = (await (await page.request.get('/api/chores')).json()).chores || [];
  return { email, csrf, chores };
}

async function tapChangeBaby(page) {
  // "Change Baby" has indicators so tapping opens the log sheet.
  const cards = page.locator('.home-chore-card');
  const count = await cards.count();
  for (let i = 0; i < count; i++) {
    const name = await cards.nth(i).locator('.home-card-name').innerText();
    if (name.trim() === 'Change Baby') {
      await cards.nth(i).click();
      return;
    }
  }
  throw new Error('Change Baby card not found');
}

async function saveLogSheet(page) {
  await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
  // Toggle the first chip
  const chip = page.locator('.log-chip').first();
  await chip.click();
  await page.locator('[data-action="save-home-log"]').click();
  await expect(page.locator('.bottom-sheet')).toHaveCount(0, { timeout: 5000 });
  await page.waitForTimeout(500);
}

// ─── Fix 1: Dedup bug — allow multiple logs per chore per day ─────────────────

test.describe('Fix 1: multiple logs per chore per day', () => {
  test('logging Change Baby twice creates two separate log entries', async ({ page }) => {
    const { chores } = await setupWithChores(page);

    const baby = chores.find(c => c.name === 'Change Baby');
    expect(baby).toBeDefined();

    // First log
    await tapChangeBaby(page);
    await saveLogSheet(page);

    // Second log (same chore, same day)
    await tapChangeBaby(page);
    await saveLogSheet(page);

    // Both must appear in today's logs
    const todayResp = await page.request.get('/api/logs/today');
    expect(todayResp.ok()).toBe(true);
    const { logs } = await todayResp.json();
    const babyLogs = logs.filter(l => l.choreId === baby.id);
    expect(babyLogs.length).toBeGreaterThanOrEqual(2);
  });

  test('second log has its own indicators, not the first log\'s indicators', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const baby = chores.find(c => c.name === 'Change Baby');

    // First log: select first chip only
    await tapChangeBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await page.locator('.log-chip').first().click();       // first chip on
    await page.locator('[data-action="save-home-log"]').click();
    await expect(page.locator('.bottom-sheet')).toHaveCount(0, { timeout: 5000 });
    await page.waitForTimeout(500);

    // Second log: select second chip only
    await tapChangeBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    const chips = page.locator('.log-chip');
    const chipCount = await chips.count();
    if (chipCount > 1) {
      await chips.nth(1).click(); // second chip on
    }
    await page.locator('[data-action="save-home-log"]').click();
    await expect(page.locator('.bottom-sheet')).toHaveCount(0, { timeout: 5000 });
    await page.waitForTimeout(500);

    const todayResp = await page.request.get('/api/logs/today');
    const { logs } = await todayResp.json();
    const babyLogs = logs.filter(l => l.choreId === baby.id);
    expect(babyLogs.length).toBeGreaterThanOrEqual(2);

    // The two logs should not be identical (different indicators)
    if (babyLogs.length >= 2) {
      const ind0 = JSON.stringify((babyLogs[0].indicators || []).sort());
      const ind1 = JSON.stringify((babyLogs[1].indicators || []).sort());
      expect(ind0).not.toBe(ind1);
    }
  });
});

// ─── Fix 2: Scroll fix — #app has no large bottom padding ────────────────────

test.describe('Fix 2: no spurious bottom padding on #app', () => {
  test('#app padding-bottom is small (not reserving space for old fixed nav)', async ({ page }) => {
    await setupWithChores(page);

    const paddingBottom = await page.evaluate(() => {
      const app = document.querySelector('#app');
      return parseInt(window.getComputedStyle(app).paddingBottom, 10);
    });

    // Old value was 80+ px; after fix it should be a normal value (≤ 20px).
    expect(paddingBottom).toBeLessThanOrEqual(20);
  });
});

// ─── Fix 3: Nav-in-header ─────────────────────────────────────────────────────

test.describe('Fix 3: nav tabs live in the header', () => {
  test('#bottom-tabs is a descendant of #top-bar', async ({ page }) => {
    await setupWithChores(page);

    const isInsideHeader = await page.evaluate(() => {
      const nav = document.querySelector('#bottom-tabs');
      const header = document.querySelector('#top-bar');
      return header ? header.contains(nav) : false;
    });

    expect(isInsideHeader).toBe(true);
  });

  test('tab text labels are not visible (hidden via CSS)', async ({ page }) => {
    await setupWithChores(page);

    // All five tab spans should have display:none
    const allHidden = await page.evaluate(() => {
      const spans = [...document.querySelectorAll('#bottom-tabs .tab-item span')];
      return spans.every(s => window.getComputedStyle(s).display === 'none');
    });

    expect(allHidden).toBe(true);
  });

  test('all five nav icon buttons are visible after login', async ({ page }) => {
    await setupWithChores(page);

    const navItems = ['today', 'chores', 'calendar', 'history', 'settings'];
    for (const nav of navItems) {
      await expect(page.locator(`.tab-item[data-nav="${nav}"]`)).toBeVisible();
    }
  });

  test('active tab highlights the current route', async ({ page }) => {
    await setupWithChores(page);

    // Home route: "today" tab should be active
    await expect(page.locator('.tab-item[data-nav="today"]')).toHaveClass(/active/);

    // Navigate to calendar
    await page.click('[data-nav="calendar"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });
    await expect(page.locator('.tab-item[data-nav="calendar"]')).toHaveClass(/active/);
    await expect(page.locator('.tab-item[data-nav="today"]')).not.toHaveClass(/active/);
  });
});
