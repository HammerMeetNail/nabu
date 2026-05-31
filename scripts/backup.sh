#!/bin/bash
# PostgreSQL backup to Cloudflare R2
# Usage: ./scripts/backup.sh
#
# Required environment variables (or in /opt/nabu/.env):
#   DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
#   BACKUP_ENCRYPTION_KEY - GPG passphrase for encrypting backups
#
# Requires rclone configured with R2 remote named "r2"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-/opt/nabu/.env}"
HOSTNAME="$(hostname)"

# Load environment if available
if [[ -f "$ENV_FILE" ]]; then
    set -a
    source "$ENV_FILE"
    set +a
fi

# Optional email notifications (Resend)
if [[ -f "${SCRIPT_DIR}/notify-email.sh" ]]; then
    source "${SCRIPT_DIR}/notify-email.sh"
fi

# Configuration
BACKUP_DIR="/tmp/nabu-backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="nabu_${TIMESTAMP}.sql.gz.gpg"
R2_BUCKET="${R2_BUCKET:-nabu-app-backups}"
CURRENT_STEP="init"

# Database connection
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-choresy}"
DB_NAME="${DB_NAME:-choresy}"

# Notify on failure (best-effort)
on_error() {
    local exit_code=$?
    set +e
    local when
    when="$(date '+%Y-%m-%d %H:%M:%S %Z')"
    if declare -F notify_email &>/dev/null; then
        notify_email "Nabu backup FAILED (${HOSTNAME})" \
            "Timestamp: ${when}
Hostname: ${HOSTNAME}
Backup file: ${BACKUP_FILE}
Step: ${CURRENT_STEP}
Exit code: ${exit_code}
Bucket: ${R2_BUCKET}

Action required: Check backup system immediately." || true
    fi
    exit "$exit_code"
}
trap on_error ERR

# Validation
if [[ -z "${DB_PASSWORD:-}" ]]; then
    echo "ERROR: DB_PASSWORD is required"
    exit 1
fi

if [[ -z "${BACKUP_ENCRYPTION_KEY:-}" ]]; then
    echo "ERROR: BACKUP_ENCRYPTION_KEY is required"
    exit 1
fi

if ! command -v rclone &> /dev/null; then
    echo "ERROR: rclone is not installed"
    exit 1
fi

mkdir -p "$BACKUP_DIR"

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Starting backup..."
CURRENT_STEP="dump"

# For containerized PostgreSQL, connect via podman if localhost
if [[ "$DB_HOST" == "localhost" ]] || [[ "$DB_HOST" == "127.0.0.1" ]]; then
    POSTGRES_CONTAINER=$(podman ps --format '{{.Names}}' 2>/dev/null | grep -E 'choresy.*postgres' | head -1)

    if [[ -n "$POSTGRES_CONTAINER" ]]; then
        echo "Using podman exec for database dump (container: $POSTGRES_CONTAINER)..."
        podman exec -e PGPASSWORD="$DB_PASSWORD" "$POSTGRES_CONTAINER" \
            pg_dump -U "$DB_USER" -d "$DB_NAME" --format=plain --no-owner --no-acl \
            | gzip \
            | gpg --symmetric --cipher-algo AES256 --batch --pinentry-mode loopback --passphrase-fd 3 3<<<"$BACKUP_ENCRYPTION_KEY" \
            > "${BACKUP_DIR}/${BACKUP_FILE}"
    else
        # Direct connection
        PGPASSWORD="$DB_PASSWORD" pg_dump \
            -h "$DB_HOST" \
            -p "$DB_PORT" \
            -U "$DB_USER" \
            -d "$DB_NAME" \
            --format=plain \
            --no-owner \
            --no-acl \
            | gzip \
            | gpg --symmetric --cipher-algo AES256 --batch --pinentry-mode loopback --passphrase-fd 3 3<<<"$BACKUP_ENCRYPTION_KEY" \
            > "${BACKUP_DIR}/${BACKUP_FILE}"
    fi
else
    # Remote database connection
    PGPASSWORD="$DB_PASSWORD" pg_dump \
        -h "$DB_HOST" \
        -p "$DB_PORT" \
        -U "$DB_USER" \
        -d "$DB_NAME" \
        --format=plain \
        --no-owner \
        --no-acl \
        | gzip \
        | gpg --symmetric --cipher-algo AES256 --batch --pinentry-mode loopback --passphrase-fd 3 3<<<"$BACKUP_ENCRYPTION_KEY" \
        > "${BACKUP_DIR}/${BACKUP_FILE}"
fi

BACKUP_SIZE=$(du -h "${BACKUP_DIR}/${BACKUP_FILE}" | cut -f1)
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Backup created: ${BACKUP_FILE} (${BACKUP_SIZE})"

# Upload to R2
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Uploading to R2..."
CURRENT_STEP="upload"
rclone copy "${BACKUP_DIR}/${BACKUP_FILE}" "r2-nabu:${R2_BUCKET}/" --progress

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Upload complete"
CURRENT_STEP="cleanup"

# Clean up local file
rm -f "${BACKUP_DIR}/${BACKUP_FILE}"

# Note: R2 lifecycle policy handles retention (30 days protected, auto-delete at 31 days)

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Backup completed successfully"
if declare -F notify_email &>/dev/null; then
    if [[ "${BACKUP_NOTIFY_SUCCESS:-1}" != "0" ]]; then
        notify_email "Nabu backup SUCCEEDED (${HOSTNAME})" \
            "Timestamp: $(date '+%Y-%m-%d %H:%M:%S %Z')
Hostname: ${HOSTNAME}
Backup file: ${BACKUP_FILE}
Size: ${BACKUP_SIZE}
Bucket: ${R2_BUCKET}

Status: OK" || true
    fi
fi
