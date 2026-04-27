#!/bin/bash
# PostgreSQL backup to Cloudflare R2
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-/opt/choresy/.env}"
HOSTNAME="$(hostname)"

if [[ -f "$ENV_FILE" ]]; then
    set -a
    source "$ENV_FILE"
    set +a
fi

BACKUP_DIR="/tmp/choresy-backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="choresy_${TIMESTAMP}.sql.gz.gpg"
R2_BUCKET="${R2_BUCKET:-choresy-backups}"
DB_NAME="${DB_NAME:-choresy}"

if [[ -z "${DB_PASSWORD:-}" ]]; then
    echo "ERROR: DB_PASSWORD is required"
    exit 1
fi

if [[ -z "${BACKUP_ENCRYPTION_KEY:-}" ]]; then
    echo "ERROR: BACKUP_ENCRYPTION_KEY is required"
    exit 1
fi

mkdir -p "$BACKUP_DIR"

echo "[$(date)] Starting backup..."

POSTGRES_CONTAINER=$(podman ps --format '{{.Names}}' 2>/dev/null | grep -E 'postgres' | head -1)

if [[ -n "$POSTGRES_CONTAINER" ]]; then
    podman exec -e PGPASSWORD="$DB_PASSWORD" "$POSTGRES_CONTAINER" \
        pg_dump -U choresy -d "$DB_NAME" --format=plain --no-owner --no-acl \
        | gzip \
        | gpg --symmetric --cipher-algo AES256 --batch --pinentry-mode loopback --passphrase-fd 3 3<<<"$BACKUP_ENCRYPTION_KEY" \
        > "${BACKUP_DIR}/${BACKUP_FILE}"
else
    PGPASSWORD="$DB_PASSWORD" pg_dump \
        -h "${DB_HOST:-localhost}" \
        -p "${DB_PORT:-5432}" \
        -U "${DB_USER:-choresy}" \
        -d "$DB_NAME" \
        --format=plain \
        --no-owner \
        --no-acl \
        | gzip \
        | gpg --symmetric --cipher-algo AES256 --batch --pinentry-mode loopback --passphrase-fd 3 3<<<"$BACKUP_ENCRYPTION_KEY" \
        > "${BACKUP_DIR}/${BACKUP_FILE}"
fi

BACKUP_SIZE=$(du -h "${BACKUP_DIR}/${BACKUP_FILE}" | cut -f1)
echo "[$(date)] Backup created: ${BACKUP_FILE} (${BACKUP_SIZE})"

echo "[$(date)] Uploading to R2..."
rclone copy "${BACKUP_DIR}/${BACKUP_FILE}" "r2:${R2_BUCKET}/" --progress

rm -f "${BACKUP_DIR}/${BACKUP_FILE}"
echo "[$(date)] Backup completed successfully"
