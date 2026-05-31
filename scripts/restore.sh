#!/bin/bash
# Restore PostgreSQL from Cloudflare R2 backup
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-/opt/nabu/.env}"

if [[ -f "$ENV_FILE" ]]; then
    set -a
    source "$ENV_FILE"
    set +a
fi

BACKUP_DIR="/tmp/nabu-backups"
R2_BUCKET="${R2_BUCKET:-nabu-app-backups}"
DB_NAME="${DB_NAME:-choresy}"

if [[ -z "${DB_PASSWORD:-}" ]]; then
    echo "ERROR: DB_PASSWORD is required"
    exit 1
fi

if [[ -z "${BACKUP_ENCRYPTION_KEY:-}" ]]; then
    echo "ERROR: BACKUP_ENCRYPTION_KEY is required"
    exit 1
fi

list_backups() {
    echo "Available backups in r2-nabu:${R2_BUCKET}/"
    rclone ls "r2-nabu:${R2_BUCKET}/" | sort -k2 | tail -20
}

if [[ "${1:-}" == "--list" ]] || [[ "${1:-}" == "-l" ]]; then
    list_backups
    exit 0
fi

if [[ "${1:-}" == "--latest" ]]; then
    BACKUP_FILE=$(rclone ls "r2-nabu:${R2_BUCKET}/" | sort -k2 | tail -1 | awk '{print $2}')
    echo "Latest backup: $BACKUP_FILE"
elif [[ -n "${1:-}" ]]; then
    BACKUP_FILE="$1"
else
    list_backups
    exit 1
fi

mkdir -p "$BACKUP_DIR"

echo ""
echo "=========================================="
echo "!!! RESTORE DATABASE: ${DB_NAME} !!!"
echo "=========================================="
read -p "Type 'yes-restore-production' to continue: " CONFIRM

if [[ "$CONFIRM" != "yes-restore-production" ]]; then
    echo "Aborted"
    exit 1
fi

echo "[$(date)] Downloading backup..."
rclone copy "r2-nabu:${R2_BUCKET}/${BACKUP_FILE}" "${BACKUP_DIR}/" --progress

POSTGRES_CONTAINER=$(podman ps --format '{{.Names}}' 2>/dev/null | grep -E 'choresy.*postgres' | head -1)

if [[ -n "$POSTGRES_CONTAINER" ]]; then
    gpg --decrypt --batch --pinentry-mode loopback --passphrase-fd 3 3<<<"$BACKUP_ENCRYPTION_KEY" "${BACKUP_DIR}/${BACKUP_FILE}" \
        | gunzip \
        | podman exec -i -e PGPASSWORD="$DB_PASSWORD" "$POSTGRES_CONTAINER" \
            psql -U choresy -d "$DB_NAME"
else
    gpg --decrypt --batch --pinentry-mode loopback --passphrase-fd 3 3<<<"$BACKUP_ENCRYPTION_KEY" "${BACKUP_DIR}/${BACKUP_FILE}" \
        | gunzip \
        | PGPASSWORD="$DB_PASSWORD" psql \
            -h "${DB_HOST:-localhost}" \
            -p "${DB_PORT:-5432}" \
            -U "${DB_USER:-choresy}" \
            -d "$DB_NAME"
fi

rm -f "${BACKUP_DIR}/${BACKUP_FILE}"
echo "[$(date)] Restore completed successfully"
