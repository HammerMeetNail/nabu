import { test, expect } from '@playwright/test';
import {signOut,signIn,register as registerOther} from './review-fixtures.js';

const uniqueEmail = () => `e2e-recovery-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
async function setup(page, metricType = 'none') {
  const email = uniqueEmail();
  await page.goto('/register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill('test123456');
  await page.locator('#reg-confirm').fill('test123456');
  await page.locator('#register-form button[type=submit]').click();
  await expect(page.locator('#hh-indicator')).toBeVisible();
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value;
  const headers = {'X-CSRF-Token':csrf};
  const householdResponse = await page.request.post('/api/household',{headers,data:{name:'Recovery A'}});
  expect(householdResponse.ok()).toBeTruthy();
  const household = (await householdResponse.json()).household;
  const response = await page.request.post('/api/chores',{headers,data:{name:'Synthetic task',icon:'📝',color:'#6080AA',metricType}});
  expect(response.ok()).toBeTruthy();
  const chore = (await response.json()).chore;
  await page.reload();
  await expect(page.locator('.home-grid')).toBeVisible();
  return {email,headers,chore,household};
}
async function openLog(page,chore) {
  await page.locator(`.home-chore-card[data-home-chore-id="${chore.id}"]`).click();
  await expect(page.locator('#log-note')).toBeVisible();
}
async function rows(page) {
  return page.evaluate(() => new Promise((resolve,reject) => {
    const open = indexedDB.open('nabu-offline',2);
    open.onerror = () => reject(open.error);
    open.onsuccess = () => {
      const db = open.result, tx = db.transaction('logQueue','readonly');
      const request = tx.objectStore('logQueue').getAll();
      let value;
      request.onsuccess = () => {value=request.result;};
      tx.oncomplete = () => {db.close();resolve(value);};
      tx.onabort = () => {db.close();reject(tx.error);};
    };
  }));
}
async function history(page,chore) {
  const response = await page.request.get('/api/logs/history');
  expect(response.ok()).toBeTruthy();
  return (await response.json()).logs.filter(log => log.choreId === chore.id);
}

for (const status of [400,403,429,500]) {
  test(`HTTP ${status} preserves a log draft and one key through retry`,async ({page}) => {
    const {chore} = await setup(page);
    const posted = [];
    await page.route('**/api/logs',async route => {
      if (route.request().method() !== 'POST') return route.continue();
      posted.push(route.request().postDataJSON());
      await route.fulfill({status,contentType:'application/json',body:JSON.stringify({error:'Synthetic rejection'})});
    });
    await openLog(page,chore);
    await page.locator('#log-note').fill('A note that must survive');
    await page.locator('[data-action="save-log"]').click();
    await expect(page.locator('[data-action="save-log"]')).toHaveText('Retry saved entry');
    await expect(page.locator('#log-note')).toHaveValue('A note that must survive');
    await expect(page.locator('#log-note')).toBeDisabled();
    await expect(page.locator('.saved-draft-error')).toContainText('unchanged');
    expect(await history(page,chore)).toHaveLength(0);
    expect(await rows(page)).toHaveLength(1);
    if (status === 500) {
      await page.reload();
      await expect(page.locator('.pending-work')).toContainText('A note that must survive');
      await page.locator('.pending-work > summary').click();
    }
    await page.unroute('**/api/logs');
    page.on('request',request => { if (new URL(request.url()).pathname === '/api/logs' && request.method() === 'POST') posted.push(request.postDataJSON()); });
    await page.locator(status === 500 ? '[data-action="retry-pending-log"]' : '[data-action="save-log"]').click();
    await expect.poll(async () => (await history(page,chore)).length).toBe(1);
    await expect.poll(async () => (await rows(page)).length).toBe(0);
    expect(posted).toHaveLength(2);
    expect(posted[1]).toEqual(posted[0]);
    expect((await history(page,chore))[0].note).toBe('A note that must survive');
  });
}

test('double Save and a lost successful response keep one logical log',async ({page}) => {
  const {chore} = await setup(page);
  let release;
  const gate = new Promise(resolve => {release=resolve;});
  const posted=[];
  await page.route('**/api/logs',async route => {
    if (route.request().method() !== 'POST') return route.continue();
    posted.push(route.request().postDataJSON());
    await route.fetch(); // committed on the server before losing the response
    await gate;
    await route.abort('connectionreset');
  });
  try {
    await openLog(page,chore);
    await page.locator('#log-note').fill('Exactly once');
    await page.locator('[data-action="save-log"]').evaluate(button => {button.click();button.click();});
    await expect.poll(() => posted.length).toBe(1);
    await expect(page.locator('[data-action="save-log"]')).toBeDisabled();
    await expect.poll(async () => (await history(page,chore)).length).toBe(1);
  } finally {release();}
  await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  await expect(page.locator('.pending-work')).toContainText('Exactly once');
  await page.unroute('**/api/logs');
  await page.locator('.pending-work > summary').click();
  await page.locator('[data-action="retry-pending-log"]').click();
  await expect.poll(async () => (await rows(page)).length).toBe(0);
  expect(await history(page,chore)).toHaveLength(1);
});

test('a journal entry is neither shown nor replayed by another household or account',async({page})=>{
  const {email,chore,household,headers}=await setup(page);
  const posted=[];
  await page.route('**/api/logs',async route=>{
    if(route.request().method()!=='POST') return route.continue();
    posted.push(route.request().postDataJSON());
    await route.fulfill({status:500,body:'{}'});
  });
  await openLog(page,chore);
  await page.locator('#log-note').fill('Only the original household may see this draft');
  await page.locator('[data-action="save-log"]').click();
  await expect(page.locator('[data-action="save-log"]')).toHaveText('Retry saved entry');
  const original=posted[0];posted.length=0;
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
  const second=await page.request.post('/api/household',{headers,data:{name:'Recovery B'}});
  expect(second.ok()).toBeTruthy();
  await page.reload();
  await expect(page.locator('#hh-indicator')).toBeVisible();
  await expect(page.locator('.pending-work')).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText(original.note);
  await signOut(page);
  await registerOther(page);
  await page.reload();
  await expect(page.locator('#hh-indicator')).toBeVisible();
  await expect(page.locator('body')).not.toContainText(original.note);
  expect(posted).toHaveLength(0);
  await signOut(page);await signIn(page,email);
  await expect(page.locator('.pending-work')).toHaveCount(0);
  expect(posted).toHaveLength(0);
  await page.locator('#hh-indicator').click();
  await page.locator(`[data-action="activate-household"][data-household-id="${household.id}"]`).click();
  await expect(page.locator('.pending-work')).toContainText(original.note);
  await page.unroute('**/api/logs');
  page.on('request',request=>{if(new URL(request.url()).pathname==='/api/logs'&&request.method()==='POST') posted.push(request.postDataJSON());});
  await page.locator('.pending-work > summary').click();
  await page.locator('[data-action="retry-pending-log"]').click();
  await expect.poll(async()=>(await rows(page)).length).toBe(0);
  expect(posted).toEqual([original]);
  expect(await history(page,chore)).toHaveLength(1);
});

test('network failure with unavailable journal storage keeps the note until online acceptance',async ({page}) => {
  const {chore} = await setup(page);
  await page.addInitScript(() => {
    const open = IDBFactory.prototype.open;
    IDBFactory.prototype.open = function(name,...args) {
      if (name === 'nabu-offline') throw new DOMException('Synthetic journal failure','InvalidStateError');
      return open.call(this,name,...args);
    };
  });
  await page.reload();
  await openLog(page,chore);
  await page.locator('#log-note').fill('Keep this page open');
  await page.route('**/api/logs',route => route.abort('internetdisconnected'));
  await page.locator('[data-action="save-log"]').click();
  await expect(page.locator('.saved-draft-error')).toContainText('Keep this draft open');
  await expect(page.locator('#log-note')).toHaveValue('Keep this page open');
  await expect(page.locator('.pending-work')).toHaveCount(0);
  expect(await history(page,chore)).toHaveLength(0);
  await page.unroute('**/api/logs');
  await page.locator('[data-action="save-log"]').click();
  await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  expect((await history(page,chore))[0].note).toBe('Keep this page open');
});

test('a failed stopped timer survives reload with frozen duration and retries once',async ({page}) => {
  const {chore} = await setup(page,'duration');
  await openLog(page,chore);
  await page.locator('[data-action="start-timer"]').click();
  await expect(page.locator('#timer-chip')).toBeVisible();
  await page.evaluate(() => {
    const key=Object.keys(localStorage).find(key => key.startsWith('nabu_timer:'));
    const timer=JSON.parse(localStorage.getItem(key));timer.startedAt=Date.now()-90000;localStorage.setItem(key,JSON.stringify(timer));
  });
  let original;
  await page.route('**/api/logs',async route => {
    original=route.request().postDataJSON();
    await route.fulfill({status:500,contentType:'application/json',body:'{"error":"synthetic failure"}'});
  });
  await page.locator('#timer-chip').dispatchEvent('click');
  await expect(page.locator('#timer-chip')).toContainText('Retry log');
  expect(original.durationSeconds).toBeGreaterThanOrEqual(90);
  expect(await history(page,chore)).toHaveLength(0);
  await page.reload();
  await expect(page.locator('#timer-chip')).toContainText('Retry log');
  await page.unroute('**/api/logs');
  let retry;
  page.on('request',request => {if (new URL(request.url()).pathname==='/api/logs' && request.method()==='POST') retry=request.postDataJSON();});
  await page.locator('#timer-chip').dispatchEvent('click');
  await expect(page.locator('#timer-chip')).toHaveCount(0);
  expect(retry).toEqual(original);
  expect(await history(page,chore)).toHaveLength(1);
  expect((await history(page,chore))[0].durationSeconds).toBe(original.durationSeconds);
});

test('two tabs share timer start and stop ownership',async ({page,context}) => {
  const {chore} = await setup(page,'duration');
  const other=await context.newPage();
  await other.goto('/');await expect(other.locator('.home-grid')).toBeVisible();
  await Promise.all([openLog(page,chore),openLog(other,chore)]);
  await Promise.all([page.locator('[data-action="start-timer"]').click(),other.locator('[data-action="start-timer"]').click()]);
  // Wait for both asynchronous start outcomes. The winning callback closes its
  // sheet; Escape also closes the losing tab without a count/click race.
  await Promise.all([page,other].map(tab=>expect(tab.locator('.toast').filter({hasText:/^(Timer started|Finish the active timer first\.)$/})).toBeVisible()));
  await Promise.all([page.keyboard.press('Escape'),other.keyboard.press('Escape')]);
  await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  await expect(other.locator('.bottom-sheet')).toHaveCount(0);
  await expect(page.locator('#timer-chip')).toBeVisible();await expect(other.locator('#timer-chip')).toBeVisible();
  let release;
  const gate=new Promise(resolve => {release=resolve;});
  const posted=[];
  await context.route('**/api/logs',async route => {
    posted.push(route.request().postDataJSON());
    const response=await route.fetch();await gate;await route.fulfill({response});
  });
  await Promise.all([page.locator('#timer-chip').dispatchEvent('click'),other.locator('#timer-chip').dispatchEvent('click')]);
  await expect.poll(() => posted.length).toBe(1);
  release();
  await expect(page.locator('#timer-chip')).toHaveCount(0);await expect(other.locator('#timer-chip')).toHaveCount(0);
  expect(new Set(posted.map(body=>body.idempotencyKey)).size).toBe(1);
  expect(posted.every(body=>JSON.stringify(body)===JSON.stringify(posted[0]))).toBeTruthy();
  expect(await history(page,chore)).toHaveLength(1);
});

test('discard while replay waits cannot recreate the discarded entry',async ({page}) => {
  const {chore}=await setup(page);
  const keys=await page.evaluate(async choreId => {
    const suffix=new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
    const journal=await import(`/static/js/offline-queue.js${suffix}`), api=await import(`/static/js/api.js${suffix}`);
    const {withBrowserLock}=await import(`/static/js/device-store.js${suffix}`);
    let acquired;
    const ready=new Promise(resolve=>{acquired=resolve;});
    const hold=new Promise(resolve=>{window.releaseReplayFixture=resolve;});
    window.replayFixtureLock=withBrowserLock('nabu-log-replay',async()=>{acquired();await hold;});
    await ready;
    window.recoveryModules={journal,api};
    const keys=[crypto.randomUUID(),crypto.randomUUID()];
    for (const [index,idempotencyKey] of keys.entries()) await journal.enqueueLog({choreId,note:`entry ${index}`,indicators:[],completedAt:new Date().toISOString(),idempotencyKey});
    return keys;
  },chore.id);
  const order=(await rows(page)).sort((a,b)=>b.createdAt-a.createdAt).map(row=>row.idempotencyKey);
  let release;
  const gate=new Promise(resolve=>{release=resolve;}),posted=[];
  await page.route('**/api/logs',async route=>{
    posted.push(route.request().postDataJSON().idempotencyKey);
    const response=await route.fetch();await gate;await route.fulfill({response});
  });
  expect(await history(page,chore)).toHaveLength(0);
  await page.evaluate(()=>{window.recoveryRun=window.recoveryModules.journal.replayQueue(window.recoveryModules.api.apiFetch);});
  await page.evaluate(async()=>{window.releaseReplayFixture();await window.replayFixtureLock;});
  await expect.poll(()=>posted.length).toBe(1);
  expect(posted[0]).toBe(order[0]);
  await page.evaluate(key=>window.recoveryModules.journal.discardQueuedLog(key),order[1]);
  release();
  await page.evaluate(()=>window.recoveryRun);
  expect(posted).toEqual([order[0]]);
  expect(await rows(page)).toHaveLength(0);
  expect(await history(page,chore)).toHaveLength(1);
  expect(keys).toHaveLength(2);
});

test('note and time edits preserve a duration metric',async ({page})=>{
  const {chore,headers}=await setup(page,'duration');
  const response=await page.request.post('/api/logs',{headers,data:{choreId:chore.id,note:'duration before edit',durationSeconds:90,completedAt:new Date().toISOString(),hour:new Date().getHours()}});
  expect(response.ok()).toBeTruthy();
  await page.goto('/activity');
  await page.locator('.hist-row').filter({hasText:'duration before edit'}).click();
  await page.locator('#log-note').fill('duration retained');
  await page.locator('[data-action="save-log"]').click();
  await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  expect((await history(page,chore))[0].durationSeconds).toBe(90);
  await page.locator('.hist-row').filter({hasText:'duration retained'}).click();
  const when=await page.locator('#log-when').inputValue();
  await page.locator('#log-when').fill(when.slice(0,14)+(when.slice(14,16)==='17'?'18':'17'));
  await page.locator('[data-action="save-log"]').click();
  await expect(page.locator('.bottom-sheet')).toHaveCount(0);
  expect((await history(page,chore))[0].durationSeconds).toBe(90);
});
