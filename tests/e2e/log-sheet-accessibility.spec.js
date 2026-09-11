import {test,expect} from '@playwright/test';
import {fixture,postLog,rerender,deferred} from './review-fixtures.js';

test('a log sheet contains keyboard focus, preserves edits and returns focus on Escape and Back',async({page})=>{
  const {chore}=await fixture(page,{metricUnit:'g'});
  const card=page.locator(`[data-home-chore-id="${chore.id}"]`);
  await card.focus(); await page.keyboard.press('Enter');
  const sheet=page.getByRole('dialog');
  await expect(sheet).toBeVisible();
  await expect.poll(()=>page.evaluate(()=>!!document.activeElement.closest('.bottom-sheet'))).toBe(true);
  await expect(page.getByLabel('When',{exact:true})).toBeVisible();
  await page.locator('#log-note').fill('Do not replace this draft');
  await page.locator('#log-volume').fill('73');
  await page.locator('#log-when').fill('2026-09-08T18:23');
  await page.getByRole('button',{name:'Cancel',exact:true}).focus();
  await page.keyboard.press('Tab');
  await expect.poll(()=>page.evaluate(()=>!!document.activeElement.closest('.bottom-sheet'))).toBe(true);
  await expect(page.locator('#bottom-tabs')).toHaveJSProperty('inert',true);
  await rerender(page);
  await expect(page.locator('#log-note')).toHaveValue('Do not replace this draft');
  await expect(page.locator('#log-volume')).toHaveValue('73');
  await expect(page.locator('#log-when')).toHaveValue('2026-09-08T18:23');
  await page.keyboard.press('Escape');
  await expect(sheet).toHaveCount(0);
  await expect(card).toBeFocused();
  await expect(page.locator('#bottom-tabs')).toHaveJSProperty('inert',false);
  await card.click(); await expect(sheet).toBeVisible();
  await page.goBack(); await expect(sheet).toHaveCount(0);
  await expect(card).toBeFocused();
});

test('an active timer cannot trap the background after a sheet rerenders and closes',async({page})=>{
  const {chore}=await fixture(page,{metricType:'duration'});
  const card=page.locator(`[data-home-chore-id="${chore.id}"]`);
  await card.click();await page.locator('[data-action="start-timer"]').click();
  await expect(page.locator('#timer-chip')).toBeVisible();
  await card.click();await expect(page.getByRole('dialog')).toBeVisible();
  await rerender(page);await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(card).toBeFocused();
  await expect(page.locator('#bottom-tabs')).toHaveJSProperty('inert',false);
  await expect(page.locator('#timer-chip')).toBeVisible();
  await page.locator('[data-nav="activity"]').click();
  await expect(page.getByLabel('Search activity')).toBeVisible();
});

for(const unit of ['g','count']) {
  test(`${unit} amounts use the configured unit and persist without volume conversion`,async({page})=>{
    const {chore}=await fixture(page,{metricUnit:unit});
    await page.setViewportSize({width:320,height:740});
    await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
    await expect(page.getByLabel('Amount',{exact:true})).toBeVisible();
    await expect(page.locator('#log-metric-unit')).toHaveValue(unit);
    await page.locator('#log-volume').fill('37');
    const dims=await page.locator('#log-when').evaluate(el=>({left:el.getBoundingClientRect().left,right:el.getBoundingClientRect().right,width:el.getBoundingClientRect().width}));
    expect(dims.left).toBeGreaterThanOrEqual(0);expect(dims.right).toBeLessThanOrEqual(320);expect(dims.width).toBeGreaterThan(240);
    await page.locator('[data-action="save-log"]').click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await page.reload();
    await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
    await expect(page.locator('.volume-recent-chip')).toHaveText(`37 ${unit}`);
    await page.locator('.volume-recent-chip').click();
    await expect(page.locator('#log-volume')).toHaveValue('37');
    const logs=await (await page.request.get('/api/logs/history')).json();
    expect(logs.logs.find(log=>log.choreId===chore.id).volumeML).toBe(37);
  });
}

