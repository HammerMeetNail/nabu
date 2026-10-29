import {test,expect} from '@playwright/test';
import {fixture,headers,postLog,deferred,observeModuleCalls} from './review-fixtures.js';

test.use({serviceWorkers:'block'});

// The page's section reads travel in one /api/stats/dashboard request; this
// household's one chart read goes straight to its endpoint. Holding either
// must not hold the other.
for (const resource of ['sections','time-series']) {
  test(`Stats renders ready charts while ${resource} is pending and stays usable`,async({page})=>{
    const joinLoads=await observeModuleCalls(page,'app','loadAllStatsData',{exported:false});
    const {chore}=await fixture(page);
    // A tab switch must retain this document, even while chart reads finish.
    const originalDocument=await page.evaluateHandle(()=>document);
    await postLog(page,chore,{volumeML:120});
    const entered=deferred(),release=deferred();
    const path=resource==='sections' ? '**/api/stats/dashboard?*' : `**/api/stats/chores/${chore.id}/time-series?*`;
    await page.route(path,async route=>{
      entered.resolve();
      await release.promise;
      await route.continue();
    });
    try {
      await page.locator('[data-nav="stats"]').click();
      await entered.promise;
      // A held HTTP response is the barrier: the page must not wait for it.
      await expect(page.locator('.stats-page')).toBeVisible();
      await expect(page.locator('.stats-page')).toContainText('Loading charts');
      if (resource==='sections') await expect(page.locator(`[data-stats-section="chore:${chore.id}"] .baby-member-row`)).toHaveCount(1);
      else await expect(page.locator('.chore-stat-card')).toHaveCount(1);
      await page.getByRole('button',{name:'Customize stats'}).click();
      await expect(page.getByRole('button',{name:'Close customize'})).toBeVisible();
      await page.locator('[data-nav="schedule"]').click();
      await expect(page.locator('.stats-page')).toHaveCount(0);
      release.resolve();
      await joinLoads();
      await expect(page.locator('.stats-page')).toHaveCount(0);
      await page.locator('[data-nav="stats"]').click();
      await joinLoads();
      await expect(page.locator('.stats-page')).not.toContainText('Loading charts');
      await expect(page.locator('.chore-stat-card')).toHaveCount(1);
      expect(await originalDocument.evaluate(original=>original===document)).toBe(true);
    } finally {release.resolve();try {await joinLoads();} finally {await originalDocument.dispose();}}
  });
}

test('Stats preserves focused period controls when loading finishes and a recap appears',async({page})=>{
  const joinLoads=await observeModuleCalls(page,'app','loadAllStatsData',{exported:false});
  const {chore}=await fixture(page,{metricType:'none'});
  await postLog(page,chore);
  const saved=await page.request.patch('/api/preferences',{headers:await headers(page),data:{statsSectionOrder:['recap','categories','chores']}});
  expect(saved.ok()).toBeTruthy();
  await page.reload();await expect(page.locator('.home-grid')).toBeVisible();
  const entered=deferred(),release=deferred();
  // Every section read of this page, the overview included, travels in one request.
  await page.route('**/api/stats/dashboard?*',async route=>{
    entered.resolve();await release.promise;await route.continue();
  });
  try {
    await page.locator('[data-nav="stats"]').click();await entered.promise;
    const selector='[data-action="stats-period"][data-section="categories"][data-period="month"]';
    await expect(page.locator(selector)).toBeVisible();
    await page.locator(selector).evaluate(button=>{window.statsPeriodButton=button;button.focus();});
    release.resolve();await joinLoads();
    await expect(page.locator('.stats-page')).not.toContainText('Loading charts');
    await expect(page.getByRole('heading',{name:'Weekly Recap'})).toBeVisible();
    await expect(page.locator('.chore-stat-card')).toHaveCount(1);
    expect(await page.locator(selector).evaluate(button=>button===window.statsPeriodButton&&document.activeElement===button)).toBe(true);
    await page.locator(selector).click();
    await expect(page.locator(selector)).toHaveClass(/period-toggle--active/);
  } finally {release.resolve();await joinLoads();}
});

// Plan §8's request budget: the Stats page's reads leave as at most three
// requests (stats-batch.js sends them as two dashboard requests, sections and
// charts), here for a household with the default activities and a log.
test('opening Stats sends at most three Stats requests',async({page})=>{
  const {chore,headers}=await fixture(page);
  expect((await page.request.post('/api/chores/seed-defaults',{headers})).ok()).toBe(true);
  await postLog(page,chore,{volumeML:120});
  await page.reload();
  await expect(page.locator('.home-grid')).toBeVisible();
  const reads=[];
  page.on('request',request=>{
    const url=new URL(request.url());
    if(url.pathname.startsWith('/api/stats')) reads.push(url.pathname+url.search);
  });
  await page.locator('[data-nav="stats"]').click();
  await expect(page.locator('.stats-page')).toBeVisible();
  await expect(page.locator('.stats-page')).not.toContainText('Loading charts');
  await expect(page.locator('.chore-stat-card').first()).toBeVisible();
  await page.waitForLoadState('networkidle');
  expect(reads.length,reads.join('\n')).toBeLessThanOrEqual(3);
});
