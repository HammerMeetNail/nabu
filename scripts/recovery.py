#!/usr/bin/env python3
"""Isolated local PostgreSQL recovery. No live database or environment defaults."""
import argparse
import contextlib
import datetime as dt
import http.cookiejar
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import uuid


class RecoveryError(Exception):
    def __init__(self, message, report=None):
        super().__init__(message)
        self.report = report


def command(stage, args, *, timeout=120, data=None, check=True):
    """Never print commands or provider/SQL output: either may contain secrets."""
    try:
        with subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE, start_new_session=True) as process:
            try:
                output, errors = process.communicate(data, timeout=timeout)
            except BaseException:
                os.killpg(process.pid, signal.SIGKILL)
                process.communicate()
                raise
            result = subprocess.CompletedProcess(args, process.returncode, output, errors)
    except subprocess.TimeoutExpired:
        raise RecoveryError(f"{stage}: timed out") from None
    except OSError:
        raise RecoveryError(f"{stage}: executable unavailable") from None
    if check and result.returncode:
        raise RecoveryError(f"{stage}: command failed (status {result.returncode})")
    return result


def pipeline(stage, commands, *, timeout=1800):
    """Stream only through pipes; join every child and reject every failed stage."""
    processes = []
    deadline = time.monotonic() + timeout
    try:
        for index, args in enumerate(commands):
            previous = processes[-1].stdout if processes else None
            processes.append(subprocess.Popen(args, stdin=previous,
                stdout=subprocess.PIPE if index + 1 < len(commands) else subprocess.DEVNULL,
                stderr=subprocess.DEVNULL, start_new_session=True))
            if previous:
                previous.close()
        for process in reversed(processes):
            process.wait(timeout=max(0.001, deadline - time.monotonic()))
        if any(process.returncode for process in processes):
            raise RecoveryError(f"{stage}: pipeline failed")
    except subprocess.TimeoutExpired:
        raise RecoveryError(f"{stage}: timed out") from None
    except OSError:
        raise RecoveryError(f"{stage}: executable unavailable") from None
    finally:
        for process in processes:
            # Kill the group even if its direct child exited with a hung descendant.
            with contextlib.suppress(ProcessLookupError):
                os.killpg(process.pid, signal.SIGKILL)
        for process in processes:
            process.wait()


def key_file(path):
    key = Path(path)
    if key.is_symlink() or not key.is_file() or key.stat().st_mode & 0o077:
        raise RecoveryError("encryption key must be a private regular file (mode 0600)")
    return str(key.resolve())


def validate_dump_stream():
    """Require pg_dump framing before allowing a logical restore to be accepted."""
    first = b""
    last = b""
    while chunk := sys.stdin.buffer.read(65536):
        first = (first + chunk)[:4096]
        last = (last + chunk)[-4096:]
        sys.stdout.buffer.write(chunk)
    sys.stdout.buffer.flush()
    if b"-- PostgreSQL database dump\n" not in first or b"-- PostgreSQL database dump complete\n" not in last:
        return 1
    return 0


def json_result(stage, args, **kwargs):
    try:
        return json.loads(command(stage, args, **kwargs).stdout)
    except (ValueError, UnicodeError):
        raise RecoveryError(f"{stage}: invalid structured result") from None


def private_file(path, data):
    path = Path(path)
    # A report/config destination must be newly created by this invocation.
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "w") as stream:
        stream.write(data)


def valid_image(image):
    if not image or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:/@-]+", image):
        raise RecoveryError("an explicit PostgreSQL image is required")
    return image


