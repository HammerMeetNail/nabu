import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createServer} from 'node:http';
import {mkdtemp,writeFile,readFile,copyFile,chmod,rm,mkdir} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';

async function execute(cmd,args,options={}) {
  const child=spawn(cmd,args,{...options,stdio:['ignore','pipe','pipe']});
  let output='';
  child.stdout.on('data',v=>output+=v); child.stderr.on('data',v=>output+=v);
  const code=await new Promise((done,reject)=>{child.on('exit',done);child.on('error',reject);});
  return {code,output};
}

for(const scenario of ['success','candidate-failure','rollback-failure','pull-failure','pull-hang']) {
  test(`deployment readiness: ${scenario}`,async t=>{
    const dir=await mkdtemp(join(tmpdir(),'nabu-deploy-test-'));
    t.after(()=>rm(dir,{recursive:true,force:true}));
    await mkdir(join(dir,'bin'));
    for(const name of ['wait-ready.sh','deploy-local.sh']) {
      await copyFile(resolve('scripts',name),join(dir,name)); await chmod(join(dir,name),0o755);
    }
    for(const [name,value] of Object.entries({'compose.yaml':'previous','compose.next.yaml':'candidate','.env':'candidate-env','.env.previous':'previous-env'})) await writeFile(join(dir,name),value);
    await writeFile(join(dir,'bin/podman-compose'),`#!/usr/bin/env bash
set -euo pipefail
case " $* " in
 *" pull "*)
   if [[ "$OPS_TEST_SCENARIO" == pull-hang ]]; then
     trap '' TERM
     while true; do sleep 10; done
   fi
   [[ "$OPS_TEST_SCENARIO" != pull-failure ]] ;;
 *" up "*) cp "$2" "$OPS_TEST_DIR/running" ;;
 *) exit 9 ;;
esac
`,{mode:0o755});
    const server=createServer(async(_req,res)=>{
      let running=''; try{running=await readFile(join(dir,'running'),'utf8');}catch{}
      const healthy=scenario==='success' || (scenario==='candidate-failure' && running==='previous');
      res.writeHead(healthy?200:503); res.end();
    });
    await new Promise(done=>server.listen(0,'127.0.0.1',done));
    t.after(()=>new Promise(done=>server.close(done)));
    const start=performance.now();
    const result=await execute('bash',[join(dir,'deploy-local.sh')],{cwd:dir,env:{...process.env,
      PATH:join(dir,'bin')+':'+process.env.PATH,OPS_TEST_DIR:dir,OPS_TEST_SCENARIO:scenario,
      DEPLOY_READY_URL:`http://127.0.0.1:${server.address().port}/ready`,DEPLOY_READY_TIMEOUT:'1',
      DEPLOY_PULL_TIMEOUT:'1',DEPLOY_START_TIMEOUT:'1',DEPLOY_KILL_GRACE:'1'}});
    assert.equal(result.code,scenario==='success'?0:scenario==='rollback-failure'?2:1,result.output);
    assert.equal(await readFile(join(dir,'compose.yaml'),'utf8'),scenario==='success'?'candidate':'previous');
    assert.equal(await readFile(join(dir,'.env'),'utf8'),scenario==='success'?'candidate-env':'previous-env');
    if(scenario==='pull-failure' || scenario==='pull-hang') await assert.rejects(readFile(join(dir,'running')),{code:'ENOENT'});
    if(scenario==='pull-hang') assert.ok(performance.now()-start<5000,'TERM-ignoring compose escaped kill escalation');
  });
}

test('readiness deadline bounds an unresponsive HTTP peer',async t=>{
  const server=createServer(()=>{});
  await new Promise(done=>server.listen(0,'127.0.0.1',done));
  t.after(()=>{server.closeAllConnections();return new Promise(done=>server.close(done));});
  const start=performance.now();
  const result=await execute('bash',['scripts/wait-ready.sh',`http://127.0.0.1:${server.address().port}/ready`,'1']);
  assert.equal(result.code,1);
  assert.ok(performance.now()-start<5000,'unresponsive peer exceeded the bounded probe deadline');
});
