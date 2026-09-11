import {test,expect} from '@playwright/test';
import {fixture,headers,postLog,signIn,deferred} from './review-fixtures.js';

async function queueRow(page,chore,note='Saved before session expired',holdReplay=false) {
  return page.evaluate(async ({choreId,note,holdReplay})=>{
    const suffix=new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
    const journal=await import(`/static/js/offline-queue.js${suffix}`);
    const api=await import(`/static/js/api.js${suffix}`);
    const {withBrowserLock}=await import(`/static/js/device-store.js${suffix}`);
    window.sessionTest={journal,api};
    if(holdReplay){
      let entered;
      const held=new Promise(resolve=>{entered=resolve;});
      const release=new Promise(resolve=>{window.sessionTest.releaseReplay=resolve;});
      window.sessionTest.replayBarrier=withBrowserLock('nabu-log-replay',async()=>{entered();await release;});
      await held;
    } else await withBrowserLock('nabu-log-replay',()=>{});
    const idempotencyKey=crypto.randomUUID();
    await journal.enqueueLog({choreId,note,completedAt:new Date().toISOString(),hour:12,idempotencyKey});
    return idempotencyKey;
  },{choreId:chore.id,note,holdReplay});
}
async function allRows(page) {
  return page.evaluate(()=>new Promise((resolve,reject)=>{
    const request=indexedDB.open('nabu-offline',2);
    request.onerror=()=>reject(request.error);
    request.onsuccess=()=>{
      const db=request.result,tx=db.transaction('logQueue','readonly'),read=tx.objectStore('logQueue').getAll();
      let result;read.onsuccess=()=>{result=read.result;};
      tx.oncomplete=()=>{db.close();resolve(result);};
      tx.onabort=()=>{db.close();reject(tx.error);};
    };
  }));
}
async function logOutClick(page) {
  await page.locator('#hh-indicator').click();
  await page.locator('[data-action="logout"]').click();
}

for (const failure of [403,429,500,'network']) {
  test(`unfinished logout ${failure} survives reload and resumes only after revocation`,async({page})=>{
    const {chore}=await fixture(page);
    const key=await queueRow(page,chore,'Only this account can see the saved draft',true);
    const posted=[];
    await page.route('**/api/logs',async route=>{posted.push(route.request().postDataJSON());await route.fulfill({status:500,body:'{}'});});
    await page.route('**/api/auth/logout',route=>failure==='network'?route.abort('connectionreset'):route.fulfill({status:failure,body:'{}'}));
    await logOutClick(page);
    await expect(page.getByRole('heading',{name:'Sign-out is unfinished'})).toBeVisible();
    await expect(page.locator('body')).not.toContainText('Only this account can see the saved draft');
    await page.evaluate(()=>{window.sessionTest.releaseReplay();return window.sessionTest.replayBarrier;});
    await page.reload();
    await expect(page.getByRole('heading',{name:'Sign-out is unfinished'})).toBeVisible();
    await page.evaluate(()=>{window.dispatchEvent(new Event('online'));document.dispatchEvent(new Event('visibilitychange'));});
    await expect(page.locator('[data-action="retry-logout"]')).toBeEnabled();
    expect(posted).toEqual([]);
    expect((await allRows(page)).map(row=>row.idempotencyKey)).toEqual([key]);
    // The rejected request did not revoke the real cookie.
    expect((await (await page.request.get('/api/me')).json()).user).not.toBeNull();
    await page.unroute('**/api/auth/logout');
    await page.locator('[data-action="retry-logout"]').click();
    await expect(page.locator('#login-form')).toBeVisible();
    expect((await (await page.request.get('/api/me')).json()).user).toBeNull();
    expect((await allRows(page)).map(row=>row.idempotencyKey)).toEqual([key]);
  });
}

test('session expiry hides account data and keeps its queue until that account returns',async({page})=>{
  const {email,chore}=await fixture(page);
  await postLog(page,chore,{note:'History must disappear on expiry'});
  await page.goto('/activity');
  await expect(page.locator('.hist-row')).toContainText('History must disappear on expiry');
  const key=await queueRow(page,chore,'Saved before session expired',true);
  const expired=await page.request.post('/api/auth/logout',{headers:await headers(page)});
  expect(expired.ok()).toBeTruthy();
  await page.evaluate(()=>window.sessionTest.api.apiFetch('/api/notifications').catch(()=>{}));
  await expect(page.locator('#login-form')).toBeVisible();
  await expect(page.locator('body')).not.toContainText('History must disappear on expiry');
  await page.evaluate(()=>{window.sessionTest.releaseReplay();return window.sessionTest.replayBarrier;});
  expect((await allRows(page)).map(row=>row.idempotencyKey)).toEqual([key]);
  await signIn(page,email);
  await expect(page.locator('.pending-work')).toContainText('Saved before session expired');
  await page.locator('.pending-work > summary').click();
  await page.locator('[data-action="retry-pending-log"]').click();
  await expect.poll(async()=>(await allRows(page)).length).toBe(0);
  const history=await (await page.request.get('/api/logs/history')).json();
  expect(history.logs.filter(log=>log.note==='Saved before session expired')).toHaveLength(1);
});

