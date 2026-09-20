import {test, expect} from '@playwright/test';
import {deferred, observeModuleCalls} from './review-fixtures.js';

test.use({serviceWorkers: 'block'});

async function observeReads(page) {
  const join = await observeModuleCalls(page, 'stats', 'loadChoreTimeSeries');
  await page.goto('/login');
  await expect(page.locator('#login-form')).toBeVisible();
  await page.evaluate(async () => {
    const suffix = new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
    const {loadChoreTimeSeries} = await import(`/static/js/stats.js${suffix}`);
    const fixture = window.observerFixture = {joined: {}, failures: []};
    window.beginObservedRead = id => {
      const pending = loadChoreTimeSeries(id, 'day');
      pending.catch(error => fixture.failures.push(error.message));
      const then = pending.then.bind(pending);
      // Promise.all uses then: this barrier proves the join actually reached
      // each held request before we collect garbage or release its response.
      pending.then = (...args) => {
        fixture.joined[id] = true;
        return then(...args);
      };
    };
  });
  return join;
}

test('observed loader joins survive garbage collection and include calls added while waiting', async ({page, browserName}) => {
  test.skip(browserName !== 'chromium', 'This regression uses Chromium CDP garbage collection.');
  const join = await observeReads(page);
  const firstEntered = deferred(), secondEntered = deferred();
  const releaseFirst = deferred(), releaseSecond = deferred();
  await page.route('**/api/stats/chores/*/time-series?*', async route => {
    const first = new URL(route.request().url()).pathname.includes('/chores/1/');
    (first ? firstEntered : secondEntered).resolve();
    await (first ? releaseFirst : releaseSecond).promise;
    await route.fulfill({json: {timeSeries: []}});
  });
  const client = await page.context().newCDPSession(page);
  let joined, finished = false;
  try {
    await page.evaluate(() => window.beginObservedRead(1));
    await firstEntered.promise;
    joined = join().then(() => ({ok: true}), error => ({ok: false, error: error.message}));
    joined.then(() => {finished = true;});
    await page.waitForFunction(() => window.observerFixture.joined[1]);
    await client.send('HeapProfiler.collectGarbage');
    expect(finished).toBe(false);

    await page.evaluate(() => window.beginObservedRead(2));
    await secondEntered.promise;
    releaseFirst.resolve();
    await page.waitForFunction(() => window.observerFixture.joined[2]);
    await client.send('HeapProfiler.collectGarbage');
    expect(finished).toBe(false);
    releaseSecond.resolve();
    expect(await joined).toEqual({ok: true});
  } finally {
    releaseFirst.resolve();
    releaseSecond.resolve();
    if (joined) await joined;
    await client.detach();
  }
});

test('observed loader joins report missing calls and already rejected reads', async ({page}) => {
  const join = await observeReads(page);
  await expect(join()).rejects.toThrow('No observed calls to loadChoreTimeSeries');
  await page.route('**/api/stats/chores/1/time-series?*', route =>
    route.fulfill({status: 500, json: {error: 'Observed read failed'}}));
  await page.evaluate(() => window.beginObservedRead(1));
  await page.waitForFunction(() => window.observerFixture.failures.length === 1);
  await expect(join()).rejects.toThrow('Observed read failed');
});
