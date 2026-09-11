import {test,expect} from '@playwright/test';
import {fixture,headers,deferred,observeModuleCalls} from './review-fixtures.js';

test.use({serviceWorkers:'block'});

test('Stats keeps the latest period after reversed responses and recovers an initial overview error',async({page})=>{
  const joinLoads=await observeModuleCalls(page,'stats-data','loadStatsResource');
  await fixture(page,{metricType:'none'});
  await page.route('**/api/stats/overview',route=>route.fulfill({status:500,body:'{}'}));
  await page.locator('[data-nav="stats"]').click();
  await expect(page.getByRole('button',{name:'Retry charts'})).toBeVisible();
  await page.unroute('**/api/stats/overview');
  await page.getByRole('button',{name:'Retry charts'}).click();
  await expect(page.getByRole('button',{name:'Retry charts'})).toHaveCount(0);
  const entered=deferred(),release=deferred(),finished=deferred();
  await page.route('**/api/stats/breakdown?period=month',async route=>{
    entered.resolve();await release.promise;
    await route.fulfill({json:{breakdown:[{category:'Obsolete category',count:7}]}});finished.resolve();
  });
  await page.route('**/api/stats/breakdown?period=day',route=>route.fulfill({json:{breakdown:[{category:'Current category',count:3}]}}));
  try {
    await page.locator('[data-action="stats-period"][data-section="categories"][data-period="month"]').click();await entered.promise;
    await page.locator('[data-action="stats-period"][data-section="categories"][data-period="day"]').click();
    await expect(page.locator('.stat-bar-label')).toContainText(['Current category']);
    release.resolve();await finished.promise;
    await joinLoads();
    await expect(page.locator('.stats-page')).not.toContainText('Obsolete category');
    await expect(page.locator('.stats-page')).not.toContainText('Loading charts');
    await expect(page.locator('[data-action="stats-period"][data-section="categories"][data-period="day"]')).toHaveClass(/period-toggle--active/);
  } finally {release.resolve();}
});

test('Home and hidden Stats sections meet their request budgets; refresh sends one overview',async({page})=>{
  const {chore}=await fixture(page);
  const hidden=['overview','last-done','baby','activity','busy-hours','leaderboard','top-chores','categories','chores','recap',`chore:${chore.id}`];
  const response=await page.request.patch('/api/preferences',{headers:await headers(page),data:{statsSectionHidden:hidden}});
  expect(response.ok()).toBeTruthy();
  const paths=[];
  page.on('request',request=>{if(request.method()==='GET'&&new URL(request.url()).pathname.startsWith('/api/')) paths.push(new URL(request.url()).pathname);});
  await page.reload();await expect(page.locator('.home-grid')).toBeVisible();
  expect(paths.filter(path=>path.startsWith('/api/stats/'))).toHaveLength(0);
  expect(paths.length).toBeLessThanOrEqual(10);
  paths.length=0;
  await page.locator('[data-nav="stats"]').click();
  await expect(page.locator('.stats-page')).toBeVisible();
  await expect(page.locator('.stats-page')).not.toContainText('Loading charts');
  expect(paths.filter(path=>path.startsWith('/api/stats/'))).toEqual(['/api/stats/overview']);
  paths.length=0;
  // Exercise the actual refresh handler via its error/retry affordance.
  await page.evaluate(()=>{const button=document.createElement('button');button.dataset.action='retry-stats';document.querySelector('#app').appendChild(button);button.click();});
  await expect(page.locator('.stats-page')).not.toContainText('Loading charts');
  expect(paths.filter(path=>path==='/api/stats/overview')).toHaveLength(1);
});

