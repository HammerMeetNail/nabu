#!/usr/bin/env python3
"""Bounded pgBackRest backups and sanitized recovery health/deadman reports."""
import argparse
import datetime as dt
import json
import os
from pathlib import Path
import re
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

from recovery import RecoveryError, command, json_result


def required(name):
    value = os.environ.get(name, "")
    if not value:
        raise RecoveryError("recovery configuration incomplete: " + name)
    return value


def container():
    value = required("RECOVERY_POSTGRES_CONTAINER")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]+", value):
        raise RecoveryError("invalid recovery container name")
    return value


def pgbackrest(*arguments, deadline=150):
    return ["podman", "exec", "--user=postgres", container(), "timeout", "-s", "TERM", "-k", "5", str(deadline), "pgbackrest", "--stanza=nabu",
            "--log-level-console=off", "--log-level-file=off", *arguments]


def repository_health(info, now):
    if len(info) != 1 or info[0].get("name") != "nabu" or info[0].get("cipher") != "aes-256-cbc" or info[0].get("status", {}).get("code") != 0:
        raise RecoveryError("encrypted repository is not healthy")
    backups = [backup for backup in info[0].get("backup", []) if not backup.get("error")]
    full = [backup for backup in backups if backup.get("type") == "full"]
    if not backups or not full:
        raise RecoveryError("no successful full backup")
    newest = max(backup["timestamp"]["stop"] for backup in backups)
    newest_full = max(backup["timestamp"]["stop"] for backup in full)
    age, full_age = now - newest, now - newest_full
    if age < -300 or full_age < -300:
        raise RecoveryError("backup timestamps are in the future")
    if age > 26 * 3600 or full_age > 8 * 86400:
        raise RecoveryError("backup freshness objective exceeded")
    return {"backup_age_seconds": round(max(0, age)), "full_backup_age_seconds": round(max(0, full_age))}


def archive_health():
    # No row values, relation names, secrets, or provider paths leave this check.
    sql = """SELECT count(*), coalesce(ceil(max(extract(epoch FROM clock_timestamp() -
        (pg_stat_file('pg_wal/archive_status/' || name)).modification))), 0)
        FROM pg_ls_dir('pg_wal/archive_status') AS name WHERE name LIKE '%.ready';"""
    result = command("archive queue", ["podman", "exec", "-i", "--user=postgres", container(),
        "psql", "-X", "-U", "nabu", "-d", "nabu", "-At", "-v", "ON_ERROR_STOP=1"], data=sql.encode(), timeout=10)
    try:
        count, age = [int(value) for value in result.stdout.decode().strip().split("|")]
    except (ValueError, UnicodeError):
        raise RecoveryError("invalid archive queue result") from None
    if age > 180:
        raise RecoveryError("WAL archive queue exceeds three minutes")
    disk = command("database disk", ["podman", "exec", "--user=postgres", container(),
        "sh", "-c", 'df -Pk "$PGDATA"'], timeout=10)
    try:
        used = int(disk.stdout.decode().splitlines()[-1].split()[-2].rstrip("%"))
    except (ValueError, UnicodeError, IndexError):
        raise RecoveryError("invalid database disk result") from None
    if used >= 85:
        raise RecoveryError("database disk use exceeds 85 percent")
    return {"archive_queue_segments": count, "oldest_ready_seconds": age, "database_disk_percent": used}


def restore_health(directory, now):
    reports = []
    for path in Path(directory).glob("restore-*.json"):
        if path.is_symlink() or path.stat().st_size > 65536:
            raise RecoveryError("invalid restore verification report")
        try:
            report = json.loads(path.read_text())
            completed = float(report["completed_at"])
        except (OSError, ValueError, KeyError, TypeError):
            raise RecoveryError("invalid restore verification report") from None
        reports.append((completed, report))
    if not reports:
        raise RecoveryError("no isolated restore verification report")
    completed, latest = max(reports, key=lambda item: item[0])
    age = now - completed
    if not -300 <= age <= 26 * 3600:
        raise RecoveryError("isolated restore verification is stale")
    required_checks = ("two_household_isolation", "constraints_validated", "rollback_write_verified", "fixture_cleanup_verified")
    if latest.get("status") != "passed" or latest.get("cleanup") != "complete" or not all(latest.get(check) is True for check in required_checks):
        raise RecoveryError("latest isolated restore verification failed")
    return {"restore_verification_age_seconds": round(max(0, age))}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def send_heartbeat(url, report):
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme != "https" and not (parsed.scheme == "http" and parsed.hostname in ("127.0.0.1", "::1", "localhost")):
        raise RecoveryError("heartbeat requires HTTPS (loopback permitted for local testing)")
    body = json.dumps(report).encode()
    request = urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.build_opener(NoRedirect()).open(request, timeout=10) as response:
            if not 200 <= response.status < 300:
                raise RecoveryError("recovery heartbeat rejected")
    except (urllib.error.URLError, TimeoutError, OSError):
        raise RecoveryError("recovery heartbeat failed") from None


def store_status(report):
    directory = Path(os.environ.get("RECOVERY_STATUS_DIR", "/opt/nabu/state"))
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    fd, path = tempfile.mkstemp(prefix=".recovery-", dir=directory)
    try:
        with os.fdopen(fd, "w") as stream:
            json.dump(report, stream)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(path, directory / "recovery-status.json")
    finally:
        if os.path.exists(path):
            os.unlink(path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("check", "full", "diff"))
    parser.add_argument("--no-notify", action="store_true", help="local inspection only; no heartbeat delivery")
    args = parser.parse_args()
    started = time.monotonic()
    report = {"service": "nabu-recovery", "status": "failed", "checked_at": dt.datetime.now(dt.timezone.utc).isoformat()}
    try:
        if args.action != "check":
            command("pgBackRest backup", pgbackrest("--type=" + args.action, "backup", deadline=3300), timeout=3330)
        info = json_result("repository inventory", pgbackrest("--output=json", "info", deadline=20), timeout=30)
        report.update(repository_health(info, time.time()))
        report.update(restore_health(os.environ.get("RECOVERY_STATUS_DIR", "/opt/nabu/state"), time.time()))
        report.update(archive_health())
        # End-to-end repository acknowledgement; no claim based on local .ready files.
        command("WAL repository acknowledgement", pgbackrest("check"), timeout=180)
        report.update(status="passed", archive_roundtrip_verified=True)
    except RecoveryError as error:
        report["stage"] = str(error)
    report["elapsed_seconds"] = round(time.monotonic() - started, 3)
    if not args.no_notify:
        try:
            send_heartbeat(required("RECOVERY_HEARTBEAT_URL"), report)
            report["heartbeat_delivered"] = True
        except RecoveryError as error:
            report.update(status="failed", alert_delivery="failed", alert_stage=str(error))
    store_status(report)
    print(json.dumps(report))
    return 0 if report["status"] == "passed" else 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (RecoveryError, OSError, ValueError, KeyError, TypeError):
        print('{"service":"nabu-recovery","status":"failed","stage":"invalid recovery configuration or structured result"}')
        sys.exit(1)