class Client:
    def __init__(self, base):
        self.base = base
        self.cookies = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.cookies))

    def call(self, method, path, body=None, expected=(200,)):
        headers = {"Content-Type": "application/json"}
        csrf = next((c.value for c in self.cookies if c.name == "nabu_csrf"), None)
        if csrf:
            headers["X-CSRF-Token"] = csrf
        request = urllib.request.Request(self.base + path, method=method, headers=headers,
                                         data=None if body is None else json.dumps(body).encode())
        try:
            response = self.opener.open(request, timeout=10)
        except urllib.error.HTTPError as error:
            response = error
        except (urllib.error.URLError, TimeoutError, OSError):
            raise RecoveryError("application smoke: request failed") from None
        with response:
            payload = response.read(2 << 20)
            if response.status not in expected:
                raise RecoveryError(f"application smoke: unexpected HTTP {response.status}")
        try:
            return json.loads(payload)
        except (ValueError, UnicodeError):
            raise RecoveryError("application smoke: invalid JSON response") from None

    def bootstrap(self, label, password):
        self.call("GET", "/api/me")
        user = self.call("POST", "/api/auth/register", {
            "email": f"recovery-{label}@example.invalid", "password": password,
            "displayName": "Recovery fixture"}, expected=(201,))["user"]
        household = self.call("POST", "/api/household", {"name": "Recovery fixture"}, expected=(201,))["household"]
        chore = self.call("POST", "/api/chores", {
            "name": "Recovery fixture", "icon": "*", "color": "#123456"}, expected=(201,))["chore"]
        return {"user": user, "household": household, "chore": chore, "email": f"recovery-{label}@example.invalid"}

    def login(self, email, password):
        self.call("GET", "/api/me")
        self.call("POST", "/api/auth/login", {"email": email, "password": password})

    def log(self, chore, marker):
        now = dt.datetime.now(dt.timezone.utc)
        return self.call("POST", "/api/logs", {"choreId": chore["id"], "note": marker,
            "date": now.date().isoformat(), "completedAt": now.isoformat(), "hour": now.hour}, expected=(201,))["log"]


