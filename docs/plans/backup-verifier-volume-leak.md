# Backup verifier volume leak fix plan

**Status:** Production hotfix installed and manually verified October 4, 2026; next scheduled-run check and source merge pending. **Incident:** September 30 to October 3, 2026.

Both production backup verifiers start a temporary `postgres:16-alpine` container and remove it with `podman rm -f`. That leaves the image's anonymous database volume behind. On October 3, the host had 438 unused PostgreSQL volumes occupying about 21 GB, with only 323 MB free on `/`. Nabu's September 30 restore failed with `No space left on device`; its October 1 test container then hit a stale-name collision. Nabu's October 2 and 3 restores passed. The 438 unused volumes were pruned after confirming that both live databases use bind mounts on `/mnt/data`; `/` then had about 21 GB free. This cleanup does not prevent new volumes from accumulating.

## Implement the cleanup in both services

1. Patch the **installed legacy verifiers** for Nabu and Year of Bingo in their respective source repositories. Give each run a unique container name and a private scratch directory, and serialize runs of the same verifier with a lock. The current fixed names and scratch paths make retries or overlapping runs unsafe.
2. Mount a uniquely named, labeled Podman volume at PostgreSQL's data directory instead of accepting an anonymous volume. Label the container and volume with the service and run ID. On normal exit, error, or termination, remove only that run's container and volume; report cleanup failure instead of declaring verification passed. Never remove an active or unowned resource. Check the installed Podman version's `rm --volumes` behavior before choosing the exact cleanup command.
3. Add a narrowly scoped reconciliation step for abandoned verifier containers and volumes after crashes. Match ownership labels, require that the run is no longer active, and log only resource IDs and status. Do not use a host-wide `podman volume prune` as routine cleanup.
4. Keep the test restore isolated from the live database bind mounts and preserve the current encrypted off-site backup and notification paths. Do not log SQL, credentials, or private rows.

Nabu's current repository `scripts/verify-backup.sh` is the **staged replacement**, not the verifier running on production. The deploy workflow in `.github/workflows/ci.yaml` preserves the legacy script until the recovery check timer and configuration gate pass. A change to the staged file alone will not fix the live leak. Track and deploy the legacy fix explicitly while the replacement remains pending; update Year of Bingo through its own repository and deployment process.

The legacy implementation now lives in `scripts/verify-backup-legacy.sh`, with the
shared resource lifecycle in `scripts/verify-backup-resources.sh`. Nabu's deploy
workflow installs it only while the installed verifier is recognizable as legacy.
It retains an already adopted recovery verifier when its health gate is unhealthy.
Year of Bingo's own repository carries the same lifecycle helper and updates its
scheduled `scripts/verify-backup.sh`. Both use the service and run labels under
`io.hammermeetnail.backup-verifier.*`; cleanup removes the owned container, then
its explicitly named volume. `podman rm --volumes` only covers anonymous volumes
in the local Podman CLI, so the named volume is removed separately. The installed
host version and scripts must still be inspected before release.

## Prove the fix and roll it out

1. Add an operations regression that runs a real test PostgreSQL container through success, restore failure, and termination. After each case, assert that no owned container or volume remains. Run two invocations back to back and check that a stale run cannot block the next one. Run the repository's required build, lint, unit, and E2E gates before release.
2. Review the exact scripts and resource labels, then deploy each service through its normal release path. Do not change the live database containers or their `/mnt/data` mounts. Run an isolated restore from a fresh off-site backup and confirm the result in each service's systemd journal.
3. After the next scheduled runs, verify that the volume count returns to its baseline, `/` remains comfortably below the disk alert threshold, both backup timers succeed, and both applications and databases remain healthy. Alert on low free space and on any failed or stale restore verification.
4. When Nabu's staged recovery verifier is adopted under `docs/recovery-runbook.md`, repeat the no-orphan-resource check and retire the legacy script only after its replacement has passed the full restore and monitoring gates.

## Rollout record

- Local real-Podman regression: `python3 tests/ops/legacy-verifier-volume.py scripts/verify-backup-legacy.sh nabu` in Nabu; run the same test with `scripts/verify-backup.sh yearofbingo` in Year of Bingo. It exercises success, SQL restore failure, termination, setup and cleanup failure reports, a stale labeled run, and a second clean invocation.
- Before each release, compare the installed script with its expected source, inspect `podman --version` and `podman volume rm --help` on the host, record baseline labeled containers/volumes and free space, and review both release diffs. Do not touch live database mounts.
- After each release, trigger an isolated verification from a fresh encrypted off-site backup, check the systemd journal and failure notification, and confirm no service-labeled container or volume remains. Repeat after the next scheduled verification and confirm both applications and live databases are healthy.
- Production host used Podman 5.8.2. Its `podman rm --volumes` removes anonymous volumes only; the fix instead removes the exact named volume after the labeled container. Both live databases remained bind-mounted under `/mnt/data`.
- The installed old scripts matched their source hashes before replacement. The Nabu hotfix hash is `fa84952ca4def6f16756c9386b706628dcac9692d1f7781c791c8c5d93c75e87`; Year of Bingo's is `151489e2572797ec650f984c1915fd01bb9815650d6cf6d0e11a73cf06e77db1`. Both use helper hash `1d53c23a4da6848d54a871f3c99de7f28e52f4f8baff8c73c51e60626307e4e9`.
- At 02:20–02:21 UTC on October 4, both fixed systemd services restored their latest successful October 3 off-site backups. `Result=success` and `ExecMainStatus=0` for each; both journals reported removal of their own container and volume and `BACKUP VERIFICATION PASSED`. Total Podman volume count returned to 0 after each run; `/` remained 43% used with about 21 GB free. The old scripts have rollback copies named `verify-backup.pre-volume-fix-20261003` in their respective script directories. Nabu's separate staged recovery verifier remains at `verify-backup.next.sh`.
- Recheck the next scheduled runs at approximately 04:10–04:11 UTC October 4. Record source commits/PRs and any tag/CI release once the hotfix is merged into both repositories; a future Year of Bingo deployment from old source would overwrite the host hotfix.
