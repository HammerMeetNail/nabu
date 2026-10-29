import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {mkdtemp,copyFile,mkdir,readFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';

// The pre-push hook runs this file, and git exports GIT_DIR to hooks. Every
// command here gets an environment without GIT_* variables, so the scratch
// repositories never become the repository being pushed.
function scratchEnv(extra={}) {
  const env=Object.fromEntries(Object.entries(process.env).filter(([key])=>!key.startsWith('GIT_')));
  return {...env,...extra};
}

function run(cmd,args,options={}) {
  const result=spawnSync(cmd,args,{encoding:'utf8',...options,env:scratchEnv(options.env)});
  if(result.error) throw result.error;
  return {code:result.status,output:(result.stdout||'')+(result.stderr||'')};
}

function git(cwd,...args) {
  const result=run('git',args,{cwd});
  assert.equal(result.code,0,`git ${args.join(' ')}: ${result.output}`);
  return result.output;
}

// make deploy names the next tag from the first commit since the last one. A
// release spans many commits, more of git's output than a pipe holds: the
// script must read it all, or git dies of SIGPIPE and pipefail aborts the
// release before it is tagged.
async function releaseDryRun(t) {
  const dir=await mkdtemp(join(tmpdir(),'nabu-release-test-'));
  t.after(()=>rm(dir,{recursive:true,force:true}));
  const origin=join(dir,'origin.git'), work=join(dir,'work');
  git(dir,'init','-q','--bare',origin);
  git(dir,'init','-q','-b','main',work);
  git(work,'config','user.email','release@example.invalid');
  git(work,'config','user.name','Release');
  git(work,'remote','add','origin',origin);
  await mkdir(join(work,'scripts'));
  await copyFile(resolve('scripts','deploy.sh'),join(work,'scripts','deploy.sh'));
  git(work,'add','scripts/deploy.sh');
  git(work,'commit','-q','-m','Add the release script');
  git(work,'tag','-a','v0.1.9','-m','v0.1.9');

  const commits=1500, when=1767225600;
  let stream='';
  for(let i=1;i<=commits;i++) {
    const message=`Change ${i}: ${'a long enough subject to fill the pipe '.repeat(3)}\n`;
    stream+=`commit refs/heads/main\ncommitter Release <release@example.invalid> ${when+i} +0000\n`+
      `data ${Buffer.byteLength(message)}\n${message}${i===1?'from refs/heads/main^0\n':''}\n`;
  }
  const imported=run('git',['fast-import','--quiet'],{cwd:work,input:stream});
  assert.equal(imported.code,0,imported.output);
  git(work,'push','-q','-u','origin','main','--tags');

  const release=run('bash',['scripts/deploy.sh'],{cwd:work,env:{DRY_RUN:'1'}});
  assert.equal(release.code,0,release.output);
  assert.match(release.output,/Next tag: v0\.1\.10\n/);
  assert.match(release.output,/would create annotated tag v0\.1\.10 \('v0\.1\.10: Change 1: /);
}

test('release dry run names the next tag after a long range',async t=>{
  await releaseDryRun(t);
});

// Run under a git hook, the scenario once re-initialised the pushed
// repository as bare and wrote its test identity into that repository's
// config. Now a decoy repository named by GIT_DIR comes out untouched.
test('the release scenario never touches the repository GIT_DIR names',async t=>{
  const dir=await mkdtemp(join(tmpdir(),'nabu-release-decoy-'));
  t.after(()=>rm(dir,{recursive:true,force:true}));
  const decoy=join(dir,'decoy');
  git(dir,'init','-q',decoy);
  const before=await readFile(join(decoy,'.git','config'),'utf8');
  const saved=process.env.GIT_DIR;
  process.env.GIT_DIR=join(decoy,'.git');
  try {
    await releaseDryRun(t);
  } finally {
    if(saved===undefined) delete process.env.GIT_DIR; else process.env.GIT_DIR=saved;
  }
  assert.equal(await readFile(join(decoy,'.git','config'),'utf8'),before);
});