class IsolatedRecovery:
    def __init__(self, image, *, keep=False):
        self.image = valid_image(image)
        self.token = uuid.uuid4().hex[:16]
        self.work = Path(tempfile.mkdtemp(prefix="nabu-recovery-"))
        self.work.chmod(0o700)
        self.network = "nabu-recovery-" + self.token
        self.containers = []
        self.keep = keep
        self.report = {"run": self.token, "scope": "isolated local recovery", "status": "running"}
        try:
            self.initialize()
        except BaseException:
            self.keep = False
            self.close()
            if self.report.get("cleanup") == "incomplete":
                raise RecoveryError("recovery initialization and cleanup failed", self.report) from None
            raise

    def initialize(self):
        uid = command("database image UID", ["podman", "run", "--rm", "--network=none", "--entrypoint=id", self.image, "-u", "postgres"]).stdout.decode().strip()
        if not uid.isdigit() or uid == "0":
            raise RecoveryError("database image must contain an unprivileged postgres user")
        self.uid = uid
        self.password = secrets.token_urlsafe(32)
        self.env = self.work / "database.env"
        private_file(self.env, f"POSTGRES_USER=nabu\nPOSTGRES_DB=nabu\nPOSTGRES_PASSWORD={self.password}\n")
        command("isolated network", ["podman", "network", "create", "--internal", "--label", "nabu.recovery.run=" + self.token, self.network])

    def close(self):
        if self.keep:
            self.report["retained_directory"] = str(self.work)
            self.report["retained_containers"] = self.containers
            self.report["retained_network"] = self.network
            return
        incomplete = False
        for name in reversed(self.containers):
            try:
                inspected = command("cleanup ownership", ["podman", "inspect", "--format", '{{index .Config.Labels "nabu.recovery.run"}}', name], check=False, timeout=10)
                owned = inspected.stdout.decode().strip()
                if owned == self.token:
                    result = command("isolated cleanup", ["podman", "rm", "--force", "--time=2", name], check=False, timeout=20)
                    incomplete |= result.returncode != 0
                elif inspected.returncode == 0:
                    incomplete = True
                else:
                    exists = command("cleanup existence", ["podman", "container", "exists", name], check=False, timeout=10)
                    incomplete |= exists.returncode != 1
            except (RecoveryError, UnicodeError, OSError):
                incomplete = True
        try:
            network = command("network cleanup ownership", ["podman", "network", "inspect", self.network], check=False, timeout=10)
            if network.returncode == 0:
                info = json.loads(network.stdout)
                if info and info[0].get("labels", {}).get("nabu.recovery.run") == self.token:
                    result = command("isolated network cleanup", ["podman", "network", "rm", self.network], check=False, timeout=20)
                    incomplete |= result.returncode != 0
                else:
                    incomplete = True
            else:
                exists = command("network cleanup existence", ["podman", "network", "exists", self.network], check=False, timeout=10)
                incomplete |= exists.returncode != 1
        except (RecoveryError, ValueError, OSError):
            incomplete = True
        if not incomplete:
            try:
                shutil.rmtree(self.work)
            except OSError:
                incomplete = True
        if incomplete:
            self.report.update(status="failed", cleanup="incomplete", retained_directory=str(self.work),
                               retained_containers=self.containers, retained_network=self.network)
        else:
            self.report["cleanup"] = "complete"

    def postgres_args(self, name, data_dir, *, config=None, repository=None):
        data_dir.mkdir(mode=0o700, exist_ok=True)
        args = ["podman", "run", "--name", name, "--label", "nabu.recovery.run=" + self.token,
                "--network", self.network, "--userns", f"keep-id:uid={self.uid},gid={self.uid}",
                "--user", f"{self.uid}:{self.uid}", "--cap-drop=ALL", "--security-opt=no-new-privileges",
                "--read-only", "--tmpfs=/tmp:rw,mode=1777", "--tmpfs=/var/run/postgresql:rw,mode=3777",
                "--env-file", str(self.env), "-v", f"{data_dir}:/var/lib/postgresql/data"]
        if config:
            args += ["-v", f"{config}:/etc/pgbackrest/pgbackrest.conf:ro"]
        if repository:
            args += ["-v", f"{repository}:/repository"]
        return args

    def start_database(self, suffix, data_dir, *, config=None, repository=None, wal=False):
        name = self.network + "-" + suffix
        args = self.postgres_args(name, data_dir, config=config, repository=repository)
        self.containers.append(name)
        args += ["-d", self.image, "postgres", "-c", "log_min_messages=panic",
                 "-c", "log_min_error_statement=panic", "-c", "log_error_verbosity=terse",
                 "-c", "log_statement=none", "-c", "log_parameter_max_length=0",
                 "-c", "log_parameter_max_length_on_error=0"]
        if wal:
            args += ["-c", "wal_level=replica", "-c", "archive_mode=on", "-c", "archive_timeout=60s",
                     "-c", "archive_command=/usr/local/bin/nabu-pgbackrest-archive-wal %p"]
        command("database start", args)
        self.wait_sql(name)
        return name

    def sql(self, name, sql):
        return command("database check", ["podman", "exec", "-i", name, "psql", "-X", "-U", "nabu", "-d", "nabu", "-At", "-v", "ON_ERROR_STOP=1"], data=sql.encode(), timeout=15).stdout.decode().strip()

    def wait_sql(self, name):
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            result = command("database readiness", ["podman", "exec", name, "psql", "-X", "-U", "nabu", "-d", "nabu", "-Atc", "SELECT NOT pg_is_in_recovery()"], timeout=3, check=False)
            if result.returncode == 0 and result.stdout.strip() == b"t":
                return
            status = command("database process status", ["podman", "inspect", "--format", "{{.State.Status}}", name], check=False).stdout.strip()
            if status in (b"exited", b"dead"):
                raise RecoveryError("database readiness: process exited")
            time.sleep(0.25)
        raise RecoveryError("database readiness: deadline exceeded")

    def start_app(self, suffix, database, image):
        name = self.network + "-app-" + suffix
        path = self.work / (suffix + "-app.env")
        private_file(path, f"DATABASE_URL=postgres://nabu:{self.password}@{database}:5432/nabu?sslmode=disable\nAPP_ENV=development\nAPP_BASE_URL=http://localhost:8080\nPORT=8080\nRATE_LIMIT_AUTH_MAX=1000\n")
        self.containers.append(name)
        command("application start", ["podman", "run", "-d", "--name", name, "--label", "nabu.recovery.run=" + self.token,
                "--network", self.network, "--cap-drop=ALL", "--security-opt=no-new-privileges", "--read-only",
                "--publish", "127.0.0.1::8080", "--env-file", str(path), valid_image(image)])
        mapping = json_result("application port", ["podman", "inspect", "--format", "{{json .NetworkSettings.Ports}}", name])
        port = int(mapping["8080/tcp"][0]["HostPort"])
        base = "http://127.0.0.1:" + str(port)
        self.wait_ready(base)
        return base, name

    @staticmethod
    def ready_status(base):
        try:
            with urllib.request.urlopen(base + "/ready", timeout=2) as response:
                return response.status
        except urllib.error.HTTPError as error:
            return error.code
        except (urllib.error.URLError, TimeoutError, OSError):
            return 0

    def wait_ready(self, base):
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            if self.ready_status(base) == 200:
                return
            time.sleep(0.25)
        raise RecoveryError("application readiness: deadline exceeded")

    def pgbackrest(self, database, *args, timeout=180):
        return command("pgBackRest " + args[-1], ["podman", "exec", "--user", self.uid, database,
            "timeout", "-s", "TERM", "-k", "5", str(timeout - 10), "pgbackrest", "--stanza=nabu", *args], timeout=timeout)

    def restore_wal(self, config, repository, target, suffix="restored"):
        name = self.network + "-" + suffix + "-tool"
        target_data = self.work / (suffix + "-data")
        args = self.postgres_args(name, target_data, config=config, repository=repository)
        self.containers.append(name)
        command("point-in-time restore", args + ["--entrypoint=pgbackrest", self.image, "--stanza=nabu", "--type=time", "--target=" + target, "--target-action=promote", "--archive-mode=off", "restore"], timeout=1800)
        return self.start_database(suffix + "-db", target_data, config=config, repository=repository)

    def check_constraints(self, database):
        if self.sql(database, "SELECT count(*) FROM pg_constraint WHERE contype IN ('f','c') AND NOT convalidated") != "0":
            raise RecoveryError("restored schema contains unvalidated constraints")

    def check_restored_schema(self, database):
        # Run before the application can initialize/migrate an empty database.
        expected = ("users", "households", "user_households", "chores", "chore_logs", "schema_migrations")
        for table in expected:
            if self.sql(database, "SELECT to_regclass('public." + table + "') IS NOT NULL") != "t":
                raise RecoveryError("logical backup is missing required application schema")
        if int(self.sql(database, "SELECT count(*) FROM schema_migrations")) == 0:
            raise RecoveryError("logical backup is missing migration history")
        self.check_constraints(database)

    def outage_and_rollback(self, database, base, app, image):
        command("isolated database outage", ["podman", "stop", "--time=2", database])
        if self.ready_status(base) != 503:
            raise RecoveryError("database outage did not become unready")
        command("isolated database recovery", ["podman", "start", database])
        self.wait_sql(database)
        self.wait_ready(base)
        command("isolated rollback setup", ["podman", "stop", "--time=2", app])
        candidate = self.network + "-failed-candidate"
        self.containers.append(candidate)
        # Exercise a real rejected startup and restart the previous image against
        # the same clone. The deployment command itself has separate regressions.
        command("isolated rejected candidate", ["podman", "run", "-d", "--name", candidate,
            "--label", "nabu.recovery.run=" + self.token, "--network", self.network,
            "--cap-drop=ALL", "--security-opt=no-new-privileges", "--read-only",
            "--env", "APP_ENV=production", valid_image(image)])
        result = command("rejected candidate termination", ["podman", "wait", candidate], timeout=30)
        if not result.stdout.strip().isdigit() or int(result.stdout.strip()) == 0:
            raise RecoveryError("invalid candidate did not fail startup")
        command("isolated previous application restart", ["podman", "start", app])
        self.wait_ready(base)
        self.report.update(outage_readiness=503, recovery_readiness=200, rollback_readiness=200)


