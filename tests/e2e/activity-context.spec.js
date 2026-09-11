import {test,expect} from '@playwright/test';
import {fixture,postLog,headers,deferred,observeModuleCalls} from './review-fixtures.js';

// Keep module interception on the page across fixture reloads. Worker cache
// behavior has its own suite; these cases observe the real loader promises.
test.use({serviceWorkers:'block'});

test('direct and shortcut Activity loads retain search across navigation and edits',async({page})=>{
  const {chore}=await fixture(page);
  await postLog(page,chore,{note:'needle original'});await postLog(page,chore,{note:'unrelated row'});
  await page.goto('/activity');
  await expect(page.locator('.hist-row')).toHaveCount(2);
  await page.getByLabel('Search activity').fill('needle');
  await expect(page.locator('.hist-row')).toHaveCount(1);
  await page.locator('[data-nav="today"]').click();await expect(page.locator('.home-grid')).toBeVisible();
  await page.locator('[data-nav="activity"]').click();
  await expect(page.getByLabel('Search activity')).toHaveValue('needle');
  await expect(page.locator('.hist-row')).toHaveCount(1);
  await expect(page.locator('.hist-row')).toContainText('needle original');
  await page.locator('.hist-row').click();await page.locator('#log-note').fill('changed outside search');
  await page.locator('[data-action="save-log"]').click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.locator('.hist-row')).toHaveCount(0);
  await page.goto('/?quicklog=activity');
  await expect(page.getByLabel('Search activity')).toBeVisible();
  await expect(page.locator('.hist-row')).toHaveCount(2);
});

test('a late search cannot overwrite a newer query or its loading state',async({page})=>{
  const joinLoads=await observeModuleCalls(page,'activity-data','loadActivity');
  const {chore}=await fixture(page);
  await postLog(page,chore,{note:'first query result'});await postLog(page,chore,{note:'second query result'});
  await page.goto('/activity');await expect(page.locator('.hist-row')).toHaveCount(2);
  const entered=deferred(),release=deferred(),finished=deferred();
  await page.route('**/api/logs/history?q=first',async route=>{
    const response=await route.fetch();entered.resolve();await release.promise;await route.fulfill({response});finished.resolve();
  });
  try {
    await page.getByLabel('Search activity').fill('first');await entered.promise;
    await page.getByLabel('Search activity').fill('second');
    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(page.locator('.hist-row')).toContainText('second query result');
    release.resolve();await finished.promise;
    await joinLoads();
    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(page.locator('.hist-row')).toContainText('second query result');
    await expect(page.getByLabel('Search activity')).toHaveValue('second');
  } finally {release.resolve();}
});

test('a failed Activity read is recoverable and distinct from empty history',async({page})=>{
  const {chore}=await fixture(page);await postLog(page,chore,{note:'Retained real history'});
  await page.route('**/api/logs/history',route=>route.fulfill({status:500,body:'{}'}));
  await page.goto('/activity');
  await expect(page.getByRole('button',{name:'Retry',exact:true})).toBeVisible();
  await page.unroute('**/api/logs/history');
  await page.getByRole('button',{name:'Retry',exact:true}).click();
  await expect(page.locator('.hist-row')).toContainText('Retained real history');
});

test('a delayed household A response cannot restore its content after switching to B',async({page})=>{
  const joinLoads=await observeModuleCalls(page,'activity-data','loadActivity');
  const {chore,household}=await fixture(page);await postLog(page,chore,{note:'Private A canary'});
  const h=await headers(page);
  const b=await page.request.post('/api/household',{headers:h,data:{name:'Second household',initials:'BB'}});
  expect(b.ok()).toBeTruthy();const householdB=(await b.json()).household;
  expect((await page.request.post('/api/chores',{headers:h,data:{name:'B task',icon:'📝',color:'#6080AA'}})).ok()).toBeTruthy();
  expect((await page.request.post(`/api/households/${household.id}/activate`,{headers:h})).ok()).toBeTruthy();
  await page.goto('/activity');await expect(page.locator('.hist-row')).toContainText('Private A canary');
  const entered=deferred(),release=deferred(),finished=deferred();
  await page.route('**/api/logs/history?q=Private',async route=>{
    const response=await route.fetch();entered.resolve();await release.promise;await route.fulfill({response});finished.resolve();
  });
  try {
    await page.getByLabel('Search activity').fill('Private');await entered.promise;
    await page.locator('#hh-indicator').click();
    await page.locator(`[data-action="activate-household"][data-household-id="${householdB.id}"]`).click();
    await expect(page.locator('#hh-indicator')).toHaveText('BB');
    await expect(page.locator('body')).not.toContainText('Private A canary');
    release.resolve();await finished.promise;
    await joinLoads();
    await expect(page.locator('body')).not.toContainText('Private A canary');
    await expect(page.locator('.hist-row')).toHaveCount(0);
  } finally {release.resolve();}
});

test('an older page finishing after a search cannot append unrelated activity',async({page})=>{
  const joinLoads=await observeModuleCalls(page,'activity-data','loadActivity');
  const {chore}=await fixture(page);
  await postLog(page,chore,{note:'current search result'});
  const oldDate=new Date();oldDate.setDate(oldDate.getDate()-10);oldDate.setHours(12,0,0,0);
  const date=`${oldDate.getFullYear()}-${String(oldDate.getMonth()+1).padStart(2,'0')}-${String(oldDate.getDate()).padStart(2,'0')}`;
  await postLog(page,chore,{note:'old page canary',completedAt:oldDate.toISOString(),date});
  await page.goto('/activity');
  const more=page.getByRole('button',{name:'Load more',exact:true});
  await expect(more).toBeVisible();
  const entered=deferred(),release=deferred(),finished=deferred();
  await page.route('**/api/logs/history?before=*',async route=>{
    const response=await route.fetch();
    expect((await response.json()).logs.some(log=>log.note==='old page canary')).toBeTruthy();
    entered.resolve();await release.promise;
    await route.fulfill({response});finished.resolve();
  });
  try {
    await more.click();await entered.promise;
    await page.getByLabel('Search activity').fill('current search');
    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(page.locator('.hist-row')).toContainText('current search result');
    release.resolve();await finished.promise;
    await joinLoads();
    await expect(page.locator('.hist-row')).toHaveCount(1);
    await expect(page.locator('body')).not.toContainText('old page canary');
  } finally {release.resolve();}
});
