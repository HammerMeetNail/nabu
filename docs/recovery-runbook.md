# Backup and recovery

The objectives are **at most five minutes of lost committed data (RPO)** and
**service restored within one hour (RTO)**. These are operational targets for a
healthy, monitored off-site archive, not guarantees during an archive outage.
Daily logical dumps alone cannot meet the data-loss target.

The prepared design uses pgBackRest full backups weekly, differential backups
daily, and continuous encrypted WAL to a dedicated off-site S3-compatible
repository. PostgreSQL switches WAL after 60 seconds of activity; the archive
command acknowledges a segment only after repository storage succeeds. Failed
archiving leaves WAL locally and reports failure; it never discards a queue to
claim success. A separate daily encrypted logical dump remains secondary
protection and supports major-version migrations.

The repository does not establish the live PostgreSQL major, data path, backup
age, R2 policy, available disk, credentials, alert delivery, or restore speed.
No production access or deployment was performed for this implementation.

## Adoption: inventory before changing the database

1. Record the actual database container/image digest, `SHOW server_version`,
   data mount, postgres UID, network, available disk and existing backup timers.
   `compose.server.yaml` deploys only the application. Its standalone PostgreSQL
   container is not defined there. Historical paths in provisioning documents
   are not evidence of the live layout.
2. Build and scan `ops/recovery/Containerfile`, then publish and record its
   immutable digest. It currently builds PostgreSQL 17.11 and pgBackRest 2.59.1
   from a checksum-pinned official source archive. Use it only with an existing
   PostgreSQL 17 cluster. A different major requires a separately tested image
   or the [major migration procedure](postgres-major-version-migration.md).
3. Create a dedicated off-site bucket/prefix and scoped repository credentials.
   Copy `ops/recovery/pgbackrest.conf.example` to `/etc/nabu/pgbackrest.conf` with
   mode 0600, readable only by the database's mapped postgres UID. Generate a
   long random encryption secret and escrow it outside the server in the
   organization's password manager. Escrow restore credentials separately.
   Losing the encryption key makes the backups unusable.
4. Prepare the verified database definition from
   `ops/recovery/compose.postgres.yaml.example`, preserving the inventoried data
   path and major. Schedule the database restart needed to enable archiving.
   Keep the previous definition/image and a tested encrypted logical backup.
   Review this concrete database change before applying it; ordinary application
   deployments deliberately do not apply it.
5. In the database container, run `pgbackrest --stanza=nabu stanza-create`,
   `pgbackrest --stanza=nabu check`, and
   `pgbackrest --stanza=nabu --type=full backup` as its postgres user. Verify
   `info --output=json`, encryption, full-backup completion and archived WAL.
   Do not print the configuration, credential environment or raw provider errors.
6. Install `ops/recovery/recovery.env.example` as `/etc/nabu/recovery.env`, mode
   0600, owned by the operations user. Supply verified image digests, the
   encrypted logical-backup remote, a private logical-backup passphrase file,
   and an alert/deadman receiver. This trusted shell configuration is separate
   from `/opt/nabu/.env`, which application deployments rewrite.
7. Run `scripts/verify-backup.sh` and an isolated off-site PITR restore before
   enabling the prepared timers. The verification script downloads only a
   complete `.sql.gz.gpg` object modified within 26 hours, restores into a new
   private clone, and exercises the real application. Record object/backup
   identity, source image/major, recovered target and elapsed time in the
   incident/operations record without copying private rows or secrets.
8. During adoption, invoke the staged `scripts/verify-backup.next.sh` directly
   for the first verified restore; normal app deployment keeps the prior
   scheduled verifier until its replacement is ready. Install the six `ops/recovery/systemd/nabu-recovery-*` units, reload systemd,
   and enable the full/diff/check timers plus `nabu-verify-backup.timer` only
   after configuration and drills pass. Keep the existing daily logical backup
   timer enabled throughout adoption and for at least a full retention window.
   Verify the actual schedules, successful runs and receiver delivery. The next
   application deploy installs the staged verifier only when the check timer is
   active and `--check-config` confirms a healthy heartbeat delivered within
   five minutes. Otherwise it retains the existing verifier and prints the
   pending-adoption message. The legacy verifier still has the risks described
   in the review until this controlled transition is completed.