def application_smoke(recovery, database, image):
    base, app = recovery.start_app("verification", database, image)
    recovery.check_constraints(database)
    tables = ("users", "households", "user_households", "chores", "chore_logs")
    counts = {table: recovery.sql(database, "SELECT count(*) FROM " + table) for table in tables}
    a, b = Client(base), Client(base)
    password = "Synthetic recovery fixture only"
    actor_a = a.bootstrap(recovery.token + "-verify-a", password)
    actor_b = b.bootstrap(recovery.token + "-verify-b", password)
    a.log(actor_a["chore"], "synthetic-restore-marker-a")
    foreign = b.log(actor_b["chore"], "synthetic-restore-marker-b")
    a.call("GET", f'/api/stats/chores/{actor_b["chore"]["id"]}/summary', expected=(403, 404))
    a.call("PATCH", f'/api/logs/{foreign["id"]}', {"note": "must-not-change"}, expected=(403, 404))
    notes = {row["note"] for row in a.call("GET", "/api/logs/history")["logs"]}
    if notes != {"synthetic-restore-marker-a"}:
        raise RecoveryError("application household read isolation failed")
    recovery.outage_and_rollback(database, base, app, image)
    # The restored rows must survive a new session and application rollback.
    reloaded = Client(base)
    reloaded.login(actor_b["email"], password)
    notes = {row["note"] for row in reloaded.call("GET", "/api/logs/history")["logs"]}
    if notes != {"synthetic-restore-marker-b"}:
        raise RecoveryError("application rollback persistence failed")
    reloaded.log(actor_b["chore"], "synthetic-after-rollback")
    a.call("DELETE", "/api/me", {"confirm": "DELETE"})
    reloaded.call("DELETE", "/api/me", {"confirm": "DELETE"})
    if any(recovery.sql(database, "SELECT count(*) FROM " + table) != count for table, count in counts.items()):
        raise RecoveryError("verification fixture cleanup changed original row counts")
    recovery.report.update(two_household_isolation=True, constraints_validated=True,
                           rollback_write_verified=True, fixture_cleanup_verified=True)


