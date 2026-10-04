#!/usr/bin/env python3
"""Real-Podman regression for the scheduled encrypted logical restore verifier.

Usage: python3 tests/ops/legacy-verifier-volume.py SCRIPT SERVICE
The test uses a local encrypted fixture and a stub R2 transport; PostgreSQL,
container creation, restore and cleanup all use the real Podman daemon.
"""

import gzip
import os
from pathlib import Path
import signal
import shutil
import subprocess
import sys
import tempfile
import time


LABEL = "io.hammermeetnail.backup-verifier"
IMAGE = "docker.io/library/postgres:16-alpine"


def command(*args, env=None, check=True):
    result = subprocess.run(args, env=env, text=True, capture_output=True)
    if check and result.returncode:
        raise RuntimeError(f"{args[0]} failed ({result.returncode}): {result.stderr[-1000:]}")
    return result


def resources(service):
    result = {}
    for kind, args in (
        ("container", ("ps", "-a", "--filter", f"label={LABEL}.service={service}", "--format", "{{.ID}}")),
        ("volume", ("volume", "ls", "--filter", f"label={LABEL}.service={service}", "--format", "{{.Name}}")),
    ):
        result[kind] = set(command("podman", *args).stdout.splitlines())
    return result


def assert_baseline(service, baseline):
    actual = resources(service)
    if actual != baseline:
        raise AssertionError(f"orphaned verifier resources: {actual}")


def fixture(work, passphrase, table, body):
    sql = f"CREATE TABLE users (id integer);\nCREATE TABLE {table} (id integer);\n{body}\n"
    sql += "-- test fixture padding\n" * 70
    source = work / "dump.sql.gz"
    source.write_bytes(gzip.compress(sql.encode()))
    destination = work / "fixture.sql.gz.gpg"
    key_file = work / "key"
    key_file.write_text(passphrase)
    key_file.chmod(0o600)
    command("gpgconf", "--homedir", str(work / "gnupg"), "--launch", "gpg-agent")
    command("gpg", "--batch", "--yes", "--pinentry-mode", "loopback", "--symmetric",
            "--cipher-algo", "AES256", "--passphrase-file", str(key_file),
            "--output", str(destination), str(source), env={**os.environ, "GNUPGHOME": str(work / "gnupg")})
    return destination


def stub_rclone(work):
    bin_dir = work / "bin"
    bin_dir.mkdir()
    podman = bin_dir / "podman"
    podman.write_text("""#!/usr/bin/env bash
if [[ "${1:-}" == volume && "${2:-}" == create && -f "${VERIFY_FAIL_CREATE_ONCE:-/dev/null}" ]]; then
  rm -f "$VERIFY_FAIL_CREATE_ONCE"
  exit 1
fi
if [[ "${1:-}" == volume && "${2:-}" == rm && -f "${VERIFY_FAIL_RM_ONCE:-/dev/null}" ]]; then
  rm -f "$VERIFY_FAIL_RM_ONCE"
  exit 1
fi
exec "$VERIFY_REAL_PODMAN" "$@"
""")
    podman.chmod(0o700)
    executable = bin_dir / "rclone"
    executable.write_text("""#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  ls) printf '2048 fixture.sql.gz.gpg\\n' ;;
  copy)
    if [[ "$2" == r2* ]]; then
      cp "$VERIFY_FIXTURE" "$3/fixture.sql.gz.gpg"
    else
      cp "$2" "$VERIFY_REPORT_DIR/"
    fi ;;
  *) exit 2 ;;
esac
""")
    executable.chmod(0o700)
    if shutil.which("flock") is None:
        flock = bin_dir / "flock"
        flock.write_text("""#!/usr/bin/env python3
import fcntl, sys, time
deadline = time.monotonic() + float(sys.argv[2])
fd = int(sys.argv[3])
while True:
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        break
    except BlockingIOError:
        if time.monotonic() > deadline:
            sys.exit(1)
        time.sleep(0.1)
""")
        flock.chmod(0o700)
    return bin_dir