Use the current stable PostgreSQL and pgBackRest patch releases when preparing a
new image, and repeat the image scan and recovery drill after updating either.
The archive wrapper's installed name contains `pgbackrest` because pgBackRest's
`check` validates the configured archive command.

## Monitoring and incident response

`recovery-job.sh check` checks encrypted repository status, a successful backup
within 26 hours, a full backup within eight days, and the latest isolated logical
restore within 26 hours. A failed or incomplete newer restore overrides an older
success. It also checks archive queue age, database disk use, and a fresh
repository acknowledgement. The check runs about once a minute after the prior
run ends; commands and provider calls have deadlines.

The receiver gets a small JSON POST containing status, time, numeric ages and
classified failed stage. Configure it to page on `status=failed` and on **five
minutes without a heartbeat**. A failed delivery makes the local job fail.
`--no-notify` is for local inspection and does not establish alert delivery.
A valid repository heartbeat alone is insufficient: restore failure, missing
reports and stale verification are unhealthy. The daily backup preserves the
existing configured email failure channel with bounded delivery and sanitized
content. Never upload SQL errors or database logs as backup reports.

Treat a WAL queue older than three minutes, 85% disk use, a missed backup, a
failed restore or a missing heartbeat as actionable. Confirm receiver routing
and an on-call owner. Restore archive access immediately; if the archive cannot
be brought current within the RPO, the incident owner should fence writes until
an off-site recovery point is current. Never delete unarchived WAL to free disk.
Provision disk headroom for the measured write rate and an extended repository
outage; the sample thresholds are alerts, not a substitute for capacity planning.

## Isolated logical restore

Use only a trusted backup. PostgreSQL dumps contain executable SQL. The command
never reads a live `.env`, discovers a running database, accepts an existing
restore target, or changes production routing:

```bash
scripts/restore.sh \
  --backup /secure/incoming/nabu_YYYYMMDD_HHMMSS.sql.gz.gpg \
  --key-file /secure/keys/logical-backup-key \
  --image registry.example/nabu-postgres@sha256:VERIFIED_DIGEST \
  --app-image quay.io/nabu/nabu@sha256:VERIFIED_DIGEST \
  --report /secure/reports/restore-NEW_RUN.json
```

Every run has unique labeled containers, an internal network with no external
provider configuration, loopback-only app access, private directories/files,
and an unprivileged database container. Decryption streams through pipes;
plaintext dump files are not written. Every pipeline stage must succeed,
`psql` uses `ON_ERROR_STOP` and a single transaction, and pg_dump framing plus
required schema/migration history are checked **before** the app can initialize
an empty database. Clone SQL statement/context logging is suppressed.

The app then runs migrations, validates constraints, creates two synthetic
households, checks read/write isolation, survives a database outage, rejects a
bad startup configuration and restarts the prior app. It verifies persistence
and new writes after rollback, then deletes only its synthetic accounts and
checks that original table counts remain. By default all owned resources are
removed. `--keep` deliberately retains the clone and private data for inspection;
its report lists those resources. Retained or incomplete cleanup does not count
as a completed scheduled verification. Review and remove retained clones promptly.

SIGTERM/interrupts trigger bounded cleanup. A force kill, host crash or unavailable
container daemon can still leave labeled resources; the report marks incomplete
cleanup when observable. The incident procedure must inventory those resources
and their private directories afterward. Never use a broad container-name pattern
or global volume prune to clean them up.

## PITR, cutover and rollback

For physical recovery, select a completed full/differential backup and a UTC
recovery target using a read-only repository identity and the escrowed encryption
key. Restore with pgBackRest into a **new, empty data directory** on a compatible
PostgreSQL major. Use `--type=time --target=... --target-action=promote` and
`--archive-mode=off` for an inspection clone so it cannot write to the original
archive. Keep it isolated while confirming the target was reached, migrations,
constraints, known records and household isolation. Missing WAL or a wrong key
must fail recovery; never accept an earlier accidental stopping point.

A controlled cutover is a separate reviewed incident action:

1. Fence application writes and all background workers; record the last accepted
   write time and recoverable WAL target. Preserve the original database volume
   and previous application/environment configuration unchanged.
