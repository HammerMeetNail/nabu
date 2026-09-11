import {test,expect} from '@playwright/test';
import {readFile} from 'node:fs/promises';
import {fixture,postLog,deferred,signOut} from './review-fixtures.js';

const logButton = page => page.locator('[data-action="export-csv"][data-kind="logs"]');
async function settings(page) {
  await page.locator('[data-nav="settings"]').click();
  await expect(logButton(page)).toBeVisible();
}

test('date filters survive an export error and retry downloads the complete selected range',async({page})=>{
  const f=await fixture(page);
  await postLog(page,f.chore,{date:'2026-09-01',completedAt:'2026-09-01T12:00:00Z',note:'selected-export-record'});
  await postLog(page,f.chore,{date:'2026-08-01',completedAt:'2026-08-01T12:00:00Z',note:'outside-export-record'});
  await settings(page);
  await page.locator('#export-start').fill('2026-09-01');
  await page.locator('#export-end').fill('2026-09-01');
  await page.route('**/api/logs/export?**',route=>route.fulfill({status:500,contentType:'application/json',body:JSON.stringify({error:'Export could not be prepared'})}),{times:1});
  await logButton(page).click();
  await expect(page.getByTestId('export-error')).toHaveText('Export could not be prepared');
  await expect(page.locator('#export-start')).toHaveValue('2026-09-01');
  const downloaded=page.waitForEvent('download');
  await logButton(page).click();
  const download=await downloaded;
  expect(await download.failure()).toBeNull();
  const csv=await readFile(await download.path(),'utf8');
  expect(csv).toContain('selected-export-record');
  expect(csv).not.toContain('outside-export-record');
  await expect(page.getByText('Export ready.',{exact:true})).toBeVisible();
});

test('HTTP and network failures never become downloaded files',async({page})=>{
  await fixture(page);await settings(page);
  const downloads=[];page.on('download',download=>downloads.push(download));
  for (const status of [400,403,413,429,500]) {
    const message=`Synthetic export error ${status}`;
    await page.route('**/api/logs/export?**',route=>route.fulfill({status,contentType:'application/json',body:JSON.stringify({error:message})}),{times:1});
    await logButton(page).click();
    await expect(page.getByTestId('export-error')).toHaveText(message);
    await expect(logButton(page)).toBeEnabled();
  }
  await page.route('**/api/logs/export?**',route=>route.abort('failed'),{times:1});
  await logButton(page).click();
  await expect(page.getByTestId('export-error')).toBeVisible();
  await expect(logButton(page)).toBeEnabled();
  expect(downloads).toHaveLength(0);
});

test('cancel releases the export UI while a delayed server response is still pending',async({page})=>{
  await fixture(page);await settings(page);
  // Keep routes registered until both callbacks finish. Removing one-shot
  // handlers mid-flight can disable interception for the other paused request.
  const entered=deferred(),release=deferred(),finished=deferred();
  await page.route('**/api/logs/export?**',async route=>{
    entered.resolve();await release.promise;
    try {await route.fulfill({status:200,contentType:'text/csv',body:'date,note\n2026-09-10,old\n'});}catch{/* aborted request */}
    finally {finished.resolve();}
  });
  await logButton(page).click();await entered.promise;
  await expect(logButton(page)).toBeDisabled();
  await page.getByRole('button',{name:'Cancel export',exact:true}).click();
  await expect(logButton(page)).toBeEnabled();
  await expect(page.getByText('Export canceled.',{exact:true})).toBeVisible();
  const newerEntered=deferred(),newerRelease=deferred(),newerFinished=deferred();
  const downloads=[];page.on('download',value=>downloads.push(value));
  await page.route('**/api/logs/export?**',async route=>{
    newerEntered.resolve();await newerRelease.promise;
    try {await route.fulfill({status:200,contentType:'text/csv',body:'date,note\n2026-09-10,new export\n'});}
    finally {newerFinished.resolve();}
  });
  await logButton(page).click();await newerEntered.promise;
  release.resolve();await finished.promise;
  // The canceled completion arrives while the newer operation still owns UI.
  await expect(logButton(page)).toBeDisabled();
  await expect(page.getByRole('button',{name:'Cancel export',exact:true})).toBeVisible();
  expect(downloads).toHaveLength(0);
  const downloaded=page.waitForEvent('download');newerRelease.resolve();
  const file=await downloaded;await newerFinished.promise;
  expect(await file.failure()).toBeNull();
  expect(await readFile(await file.path(),'utf8')).toContain('new export');
  await expect(page.getByText('Export ready.',{exact:true})).toBeVisible();
  expect(downloads).toHaveLength(1);
});

test('a CSV body completed after logout cannot create a download',async({page})=>{
  await fixture(page);
  await page.evaluate(async()=>{
    const suffix=new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
    const {downloadCSV}=await import(`/static/js/exports.js${suffix}`);
    const {contextSnapshot}=await import(`/static/js/browser-context.js${suffix}`);
    const blob=Response.prototype.blob;
    let resume;const gate=new Promise(resolve=>{resume=resolve;});
    window.__resumeExportBody=resume;
    Response.prototype.blob=async function(){const result=await blob.call(this);window.__exportBodyReady=true;await gate;return result;};
    const create=URL.createObjectURL;window.__exportObjectURLs=0;
    URL.createObjectURL=function(value){window.__exportObjectURLs++;return create.call(this,value);};
    window.__exportResult=downloadCSV('logs',{},contextSnapshot(),new AbortController().signal).then(()=>({ok:true}),err=>({error:err.name}));
  });
  await page.waitForFunction(()=>window.__exportBodyReady);
  await signOut(page);
  const result=await page.evaluate(async()=>{window.__resumeExportBody();return await window.__exportResult;});
  expect(result.ok).not.toBe(true);
  expect(await page.evaluate(()=>window.__exportObjectURLs)).toBe(0);
});
