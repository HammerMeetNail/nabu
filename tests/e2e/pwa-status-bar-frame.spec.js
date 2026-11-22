// tests/e2e/pwa-status-bar-frame.spec.js
// iOS doesn't always open a Home Screen app under the status bar style it was
// added with: the same app opens under a translucent status bar one time and
// below an opaque one the next. Each launch must size the frame (--app-h) for
// the status bar it got. The tab bar fell 62pt below the screen when a launch
// below an opaque bar kept the screen height a translucent launch remembered.
//
// Chromium stands in for an iPhone 17 Pro's Home Screen app: a 874pt screen,
// 812pt of viewport below an opaque status bar, navigator.standalone, and the
// safe-area insets set through the DevTools protocol. As on the phone, the
// insets read 0 and the viewport 812pt at first paint, and arrive just after.

import { test, expect } from '@playwright/test';

test.use({ viewport: { width: 402, height: 812 }, screen: { width: 402, height: 874 } });

function uniqueEmail() {
  return `e2e-statusbar-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', { data: { name: `Status bar ${Date.now()}` }, headers: { 'X-CSRF-Token': csrf } });
  await page.request.post('/api/chores/seed-defaults', { headers: { 'X-CSRF-Token': csrf } });
}

// Every later load runs as a Home Screen app, and records each --app-h it sets.
async function installAsHomeScreenApp(page) {
  await page.addInitScript(() => {
    Object.defineProperty(Navigator.prototype, 'standalone', { configurable: true, get: () => true });
    window.__appH = [];
    // The document has no <html> element yet when this runs.
    new MutationObserver((records) => {
      if (!records.some(record => record.target === document.documentElement)) return;
      const value = document.documentElement.style.getPropertyValue('--app-h');
      if (window.__appH[window.__appH.length - 1] !== value) window.__appH.push(value);
    }).observe(document, { subtree: true, attributes: true, attributeFilter: ['style'] });
  });
  return page.context().newCDPSession(page);
}

// The viewport's height and the safe-area insets; the screen stays 874pt
// (page.setViewportSize would resize the screen with the viewport).
async function geometry(cdp, height, top, bottom) {
  await cdp.send('Emulation.setDeviceMetricsOverride', {
    width: 402, height, deviceScaleFactor: 1, mobile: false, screenWidth: 402, screenHeight: 874,
  });
  await cdp.send('Emulation.setSafeAreaInsetsOverride', { insets: { top, bottom } });
}

// Opens the app as iOS does, under `statusBar`: first paint sees no insets and
// an 812pt viewport, then the insets arrive, and under a translucent bar the
// viewport grows to the screen.
async function launch(page, cdp, statusBar) {
  await geometry(cdp, 812, 0, 0);
  await page.reload({ waitUntil: 'domcontentloaded' });
  if (statusBar === 'translucent') await geometry(cdp, 874, 62, 34);
  else await geometry(cdp, 812, 0, 34);
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  await expect(page.locator('#bottom-tabs')).toBeVisible();
}

function frame(page) {
  return page.evaluate(() => ({
    appH: document.documentElement.style.getPropertyValue('--app-h'),
    tabsBottom: Math.round(document.querySelector('#bottom-tabs').getBoundingClientRect().bottom),
    viewport: window.innerHeight,
    remembered: localStorage.getItem('nabu-translucent-status-bar'),
  }));
}

test('the tab bar stays at the bottom of the screen as iOS switches the status bar between launches', async ({ page }) => {
  await setupWithChores(page);
  const cdp = await installAsHomeScreenApp(page);

  // Under a translucent status bar the app fills the 874pt screen.
  await launch(page, cdp, 'translucent');
  await expect.poll(() => frame(page)).toEqual({ appH: '874px', tabsBottom: 874, viewport: 874, remembered: '1' });

  // The next launch is below an opaque bar. First paint repeats the last
  // launch's screen height; once the insets arrive, the frame is the 812pt
  // viewport and the tab bar ends at the bottom of the screen, not 62pt below.
  await launch(page, cdp, 'opaque');
  await expect.poll(() => frame(page)).toEqual({ appH: '812px', tabsBottom: 812, viewport: 812, remembered: null });
  expect(await page.evaluate(() => window.__appH)).toEqual(['874px', '812px']);

  // And translucent again.
  await launch(page, cdp, 'translucent');
  await expect.poll(() => frame(page)).toEqual({ appH: '874px', tabsBottom: 874, viewport: 874, remembered: '1' });
});

test('an app that is always below an opaque status bar keeps its viewport height and never remembers a translucent one', async ({ page }) => {
  await setupWithChores(page);
  const cdp = await installAsHomeScreenApp(page);

  await launch(page, cdp, 'opaque');
  await expect.poll(() => frame(page)).toEqual({ appH: '812px', tabsBottom: 812, viewport: 812, remembered: null });
  expect(await page.evaluate(() => window.__appH)).toEqual(['812px']);
});
