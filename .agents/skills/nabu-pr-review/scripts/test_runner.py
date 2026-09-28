import copy
import importlib.util
import json
import os
from pathlib import Path
import signal
import shutil
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

SKILL = Path(__file__).resolve().parents[1]
RUNNER = SKILL/'scripts/run_review.py'
REPO = None
BASE = HEAD = None
spec = importlib.util.spec_from_file_location('runner', RUNNER)
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
REF = {'path':'AGENTS.md','line':1,'revision':'head','fact':'causal source'}
CANDIDATE = dict(id='behavior-1',category='product',title='fixture',priority='P2',trigger='input',expected='contract',actual='result',base_comparison='new path',evidence_kind='static',trace=[REF],validation_limit='static only')
RECORD = dict(coverage=[dict(path=REF['path'],scenario='input to result',result='candidate',candidate_ids=['behavior-1'],evidence=[REF])],candidates=[CANDIDATE],checks_run=[])
FINDING = dict(title='fixture',priority='P2',category='product',body='cause and impact',location=REF)
DECISION = dict(id='behavior-1',verdict='confirmed',reason='evidence',trace=[REF],finding=FINDING)
AUDIT = dict(decisions=[DECISION],audit_questions=[],checks_run=[])

class Protocol(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        global REPO, BASE, HEAD
        cls.root = tempfile.TemporaryDirectory(prefix='nabu-review-protocol-')
        cls.origin = Path(subprocess.check_output(['git','-C',str(SKILL),'rev-parse','--show-toplevel'],text=True).strip())
        REPO = Path(cls.root.name)/'review'
        HEAD = BASE = subprocess.check_output(['git','-C',str(cls.origin),'rev-parse','HEAD'],text=True).strip()
        subprocess.run(['git','-C',str(cls.origin),'worktree','add','--detach',str(REPO),HEAD],check=True,capture_output=True)

    @classmethod
    def tearDownClass(cls):
        subprocess.run(['git','-C',str(cls.origin),'worktree','remove',str(REPO)],check=True,capture_output=True)
        cls.root.cleanup()

    def setUp(self): self.source=m.Source(REPO,BASE,HEAD)
    def test_valid_record_and_derived_verdict(self):
        self.assertEqual(m.validate_pass(RECORD,'behavior',self.source),{'behavior-1'})
        m.validate_audit(AUDIT,{'behavior-1'},self.source)
        self.assertIn('classified 1 findings',m.render(AUDIT,{'behavior':RECORD},BASE,HEAD))
    def test_missing_candidate_rejected(self):
        with self.assertRaises(ValueError):m.validate_audit(dict(decisions=[],audit_questions=[],checks_run=[]),{'behavior-1'},self.source)
    def test_coverage_cannot_hide_a_candidate(self):
        r=copy.deepcopy(RECORD);r['candidates']=[]
        with self.assertRaises(ValueError):m.validate_pass(r,'behavior',self.source)
        r['coverage'][0]['candidate_ids']=[]
        with self.assertRaises(ValueError):m.validate_pass(r,'behavior',self.source)
    def test_duplicate_decision_rejected(self):
        a=copy.deepcopy(AUDIT);a['decisions']*=2
        with self.assertRaises(ValueError):m.validate_audit(a,{'behavior-1'},self.source)
    def test_dismissal_cannot_retain_finding(self):
        a=copy.deepcopy(AUDIT);a['decisions'][0]['verdict']='dismissed'
        with self.assertRaises(ValueError):m.validate_audit(a,{'behavior-1'},self.source)
    def test_invalid_source_rejected(self):
        for updates in [{'line':1000000},{'path':'../AGENTS.md'},{'line':True}]:
            with self.subTest(updates=updates),self.assertRaises(ValueError):self.source.reference(dict(REF,**updates))
    def test_multiple_conclusions_rejected(self):
        with self.assertRaises(ValueError):m.parse_record('{}\n{}')
        self.assertEqual(m.parse_record('Some introductory text.\n```json\n{"a":1}\n```'),{'a':1})
        with self.assertRaises(ValueError):m.parse_record('```json\n{}\n```\n```json\n{}\n```')
    def test_fenced_source_is_not_a_second_review_record(self):
        self.assertEqual(m.parse_record('```js\nreturn `<li>${name}</li>`;\n```\n```json\n{"a":1}\n```'),{'a':1})
        with self.assertRaises(ValueError):m.parse_record('```\n{}\n```\n```json\n{}\n```')
        with self.assertRaises(ValueError):m.parse_record('```js\n{}\n```\n```json\n{}\n```')
        with self.assertRaises(ValueError):m.parse_record('```json\n{broken}\n```\n```json\n{}\n```')
        with self.assertRaises(ValueError):m.parse_record('```\n{broken}\n```\n```json\n{}\n```')
        with self.assertRaises(ValueError):m.parse_record('```json\n[]\n```')
        with self.assertRaises(ValueError):m.parse_record('```json\n{}\n```\n{"another":"record"}')
    def test_incomplete_assistant_turn_rejected(self):
        assistant=dict(type='assistant',model=dict(providerID='fixture',id='static'),finish='stop',time={'completed':1})
        session=dict(info={'tokens':{}},messages=[assistant])
        self.assertEqual(m.session_runtime(session),(['fixture/static'],{}))
        for updates in [{'finish':'length'},{'finish':'tool-calls'},{'time':{}},{'error':{'message':'failed'}}]:
            with self.subTest(updates=updates),self.assertRaises(RuntimeError):
                m.session_runtime(dict(session,messages=[dict(assistant,**updates)]))
    def test_old_cli_rejected_before_inference(self):
        with tempfile.TemporaryDirectory(prefix='nabu-cli-check-') as directory:
            fake=Path(directory)/'opencode'
            marker=Path(directory)/'model-started'
            fake.write_text('#!/usr/bin/env python3\nimport sys\nfrom pathlib import Path\nif sys.argv[1:]==["session","--help"]:print("opencode session list\\nopencode session delete")\nelse:Path('+repr(str(marker))+').touch()\n')
            fake.chmod(0o700)
            with self.assertRaisesRegex(RuntimeError,'requires the OpenCode2'):
                m.run_model(str(fake),REPO,Path(directory)/'out','behavior','prompt','body',None,10)
            self.assertFalse(marker.exists())
    def test_omission_search_is_unprimed_and_candidates_reach_audit(self):
        seen = []
        def model(cli, repo, out, name, prompt, *args):
            seen.append(name)
            if name == 'audit':
                records = json.loads(prompt.split('Candidate records:\n', 1)[1])
                self.assertEqual(set(records), {'omissions', 'behavior'})
                decisions = []
                for key, record in records.items():
                    self.assertEqual(record['candidates'][0]['id'], key+'-1')
                    d = copy.deepcopy(DECISION)
                    d['id'] = key+'-1'
                    decisions.append(d)
                return dict(decisions=decisions, audit_questions=[], checks_run=[])
            self.assertNotIn('PRIVATE_CANDIDATE_SENTINEL', prompt)
            r = copy.deepcopy(RECORD)
            r['candidates'][0].update(id=name+'-1', actual='PRIVATE_CANDIDATE_SENTINEL')
            r['coverage'][0]['candidate_ids'] = [name+'-1']
            return r
        with tempfile.TemporaryDirectory(prefix='nabu-omission-routing-') as root:
            out = Path(root)/'out'
            argv = ['run_review.py','--repo',str(REPO),'--base',BASE,'--head',HEAD,
                    '--intent','bounded protocol fixture','--passes','behavior','--out',str(out)]
            with patch.object(sys, 'argv', argv), patch.object(m, 'run_model', side_effect=model):
                self.assertEqual(m.main(), 0)
            self.assertEqual(seen, ['omissions', 'behavior', 'audit'])
            manifest = json.loads((out/'manifest.json').read_text())
            self.assertEqual(manifest['passes'], ['omissions', 'behavior'])
            self.assertEqual(manifest['confirmed'], 2)
            self.assertIn('omissions-1', (out/'review.md').read_text())

    def test_resume_rejects_changed_discovery_guidance(self):
        with tempfile.TemporaryDirectory(prefix='nabu-guidance-resume-') as root:
            root = Path(root)
            skill = root/'skill'
            shutil.copytree(SKILL, skill, ignore=shutil.ignore_patterns('__pycache__', '*.pyc'))
            out = root/'out'
            argv = [sys.executable,str(skill/'scripts/run_review.py'),
                    '--repo',str(REPO),'--base',BASE,'--head',HEAD,
                    '--intent','guidance resume fixture','--passes','behavior',
                    '--out',str(out),'--prepare-only']
            subprocess.run(argv, check=True, capture_output=True, text=True)
            original = (out/'manifest.json').read_bytes()
            guidance = skill/'references/discovery.md'
            guidance.write_text(guidance.read_text()+'\nChanged guidance fixture.\n')
            result = subprocess.run(argv+['--resume'], capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('Resume requires unchanged skill guidance', result.stderr)
            self.assertEqual((out/'manifest.json').read_bytes(), original)

    def test_failed_omission_pass_cannot_produce_completed_review(self):
        with tempfile.TemporaryDirectory(prefix='nabu-omission-failure-') as root:
            out = Path(root)/'out'
            argv = ['run_review.py','--repo',str(REPO),'--base',BASE,'--head',HEAD,
                    '--intent','bounded protocol fixture','--passes','behavior','--out',str(out)]
            with patch.object(sys, 'argv', argv), patch.object(m, 'run_model', side_effect=RuntimeError('omission failed')) as run:
                with self.assertRaisesRegex(RuntimeError, 'omission failed'):
                    m.main()
            self.assertEqual(run.call_count, 1)
            self.assertEqual(run.call_args.args[3], 'omissions')
            self.assertEqual(json.loads((out/'manifest.json').read_text())['status'], 'incomplete')
            self.assertFalse((out/'review.md').exists())

    def test_process_completion_and_termination(self):
        with tempfile.TemporaryDirectory(prefix='nabu-runner-test-') as directory:
            root=Path(directory)
            fixture=root/'fixture.json'
            empty=copy.deepcopy(RECORD);empty['candidates']=[]
            empty['coverage'][0].update(result='no_issue_found',candidate_ids=[])
            fixture.write_text(json.dumps(dict(body=(SKILL/'SKILL.md').read_text().split('---',2)[2].strip(),record=empty)))
            fake=root/'fake-cli'
            fake.write_text('''#!/usr/bin/env python3
import sys,json,time
from pathlib import Path
f=json.loads((Path(__file__).parent/'fixture.json').read_text())
if sys.argv[1:]==['session','--help']:
 print('  export    Export session data as JSON')
elif sys.argv[1:3]==['session','export']:
 print(json.dumps({'info':{'tokens':{}},'messages':[{'type':'assistant','model':{'providerID':'fixture','id':'static'},'finish':'stop','time':{'completed':1}}]}))
elif sys.argv[1]=='run':
 task=Path(sys.argv[sys.argv.index('--file')+1])
 if (Path(__file__).parent/'hold').exists():time.sleep(120)
 rec={'decisions':[],'audit_questions':[],'checks_run':[]} if task.parent.name=='audit' else f['record']
 print(json.dumps({'type':'tool_use','sessionID':'fixture','part':{'tool':'skill','state':{'output':f['body']}}}))
 print(json.dumps({'type':'text','sessionID':'fixture','part':{'text':json.dumps(rec)}}))
''')
            fake.chmod(0o700)
            argv=['python3',str(RUNNER),'--repo',str(REPO),'--base',BASE,'--head',HEAD,'--intent','protocol fixture','--passes','behavior','--cli',str(fake),'--expect-model','fixture/static']
            subprocess.run(argv+['--out',str(root/'complete')],check=True,capture_output=True,text=True)
            done=json.loads((root/'complete/manifest.json').read_text())
            self.assertEqual(done['status'],'completed');self.assertEqual(done['confirmed'],0)
            self.assertTrue((root/'complete/review.md').exists())
            original=(root/'complete/behavior/events.jsonl').read_bytes()
            subprocess.run(argv+['--out',str(root/'complete'),'--resume'],check=True,capture_output=True,text=True)
            self.assertEqual(original,(root/'complete/behavior/events.jsonl').read_bytes())
            exit_path=root/'complete/behavior/exit.json'
            terminal=json.loads(exit_path.read_text());terminal['status']='interrupted';terminal['exit_code']=0
            exit_path.write_text(json.dumps(terminal))
            refused=subprocess.run(argv+['--out',str(root/'complete'),'--resume'],capture_output=True,text=True)
            self.assertNotEqual(refused.returncode,0)
            self.assertIn('cannot resume unfinished/failed model process',refused.stderr)
            (root/'hold').touch()
            with (root/'signal.log').open('w') as log:
                proc=subprocess.Popen(argv+['--out',str(root/'interrupted')],stdout=log,stderr=log)
                try:
                    pidfile=root/'interrupted/omissions/pid'
                    deadline=time.monotonic()+10
                    while not pidfile.exists() and time.monotonic()<deadline:time.sleep(.02)
                    self.assertTrue(pidfile.exists())
                    child=int(pidfile.read_text())
                    proc.send_signal(signal.SIGTERM)
                    self.assertNotEqual(proc.wait(timeout=15),0)
                    with self.assertRaises(ProcessLookupError):os.kill(child,0)
                    self.assertEqual(json.loads((root/'interrupted/manifest.json').read_text())['status'],'incomplete')
                    self.assertFalse((root/'interrupted/review.md').exists())
                finally:
                    if proc.poll() is None:
                        proc.terminate()
                        try:
                            proc.wait(timeout=15)
                        except subprocess.TimeoutExpired:
                            proc.kill()
                            proc.wait()

if __name__=='__main__':unittest.main(verbosity=2)
