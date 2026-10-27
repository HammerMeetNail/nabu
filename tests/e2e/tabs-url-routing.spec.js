// tests/e2e/tabs-url-routing.spec.js
// Tabs put their route in the address bar, so Back and Forward move between
// tabs, reload keeps the tab, and links open a tab directly. A Back press
// that only closes a sheet never changes the tab, and a Stats link can point
// at one activity's section.

import { test, expect } from '@playwright/test';

const PASSWORD = 'test123456';

function uniqueEmail() {
  return `e2e-tabs-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  const email = uniqueEmail();
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', PASSWORD);
  await page.fill('#reg-confirm', PASSWORD);
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', { data: { name: `Tabs ${Date.now()}` }, headers: { 'X-CSRF-Token': csrf } });
  await page.request.post('/api/chores/seed-defaults', { headers: { 'X-CSRF-Token': csrf } });
  await page.goto('/today');
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { email };
}

// The last seeded activity with its own Stats section, near the page bottom.
async function analyticsChore(page) {
  const res = await page.request.get('/api/chores');
  const body = await res.json();
  const chores = (body.chores || body).filter(c => !['Feed Baby', 'Change Baby'].includes(c.name)
    && ((c.metricType && c.metricType !== 'none') || (c.indicatorLabels || []).length > 0));
  expect(chores.length).toBeGreaterThan(0);
  return chores[chores.length - 1];
}

const tab = (page, nav) => page.locator(`#bottom-tabs .tab-item[data-nav="${nav}"]`);

async function expectTab(page, nav, view) {
  await expect(page.locator(view)).toBeVisible({ timeout: 15000 });
  await expect(tab(page, nav)).toHaveClass(/active/);
  await expect(tab(page, nav)).toHaveAttribute('aria-current', 'page');
  await expect(page.locator('#bottom-tabs [aria-current="page"]')).toHaveCount(1);
}

test.describe('Tab URLs', () => {
  test('tabs update the URL; Back and Forward move between tabs', async ({ page }) => {
    await setupWithChores(page);
    await expectTab(page, 'today', '.home-grid');

    await tab(page, 'stats').click();
    await expect(page).toHaveURL(/\/stats$/);
    await expectTab(page, 'stats', '.stats-page');

    await tab(page, 'activity').click();
    await expect(page).toHaveURL(/\/activity$/);
    await expectTab(page, 'activity', '.history-view');

    // Tapping the tab that is already open adds no entry.
    await tab(page, 'activity').click();

    await page.goBack();
    await expect(page).toHaveURL(/\/stats$/);
    await expectTab(page, 'stats', '.stats-page');

    await page.goBack();
    await expect(page).toHaveURL(/\/today$/);
    await expectTab(page, 'today', '.home-grid');

    await page.goForward();
    await expect(page).toHaveURL(/\/stats$/);
    await expectTab(page, 'stats', '.stats-page');
  });

  test('reload and a direct link keep the tab', async ({ page }) => {
    await setupWithChores(page);
    await tab(page, 'schedule').click();
    await expect(page).toHaveURL(/\/schedule$/);
    await page.reload();
    await expectTab(page, 'schedule', '.schedule-view');

    await page.goto('/activity');
    await expectTab(page, 'activity', '.history-view');

    // The profile menu's Settings link is a navigation too.
    await page.goto('/today');
    await expectTab(page, 'today', '.home-grid');
    await page.click('#hh-indicator');
    await page.click('[data-action="profile-nav-settings"]');
    await expect(page).toHaveURL(/\/settings$/);
    await expectTab(page, 'settings', '.settings-view');
    const notifications = page.locator('.settings-view h3', { hasText: 'Notifications' });
    await expect(notifications).toBeVisible({ timeout: 10000 });

    // A reload on Settings loads what opening the tab loads.
    await page.reload();
    await expectTab(page, 'settings', '.settings-view');
    await expect(notifications).toBeVisible({ timeout: 10000 });

    await page.goBack();
    await expectTab(page, 'today', '.home-grid');
  });

  test('Back closes an open sheet without leaving the tab', async ({ page }) => {
    await setupWithChores(page);
    await tab(page, 'stats').click();
    await expectTab(page, 'stats', '.stats-page');
    await tab(page, 'today').click();
    await expectTab(page, 'today', '.home-grid');

    const sheet = page.locator('.bottom-sheet');
    await page.locator('.home-chore-card').first().click();
    await expect(sheet).toBeVisible();
    await page.goBack();
    await expect(sheet).toHaveCount(0);
    await expect(page).toHaveURL(/\/today$/);
    await expectTab(page, 'today', '.home-grid');

    // Cancel removes the sheet's own entry; the tab stack is unchanged.
    await page.locator('.home-chore-card').first().click();
    await expect(sheet).toBeVisible();
    await page.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(sheet).toHaveCount(0);
    await expect(page).toHaveURL(/\/today$/);
    await expectTab(page, 'today', '.home-grid');

    await page.goBack();
    await expect(page).toHaveURL(/\/stats$/);
    await expectTab(page, 'stats', '.stats-page');
  });

  test('a tab tapped right after a sheet closes keeps one entry per tab', async ({ page }) => {
    await setupWithChores(page);
    for (let i = 0; i < 3; i++) {
      await page.locator('.home-chore-card').first().click();
      await expect(page.locator('.bottom-sheet')).toBeVisible();
      // No wait: the sheet's own back step is still pending when the tab is tapped.
      await page.getByRole('button', { name: 'Cancel', exact: true }).click();
      await tab(page, 'activity').click();
      await expect(page).toHaveURL(/\/activity$/);
      await expectTab(page, 'activity', '.history-view');
      await page.goBack();
      await expect(page).toHaveURL(/\/today$/);
      await expectTab(page, 'today', '.home-grid');
    }
    await page.goForward();
    await expect(page).toHaveURL(/\/activity$/);
    await expectTab(page, 'activity', '.history-view');
  });

  test('a modified click on a tab is left to the browser', async ({ page }) => {
    await setupWithChores(page);
    // Headless Chromium never finishes loading a background tab, so check the
    // click itself: the app leaves Cmd/Ctrl/Shift-click to the browser (a new
    // tab or window) and handles a plain click in place.
    const clicks = await page.evaluate(() => {
      const link = document.querySelector('#bottom-tabs .tab-item[data-nav="stats"]');
      const click = modifiers => {
        let prevented = null;
        // Runs after the app's document listener; stops any real navigation.
        window.addEventListener('click', e => { prevented = e.defaultPrevented; e.preventDefault(); }, { once: true });
        link.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, ...modifiers }));
        return { prevented, path: location.pathname };
      };
      return [click({ metaKey: true }), click({ ctrlKey: true }), click({ shiftKey: true }), click({})];
    });
    expect(clicks).toEqual([
      { prevented: false, path: '/today' },
      { prevented: false, path: '/today' },
      { prevented: false, path: '/today' },
      { prevented: true, path: '/stats' },
    ]);
    await expectTab(page, 'stats', '.stats-page');
  });
});

