import {test, expect} from '@playwright/test';
import {execFile, spawn} from 'node:child_process';
import {promisify} from 'node:util';
import {mkdtemp, readFile, rm} from 'node:fs/promises';
import {openSync, closeSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createServer} from 'node:net';

// A separate low-budget process exercises the real middleware without changing
// or exhausting the shared PostgreSQL E2E server's limits.
test.describe.configure({mode:'serial'});
let artifacts, binary;
test.beforeAll(async()=>{
  test.setTimeout(180000);
  artifacts=await mkdtemp(join(tmpdir(),'nabu-ops-e2e-'));
  binary=join(artifacts,'server');
  await promisify(execFile)('go',['build','-o',binary,'./cmd/server'],{timeout:150000});
});
test.afterAll(async()=>{if(binary) await rm(binary,{force:true});});

async function withServer(request, run, overrides={}) {
  const reservation=createServer();
  await new Promise(resolve=>reservation.listen(0,'127.0.0.1',resolve));
  const port=reservation.address().port;
  await new Promise(resolve=>reservation.close(resolve));
  const url=`http://127.0.0.1:${port}`;
  const logPath=join(artifacts,`server-${port}.log`), fd=openSync(logPath,'w',0o600);
  const child=spawn(binary,[],{stdio:['ignore',fd,fd],env:{...process.env,
    PORT:String(port),APP_ENV:'development',APP_BASE_URL:url,SERVER_SECURE:'false',
    DATABASE_URL:'',SMTP_HOST:'',GOOGLE_CLIENT_ID:'',GOOGLE_CLIENT_SECRET:'',
    VAPID_PRIVATE_KEY:'',VAPID_PUBLIC_KEY:'',APNS_AUTH_KEY_P8:'',
    TRUSTED_PROXY_CIDRS:'127.0.0.1/32,::1/128',RATE_LIMIT_AUTH_MAX:'3',
    RATE_LIMIT_GLOBAL_MAX:'8',RATE_LIMIT_JOIN_MAX:'2',RATE_LIMIT_MAX_CLIENTS:'8',...overrides}});
  closeSync(fd);
  const exit=new Promise(resolve=>{
    child.once('exit',(code,signal)=>resolve({code,signal}));
    child.once('error',error=>resolve({error}));
  });
  try {
    await expect.poll(async()=>{
      if(child.exitCode!==null) return -1;
      try{return (await request.get(`${url}/ready`,{timeout:1000})).status();}catch{return 0;}
    }).toBe(200);
    await run(url,logPath);
  } finally {
    child.kill('SIGTERM');
    let forced=false;
    const kill=setTimeout(()=>{forced=true;child.kill('SIGKILL');},20000);
    const result=await exit;
    clearTimeout(kill);
    if(result.error) throw result.error;
    expect(forced,'server needed forced shutdown').toBe(false);
    expect(result.signal,'server terminated by signal').toBeNull();
    expect(result.code,'server shutdown exit status').toBe(0);
  }
}

test('resource path changes share one API allowance and auth retains its stricter budget',async({request})=>{
  await withServer(request,async url=>{
    const headers={'X-Forwarded-For':'203.0.113.10'};
    for(let i=0;i<8;i++) expect((await request.get(`${url}/api/chores/${i}`,{headers})).status()).not.toBe(429);
    const rejected=await request.get(`${url}/api/logs/new-resource`,{headers});
    expect(rejected.status()).toBe(429);
    expect(Number(rejected.headers()['retry-after'])).toBeGreaterThanOrEqual(1);
    expect(Number(rejected.headers()['retry-after'])).toBeLessThanOrEqual(60);
    const other={'X-Forwarded-For':'203.0.113.11'};
    for(let i=0;i<3;i++) expect((await request.get(`${url}/api/auth/unknown-${i}`,{headers:other})).status()).not.toBe(429);
    expect((await request.get(`${url}/api/auth/login`,{headers:other})).status()).toBe(429);
    expect((await request.get(`${url}/api/me`,{headers:other})).status()).toBe(200);
    expect((await request.get(`${url}/health`)).status()).toBe(200);
  });
});

test('source saturation rejects new clients without resetting existing allowances',async({request})=>{
  await withServer(request,async url=>{
    for(let i=1;i<=8;i++) expect((await request.get(`${url}/api/me`,{headers:{'X-Forwarded-For':`198.51.100.${i}`}})).status()).toBe(200);
    const rejected=await request.get(`${url}/api/me`,{headers:{'X-Forwarded-For':'198.51.100.9'}});
    expect(rejected.status()).toBe(429);
    expect(rejected.headers()['retry-after']).toBeTruthy();
    expect((await request.get(`${url}/api/me`,{headers:{'X-Forwarded-For':'198.51.100.1'}})).status()).toBe(200);
  });
});

test('household audit events omit names while recording the operation',async({request})=>{
  await withServer(request,async(url,logPath)=>{
    await request.get(`${url}/login`);
    const cookies=(await request.storageState()).cookies;
    const headers={'X-CSRF-Token':cookies.find(c=>c.name==='nabu_csrf').value};
    const reg=await request.post(`${url}/api/auth/register`,{headers,data:{email:`ops-${Date.now()}@example.invalid`,password:'test123456'}});
    expect(reg.status()).toBe(201);
    const secret='RECOGNIZABLE-PRIVATE-HOUSEHOLD-NAME';
    expect((await request.post(`${url}/api/household`,{headers,data:{name:secret}})).status()).toBe(201);
    expect((await request.patch(`${url}/api/household`,{headers,data:{name:secret+'-changed'}})).status()).toBe(200);
    const output=await readFile(logPath,'utf8');
    expect(output.includes('household.created')).toBe(true);
    expect(output.includes('household.updated')).toBe(true);
    expect(output.includes(secret)).toBe(false);
  });
});