test('recent amounts survive restart, absent latest amount, searches, failed reads and delayed responses',async({page})=>{
  const {chore}=await fixture(page);
  for(const [i,amount] of [60,90,120,0].entries()) await postLog(page,chore,{volumeML:amount,completedAt:`2026-09-09T1${i}:00:00Z`,date:'2026-09-09'});
  const open=async()=>{await page.locator(`[data-home-chore-id="${chore.id}"]`).click();await expect(page.locator('#log-note')).toBeVisible();};
  const chips=()=>page.locator('.volume-recent-chip');
  await page.reload();await open();
  await expect(chips()).toHaveText(['120 mL','90 mL','60 mL']);
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await page.locator('[data-nav="activity"]').click();
  await page.getByLabel('Search activity').fill('no-matching-note');
  await expect(page.locator('.hist-entry')).toHaveCount(0);
  await page.locator('[data-nav="today"]').click();await open();
  await expect(chips()).toHaveText(['120 mL','90 mL','60 mL']);
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await page.route('**/api/logs/latest-per-chore',route=>route.fulfill({status:500,body:'{}'}));
  await page.reload();
  const gate=deferred(),entered=deferred();
  await page.route('**/api/logs/recent-amounts?*',async route=>{const response=await route.fetch();entered.resolve();await gate.promise;await route.fulfill({response});});
  try {
    await open();await entered.promise;
    await page.locator('#log-note').fill('Retain while recents load');
    await page.locator('#log-when').fill('2026-09-08T10:23');
    gate.resolve();
    await expect(chips()).toHaveText(['120 mL','90 mL','60 mL']);
    await expect(page.locator('#log-note')).toHaveValue('Retain while recents load');
    await expect(page.locator('#log-when')).toHaveValue('2026-09-08T10:23');
    await page.getByRole('button',{name:'Cancel',exact:true}).click();
    await page.unroute('**/api/logs/recent-amounts?*');
    await page.route('**/api/logs/recent-amounts?*',route=>route.fulfill({status:500,body:'{}'}));
    await open();await expect(chips()).toHaveText(['120 mL','90 mL','60 mL']);
  } finally {gate.resolve();}
});

test('log controls remain usable at 200 percent page zoom',async({page})=>{
  const {chore}=await fixture(page,{metricUnit:'g'});
  await page.setViewportSize({width:640,height:1000});
  await page.addStyleTag({content:'html { zoom: 2; }'});
  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
  await page.getByLabel('When',{exact:true}).fill('2026-09-08T18:23');
  await page.getByLabel('Amount',{exact:true}).fill('37');
  const bounds=await page.getByLabel('When',{exact:true}).evaluate(el=>({left:el.getBoundingClientRect().left,right:el.getBoundingClientRect().right}));
  expect(bounds.left).toBeGreaterThanOrEqual(0);
  expect(bounds.right).toBeLessThanOrEqual(640);
  await expect(page.getByLabel('When',{exact:true})).toHaveValue('2026-09-08T18:23');
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
});

test('recent amounts remain available after local midnight and a fresh page load',async({page})=>{
  const {chore}=await fixture(page);
  await page.clock.install({time:new Date('2026-09-09T23:58:00')});
  for(const [index,amount] of [60,90,120,0].entries()) {
    const at=new Date(2026,8,9,20,index);
    await postLog(page,chore,{volumeML:amount,completedAt:at.toISOString(),date:'2026-09-09'});
  }
  const open=async()=>{
    await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
    await expect(page.locator('.volume-recent-chip')).toHaveText(['120 mL','90 mL','60 mL']);
  };
  await page.reload();await open();
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await page.clock.setSystemTime(new Date('2026-09-10T00:02:00'));
  await page.reload();await open();
  await expect(page.locator('#log-when')).toHaveValue('2026-09-10T00:02');
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
});