def invoke(script, env, expected_success, terminate=False, service=None, baseline=None):
    proc = subprocess.Popen(["bash", str(script)], env=env, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, text=True, start_new_session=True)
    try:
        if terminate:
            deadline = time.monotonic() + 90
            while time.monotonic() < deadline:
                current = resources(service)
                if current["container"] - baseline["container"]:
                    os.killpg(proc.pid, signal.SIGTERM)
                    break
                if proc.poll() is not None:
                    raise AssertionError("verifier exited before a container started")
                time.sleep(0.2)
            else:
                raise AssertionError("verifier never started a test container")
        stdout, stderr = proc.communicate(timeout=100)
        if (proc.returncode == 0) != expected_success:
            raise AssertionError(f"unexpected verifier status {proc.returncode}: {stdout[-1500:]} {stderr[-1500:]}")
        if expected_success and "BACKUP VERIFICATION PASSED" not in stdout:
            raise AssertionError("successful verifier did not report a completed restore")
        if not expected_success and "BACKUP VERIFICATION PASSED" in stdout:
            raise AssertionError("failed verifier declared a pass")
    finally:
        if proc.poll() is None:
            os.killpg(proc.pid, signal.SIGTERM)
            proc.communicate(timeout=20)


def concurrent_invocations(script, env, service, baseline):
    first = subprocess.Popen(["bash", str(script)], env=env, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, text=True, start_new_session=True)
    second = None
    try:
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline:
            if resources(service)["container"] - baseline["container"]:
                break
            if first.poll() is not None:
                raise AssertionError("first concurrent verifier exited before starting")
            time.sleep(0.2)
        else:
            raise AssertionError("first concurrent verifier never started")
        second = subprocess.Popen(["bash", str(script)], env=env, stdout=subprocess.PIPE,
                                  stderr=subprocess.PIPE, text=True, start_new_session=True)
        time.sleep(1)
        if len(resources(service)["container"] - baseline["container"]) != 1:
            raise AssertionError("concurrent verifier did not wait for the lock")
        for process in (first, second):
            stdout, stderr = process.communicate(timeout=100)
            if process.returncode or "BACKUP VERIFICATION PASSED" not in stdout:
                raise AssertionError(f"concurrent verifier failed ({process.returncode}): {stderr[-1000:]}")
    finally:
        for process in (first, second):
            if process is not None and process.poll() is None:
                os.killpg(process.pid, signal.SIGTERM)
                process.communicate(timeout=20)


def stale_run(service):
    run = f"stale-{os.getpid()}"
    name = f"{service}-backup-verify-{run}"
    labels = ["--label", f"{LABEL}.service={service}",
              "--label", f"{LABEL}.run={run}"]
    command("podman", "volume", "create", *labels, "--label", f"{LABEL}.kind=volume", f"{name}-data")
    command("podman", "run", "-d", "--name", name, *labels,
            "--label", f"{LABEL}.kind=container",
            "--mount", f"type=volume,source={name}-data,destination=/var/lib/postgresql/data",
            "-e", "POSTGRES_PASSWORD=fixture-only", IMAGE)