test('foreground confirms session before polling, push or journal replay',async({page})=>{
  await page.clock.install();
  const {chore}=await fixture(page);
  await page.clock.runFor(29000);
  await queueRow(page,chore);
  const entered=deferred(),release=deferred();
  let checked=false;
  await page.route('**/api/me',async route=>{entered.resolve();await release.promise;checked=true;await route.continue();});
  const early=[];
  page.on('request',request=>{
    const path=new URL(request.url()).pathname;
    if (!checked && (path==='/api/notifications'||path==='/api/push/subscribe'||(path==='/api/logs'&&request.method()==='POST'))) early.push(path);
  });
  try {
    await page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')));
    await entered.promise;
    await page.clock.runFor(2000);
    expect(early).toEqual([]);
  } finally {release.resolve();}
  await expect.poll(async()=>(await allRows(page)).length).toBe(0);
});

test('an unreadable authoritative store cannot bypass a pending logout through an old mirror',async({page})=>{
  await fixture(page);
  await page.evaluate(()=>{
    const set=Storage.prototype.setItem;
    Storage.prototype.setItem=function(key,value){if(key==='nabu-browser-identity') throw new Error('mirror unavailable');return set.call(this,key,value);};
  });
  await page.route('**/api/auth/logout',route=>route.fulfill({status:500,body:'{}'}));
  await logOutClick(page);
  await expect(page.getByRole('heading',{name:'Sign-out is unfinished'})).toBeVisible();
  expect(await page.evaluate(()=>JSON.parse(localStorage.getItem('nabu-browser-identity')).status)).toBe('active');
  await page.addInitScript(()=>{
    const open=IDBFactory.prototype.open;
    IDBFactory.prototype.open=function(name,...args){if(name==='nabu-device') throw new Error('authority unavailable');return open.call(this,name,...args);};
  });
  let meReads=0;
  page.on('request',request=>{if(new URL(request.url()).pathname==='/api/me') meReads++;});
  await page.reload();
  await expect(page.getByRole('heading',{name:'Check your session'})).toBeVisible();
  expect(meReads).toBe(0);
  await expect(page.locator('.home-grid')).toHaveCount(0);
});

test('a late 401 from account A cannot clear a newer login for B',async({page,browser})=>{
  const other=await browser.newContext({baseURL:process.env.BASE_URL || 'http://localhost:8080'});
  const otherPage=await other.newPage();
  const accountB=await fixture(otherPage);
  await other.close();
  await fixture(page);
  const entered=deferred(),release=deferred();
  await page.route('**/api/notifications?obsolete=1',async route=>{
    entered.resolve();await release.promise;await route.fulfill({status:401,body:'unauthorized'});
  });
  await page.evaluate(async()=>{
    const suffix=new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
    const api=await import(`/static/js/api.js${suffix}`);
    window.oldAccountRead=api.apiFetch('/api/notifications?obsolete=1').catch(error=>error.status);
  });
  try {
    await entered.promise;
    await logOutClick(page);
    await expect(page.locator('#login-form')).toBeVisible();
    await signIn(page,accountB.email);
    release.resolve();
    expect(await page.evaluate(()=>window.oldAccountRead)).toBe(401);
    await expect(page.locator('.home-grid')).toBeVisible();
    expect((await (await page.request.get('/api/me')).json()).user.email).toBe(accountB.email);
    await expect(page.getByRole('heading',{name:'Check your session'})).toHaveCount(0);
  } finally {release.resolve();}
});

