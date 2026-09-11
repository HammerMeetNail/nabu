import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {mkdtemp,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

test('recovery boundaries, cancellation, completeness, freshness and alert receipt', {timeout:30000}, async t=>{
  const cache=await mkdtemp(join(tmpdir(),'nabu-recovery-python-'));
  t.after(()=>rm(cache,{recursive:true,force:true}));
  const child=spawn('python3',['tests/ops/recovery_test.py','-v'],{
    env:{...process.env,PYTHONPYCACHEPREFIX:cache},stdio:['ignore','pipe','pipe'],signal:t.signal});
  let output='';
  child.stdout.on('data',chunk=>output+=chunk);
  child.stderr.on('data',chunk=>output+=chunk);
  const code=await new Promise((resolve,reject)=>{child.on('error',reject);child.on('close',resolve);});
  assert.equal(code,0,output);
  assert.match(output,/Ran 13 tests/);
  assert.match(output,/\nOK\n/);
});
