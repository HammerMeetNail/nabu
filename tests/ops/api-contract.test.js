import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawn,spawnSync} from 'node:child_process';
import {createServer} from 'node:http';
import {mkdtemp,copyFile,mkdir,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';

// The pre-push hook runs this file, and git exports GIT_DIR to hooks. Every
// command here gets an environment without GIT_* variables, so the scratch
// repositories never become the repository being pushed.
function scratchEnv(extra={}) {
  const env=Object.fromEntries(Object.entries(process.env).filter(([key])=>!key.startsWith('GIT_')));
  return {...env,...extra};
}

function git(cwd,...args) {
  const result=spawnSync('git',args,{cwd,encoding:'utf8',env:scratchEnv()});
  assert.equal(result.status,0,`git ${args.join(' ')}: ${result.stdout}${result.stderr}`);
}

// The script asks the fake production server for /health, so it runs
// alongside this process instead of blocking it.
function runCheck(cwd,url) {
  return new Promise((done,fail)=>{
    const child=spawn('bash',['scripts/api-contract-changed.sh'],{cwd,env:scratchEnv({NABU_URL:url})});
    let stdout='';
    let stderr='';
    child.stdout.on('data',chunk=>{stdout+=chunk;});
    child.stderr.on('data',chunk=>{stderr+=chunk;});
    child.on('error',fail);
    child.on('close',code=>done({code,stdout,stderr}));
  });
}

// A repository released as v0.1.2 with an API contract, and a production
// server reporting the version health answers with.
async function released(t,health={status:'ok',version:'0.1.2'}) {
  const dir=await mkdtemp(join(tmpdir(),'nabu-api-contract-test-'));
  t.after(()=>rm(dir,{recursive:true,force:true}));
  const work=join(dir,'work');
  git(dir,'init','-q','-b','main',work);
  git(work,'config','user.email','release@example.invalid');
  git(work,'config','user.name','Release');
  for(const path of ['scripts','api/contract','internal/app']) await mkdir(join(work,path),{recursive:true});
  await copyFile(resolve('scripts','api-contract-changed.sh'),join(work,'scripts','api-contract-changed.sh'));
  await writeFile(join(work,'api/contract/me.json'),'{"status":200,"response":{"user":null}}\n');
  await writeFile(join(work,'internal/app/server.go'),'package app\n');
  git(work,'add','.');
  git(work,'commit','-q','-m','Release 0.1.2');
  git(work,'tag','-a','v0.1.2','-m','v0.1.2');

  const server=createServer((req,res)=>{
    if(req.url!=='/health'||!health) { res.writeHead(503); res.end(); return; }
    res.writeHead(200,{'Content-Type':'application/json'});
    res.end(JSON.stringify(health)+'\n');
  });
  await new Promise(ready=>server.listen(0,'127.0.0.1',ready));
  t.after(()=>new Promise(closed=>server.close(closed)));
  const change=async(path,text,message)=>{
    await writeFile(join(work,path),text);
    git(work,'add',path);
    git(work,'commit','-q','-m',message);
  };
  return {work,url:`http://127.0.0.1:${server.address().port}`,change};
}

test('skips the iOS tests when the contract is the one production answers with',async t=>{
  const {work,url,change}=await released(t);
  await change('internal/app/server.go','package app // faster, same answers\n','Server change');
  const result=await runCheck(work,url);
  assert.equal(result.code,0,result.stderr);
  assert.equal(result.stdout,'changed=false\n');
  assert.match(result.stderr,/production's v0\.1\.2/);
});

test('runs the iOS tests when an answer changed since production',async t=>{
  const {work,url,change}=await released(t);
  await change('api/contract/me.json','{"status":200,"response":{"account":null}}\n','Rename user');
  const result=await runCheck(work,url);
  assert.equal(result.code,0,result.stderr);
  assert.equal(result.stdout,'changed=true\n');
  assert.match(result.stderr,/api\/contract\/me\.json/);
});

test('runs the iOS tests when an answer is new',async t=>{
  const {work,url,change}=await released(t);
  await change('api/contract/packs.json','{"status":200,"response":{"packs":[]}}\n','New answer');
  const result=await runCheck(work,url);
  assert.equal(result.stdout,'changed=true\n');
  assert.match(result.stderr,/A\s+api\/contract\/packs\.json/);
});

test('runs the iOS tests when production\'s release is unknown',async t=>{
  const down=await released(t,null);
  let result=await runCheck(down.work,down.url);
  assert.equal(result.code,0,result.stderr);
  assert.equal(result.stdout,'changed=true\n');
  assert.match(result.stderr,/Could not read/);

  const unversioned=await released(t,{status:'ok'});
  result=await runCheck(unversioned.work,unversioned.url);
  assert.equal(result.stdout,'changed=true\n');
  assert.match(result.stderr,/doesn't report a version/);

  const untagged=await released(t,{status:'ok',version:'0.1.9'});
  result=await runCheck(untagged.work,untagged.url);
  assert.equal(result.stdout,'changed=true\n');
  assert.match(result.stderr,/no tag v0\.1\.9/);
});