test('feeding ranges use Tokyo calendar dates and keep empty controls visible',async({browser})=>{
  const context=await browser.newContext({timezoneId:'Asia/Tokyo'});
  try {
    const page=await context.newPage();
    await fixture(page,{name:'Feed Baby',indicatorLabels:['Formula']});
    await page.clock.install({time:new Date('2026-09-10T00:10:00+09:00')});
    const ranges=[];
    page.on('request',request=>{const url=new URL(request.url());if(url.pathname==='/api/stats/feeding-gaps') ranges.push(Object.fromEntries(url.searchParams));});
    await page.locator('[data-nav="stats"]').click();
    const day=page.locator('[data-action="stats-feeding-gaps-quick"][data-days="1"]');
    await expect(day).toBeVisible();await day.click();
    await expect.poll(()=>ranges.at(-1)).toMatchObject({start:'2026-09-10',end:'2026-09-11'});
    await expect(page.getByLabel('Start date',{exact:true})).toHaveValue('2026-09-10');
    await expect(page.getByLabel('End date',{exact:true})).toHaveValue('2026-09-10');
    await expect(day).toBeVisible();
  } finally {await context.close();}
});

test('all 20 saved widgets load real values with one shared summary request',async({page})=>{
  const {chore}=await fixture(page,{metricType:'none'});
  const posted=await page.request.post('/api/logs',{headers:await headers(page),data:{choreId:chore.id,completedAt:new Date().toISOString(),hour:12}});
  expect(posted.ok()).toBeTruthy();
  const widgets=Array.from({length:20},(_,index)=>({id:`w${index}`,type:'total',choreIds:[chore.id],metric:'count',agg:'sum',period:'week',grain:'',title:`Count ${index+1}`}));
  const saved=await page.request.patch('/api/preferences',{headers:await headers(page),data:{statsWidgets:widgets}});
  expect(saved.ok()).toBeTruthy();
  expect((await saved.json()).preferences.statsWidgets).toHaveLength(20);
  await page.reload();await expect(page.locator('.home-grid')).toBeVisible();
  let summaries=0;
  page.on('request',request=>{if(new URL(request.url()).pathname===`/api/stats/chores/${chore.id}/summary`) summaries++;});
  await page.locator('[data-nav="stats"]').click();
  await expect(page.locator('.widget-card')).toHaveCount(20);
  await expect(page.locator('.widget-big-number')).toHaveText(Array(20).fill('1'));
  expect(summaries).toBe(1);
});

test('household switching rejects a held Stats result and keeps its timer scoped',async({page})=>{
  const joinLoads=await observeModuleCalls(page,'stats-data','loadStatsResource');
  const {chore,household}=await fixture(page,{metricType:'duration',name:'A private timer'});
  const h=await headers(page);
  const created=await page.request.post('/api/household',{headers:h,data:{name:'Stats second household',initials:'SB'}});
  expect(created.ok()).toBeTruthy();const b=(await created.json()).household;
  expect((await page.request.post('/api/chores',{headers:h,data:{name:'B task',icon:'📝',color:'#6080AA'}})).ok()).toBeTruthy();
  expect((await page.request.post(`/api/households/${household.id}/activate`,{headers:h})).ok()).toBeTruthy();
  await page.reload();
  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
  await page.locator('[data-action="start-timer"]').click();
  await expect(page.locator('#timer-chip')).toBeVisible();
  const entered=deferred(),release=deferred(),finished=deferred();
  let held=false;
  await page.route('**/api/stats/breakdown?period=*',async route=>{
    if(held) {await route.continue();return;}
    held=true;const response=await route.fetch();entered.resolve();await release.promise;
    await route.fulfill({response,json:{breakdown:[{category:'A stats canary',count:88}]}});finished.resolve();
  });
  try {
    await page.locator('[data-nav="stats"]').click();await entered.promise;
    await page.locator('#hh-indicator').click();
    await page.locator(`[data-action="activate-household"][data-household-id="${b.id}"]`).click();
    await expect(page.locator('#hh-indicator')).toHaveText('SB');
    await expect(page.locator('#timer-chip')).toHaveCount(0);
    release.resolve();await finished.promise;
    await joinLoads();
    await expect(page.locator('body')).not.toContainText('A stats canary');
    await expect(page.locator('.stats-page')).not.toContainText('Loading charts');
    await page.locator('#hh-indicator').click();
    await page.locator(`[data-action="activate-household"][data-household-id="${household.id}"]`).click();
    await expect(page.locator('#timer-chip')).toBeVisible();
  } finally {release.resolve();}
});
