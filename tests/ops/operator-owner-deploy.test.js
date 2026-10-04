import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {mkdtemp,readFile,writeFile,stat,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';

async function execute(cmd,args,options={}) {
  const child=spawn(cmd,args,{...options,stdio:['ignore','pipe','pipe']});
  let output='';
  child.stdout.on('data',v=>output+=v); child.stderr.on('data',v=>output+=v);
  const code=await new Promise((done,reject)=>{child.on('close',done);child.on('error',reject);});
  return {code,output};
}

const workflow=await readFile(resolve('.github/workflows/ci.yaml'),'utf8');
const deployStep=workflow.slice(workflow.indexOf('      - name: Deploy to server'));
assert.ok(deployStep.startsWith('      - name: Deploy to server\n'));
assert.match(deployStep,/OPERATOR_OWNER_USER_ID: \$\{\{ secrets\.OPERATOR_OWNER_USER_ID \}\}/);
const runStart=deployStep.indexOf('        run: |\n');
assert.ok(runStart>=0);
const runLines=[];
for(const line of deployStep.slice(runStart+'        run: |\n'.length).split('\n')) {
  if(line && !line.startsWith('          ')) break;
  runLines.push(line.startsWith('          ')?line.slice(10):'');
}
const run=runLines.join('\n');
const preflight=run.slice(0,run.indexOf('mkdir -p ~/.ssh'));
assert.ok(preflight.startsWith("python3 - << 'PY'\n"));
const remote=run.slice(run.indexOf('ssh ssh.yearofbingo.com << EOF\n'));
assert.ok(remote.endsWith('EOF\n'));

const maxID='9223372036854775807';
const syntheticEnv={PATH:process.env.PATH,LANG:'C.UTF-8'};

test('operator owner deploy preflight rejects malformed IDs before mutation',async()=>{
  const rejected=['0','-1','+1','01',' 42 ','42\n','٤٢','9223372036854775808',
    '1'.repeat(10000),'1; echo injected','$(echo injected)','1\nENVFILE\necho injected'];
  for(const owner of rejected) {
    const result=await execute('bash',['-e','-o','pipefail','-c',preflight+'printf "server-mutation\\n"'],
      {env:{...syntheticEnv,OPERATOR_OWNER_USER_ID:owner}});
    assert.equal(result.code,1,`accepted ${JSON.stringify(owner)}`);
    assert.equal(result.output.trim(),'OPERATOR_OWNER_USER_ID must be a positive 64-bit user ID');
    assert.doesNotMatch(result.output,/server-mutation/);
  }
});

async function composeCommand() {
  const docker=await execute('docker',['compose','version']).catch(()=>({code:1}));
  if(docker.code===0) return ['docker',['compose']];
  const podman=await execute('podman-compose',['version']).catch(()=>({code:1}));
  assert.equal(podman.code,0,'docker compose or podman-compose is required for deploy contract tests');
  return ['podman-compose',[]];
}

test('operator owner reaches the remote env file and Compose, including clearing an old owner',async t=>{
  const compose=await composeCommand();
  const composeFile=resolve('compose.server.yaml');
  const composeSource=await readFile(composeFile,'utf8');
  assert.equal((composeSource.match(/^\s+- OPERATOR_OWNER_USER_ID=/gm)||[]).length,1);

  for(const [name,owner,previous] of [
    ['unset',undefined,undefined],['clear','', '42'],['ordinary','42',undefined],['max',maxID,undefined],
  ]) {
    const dir=await mkdtemp(join(tmpdir(),'nabu-operator-deploy-'));
    t.after(()=>rm(dir,{recursive:true,force:true}));
    if(previous!==undefined) await writeFile(join(dir,'.env'),`OPERATOR_OWNER_USER_ID=${previous}\n`);
    const remotePath=join(dir,'remote.sh');
    const env={...syntheticEnv,PROBE_REMOTE:remotePath};
    if(owner!==undefined) env.OPERATOR_OWNER_USER_ID=owner;
    const capture=await execute('bash',['-e','-o','pipefail','-c',
      preflight+'ssh() { cat > "$PROBE_REMOTE"; }\n'+remote],{cwd:dir,env});
    assert.equal(capture.code,0,`${name}: ${capture.output}`);
    const captured=await readFile(remotePath,'utf8');
    const writerStart=captured.indexOf('if [ -f .env ]; then cp .env .env.previous; chmod 600 .env.previous; fi');
    const writerEnd=captured.indexOf('chmod 600 .env',captured.indexOf('ENVFILE\n',writerStart));
    assert.ok(writerStart>=0 && writerEnd>writerStart,`${name}: missing env writer`);
    const writer=captured.slice(writerStart,writerEnd+'chmod 600 .env'.length)+'\n';
    const written=await execute('bash',['-e','-o','pipefail','-c',writer],{cwd:dir,env:syntheticEnv});
    assert.equal(written.code,0,`${name}: ${written.output}`);
    const envFile=await readFile(join(dir,'.env'),'utf8');
    assert.equal(envFile.split('\n').filter(line=>line.startsWith('OPERATOR_OWNER_USER_ID=')).length,1);
    assert.ok(envFile.includes(`\nOPERATOR_OWNER_USER_ID=${owner??''}\n`),name);
    assert.equal((await stat(join(dir,'.env'))).mode&0o777,0o600);
    if(previous!==undefined) {
      assert.match(await readFile(join(dir,'.env.previous'),'utf8'),/^OPERATOR_OWNER_USER_ID=42\n$/);
      assert.equal((await stat(join(dir,'.env.previous'))).mode&0o777,0o600);
    }
    const [cmd,prefix]=compose;
    const rendered=await execute(cmd,[...prefix,'--env-file',join(dir,'.env'),'-f',composeFile,'config'],
      {cwd:dir,env:syntheticEnv});
    assert.equal(rendered.code,0,`${name}: ${rendered.output}`);
    const matches=[...rendered.output.matchAll(/^\s+OPERATOR_OWNER_USER_ID:\s*['"]?([^'"\n]*)['"]?\s*$/gm)];
    assert.equal(matches.length,1,`${name}: ${rendered.output}`);
    assert.equal(matches[0][1],owner??'',name);
  }
});
