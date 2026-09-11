#!/usr/bin/env python3
"""Opt-in real recovery integration drill; never contacts production or R2."""
import argparse
import contextlib
import gzip
import json
import os
from pathlib import Path
import secrets
import signal
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from types import SimpleNamespace

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
import recovery


def run(args, *, env=None, timeout=180):
    with subprocess.Popen(args, cwd=ROOT, env=env, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, start_new_session=True) as process:
        try:
            output, errors = process.communicate(timeout=timeout)
        except BaseException:
            try:
                os.killpg(process.pid, signal.SIGTERM)
                process.communicate(timeout=30)
            except (ProcessLookupError, subprocess.TimeoutExpired):
                with contextlib.suppress(ProcessLookupError):
                    os.killpg(process.pid, signal.SIGKILL)
                process.communicate()
            raise
        return subprocess.CompletedProcess(args, process.returncode, output, errors)


def cleanup(report):
    if 'retained_directory' not in report:
        return
    owned = object.__new__(recovery.IsolatedRecovery)
    owned.keep = False
    owned.token = report['run']
    owned.work = Path(report['retained_directory'])
    owned.network = report['retained_network']
    owned.containers = report['retained_containers']
    owned.report = report
    owned.close()
    if report.get('cleanup') != 'complete':
        raise recovery.RecoveryError('integration fixture cleanup incomplete')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--image', required=True)
    parser.add_argument('--app-image', required=True)
    parser.add_argument('--report', required=True)
    args = parser.parse_args()
    reports = []
    result = {'status': 'failed', 'scope': 'synthetic local recovery integration'}
    try:
        wal = recovery.wal_drill(SimpleNamespace(image=args.image, app_image=args.app_image, keep=True))
        reports.append(wal)
        if wal['status'] != 'passed':
            raise recovery.RecoveryError('WAL drill failed')
        result['wal'] = {key:value for key,value in wal.items() if not key.startswith('retained_')}
        with tempfile.TemporaryDirectory(prefix='nabu-logical-integration-') as directory:
            work = Path(directory)
            key = work / 'key'
            recovery.private_file(key, secrets.token_urlsafe(48) + '\n')
            gnupg = work / 'gnupg'
            gnupg.mkdir(mode=0o700)
            stub = work / 'bin'
            stub.mkdir()
            rclone = stub / 'rclone'
            rclone.write_text('#!/bin/sh\nset -eu\n[ "$1" = copyto ]\ncp -- "$2" "$NABU_DRILL_BACKUP_DIR/$(basename "$2")"\n')
            rclone.chmod(0o700)
            backups = work / 'backups'
            backups.mkdir(mode=0o700)
            state = work / 'state'
            state.mkdir(mode=0o700)
            source = wal['retained_network'] + '-source-db'
            env = {name:os.environ[name] for name in ('PATH','HOME','XDG_RUNTIME_DIR') if name in os.environ}
            env.update(PATH=str(stub)+':'+env['PATH'], ENV_FILE='/dev/null',
                DB_PASSWORD='synthetic-local-socket-only', BACKUP_ENCRYPTION_KEY=key.read_text().strip(),
                POSTGRES_CONTAINER=source, BACKUP_STATUS_DIR=str(state), BACKUP_REMOTE='local-fixture:backup',
                NABU_DRILL_BACKUP_DIR=str(backups), RCLONE_CONFIG=str(work/'unused.conf'))
            backup = run(['bash', 'scripts/backup.sh'], env=env)
            if backup.returncode:
                raise recovery.RecoveryError('actual encrypted logical backup failed')
            files = list(backups.glob('*.sql.gz.gpg'))
            if len(files) != 1:
                raise recovery.RecoveryError('logical backup fixture inventory failed')

            def restore(file, label, key_path=key, keep=False):
                report = recovery.restore_dump(SimpleNamespace(backup=str(file),remote=None,key_file=str(key_path),
                    image=args.image,app_image=args.app_image,keep=keep))
                report['completed_at'] = time.time()
                if 'retained_directory' in report:
                    reports.append(report)
                if report.get('stage') == 'recovery interrupted':
                    raise recovery.RecoveryError('integration interrupted')
                return (0 if report['status']=='passed' else 1), report

            code, logical = restore(files[0], 'valid')
            if code or logical['status'] != 'passed' or logical.get('cleanup') != 'complete':
                raise recovery.RecoveryError('valid logical restore failed')
            result['logical'] = logical
            bad_inputs = {
                'empty': b'',
                'incomplete': b'-- PostgreSQL database dump\nCREATE TABLE users(id integer);\n',
                'private-error': b"-- PostgreSQL database dump\nINSERT INTO missing_table VALUES ('private-SQL-recovery-canary');\n-- PostgreSQL database dump complete\n",
            }
            result['logical_rejections'] = {}
            for label, sql in bad_inputs.items():
                path = work / (label + '.sql.gz.gpg')
                encrypted = subprocess.run(['gpg','--no-options','--homedir',str(gnupg),'--symmetric',
                    '--cipher-algo=AES256','--batch','--no-symkey-cache','--pinentry-mode=loopback',
                    '--passphrase-file',str(key),'--output',str(path)], input=gzip.compress(sql),
                    stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=30)
                if encrypted.returncode:
                    raise recovery.RecoveryError('encrypted negative fixture creation failed')
                code, rejected = restore(path, label, keep=True)
                if code == 0 or rejected['status'] != 'failed':
                    raise recovery.RecoveryError('invalid logical backup accepted')
                if any('-app-' in name for name in rejected['retained_containers']):
                    raise recovery.RecoveryError('application started before invalid dump rejection')
                for container in rejected['retained_containers']:
                    logs = recovery.command('private log regression', ['podman','logs',container],check=False)
                    if b'private-SQL-recovery-canary' in logs.stdout + logs.stderr:
                        raise recovery.RecoveryError('clone database logged private SQL')
                cleanup(rejected)
                result['logical_rejections'][label] = True
            wrong = work/'wrong-key'
            recovery.private_file(wrong, secrets.token_urlsafe(48)+'\n')
            code, rejected = restore(files[0], 'wrong-key', key_path=wrong)
            if code == 0 or rejected.get('cleanup') != 'complete':
                raise recovery.RecoveryError('wrong-key logical restore not rejected and cleaned')
            result['logical_rejections']['wrong-key'] = True

            received = []
            receiver_status = [204]
            class Receiver(BaseHTTPRequestHandler):
                def do_POST(self):
                    received.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
                    self.send_response(receiver_status[0])
                    self.end_headers()
                def log_message(self, *args):
                    pass
            server = ThreadingHTTPServer(('127.0.0.1',0),Receiver)
            worker = threading.Thread(target=server.serve_forever)
            worker.start()
            try:
                health_env = {**env, 'RECOVERY_POSTGRES_CONTAINER':source, 'RECOVERY_STATUS_DIR':str(state),
                    'RECOVERY_HEARTBEAT_URL':'http://127.0.0.1:'+str(server.server_port),
                    'PYTHONPYCACHEPREFIX':str(work/'pycache')}
                latest = state/'restore-integration.json'
                for healthy, expected in ((False,1),(True,0)):
                    recovery.private_file(latest, json.dumps(logical if healthy else {**logical,'status':'failed'}))
                    process = run([sys.executable,'scripts/recovery-status.py','check'],env=health_env)
                    if process.returncode != expected or received[-1]['status'] != ('passed' if healthy else 'failed'):
                        raise recovery.RecoveryError('restore outcome did not control real heartbeat')
                    latest.unlink()
                recovery.private_file(latest,json.dumps(logical))
                receiver_status[0] = 500
                failed = run([sys.executable,'scripts/recovery-status.py','check'],env=health_env)
                if failed.returncode != 1 or json.loads(failed.stdout).get('alert_delivery') != 'failed':
                    raise recovery.RecoveryError('heartbeat rejection did not fail the check')
                result['alert_failure_recovery_and_delivery_rejection'] = True
            finally:
                server.shutdown()
                server.server_close()
                worker.join(timeout=5)
                if worker.is_alive():
                    raise recovery.RecoveryError('heartbeat fixture did not stop')
        result['status'] = 'passed'
    except (recovery.RecoveryError, OSError, ValueError, KeyError, subprocess.TimeoutExpired) as error:
        if isinstance(error,recovery.RecoveryError) and error.report:
            reports.append(error.report)
        stage = str(error) if isinstance(error,recovery.RecoveryError) else 'invalid integration input or subprocess deadline'
        result.update(status='failed',stage=stage)
    finally:
        incomplete = []
        for report in reversed(reports):
            if Path(report.get('retained_directory','/nonexistent')).exists():
                try:
                    cleanup(report)
                except (recovery.RecoveryError, OSError, ValueError):
                    incomplete.append(report)
        if incomplete:
            result.update(status='failed',cleanup='incomplete',retained_runs=incomplete)
        else:
            result['cleanup'] = 'complete'
    recovery.private_file(args.report, json.dumps(result,indent=2)+'\n')
    print(json.dumps(result))
    return 0 if result['status']=='passed' else 1


if __name__=='__main__':
    signal.signal(signal.SIGTERM,recovery.interrupted)
    signal.signal(signal.SIGINT,recovery.interrupted)
    raise SystemExit(main())