def latest_dump(remote, destination, max_age_hours):
    if not re.fullmatch(r"[A-Za-z0-9_-]+:[A-Za-z0-9/_-]+", remote):
        raise RecoveryError("an explicit backup remote is required")
    entries = json_result("backup inventory", ["rclone", "lsjson", "--files-only", "--max-depth=1",
        "--include=nabu_*.sql.gz.gpg", remote], timeout=120)
    eligible = []
    for entry in entries:
        if re.fullmatch(r"nabu_[0-9]{8}_[0-9]{6}(?:_[a-f0-9]+)?\.sql\.gz\.gpg", entry.get("Path", "")) and entry.get("Size", 0) > 0:
            try:
                modified = dt.datetime.fromisoformat(entry["ModTime"].replace("Z", "+00:00"))
                age = (dt.datetime.now(dt.timezone.utc) - modified).total_seconds()
            except (KeyError, ValueError, TypeError):
                continue
            if -300 <= age <= max_age_hours * 3600:
                eligible.append((modified, entry["Path"], age))
    if not eligible:
        raise RecoveryError("no complete encrypted backup within the freshness objective")
    _, name, age = max(eligible)
    command("encrypted backup download", ["rclone", "copyto", remote.rstrip("/") + "/" + name,
        str(destination), "--contimeout=10s", "--timeout=60s", "--retries=2"], timeout=1200)
    destination.chmod(0o600)
    return round(max(0, age), 1)


def restore_dump(args):
    secret = key_file(args.key_file)
    recovery = IsolatedRecovery(args.image, keep=args.keep)
    started = time.monotonic()
    try:
        if args.remote:
            backup = recovery.work / "download.sql.gz.gpg"
            recovery.report["backup_age_seconds"] = latest_dump(args.remote, backup, args.max_age_hours)
        else:
            backup = Path(args.backup).resolve(strict=True)
            if not backup.is_file() or not backup.name.endswith(".sql.gz.gpg"):
                raise RecoveryError("an encrypted .sql.gz.gpg backup is required")
        database = recovery.start_database("dump-db", recovery.work / "dump-data")
        gnupg = recovery.work / "gnupg"
        gnupg.mkdir(mode=0o700)
        pipeline("encrypted logical restore", [
            ["gpg", "--no-options", "--homedir", str(gnupg), "--batch", "--no-symkey-cache",
             "--pinentry-mode=loopback", "--passphrase-file", secret, "--decrypt", str(backup)],
            ["gzip", "--decompress", "--stdout"],
            [sys.executable, str(Path(__file__).resolve()), "validate-dump"],
            ["podman", "exec", "-i", database, "psql", "-X", "-q", "-U", "nabu", "-d", "nabu",
             "--single-transaction", "-v", "ON_ERROR_STOP=1", "-v", "VERBOSITY=terse", "-v", "SHOW_CONTEXT=never"]])
        recovery.check_restored_schema(database)
        recovery.report["logical_restore_seconds"] = round(time.monotonic() - started, 3)
        application_smoke(recovery, database, args.app_image)
        elapsed = time.monotonic() - started
        if elapsed > 3600:
            raise RecoveryError("local restore exceeded the one-hour objective")
        recovery.report.update(status="passed", format="encrypted logical dump", total_seconds=round(elapsed, 3))
    except RecoveryError as error:
        recovery.report.update(status="failed", stage=str(error))
    finally:
        recovery.close()
    return recovery.report


