import {expect} from '@playwright/test';

export const uniqueEmail = () => `e2e-context-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
export async function register(page,email=uniqueEmail()) {
  await page.goto('/register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill('test123456');
  await page.locator('#reg-confirm').fill('test123456');
  await page.locator('#register-form button[type=submit]').click();
  await expect(page.locator('#hh-indicator')).toBeVisible();
  return email;
}
export async function headers(page) {
  return {'X-CSRF-Token':(await page.context().cookies()).find(c=>c.name==='nabu_csrf')?.value};
}
export async function fixture(page,props={}) {
  const email=await register(page), h=await headers(page);
  const response=await page.request.post('/api/household',{headers:h,data:{name:'Synthetic review home'}});
  expect(response.ok()).toBeTruthy();
  const household=(await response.json()).household;
  const res=await page.request.post('/api/chores',{headers:h,data:{name:'Synthetic amount',icon:'📝',color:'#6080AA',metricType:'amount',metricUnit:'mL',...props}});
  expect(res.ok()).toBeTruthy();
  const chore=(await res.json()).chore;
  await page.reload();
  await expect(page.locator('.home-grid')).toBeVisible();
  return {email,household,chore,headers:h};
}
export async function postLog(page,chore,fields={}) {
  const res=await page.request.post('/api/logs',{headers:await headers(page),data:{choreId:chore.id,completedAt:new Date().toISOString(),hour:12,...fields}});
  expect(res.ok()).toBeTruthy();
  return (await res.json()).log;
}
export async function rerender(page) {
  await page.evaluate(async()=>{
    const suffix=new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
    const {render}=await import(`/static/js/app.js${suffix}`);
    render(document.querySelector('#app'));
  });
}
export async function signOut(page) {
  await page.locator('#hh-indicator').click();
  await page.locator('[data-action="logout"]').click();
  await expect(page.locator('#login-form')).toBeVisible();
}
export async function signIn(page,email) {
  await page.locator('#login-email').fill(email);
  await page.locator('#login-password').fill('test123456');
  await page.locator('#login-form button[type=submit]').click();
  await expect(page.locator('#hh-indicator')).toBeVisible();
}
export function deferred() {let resolve;const promise=new Promise(r=>{resolve=r;});return {promise,resolve};}

// Observe the real loader promises in the served module. Joining a route handler
// alone does not prove that the app consumed its response or finished publishing.
export async function observeModuleCalls(page,moduleName,symbol,{exported=true}={}) {
  await page.route(`**/static/js/${moduleName}.js*`,async route=>{
    const response=await route.fetch();
    const source=await response.text();
    const declaration=new RegExp(`^${exported?'export ':''}(async )?function ${symbol}\\(`,'m');
    expect(source).toMatch(declaration);
    const body=source.replace(declaration,`$1function __observed_${symbol}(`)+`
${exported?'export ':''}function ${symbol}(...args) {
  const pending=__observed_${symbol}(...args);
  ((globalThis.__reviewModuleCalls ||= {})['${symbol}'] ||= []).push(Promise.resolve(pending));
  return pending;
}
`;
    const responseHeaders={...response.headers()};
    delete responseHeaders['content-length'];delete responseHeaders['content-encoding'];
    await route.fulfill({response,headers:responseHeaders,body});
  });
  return async()=>{
    // Retain the join in a remote object until Playwright finishes awaiting it.
    // Chromium can otherwise collect the temporary evaluation promise and
    // report "Promise was collected", surfaced by Playwright as navigation.
    const waiter=await page.evaluateHandle(name=>{
      const calls=globalThis.__reviewModuleCalls?.[name];
      if(!calls?.length) throw new Error(`No observed calls to ${name}`);
      const pending=(async()=>{
        let count=0;
        do {count=calls.length;await Promise.all(calls.slice());} while(count!==calls.length);
      })();
      // The original promise still rejects when joined below. Mark it handled
      // during the protocol round trip that obtains this retaining handle.
      pending.catch(()=>{});
      return {pending};
    },symbol);
    try {await waiter.evaluate(({pending})=>pending);}
    finally {await waiter.dispose();}
  };
}