test.describe('Stats deep links', () => {
  test('a Stats link opens at that activity, including after signing in', async ({ page, browser, baseURL }) => {
    const { email } = await setupWithChores(page);
    const chore = await analyticsChore(page);
    const section = p => p.locator(`[data-stats-section="chore:${chore.id}"]`);

    await page.goto(`/stats/chore/${chore.id}`);
    await expectTab(page, 'stats', '.stats-page');
    await expect(section(page)).toBeInViewport({ timeout: 15000 });
    await expect(page).toHaveURL(new RegExp(`/stats/chore/${chore.id}$`));

    // Signed out, the same link asks for sign-in, then opens the section.
    const fresh = await browser.newContext({ baseURL });
    const other = await fresh.newPage();
    await other.goto(`/stats/chore/${chore.id}`);
    await other.waitForSelector('#login-form', { timeout: 15000 });
    await other.fill('#login-email', email);
    await other.fill('#login-password', PASSWORD);
    await other.click('#login-form button[type="submit"]');
    await expectTab(other, 'stats', '.stats-page');
    await expect(section(other)).toBeInViewport({ timeout: 15000 });
    await fresh.close();
  });

  test('a link to an unknown activity opens Stats at the top', async ({ page }) => {
    await setupWithChores(page);
    await page.goto('/stats/chore/999999999');
    await expectTab(page, 'stats', '.stats-page');
    await expect(page.locator('.stats-header-row')).toBeInViewport();
  });
});
