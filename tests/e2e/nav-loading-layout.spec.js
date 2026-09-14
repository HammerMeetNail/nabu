import { test, expect } from '@playwright/test';
import { fixture, postLog, deferred, observeModuleCalls } from './review-fixtures.js';

test.use({ serviceWorkers: 'block' });

for (const tab of [
  { name: 'Activity', nav: 'activity', loader: 'loadActivityPage', path: '**/api/logs/history', content: '.hist-row', status: 'Loading activity' },
  { name: 'Stats', nav: 'stats', loader: 'loadAllStatsData', path: '**/api/stats/**', content: '.stats-sections', status: 'Loading charts' },
]) {
  test(`${tab.name} content stays in place while tab navigation refreshes`, async ({ page }, testInfo) => {
    const joinLoads = await observeModuleCalls(page, 'app', tab.loader, { exported: false });
    const { chore } = await fixture(page);
    await postLog(page, chore, { volumeML: 120, note: 'Navigation layout regression' });
    await page.locator(`[data-nav="${tab.nav}"]`).click();
    await joinLoads();
    const content = page.locator(tab.content).first();
    await expect(content).toBeVisible();
    await page.locator('[data-nav="today"]').click();
    await expect(page.locator('.home-grid')).toBeVisible();

    const entered = deferred(), release = deferred();
    await page.route(tab.path, async route => {
      entered.resolve();
      await release.promise;
      await route.continue();
    });
    try {
      await page.locator(`[data-nav="${tab.nav}"]`).click();
      await entered.promise;
      // Hold real requests open so even a fast server exercises the loading render.
      const status = page.getByRole('status').filter({ hasText: tab.status });
      await expect(status).toHaveCount(1);
      await expect(content).toBeVisible();
      const pendingBounds = await content.boundingBox();
      await page.screenshot({ path: testInfo.outputPath('loading.png') });

      release.resolve();
      await joinLoads();
      await expect(status).toHaveCount(0);
      await expect(content).toBeVisible();
      const readyBounds = await content.boundingBox();
      await page.screenshot({ path: testInfo.outputPath('ready.png') });
      expect(Math.abs(pendingBounds.y - readyBounds.y)).toBeLessThan(1);
      expect(Math.abs(pendingBounds.x - readyBounds.x)).toBeLessThan(1);
    } finally {
      release.resolve();
      await joinLoads();
    }
  });
}
