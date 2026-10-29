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
    const child=spawn('bash',['scripts/ios-release-web-first.sh'],{cwd,env:scratchEnv({NABU_URL:url})});
    let output='';
    child.stdout.on('data',chunk=>{output+=chunk;});
    child.stderr.on('data',chunk=>{output+=chunk;});
    child.on('error',fail);
    child.on('close',code=>done({code,output}));
  });
}

// A repository released as v0.1.2, and a production server reporting the
// version health answers with.
async function released(t,health={status:'ok',version:'0.1.2'}) {
  const dir=await mkdtemp(join(tmpdir(),'nabu-ios-release-test-'));
  t.after(()=>rm(dir,{recursive:true,force:true}));
  const work=join(dir,'work');
  git(dir,'init','-q','-b','main',work);
  git(work,'config','user.email','release@example.invalid');
  git(work,'config','user.name','Release');
  for(const path of ['scripts','internal/app','web/static','migrations']) await mkdir(join(work,path),{recursive:true});
  await copyFile(resolve('scripts','ios-release-web-first.sh'),join(work,'scripts','ios-release-web-first.sh'));
  await writeFile(join(work,'internal/app/server.go'),'package app\n');
  await writeFile(join(work,'internal/app/server_test.go'),'package app\n');
  await writeFile(join(work,'web/static/app.js'),'// app\n');
  git(work,'add','.');
  git(work,'commit','-q','-m','Release 0.1.2');
  git(work,'tag','-a','v0.1.2','-m','v0.1.2');

  const server=createServer((req,res)=>{
    if(req.url!=='/health') { res.writeHead(404); res.end(); return; }
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

test('releases the app when production runs this commit',async t=>{
  const {work,url}=await released(t);
  const result=await runCheck(work,url);
  assert.equal(result.code,0,result.output);
  assert.match(result.output,/Production runs v0\.1\.2/);
});

test('releases without a web deploy when only the PWA, docs or tests changed',async t=>{
  const {work,url,change}=await released(t);
  await change('web/static/app.js','// app, changed\n','PWA change');
  await change('internal/app/server_test.go','package app // more tests\n','Test change');
  const result=await runCheck(work,url);
  assert.equal(result.code,0,result.output);
});

test('waits for a web deploy when the server changed since production',async t=>{
  const {work,url,change}=await released(t);
  await change('internal/app/server.go','package app // new endpoint\n','Server change');
  const result=await runCheck(work,url);
  assert.equal(result.code,1,result.output);
  assert.match(result.output,/internal\/app\/server\.go/);
  assert.match(result.output,/Deploy the web app first/);
});

test('waits for a web deploy when a migration is new',async t=>{
  const {work,url,change}=await released(t);
  await change('migrations/062_new.sql','SELECT 1;\n','Migration');
  const result=await runCheck(work,url);
  assert.equal(result.code,1,result.output);
  assert.match(result.output,/migrations\/062_new\.sql/);
});

test('refuses when production reports no version, or one with no tag',async t=>{
  const unversioned=await released(t,{status:'ok'});
  let result=await runCheck(unversioned.work,unversioned.url);
  assert.equal(result.code,1,result.output);
  assert.match(result.output,/doesn't report a version/);

  const untagged=await released(t,{status:'ok',version:'0.1.9'});
  result=await runCheck(untagged.work,untagged.url);
  assert.equal(result.code,1,result.output);
  assert.match(result.output,/no tag v0\.1\.9/);
});