def wal_drill(args):
    recovery = IsolatedRecovery(args.image, keep=args.keep)
    started = time.monotonic()
    try:
        repository = recovery.work / "repository"
        repository.mkdir(mode=0o700)
        config = recovery.work / "pgbackrest.conf"
        private_file(config, "[global]\nrepo1-type=posix\nrepo1-path=/repository\nrepo1-cipher-type=aes-256-cbc\nrepo1-cipher-pass=" + secrets.token_urlsafe(48) + "\nrepo1-retention-full=2\narchive-async=n\narchive-timeout=60\nstart-fast=y\nprocess-max=2\nlog-level-console=off\nlog-level-file=off\n[nabu]\npg1-path=/var/lib/postgresql/data\npg1-socket-path=/var/run/postgresql\npg1-user=nabu\n")
        primary = recovery.start_database("source-db", recovery.work / "source-data", config=config, repository=repository, wal=True)
        base, _ = recovery.start_app("source", primary, args.app_image)
        a, b = Client(base), Client(base)
        password = "Synthetic recovery fixture only"
        actor_a = a.bootstrap(recovery.token + "-a", password)
        actor_b = b.bootstrap(recovery.token + "-b", password)
        a.log(actor_a["chore"], "synthetic-full-marker")
        other_log = b.log(actor_b["chore"], "synthetic-foreign-marker")
        recovery.pgbackrest(primary, "stanza-create")
        recovery.pgbackrest(primary, "check")
        recovery.pgbackrest(primary, "--type=full", "backup", timeout=1800)
        marker_segment = recovery.sql(primary, "SELECT pg_walfile_name(pg_current_wal_lsn())")
        a.log(actor_a["chore"], "synthetic-wal-marker")
        target = recovery.sql(primary, "SELECT clock_timestamp()")
        a.log(actor_a["chore"], "synthetic-after-target-marker")
        archive_start = time.monotonic()
        recovery.pgbackrest(primary, "check")
        recovery.report["archive_verification_seconds"] = round(time.monotonic() - archive_start, 3)
        recovery.pgbackrest(primary, "verify", timeout=1800)
        info = json.loads(recovery.pgbackrest(primary, "--output=json", "info").stdout)
        if not info or info[0].get("cipher") != "aes-256-cbc" or not info[0].get("backup"):
            raise RecoveryError("encrypted repository verification failed")
        restore_start = time.monotonic()
        restored = recovery.restore_wal(config, repository, target)
        restored_base, restored_app = recovery.start_app("restored", restored, args.app_image)
        recovery.check_constraints(restored)
        restored_a, restored_b = Client(restored_base), Client(restored_base)
        restored_a.login(actor_a["email"], password)
        restored_b.login(actor_b["email"], password)
        history = restored_a.call("GET", "/api/logs/history")
        notes = {row["note"] for row in history["logs"]}
        if "synthetic-full-marker" not in notes or "synthetic-wal-marker" not in notes or "synthetic-after-target-marker" in notes or "synthetic-foreign-marker" in notes:
            raise RecoveryError("PITR marker or household isolation check failed")
        restored_a.call("GET", f'/api/stats/chores/{actor_b["chore"]["id"]}/summary', expected=(403, 404))
        restored_a.call("PATCH", f'/api/logs/{other_log["id"]}', {"note": "must-not-change"}, expected=(403, 404))
        own_b = restored_b.call("GET", "/api/logs/history")
        if not any(row["note"] == "synthetic-foreign-marker" for row in own_b["logs"]):
            raise RecoveryError("cross-household write changed the other fixture")
        recovery.report["restore_and_application_smoke_seconds"] = round(time.monotonic() - restore_start, 3)
        recovery.outage_and_rollback(restored, restored_base, restored_app, args.app_image)
        restored_a.log(actor_a["chore"], "synthetic-after-rollback")
        if not any(row["note"] == "synthetic-wal-marker" for row in restored_a.call("GET", "/api/logs/history")["logs"]):
            raise RecoveryError("application rollback lost restored data")
        wrong_key = recovery.work / "wrong-key.conf"
        private_file(wrong_key, re.sub(r"repo1-cipher-pass=.*", "repo1-cipher-pass=wrong-synthetic-test-key", config.read_text()))
        try:
            recovery.restore_wal(wrong_key, repository, target, "wrong-key")
        except RecoveryError:
            recovery.report["wrong_key_rejected"] = True
        else:
            raise RecoveryError("restore accepted the wrong encryption key")
        broken_repository = recovery.work / "missing-wal-repository"
        shutil.copytree(repository, broken_repository)
        matches = list(broken_repository.glob("archive/nabu/*/*/" + marker_segment + "*"))
        if not matches:
            raise RecoveryError("archive gap fixture segment not found")
        for path in matches:
            path.unlink()
        try:
            recovery.restore_wal(config, broken_repository, target, "missing-wal")
        except RecoveryError:
            recovery.report["missing_wal_rejected"] = True
        else:
            raise RecoveryError("restore accepted an incomplete WAL chain")
        if recovery.report["archive_verification_seconds"] > 300 or recovery.report["restore_and_application_smoke_seconds"] > 3600:
            raise RecoveryError("local recovery objective exceeded")
        recovery.report.update(status="passed", encrypted_repository=True, wal_marker_replayed=True,
                               after_target_marker_excluded=True, two_household_isolation=True,
                               constraints_validated=True, rollback_write_verified=True,
                               outage_readiness=503, recovery_readiness=200,
                               total_seconds=round(time.monotonic() - started, 3))
    except RecoveryError as error:
        recovery.report.update(status="failed", stage=str(error))
    finally:
        recovery.close()
    return recovery.report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="action", required=True)
    sub.add_parser("validate-dump", help=argparse.SUPPRESS)
    drill = sub.add_parser("wal-drill", help="create, back up and PITR-restore synthetic data in isolated containers")
    drill.add_argument("--image", required=True)
    drill.add_argument("--app-image", required=True)
    drill.add_argument("--keep", action="store_true", help="retain only this invocation's isolated resources for inspection")
    drill.add_argument("--report", required=True, help="new private JSON report path outside the repository")
    logical = sub.add_parser("restore-dump", help="restore a trusted encrypted dump into a fresh isolated database and verify the app")
    source = logical.add_mutually_exclusive_group(required=True)
    source.add_argument("--backup", help="local encrypted .sql.gz.gpg file")
    source.add_argument("--remote", help="explicit rclone remote/path; select only fresh complete encrypted dumps")
    logical.add_argument("--max-age-hours", type=float, default=26)
    logical.add_argument("--key-file", required=True, help="0600 encryption passphrase file; never a command-line secret")
    logical.add_argument("--image", required=True, help="explicit PostgreSQL image compatible with the source dump")
    logical.add_argument("--app-image", required=True, help="explicit application image to run on the restored clone")
    logical.add_argument("--keep", action="store_true")
    logical.add_argument("--report", required=True)
    args = parser.parse_args()
    if args.action == "validate-dump":
        return validate_dump_stream()
    try:
        if args.action == "restore-dump" and not 0 < args.max_age_hours <= 720:
            raise RecoveryError("invalid backup freshness limit")
        report = wal_drill(args) if args.action == "wal-drill" else restore_dump(args)
    except RecoveryError as error:
        report = error.report or {}
        report.update(status="failed", stage=str(error))
    except (OSError, ValueError):
        report = {"status": "failed", "stage": "invalid or inaccessible recovery input"}
    report["completed_at"] = time.time()
    try:
        private_file(args.report, json.dumps(report, indent=2) + "\n")
    except OSError:
        report.update(status="failed", report_write="failed")
    print(json.dumps(report))
    return 0 if report["status"] == "passed" else 1


def interrupted(signum, frame):
    # Ignore subsequent termination while bounded cleanup joins owned resources.
    signal.signal(signal.SIGTERM, signal.SIG_IGN)
    signal.signal(signal.SIGINT, signal.SIG_IGN)
    raise RecoveryError("recovery interrupted")


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    try:
        raise SystemExit(main())
    except RecoveryError:
        print('{"status":"failed","stage":"recovery interrupted during initialization"}')
        raise SystemExit(1)