test('a context mismatch during foreground confirmation fences the old account immediately',async({page})=>{
  const {chore}=await fixture(page);
  const key=await queueRow(page,chore,'Keep this only in the first household');
  const entered=deferred(),release=deferred();
  let meReads=0;
  await page.route('**/api/me',async route=>{
    meReads++;
    if(meReads===1){const response=await route.fetch();entered.resolve();await release.promise;await route.fulfill({response});}
    else await route.continue();
  });
  const posts=[];
  page.on('request',request=>{if(new URL(request.url()).pathname==='/api/logs'&&request.method()==='POST') posts.push(request.postDataJSON());});
  try {
    await page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')));
    await entered.promise;
    const response=await page.request.post('/api/household',{headers:await headers(page),data:{name:'Canonical new household',initials:'BX'}});
    expect(response.ok()).toBeTruthy();
    await page.evaluate(()=>window.sessionTest.api.apiFetch('/api/notifications').catch(()=>{}));
    await expect(page.getByRole('heading',{name:'Check your session'})).toBeVisible();
    const fenced=await page.evaluate(async()=>{
      const suffix=new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
      const {contextSnapshot}=await import(`/static/js/browser-context.js${suffix}`);
      return contextSnapshot().status;
    });
    expect(fenced).toBe('unconfirmed');
    release.resolve();
    await expect(page.locator('#hh-indicator')).toHaveText('BX');
    expect(meReads).toBeGreaterThanOrEqual(2);
    expect(posts).toEqual([]);
    expect((await allRows(page)).map(row=>row.idempotencyKey)).toEqual([key]);
    await expect(page.locator('body')).not.toContainText('Keep this only in the first household');
  } finally {release.resolve();}
});

test('an active page hides data when foreground identity storage cannot be read',async({page})=>{
  // History can render before bootstrap finishes registering foreground
  // recovery. Wait for the listener before injecting its storage failure.
  await page.addInitScript(() => {
    const addEventListener = document.addEventListener;
    document.addEventListener = function(type, listener, options) {
      addEventListener.call(this, type, listener, options);
      if (type === 'visibilitychange') window.foregroundRecoveryReady = true;
    };
    const fetch = window.fetch;
    window.fetch = async function(input, options) {
      const response = await fetch.call(this, input, options);
      const url = input instanceof Request ? input.url : input;
      if (window.foregroundRecoveryReady && new URL(url, location.href).pathname === '/api/me') {
        window.foregroundBootstrapResponseReceived = true;
      }
      return response;
    };
  });
  const {chore}=await fixture(page);
  await postLog(page,chore,{note:'Private foreground history'});
  await page.goto('/activity');
  await expect(page.locator('.hist-row')).toContainText('Private foreground history');
  // Bootstrap starts a queue session check after registering the listener.
  // Let its response arrive, then drain its identity lock so our foreground
  // event cannot reuse an already-running check that read storage earlier.
  await page.waitForFunction(() => window.foregroundBootstrapResponseReceived === true && !document.hidden);
  await page.evaluate(async () => {
    const suffix = new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
    const {withBrowserLock} = await import(`/static/js/device-store.js${suffix}`);
    await withBrowserLock('nabu-identity', () => {});
  });
  await page.evaluate(()=>{
    window.originalIdentityOpen=IDBFactory.prototype.open;
    IDBFactory.prototype.open=function(name,...args){if(name==='nabu-device') throw new Error('authority unavailable');return window.originalIdentityOpen.call(this,name,...args);};
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await expect(page.getByRole('heading',{name:'Check your session'})).toBeVisible();
  await expect(page.locator('body')).not.toContainText('Private foreground history');
  await page.evaluate(()=>{IDBFactory.prototype.open=window.originalIdentityOpen;});
  await page.locator('[data-action="retry-session"]').click();
  await expect(page.locator('.home-grid')).toBeVisible();
});

test('logout remains retryable after repeated identity storage failures',async({page})=>{
  await fixture(page);
  await page.evaluate(()=>{
    window.originalIdentityOpen=IDBFactory.prototype.open;
    window.identityFailures=0;
    IDBFactory.prototype.open=function(name,...args){if(name==='nabu-device'){window.identityFailures++;throw new Error('authority unavailable');}return window.originalIdentityOpen.call(this,name,...args);};
  });
  await logOutClick(page);
  await expect(page.getByRole('heading',{name:'Sign-out is unfinished'})).toBeVisible();
  await expect(page.locator('[data-action="retry-logout"]')).toBeEnabled();
  const firstFailures=await page.evaluate(()=>window.identityFailures);
  await page.locator('[data-action="retry-logout"]').click();
  await expect.poll(()=>page.evaluate(()=>window.identityFailures)).toBeGreaterThan(firstFailures);
  await expect(page.locator('[data-action="retry-logout"]')).toBeEnabled();
  await page.evaluate(()=>{IDBFactory.prototype.open=window.originalIdentityOpen;});
  await page.locator('[data-action="retry-logout"]').click();
  await expect(page.locator('#login-form')).toBeVisible();
  expect((await (await page.request.get('/api/me')).json()).user).toBeNull();
});
