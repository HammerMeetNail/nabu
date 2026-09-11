import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import threading
import textwrap
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
import recovery
spec = importlib.util.spec_from_file_location('recovery_status', ROOT / 'scripts/recovery-status.py')
status = importlib.util.module_from_spec(spec)
spec.loader.exec_module(status)
drill_spec = importlib.util.spec_from_file_location('recovery_drill', ROOT / 'tests/ops/recovery-drill.py')
drill = importlib.util.module_from_spec(drill_spec)
drill_spec.loader.exec_module(drill)


class RecoveryTests(unittest.TestCase):
    def test_empty_and_incomplete_sql_never_validate(self):
        for content, expected in [(b'',1), (b'-- PostgreSQL database dump\nSELECT 1;\n',1),
            (b'-- PostgreSQL database dump\nSELECT 1;\n-- PostgreSQL database dump complete\n',0)]:
            with self.subTest(expected=expected, length=len(content)):
                result = subprocess.run([sys.executable, str(ROOT/'scripts/recovery.py'), 'validate-dump'],
                    input=content, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=5)
                self.assertEqual(result.returncode, expected)
                self.assertEqual(result.stdout, content)

    def test_no_application_before_restored_schema_is_validated(self):
        clone = object.__new__(recovery.IsolatedRecovery)
        calls = []
        def missing(table):
            calls.append(table)
            return 'f'
        clone.sql = lambda _database, query: missing(query)
        with self.assertRaisesRegex(recovery.RecoveryError, 'missing required application schema'):
            clone.check_restored_schema('synthetic')
        self.assertEqual(len(calls), 1)

    def test_pipeline_rejects_failure_after_partial_output(self):
        with self.assertRaisesRegex(recovery.RecoveryError, 'pipeline failed'):
            recovery.pipeline('restore', [
                [sys.executable,'-c','import sys; print("partial SQL"); sys.exit(3)'],
                [sys.executable,'-c','import sys; sys.stdin.read()']], timeout=5)

    def test_command_deadline_kills_descendants_and_hides_stderr(self):
        # Descendant retains stdout/stderr after its parent exits. communicate()
        # must kill the whole group rather than wait forever on those pipes.
        code = 'import subprocess,sys; subprocess.Popen([sys.executable,"-c","import time; time.sleep(60)"]); print("private-SQL-canary", file=sys.stderr)'
        start = time.monotonic()
        with self.assertRaisesRegex(recovery.RecoveryError, '^operation: timed out$'):
            recovery.command('operation', [sys.executable,'-c',code], timeout=0.2)
        self.assertLess(time.monotonic()-start, 3)

    def test_cleanup_continues_after_one_failed_owner_lookup(self):
        with tempfile.TemporaryDirectory() as directory:
            clone = object.__new__(recovery.IsolatedRecovery)
            clone.keep = False
            clone.token = 'owned'
            clone.work = Path(directory)
            clone.containers = ['first','second']
            clone.network = 'network'
            clone.report = {'status':'passed'}
            removed = []
            def command(stage, args, **_kwargs):
                if args[1]=='inspect' and args[-1]=='second':
                    raise recovery.RecoveryError('ownership timed out')
                if args[1]=='inspect':
                    return subprocess.CompletedProcess(args,0,b'owned\n',b'')
                if args[1]=='rm':
                    removed.append(args[-1])
                    return subprocess.CompletedProcess(args,0,b'',b'')
                if args[1:3]==['network','inspect']:
                    return subprocess.CompletedProcess(args,0,b'[{"labels":{"nabu.recovery.run":"owned"}}]',b'')
                return subprocess.CompletedProcess(args,0,b'',b'')
            with patch.object(recovery,'command',command):
                clone.close()
            self.assertEqual(removed,['first'])
            self.assertEqual(clone.report['cleanup'],'incomplete')
            self.assertEqual(clone.report['status'],'failed')
            self.assertTrue(clone.work.exists())

    def test_termination_runs_cleanup(self):
        for script, action in (('scripts/recovery.py',['wal-drill']),('tests/ops/recovery-drill.py',[])):
            with tempfile.TemporaryDirectory() as directory:
                fixture=Path(directory)
                executable=fixture/'podman'
                executable.write_text(textwrap.dedent("""    #!/usr/bin/env python3
    import json,os,pathlib,sys,time
    root=pathlib.Path(os.environ['RECOVERY_TEST_DIR'])
    a=sys.argv[1:]
    if a[0]=='run' and '--entrypoint=id' in a:
        print('70')
    elif a[:2]==['network','create']:
        (root/'network').write_text(a[-1]); print(a[-1])
    elif a[:2]==['network','inspect']:
        print(json.dumps([{'labels':{'nabu.recovery.run':a[-1].split('-')[2]}}]))
    elif a[:2]==['network','rm']:
        (root/'network').unlink()
    elif a[0]=='run':
        name=a[a.index('--name')+1]
        (root/'container').write_text(name)
        (root/'scratch').write_text(str(pathlib.Path(a[a.index('--env-file')+1]).parent))
        print(name)
    elif a[0]=='exec':
        (root/'entered').write_text('entered')
        time.sleep(60)
    elif a[0]=='inspect':
        print(a[-1].split('-')[2])
    elif a[0]=='rm':
        (root/'container').unlink()
    else:
        sys.exit(1)
    """))
                executable.chmod(0o700)
                report=fixture/'report.json'
                child=subprocess.Popen([sys.executable,str(ROOT/script),*action,
                    '--image','synthetic-postgres:17','--app-image','synthetic-app:test','--report',str(report)],
                    env={**os.environ,'PATH':str(fixture)+':'+os.environ['PATH'],'RECOVERY_TEST_DIR':str(fixture)},
                    stdout=subprocess.PIPE,stderr=subprocess.PIPE)
                try:
                    deadline=time.monotonic()+5
                    while not (fixture/'entered').exists() and child.poll() is None and time.monotonic()<deadline:
                        time.sleep(0.01)
                    if not (fixture/'entered').exists():
                        output,errors=child.communicate(timeout=1)
                        self.fail(f'database probe did not enter: {output!r} {errors!r}')
                    child.send_signal(signal.SIGTERM)
                    output,errors=child.communicate(timeout=5)
                    self.assertEqual(child.returncode,1,(output,errors))
                    result=json.loads(report.read_text())
                    self.assertEqual(result['status'],'failed')
                    self.assertIn(result['stage'],('recovery interrupted','WAL drill failed'))
                    self.assertEqual(result['cleanup'],'complete')
                    self.assertFalse((fixture/'container').exists())
                    self.assertFalse((fixture/'network').exists())
                    self.assertFalse(Path((fixture/'scratch').read_text()).exists())
                finally:
                    if child.poll() is None:
                        child.kill()
                    child.communicate(timeout=5)

    def test_initialization_cleanup_failure_preserves_resource_report(self):
        with tempfile.TemporaryDirectory() as directory:
            report=Path(directory)/'report.json'
            args=['recovery.py','wal-drill','--image','fixture:17','--app-image','fixture:app','--report',str(report)]
            with patch.object(sys,'argv',args), patch.object(recovery.IsolatedRecovery,'initialize',side_effect=recovery.RecoveryError('initialization failed')), patch.object(recovery,'command',side_effect=recovery.RecoveryError('daemon unavailable')), contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(recovery.main(),1)
            recorded=json.loads(report.read_text())
            self.assertEqual(recorded['cleanup'],'incomplete')
            self.assertEqual(recorded['status'],'failed')
            retained=Path(recorded['retained_directory'])
            self.assertTrue(retained.is_dir())
            retained.rmdir()

    def test_integration_subprocess_timeout_allows_graceful_cleanup(self):
        with tempfile.TemporaryDirectory() as directory:
            marker=Path(directory)/'cleaned'
            code="import signal,sys,time; from pathlib import Path; signal.signal(signal.SIGTERM,lambda *_:(Path(sys.argv[1]).write_text('cleaned'),sys.exit(0))); print('entered',flush=True); time.sleep(60)"
            with self.assertRaises(subprocess.TimeoutExpired):
                drill.run([sys.executable,'-c',code,str(marker)],timeout=1)
            self.assertEqual(marker.read_text(),'cleaned')

    def test_verifier_transition_requires_recent_delivered_health(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            key=root/'key'
            recovery.private_file(key,'synthetic-key')
            configuration=root/'recovery.env'
            configuration.write_text('\n'.join([
                'RECOVERY_POSTGRES_IMAGE=fixture:17','RECOVERY_APP_IMAGE=fixture:app',
                'RECOVERY_BACKUP_REMOTE=fixture:bucket','RECOVERY_KEY_FILE='+str(key),
                'RECOVERY_STATUS_DIR='+str(root),'RECOVERY_HEARTBEAT_URL=http://127.0.0.1:1']))
            report={'status':'passed','heartbeat_delivered':True,'checked_at':recovery.dt.datetime.now(recovery.dt.timezone.utc).isoformat()}
            for changes,code in (({},0),({'heartbeat_delivered':False},1),({'status':'failed'},1),({'checked_at':'2000-01-01T00:00:00+00:00'},1)):
                (root/'recovery-status.json').write_text(json.dumps({**report,**changes}))
                result=subprocess.run(['bash',str(ROOT/'scripts/verify-backup.sh'),'--check-config'],
                    env={**os.environ,'RECOVERY_ENV_FILE':str(configuration)},capture_output=True,timeout=5)
                self.assertEqual(result.returncode,code,result.stderr)

    def test_backup_inventory_ignores_reports_and_stale_dumps(self):
        now = time.time()
        def date(age):
            return recovery.dt.datetime.fromtimestamp(now-age,recovery.dt.timezone.utc).isoformat()
        entries = [
            {'Path':'BACKUP_VERIFICATION_FAILED_20260910.txt','Size':20,'ModTime':date(0)},
            {'Path':'nabu_20260909_120000.sql.gz.gpg','Size':100,'ModTime':date(27*3600)},
            {'Path':'nabu_20260910_120000.sql.gz.gpg','Size':100,'ModTime':date(10)},
            {'Path':'../nabu_20260910_120001.sql.gz.gpg','Size':100,'ModTime':date(0)}]
        with tempfile.TemporaryDirectory() as directory:
            destination = Path(directory)/'backup'
            calls=[]
            def download(_stage,args,**_kwargs):
                calls.append(args)
                destination.write_bytes(b'encrypted')
            with patch.object(recovery,'json_result',return_value=entries), patch.object(recovery,'command',download):
                recovery.latest_dump('r2-fixture:bucket',destination,26)
            self.assertIn('r2-fixture:bucket/nabu_20260910_120000.sql.gz.gpg',calls[0])
            self.assertEqual(destination.stat().st_mode & 0o777,0o600)

    def test_failed_and_stale_restores_block_healthy_heartbeat(self):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'restore-fixture.json'
            report={'status':'passed','cleanup':'complete','completed_at':time.time(),
                    'two_household_isolation':True,'constraints_validated':True,'rollback_write_verified':True,'fixture_cleanup_verified':True}
            path.write_text(json.dumps(report))
            self.assertIn('restore_verification_age_seconds',status.restore_health(directory,time.time()))
            for changes in ({'status':'failed'}, {'completed_at':time.time()-27*3600}, {'cleanup':'incomplete'}):
                with self.subTest(changes=changes):
                    path.write_text(json.dumps({**report,**changes}))
                    with self.assertRaises(recovery.RecoveryError):
                        status.restore_health(directory,time.time())

    def test_real_loopback_receiver_gets_sanitized_failure_and_recovery(self):
        received=[]
        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                received.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
                self.send_response(204)
                self.end_headers()
            def log_message(self,*args):
                pass
        server=ThreadingHTTPServer(('127.0.0.1',0),Handler)
        worker=threading.Thread(target=server.serve_forever)
        worker.start()
        try:
            for value in ('failed','passed'):
                status.send_heartbeat('http://127.0.0.1:'+str(server.server_port),
                    {'service':'nabu-recovery','status':value,'stage':'fixture'})
            self.assertEqual([report['status'] for report in received],['failed','passed'])
        finally:
            server.shutdown()
            server.server_close()
            worker.join(timeout=5)
            self.assertFalse(worker.is_alive())

    def test_expired_physical_backups_and_plain_repository_fail(self):
        now=time.time()
        info=[{'name':'nabu','cipher':'aes-256-cbc','status':{'code':0},
               'backup':[{'type':'full','timestamp':{'stop':now-100},'error':False}]}]
        self.assertEqual(status.repository_health(info,now)['backup_age_seconds'],100)
        for invalid in ('old','plain'):
            broken=json.loads(json.dumps(info))
            if invalid=='old': broken[0]['backup'][0]['timestamp']['stop']=now-27*3600
            else: broken[0]['cipher']='none'
            with self.assertRaises(recovery.RecoveryError):
                status.repository_health(broken,now)


if __name__=='__main__':
    unittest.main()