def main():
    script = Path(sys.argv[1]).resolve()
    app_service = sys.argv[2]
    table = {"nabu": "chores", "yearofbingo": "bingo_cards"}[app_service]
    command("podman", "image", "exists", IMAGE)
    # Keep GNUPGHOME short enough for macOS Unix-domain agent socket paths.
    with tempfile.TemporaryDirectory(prefix="bv-", dir="/tmp") as directory:
        work = Path(directory)
        service = f"{app_service}-test-{os.getpid()}"
        baseline = resources(service)
        test_script = work / "verify-backup.sh"
        source = script.read_text()
        setup = f"verify_setup {app_service}"
        if source.count(setup) != 1:
            raise AssertionError("expected one literal verifier service name")
        test_script.write_text(source.replace(setup, f"verify_setup {service}"))
        shutil.copyfile(script.parent / "verify-backup-resources.sh", work / "verify-backup-resources.sh")
        (work / "gnupg").mkdir(mode=0o700)
        bin_dir = stub_rclone(work)
        reports = work / "reports"
        reports.mkdir()
        env = {**os.environ, "ENV_FILE": "/dev/null", "BACKUP_ENCRYPTION_KEY": "fixture-key",
               "BACKUP_NOTIFY_VERIFY_SUCCESS": "0", "VERIFY_STATE_DIR": str(work / "state"),
               "VERIFY_FIXTURE": "", "TMPDIR": str(work), "GNUPGHOME": str(work / "gnupg"),
               "VERIFY_REAL_PODMAN": shutil.which("podman"), "VERIFY_REPORT_DIR": str(reports),
               "PATH": str(bin_dir) + os.pathsep + os.environ["PATH"]}
        for body, success, terminate in (
            ("INSERT INTO users VALUES (1);", True, False),
            ("SELECT 1/0;", False, False),
            ("SELECT pg_sleep(30);", False, True),
        ):
            env["VERIFY_FIXTURE"] = str(fixture(work, env["BACKUP_ENCRYPTION_KEY"], table, body))
            invoke(test_script, env, success, terminate, service, baseline)
            assert_baseline(service, baseline)
        env["VERIFY_FIXTURE"] = str(fixture(work, env["BACKUP_ENCRYPTION_KEY"], table, "SELECT 1;"))
        marker = work / "fail-volume-rm-once"
        marker.touch()
        env["VERIFY_FAIL_RM_ONCE"] = str(marker)
        report_count = len(list(reports.iterdir()))
        invoke(test_script, env, False, service=service, baseline=baseline)
        assert_baseline(service, baseline)
        if len(list(reports.iterdir())) != report_count + 1:
            raise AssertionError("cleanup failure did not send an R2 failure report")
        env.pop("VERIFY_FAIL_RM_ONCE")
        marker = work / "fail-volume-create-once"
        marker.touch()
        env["VERIFY_FAIL_CREATE_ONCE"] = str(marker)
        report_count = len(list(reports.iterdir()))
        invoke(test_script, env, False, service=service, baseline=baseline)
        assert_baseline(service, baseline)
        if len(list(reports.iterdir())) != report_count + 1:
            raise AssertionError("setup failure did not send an R2 failure report")
        env.pop("VERIFY_FAIL_CREATE_ONCE")
        stale_run(service)
        env["VERIFY_FIXTURE"] = str(fixture(work, env["BACKUP_ENCRYPTION_KEY"], table, "SELECT 1;"))
        invoke(test_script, env, True, service=service, baseline=baseline)
        assert_baseline(service, baseline)
        invoke(test_script, env, True, service=service, baseline=baseline)
        assert_baseline(service, baseline)
        env["VERIFY_FIXTURE"] = str(fixture(work, env["BACKUP_ENCRYPTION_KEY"], table, "SELECT pg_sleep(6);"))
        concurrent_invocations(test_script, env, service, baseline)
        assert_baseline(service, baseline)
        unowned = f"{service}-backup-verify-unowned-data"
        command("podman", "volume", "create", unowned)
        try:
            env["VERIFY_FIXTURE"] = str(fixture(work, env["BACKUP_ENCRYPTION_KEY"], table, "SELECT 1;"))
            invoke(test_script, env, True, service=service, baseline=baseline)
            command("podman", "volume", "exists", unowned)
            assert_baseline(service, baseline)
        finally:
            command("podman", "volume", "rm", unowned)
    print(f"{app_service}: restore, failure alerts, termination, reconciliation, serial runs and unowned retention passed")


if __name__ == "__main__":
    main()
