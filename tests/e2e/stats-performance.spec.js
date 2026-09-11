import {test,expect} from '@playwright/test';
import {fixture,headers,postLog,deferred,observeModuleCalls} from './review-fixtures.js';

test.use({serviceWorkers:'block'});

for (const resource of ['overview','time-series']) {
  test(`Stats renders ready charts while ${resource} is pending and stays usable`,async({page})=>{
    const joinLoads=await observeModuleCalls(page,'stats-data','loadStatsResource');
    const {chore}=await fixture(page);
    await postLog(page,chore,{volumeML:120});
    const entered=deferred(),release=deferred();
    const path=resource==='overview' ? '**/api/stats/overview' : `**/api/stats/chores/${chore.id}/time-series?*`;
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
      await expect(page.locator('.chore-stat-card')).toHaveCount(1);
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
    } finally {release.resolve();await joinLoads();}
  });
}

test('Stats preserves focused period controls when loading finishes and a recap appears',async({page})=>{
  const joinLoads=await observeModuleCalls(page,'stats-data','loadStatsResource');
  const {chore}=await fixture(page,{metricType:'none'});
  await postLog(page,chore);
  const saved=await page.request.patch('/api/preferences',{headers:await headers(page),data:{statsSectionOrder:['recap','categories','chores']}});
  expect(saved.ok()).toBeTruthy();
  await page.reload();await expect(page.locator('.home-grid')).toBeVisible();
  const entered=deferred(),release=deferred();
  await page.route('**/api/stats/overview',async route=>{
    entered.resolve();await release.promise;await route.continue();
  });
  try {
    await page.locator('[data-nav="stats"]').click();await entered.promise;
    await expect(page.locator('.chore-stat-card')).toHaveCount(1);
    const selector='[data-action="stats-period"][data-section="categories"][data-period="month"]';
    await page.locator(selector).evaluate(button=>{window.statsPeriodButton=button;button.focus();});
    release.resolve();await joinLoads();
    await expect(page.locator('.stats-page')).not.toContainText('Loading charts');
    await expect(page.getByRole('heading',{name:'Weekly Recap'})).toBeVisible();
    expect(await page.locator(selector).evaluate(button=>button===window.statsPeriodButton&&document.activeElement===button)).toBe(true);
    await page.locator(selector).click();
    await expect(page.locator(selector)).toHaveClass(/period-toggle--active/);
  } finally {release.resolve();await joinLoads();}
});
