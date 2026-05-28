// tests/e2e/three-fixes.spec.js
// Regression tests for three fixes:
//   1. Dedup bug: logging "Change Baby" (or any chore with indicators) twice in
//      the same day silently returned the existing log — both logs must now be
//      created and appear in /api/logs/today.
//   2. Layout: .app-shell (scroll container) ends where #bottom-tabs begins;
//      no content is hidden under an overlapping tab bar.
//   3. Nav tabs: #bottom-tabs is a static flex item at the page bottom —
//      no position:sticky/fixed so there is no iOS PWA cold-open gap.
//      The body height is driven by --app-h (set from window.innerHeight in
//      an inline <head> script) rather than 100dvh, so the body always covers
//      the full visual viewport including the iOS safe-area-inset-bottom.

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
  await page.locator('[data-action="save-log"]').click();
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

    // Both must appear in today's logs.
    // Pass the local date so the server matches logs by log_date
    // rather than the server's UTC clock.
    const today = new Date();
    const pad = n => String(n).padStart(2, '0');
    const localDate = `${today.getFullYear()}-${pad(today.getMonth()+1)}-${pad(today.getDate())}`;
    const todayResp = await page.request.get(`/api/logs/today?date=${localDate}`);
    expect(todayResp.ok()).toBe(true);
    const { logs } = await todayResp.json();
    const babyLogs = (logs || []).filter(l => l.choreId === baby.id);
    expect(babyLogs.length).toBeGreaterThanOrEqual(2);
  });

  test('second log has its own indicators, not the first log\'s indicators', async ({ page }) => {
    const { chores } = await setupWithChores(page);
    const baby = chores.find(c => c.name === 'Change Baby');

    // First log: select first chip only
    await tapChangeBaby(page);
    await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });
    await page.locator('.log-chip').first().click();       // first chip on
    await page.locator('[data-action="save-log"]').click();
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
    await page.locator('[data-action="save-log"]').click();
    await expect(page.locator('.bottom-sheet')).toHaveCount(0, { timeout: 5000 });
    await page.waitForTimeout(500);

    const today = new Date();
    const pad = n => String(n).padStart(2, '0');
    const localDate = `${today.getFullYear()}-${pad(today.getMonth()+1)}-${pad(today.getDate())}`;
    const todayResp = await page.request.get(`/api/logs/today?date=${localDate}`);
    const { logs } = await todayResp.json();
    const babyLogs = (logs || []).filter(l => l.choreId === baby.id);
    expect(babyLogs.length).toBeGreaterThanOrEqual(2);

    // The two logs should not be identical (different indicators)
    if (babyLogs.length >= 2) {
      const ind0 = JSON.stringify((babyLogs[0].indicators || []).sort());
      const ind1 = JSON.stringify((babyLogs[1].indicators || []).sort());
      expect(ind0).not.toBe(ind1);
    }
  });
});

// ─── Fix 2: Content area does not overlap with bottom tabs ───────────────────

test.describe('Fix 2: content area does not overlap with bottom tabs', () => {
  test('.app-shell ends where #bottom-tabs begins (no overlap)', async ({ page }) => {
    await setupWithChores(page);

    const result = await page.evaluate(() => {
      const shell = document.querySelector('.app-shell');
      const tabs  = document.querySelector('#bottom-tabs');
      if (!shell || !tabs) return null;
      const shellRect = shell.getBoundingClientRect();
      const tabsRect  = tabs.getBoundingClientRect();
      return {
        shellBottom: Math.round(shellRect.bottom),
        tabsTop:     Math.round(tabsRect.top),
        overlap:     shellRect.bottom - tabsRect.top,
      };
    });

    expect(result).not.toBeNull();
    expect(result.overlap).toBeLessThanOrEqual(1);
  });
});

// ─── Fix 3: Nav tabs at bottom ───────────────────────────────────────────────

test.describe('Fix 3: nav tabs are a static flex item at the page bottom', () => {
  test('#bottom-tabs is NOT a descendant of #top-bar', async ({ page }) => {
    await setupWithChores(page);

    const isInsideHeader = await page.evaluate(() => {
      const nav = document.querySelector('#bottom-tabs');
      const header = document.querySelector('#top-bar');
      return header ? header.contains(nav) : false;
    });

    expect(isInsideHeader).toBe(false);
  });

  test('body height equals window.innerHeight (--app-h JS fix)', async ({ page }) => {
    await setupWithChores(page);

    const result = await page.evaluate(() => {
      const bodyH   = Math.round(document.body.getBoundingClientRect().height);
      const innerH  = window.innerHeight;
      const appHPx  = getComputedStyle(document.documentElement).getPropertyValue('--app-h').trim();
      return { bodyH, innerH, appHPx };
    });

    expect(result.appHPx).toBe(result.innerH + 'px');
    expect(result.bodyH).toBe(result.innerH);
  });

  test('#bottom-tabs is NOT positioned (static in normal flow)', async ({ page }) => {
    await setupWithChores(page);

    const position = await page.evaluate(() => {
      return window.getComputedStyle(document.querySelector('#bottom-tabs')).position;
    });

    expect(position).toBe('static');
  });

  test('tab text labels are visible', async ({ page }) => {
    await setupWithChores(page);

    // All tab spans should be rendered (not display:none)
    const allVisible = await page.evaluate(() => {
      const spans = [...document.querySelectorAll('#bottom-tabs .tab-item span')];
      return spans.length === 4 && spans.every(s => window.getComputedStyle(s).display !== 'none');
    });

    expect(allVisible).toBe(true);
  });

  test('tab order is activity, schedule, home, settings', async ({ page }) => {
    await setupWithChores(page);

    const order = await page.evaluate(() => {
      return [...document.querySelectorAll('#bottom-tabs .tab-item')].map(el => el.dataset.nav);
    });

    expect(order).toEqual(['activity', 'schedule', 'today', 'settings']);
  });

  test('all four nav icon buttons are visible after login', async ({ page }) => {
    await setupWithChores(page);

    const navItems = ['today', 'activity', 'schedule', 'settings'];
    for (const nav of navItems) {
      await expect(page.locator(`.tab-item[data-nav="${nav}"]`)).toBeVisible();
    }
  });

  test('active tab highlights the current route', async ({ page }) => {
    await setupWithChores(page);

    // Home route: "today" tab should be active
    await expect(page.locator('.tab-item[data-nav="today"]')).toHaveClass(/active/);

    // Navigate to calendar
    await page.click('[data-nav="activity"]');
    await page.click('[data-action="switch-view"][data-view="day"]');
    await page.waitForSelector('.cal-date', { timeout: 15000 });
    await expect(page.locator('.tab-item[data-nav="activity"]')).toHaveClass(/active/);
    await expect(page.locator('.tab-item[data-nav="today"]')).not.toHaveClass(/active/);
  });
});
