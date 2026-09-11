# PostgreSQL major-version migration

A PostgreSQL data directory can only be started by its compatible major version.
Changing an image tag is not a migration. `compose.server.yaml` deploys the app;
it does not define the standalone production PostgreSQL image or data mount.
Inventory the actual cluster before choosing a target version or path.

Follow the [backup and recovery runbook](recovery-runbook.md) for encryption,
isolated restore, application checks, write fencing, cutover and rollback.

1. Verify the source major/image digest and data path. Preserve its volume and
   image. Confirm a current, restorable encrypted backup and key escrow.
2. Test the intended target major/image in a fresh isolated database. Use a
   trusted logical dump from a compatible `pg_dump`, the target PostgreSQL image,
   and the real app. Physical backups and WAL cannot cross PostgreSQL majors.
3. Run migrations, validate constraints and representative records, and perform
   two-household read/write isolation and persistence checks. Measure transfer,
   restore, index creation, app startup and validation against the one-hour RTO.
4. During the approved maintenance window, fence HTTP writes and background
   workers, take the final encrypted logical dump, and restore it into a new
   volume. Never wipe or reuse the old volume as the restore destination.
5. Prepare and review the exact app connection/image change. Cut over only after
   dependency-aware `/ready` and authenticated smoke checks pass. Establish a new
   full physical backup and working WAL archive for the target major.
6. Before accepting new writes, rollback can use the preserved old cluster and
   prior app configuration. After new writes, fence traffic and reconcile those
   writes before reverting. Retain the old volume until rollback is no longer
   needed and the backup retention/access policy permits deletion.

The restore tooling deliberately has no option to overwrite a running database.
Production timing, paths, credentials, image compatibility and cutover approval
must come from that maintenance operation, not from local fixtures.
