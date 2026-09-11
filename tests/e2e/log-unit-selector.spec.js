import {test, expect} from '@playwright/test';
import {fixture, postLog, rerender} from './review-fixtures.js';

test('Cat Meds and Baby Meds share amount configuration', async ({page}) => {
  const {headers}=await fixture(page);
  expect((await page.request.post('/api/chores/seed-defaults',{headers})).ok()).toBeTruthy();
  const {chores}=await (await page.request.get('/api/chores')).json();
  for (const name of ['Cat Meds','Baby Meds']) {
    const chore=chores.find(c=>c.name===name);
    expect(chore).toMatchObject({metricType:'amount',metricUnit:'mg',hasVolumeML:true});
    await page.reload();
    await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
    await expect(page.locator('#log-metric-unit')).toHaveValue('mg');
    await page.locator('#log-metric-unit').selectOption('g');
    await page.locator('#log-volume').fill('5');
    await page.locator('[data-action="save-log"]').click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    const {logs}=await (await page.request.get('/api/logs/history')).json();
    expect(logs.find(l=>l.choreId===chore.id)).toMatchObject({volumeML:5,metricUnit:'g'});
  }
});

test('editing a saved unit supports cancel, custom amount and reload without relabeling other entries', async ({page}) => {
  const {chore,headers}=await fixture(page,{metricUnit:'mg'});
  const original=await postLog(page,chore,{volumeML:20});
  const other=await postLog(page,chore,{volumeML:40});
  await page.reload();
  await page.locator('[data-nav="activity"]').click();
  const open=async()=>{await page.locator(`[data-action="view-log"][data-log-id="${original.id}"]`).click();await expect(page.locator('#log-metric-unit')).toHaveValue('mg');};
  await open();
  await page.locator('#log-metric-unit').selectOption('g');
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await open();
  await page.locator('#log-metric-unit').selectOption('g');
  await page.locator('#log-volume').fill('321');
  await rerender(page);
  await expect(page.locator('#log-metric-unit')).toHaveValue('g');
  await expect(page.locator('#log-volume')).toHaveValue('321');
  await page.locator('[data-action="save-log"]').click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  expect((await page.request.patch(`/api/chores/${chore.id}`,{headers,data:{metricType:'amount',metricUnit:'kg'}})).ok()).toBeTruthy();
  await page.reload();
  const {logs}=await (await page.request.get('/api/logs/history')).json();
  expect(logs.find(l=>l.id===original.id)).toMatchObject({volumeML:321,metricUnit:'g'});
  expect(logs.find(l=>l.id===other.id)).toMatchObject({volumeML:40,metricUnit:'mg'});
  await page.locator('[data-nav="activity"]').click();
  await expect(page.locator(`[data-action="view-log"][data-log-id="${original.id}"]`)).toContainText('321 g');
});

test('unit-specific recent amounts and indicator custom values stay attached to the selected unit', async ({page}) => {
  const {chore}=await fixture(page,{metricUnit:'mg',indicatorLabels:['Dose'],indicatorDefaults:['Dose']});
  await postLog(page,chore,{metricUnit:'mg',indicators:['Dose'],indicatorVolumes:{Dose:20}});
  await postLog(page,chore,{metricUnit:'g',indicators:['Dose'],indicatorVolumes:{Dose:321}});
  await page.reload();
  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
  await expect(page.locator('.volume-recent-chip')).toHaveText('20 mg');
  await page.locator('#log-metric-unit').selectOption('g');
  await expect(page.locator('.volume-recent-chip')).toHaveText('321 g');
  await page.locator('.volume-recent-chip').click();
  await expect(page.locator('.indicator-volume-select')).toHaveValue('321');
  await page.locator('.indicator-volume-select').fill('222');
  await page.locator('[data-action="toggle-indicator"]').click();
  await expect(page.locator('.indicator-volume-select')).toBeHidden();
  await page.locator('[data-action="toggle-indicator"]').click();
  await expect(page.locator('.indicator-volume-select')).toBeVisible();
  await expect(page.locator('.indicator-volume-select')).toHaveValue('222');
  await page.locator('[data-action="save-log"]').click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  const {logs}=await (await page.request.get('/api/logs/history')).json();
  expect(logs.find(l=>l.indicatorVolumes?.Dose===222)).toMatchObject({metricUnit:'g'});
});