2. Complete the final restore into a separately named database/volume. Inspect
   the recovery target and run the application smoke checks in isolation. For
   a major migration, take the final logical backup while writes are fenced;
   WAL from one major cannot recover a different major.
3. Prepare the application connection change to the new database and the tested
   application image. Review the exact source, target, routing and rollback
   values. Switch traffic only after `/ready` and authenticated checks pass.
4. Enable archiving for the newly promoted writable database with an appropriate
   repository/timeline plan and obtain a fresh full backup. Confirm the monitor
   and receiver before lifting the write fence.
5. If cutover fails before accepting new writes, restore the previous application
   configuration/connection and check readiness. After accepting new writes,
   switching back can lose those writes: fence traffic and reconcile them with
   the incident owner before reverting. Preserve both volumes until resolved.

`deploy-local.sh` handles bounded application readiness and app rollback. It
does not roll back database migrations or execute a database cutover. The local
recovery drill exercises a real rejected app startup and restart against the
clone; deployment command branches have separate operations regressions.

## Retention and access

The physical repository keeps at least 35 days of full-backup history, daily
differentials and the WAL needed for retained recovery chains. With weekly full
backups, healthy expiration can retain a deleted record in older encrypted base
backups for roughly 42 days. Failed/paused backups, object locks or delayed expiry
can extend that period; 35 days is not a promised deletion deadline. Logical
backup retention is a separately verified bucket policy. Keep existing logical
backups during transition, then document that policy and its actual expiry.

Do not independently expire WAL objects through a bucket lifecycle rule. Let
pgBackRest expire complete recovery chains. Separate repository write/expiration
access from read-only restore access; restrict the bucket and key escrow to
operators who need them. Keep an off-site credential/key inventory. Test any
object-lock policy with pgBackRest expiration before enabling it. Deleting an
account removes live records; it does not rewrite encrypted historical backups.
After a recovery, reconcile deletions made after the chosen recovery point before
opening service to users.

## Repeatable local evidence and limits

```bash
podman build -f ops/recovery/Containerfile \
  -t localhost/nabu-recovery-postgres:17.11-pgbackrest2.59.1 .
make recovery-drill PG_IMAGE=localhost/nabu-recovery-postgres:17.11-pgbackrest2.59.1 \
  APP_IMAGE=LOCAL_TESTED_APP_IMAGE REPORT=/tmp/nabu-wal-drill-NEW.json
node --test tests/ops/recovery.test.js
python3 tests/ops/recovery-drill.py \
  --image localhost/nabu-recovery-postgres:17.11-pgbackrest2.59.1 \
  --app-image LOCAL_TESTED_APP_IMAGE --report /tmp/nabu-recovery-full-NEW.json
```

The final isolated PostgreSQL 17.11 / pgBackRest 2.59.1 encrypted POSIX-repository
run passed in 18.756 seconds: archive acknowledgement 0.265s and
restore/application smoke 3.236s. It verified WAL marker replay, later-target
exclusion, household isolation, constraints, outage/readiness recovery, app
rollback, wrong-key rejection and missing-WAL rejection. The encrypted logical
dump restored in 3.732s and completed the full application/outage/rollback and
fixture-cleanup drill in 9.268s. Empty/truncated dumps, private SQL errors and wrong
keys were rejected; loopback alert failure, recovery and delivery rejection were
also exercised. All owned containers/networks and private temporary data were
cleaned up (`/tmp/nabu-recovery-integrated-v3.json`).

This run used local app image `78fd77b4a2ae` and recovery image `e49b32ffab9f`.
The recovery image upgrades base packages and rebuilds checksum-pinned gosu 1.19
with Go 1.26.8; its final HIGH/CRITICAL scan had zero findings. These are tiny
synthetic fixtures on local hardware; they do not measure off-site transfer,
production data volume, external alert delivery, or production RPO/RTO. Repeat
the drill and scan for the actual image digests and infrastructure at adoption.

Primary references: [PostgreSQL continuous archiving](https://www.postgresql.org/docs/17/continuous-archiving.html),
[pgBackRest user guide](https://pgbackrest.org/user-guide.html),
[pgBackRest configuration](https://pgbackrest.org/configuration.html), and
[pgBackRest commands](https://pgbackrest.org/command.html).
