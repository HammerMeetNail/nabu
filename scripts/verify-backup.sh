#!/bin/bash
# Verify the latest backup can be restored
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-/opt/choresy/.env}"

if [[ -f "$ENV_FILE" ]]; then
    set -a
    source "$ENV_FILE"
    set +a
fi

BACKUP_DIR="/tmp/choresy-backups"
R2_BUCKET="${R2_BUCKET:-choresy-backups}"
TEST_CONTAINER="choresy-backup-verify"
TEST_DB_PASSWORD="verify_$(date +%s)"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

if [[ -z "${BACKUP_ENCRYPTION_KEY:-}" ]]; then
    echo "ERROR: BACKUP_ENCRYPTION_KEY is required"
    exit 1
fi

cleanup() {
    podman rm -f "$TEST_CONTAINER" 2>/dev/null || true
    rm -f "${BACKUP_DIR}/"*.sql 2>/dev/null || true
    rm -f "${BACKUP_DIR}/"*.gpg 2>/dev/null || true
}
trap cleanup EXIT

BACKUP_FILE=$(rclone ls "r2:${R2_BUCKET}/" 2>/dev/null | grep -E '\.sql\.gz\.gpg$' | sort -k2 | tail -1 | awk '{print $2}')

if [[ -z "$BACKUP_FILE" ]]; then
    echo "ERROR: No backup files found"
    exit 1
fi

echo "[$(date)] Verifying: ${BACKUP_FILE}"
mkdir -p "$BACKUP_DIR"

echo "[$(date)] Downloading..."
rclone copy "r2:${R2_BUCKET}/${BACKUP_FILE}" "${BACKUP_DIR}/" || exit 1

echo "[$(date)] Decrypting..."
gpg --decrypt --batch --pinentry-mode loopback --passphrase-fd 3 3<<<"$BACKUP_ENCRYPTION_KEY" "${BACKUP_DIR}/${BACKUP_FILE}" \
    | gunzip > "${BACKUP_DIR}/verify_restore.sql" || exit 1

SQL_SIZE=$(stat -c%s "${BACKUP_DIR}/verify_restore.sql" 2>/dev/null)
if [[ "$SQL_SIZE" -lt 1000 ]]; then
    echo "ERROR: Decrypted SQL file too small (${SQL_SIZE} bytes)"
    exit 1
fi

echo "[$(date)] Starting test container..."
podman run -d \
    --name "$TEST_CONTAINER" \
    -e POSTGRES_USER=choresy \
    -e POSTGRES_PASSWORD="$TEST_DB_PASSWORD" \
    -e POSTGRES_DB=choresy \
    docker.io/library/postgres:17-alpine

for i in {1..60}; do
    if podman exec -e PGPASSWORD="$TEST_DB_PASSWORD" "$TEST_CONTAINER" \
        psql -U choresy -d choresy -c "SELECT 1" &>/dev/null; then
        break
    fi
    if [[ $i -eq 60 ]]; then
        echo "ERROR: Test container failed to become ready"
        exit 1
    fi
    sleep 1
done

echo "[$(date)] Restoring..."
podman exec -i -e PGPASSWORD="$TEST_DB_PASSWORD" "$TEST_CONTAINER" \
    psql -U choresy -d choresy -v ON_ERROR_STOP=1 \
    < "${BACKUP_DIR}/verify_restore.sql" || exit 1

echo "[$(date)] Validation passed"
