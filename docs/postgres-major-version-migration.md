# PostgreSQL Major Version Migration

PostgreSQL cannot start if the data directory was initialized by a different
major version (e.g. 16 → 17). This document covers how to migrate in place on
the production server.

## When this applies

You bumped the `postgres` image tag in `compose.server.yaml` to a new major
version (e.g. `16-alpine` → `17-alpine`) while an existing data volume at
`/mnt/data/nabu/postgres` already contains a pg16-initialized cluster.

Symptom in `podman logs <postgres-container>`:

```
FATAL: database files are incompatible with server
DETAIL: The data directory was initialized by PostgreSQL version 16,
        which is not compatible with this version 17.x
```

## Pre-migration checklist

- [ ] Confirm you have a recent snapshot/backup of `/mnt/data/nabu/postgres`
      (or take one now via your cloud provider's volume snapshot).
- [ ] Note the `DB_PASSWORD` from `/opt/nabu/.env`.
- [ ] Schedule a maintenance window — the app will be down for the duration.

## Migration steps

All commands run on the production server as a user with `sudo` and
`podman` access.

### 1. Stop the stack

```bash
cd /opt/nabu
podman-compose down
```

### 2. Dump the database using the old major version

Spin up a temporary pg16 container (or whatever the current major version is)
pointing at the existing data volume, then export with `pg_dump`.

```bash
DB_PASSWORD=$(grep DB_PASSWORD /opt/nabu/.env | cut -d= -f2)

podman run -d --name pg_old \
  -e POSTGRES_USER=nabu \
  -e POSTGRES_PASSWORD="${DB_PASSWORD}" \
  -e POSTGRES_DB=nabu \
  -v /mnt/data/nabu/postgres:/var/lib/postgresql/data \
  docker.io/library/postgres:16-alpine   # <-- old major version

# Wait for postgres to finish starting
sleep 8
podman logs pg_old | tail -5  # should end with "ready to accept connections"

podman exec pg_old pg_dump -U nabu nabu > /tmp/nabu_backup.sql
wc -l /tmp/nabu_backup.sql  # sanity check — should not be zero

podman stop pg_old && podman rm pg_old
```

### 3. Wipe the old data directory

```bash
sudo rm -rf /mnt/data/nabu/postgres/*
```

### 4. Update the compose file to the new major version

Edit `/opt/nabu/compose.yaml` (or re-run CI to deploy the updated
`compose.server.yaml` that already has the new version pinned).

```bash
sed -i 's|postgres:16-alpine|postgres:17-alpine|' /opt/nabu/compose.yaml
```

### 5. Start postgres and restore

```bash
cd /opt/nabu
podman-compose up -d postgres
sleep 8

POSTGRES_CONTAINER=$(podman ps -qf name=postgres)
podman exec -i "${POSTGRES_CONTAINER}" psql -U nabu nabu \
  < /tmp/nabu_backup.sql
```

### 6. Verify the restore

```bash
podman exec "${POSTGRES_CONTAINER}" psql -U nabu nabu \
  -c "\dt"                          # list tables
podman exec "${POSTGRES_CONTAINER}" psql -U nabu nabu \
  -c "SELECT count(*) FROM chores;" # spot-check row count
```

### 7. Bring up the full stack

```bash
podman-compose up -d
```

Check the app is healthy:

```bash
podman-compose ps
curl -sf http://localhost:8080/health && echo "OK"
```

### 8. Clean up

```bash
rm /tmp/nabu_backup.sql
```

## Rolling back

If anything goes wrong before step 7, stop everything, restore the volume from
the snapshot taken in the pre-migration checklist, pin the compose file back to
the old major version, and bring the stack back up.

```bash
podman-compose down
# restore snapshot to /mnt/data/nabu/postgres ...
sed -i 's|postgres:17-alpine|postgres:16-alpine|' /opt/nabu/compose.yaml
podman-compose up -d
```
