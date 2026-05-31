#!/bin/bash
# Verify the latest backup can be restored
# Usage: ./scripts/verify-backup.sh
#
# Runs daily after backup to verify integrity.
# On failure, writes error details to R2 bucket.
#
# Required environment variables (or in /opt/nabu/.env):
#   BACKUP_ENCRYPTION_KEY - GPG passphrase used when creating backup

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

# Use dedicated rclone config to avoid conflicts with yearofbingo
export RCLONE_CONFIG="$HOME/.config/rclone/rclone-nabu.conf"

# Optional email notifications (Resend)
if [[ -f "${SCRIPT_DIR}/notify-email.sh" ]]; then
    source "${SCRIPT_DIR}/notify-email.sh"
fi

# Configuration
BACKUP_DIR="/tmp/nabu-backups"
R2_BUCKET="${R2_BUCKET:-nabu-app-backups}"
TEST_CONTAINER="nabu-backup-verify"
TEST_DB_PASSWORD="verify_password_$(date +%s)"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
ERROR_FILE="BACKUP_VERIFICATION_FAILED_${TIMESTAMP}.txt"
TEST_DB_NAME=""

# Validation
if [[ -z "${BACKUP_ENCRYPTION_KEY:-}" ]]; then
    echo "ERROR: BACKUP_ENCRYPTION_KEY is required"
    exit 1
fi

if ! command -v rclone &> /dev/null; then
    echo "ERROR: rclone is not installed"
    exit 1
fi

if ! command -v podman &> /dev/null; then
    echo "ERROR: podman is not installed"
    exit 1
fi

# Write error to R2
write_error() {
    local error_message="$1"
    local error_file="/tmp/${ERROR_FILE}"

    if declare -F notify_email &>/dev/null; then
        local safe_error
        safe_error="$(printf '%s' "$error_message" | head -n 1)"
        notify_email "Nabu backup verification FAILED (${HOSTNAME})" \
            "Timestamp: $(date '+%Y-%m-%d %H:%M:%S %Z')
Hostname: ${HOSTNAME}
Backup file: ${BACKUP_FILE:-unknown}
Test database: ${TEST_DB_NAME:-unknown}
Bucket: ${R2_BUCKET}
Report file: ${ERROR_FILE}

Error: ${safe_error}

Action required: Check backup system immediately." || true
    fi

    cat > "$error_file" << EOF
BACKUP VERIFICATION FAILED
==========================
Timestamp: $(date '+%Y-%m-%d %H:%M:%S %Z')
Hostname: ${HOSTNAME}
Backup file: ${BACKUP_FILE:-unknown}
Test database: ${TEST_DB_NAME:-unknown}

Error:
${error_message}

Action required: Check backup system immediately.
EOF

    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Writing error report to R2..."
    rclone copy "$error_file" "r2-nabu:${R2_BUCKET}/" 2>/dev/null || true
    rm -f "$error_file"

    echo "ERROR: $error_message"
    exit 1
}

# Cleanup function
cleanup() {
    podman rm -f "$TEST_CONTAINER" 2>/dev/null || true
    rm -f "${BACKUP_DIR}/${BACKUP_FILE:-}" 2>/dev/null || true
    rm -f "${BACKUP_DIR}/verify_restore.sql" 2>/dev/null || true
    rm -f "${BACKUP_DIR}/verify_restore_psql.log" 2>/dev/null || true
    rm -f "${BACKUP_DIR}/verify_decrypt.log" 2>/dev/null || true
}
trap cleanup EXIT

# Get latest backup
BACKUP_FILE=$(rclone ls "r2-nabu:${R2_BUCKET}/" 2>/dev/null | grep -E '\.sql\.gz\.gpg$' | sort -k2 | tail -1 | awk '{print $2}')

if [[ -z "$BACKUP_FILE" ]]; then
    write_error "No backup files found in r2-nabu:${R2_BUCKET}/"
fi

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Verifying backup: ${BACKUP_FILE}"

mkdir -p "$BACKUP_DIR"

# Step 1: Download
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Downloading backup..."
if ! rclone copy "r2-nabu:${R2_BUCKET}/${BACKUP_FILE}" "${BACKUP_DIR}/" 2>&1; then
    write_error "Failed to download backup from R2"
fi

# Step 2: Decrypt
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Decrypting backup..."
DECRYPT_LOG="${BACKUP_DIR}/verify_decrypt.log"
if ! gpg --decrypt --batch --pinentry-mode loopback --passphrase-fd 3 3<<<"$BACKUP_ENCRYPTION_KEY" "${BACKUP_DIR}/${BACKUP_FILE}" 2>"$DECRYPT_LOG" \
    | gunzip > "${BACKUP_DIR}/verify_restore.sql" 2>>"$DECRYPT_LOG"; then
    write_error "Failed to decrypt/decompress backup. Encryption key may be wrong or file corrupted.

Decrypt output (tail):
$(tail -n 80 "$DECRYPT_LOG" 2>/dev/null || true)"
fi

SQL_SIZE=$(stat -f%z "${BACKUP_DIR}/verify_restore.sql" 2>/dev/null || stat -c%s "${BACKUP_DIR}/verify_restore.sql" 2>/dev/null)
if [[ "$SQL_SIZE" -lt 1000 ]]; then
    write_error "Decrypted SQL file too small (${SQL_SIZE} bytes). Backup may be corrupted."
fi

# Derive database name from dump (pg_dump includes a leading \connect).
TEST_DB_NAME="$(
    awk '
        match($0, /^\\\\connect( -reuse-previous=on)?[[:space:]]+\"?([^\"[:space:]]+)\"?/, m) { print m[2]; exit }
    ' "${BACKUP_DIR}/verify_restore.sql" 2>/dev/null || true
)"
TEST_DB_NAME="${TEST_DB_NAME:-choresy}"

# Step 3: Start test container
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Starting test PostgreSQL container..."
if ! podman run -d \
    --name "$TEST_CONTAINER" \
    -e POSTGRES_USER=choresy \
    -e POSTGRES_PASSWORD="$TEST_DB_PASSWORD" \
    -e POSTGRES_DB="$TEST_DB_NAME" \
    docker.io/library/postgres:16-alpine 2>&1; then
    write_error "Failed to start test PostgreSQL container"
fi

# Wait for PostgreSQL (pg_isready can report ready during init/restart)
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Waiting for PostgreSQL..."
for i in {1..60}; do
    if podman exec -e PGPASSWORD="$TEST_DB_PASSWORD" "$TEST_CONTAINER" \
        psql -U choresy -d "$TEST_DB_NAME" -c "SELECT 1" &>/dev/null; then
        break
    fi
    if [[ $i -eq 60 ]]; then
        POSTGRES_LOG_TAIL="$(podman logs --tail 120 "$TEST_CONTAINER" 2>&1 || true)"
        write_error "Test PostgreSQL container failed to become ready

postgres logs (tail):
${POSTGRES_LOG_TAIL}"
    fi
    sleep 1
done

# Step 4: Restore
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Restoring to test container..."
if ! podman exec -i -e PGPASSWORD="$TEST_DB_PASSWORD" "$TEST_CONTAINER" \
    psql -U choresy -d "$TEST_DB_NAME" -v ON_ERROR_STOP=1 -v VERBOSITY=terse -v SHOW_CONTEXT=never \
    < "${BACKUP_DIR}/verify_restore.sql" > "${BACKUP_DIR}/verify_restore_psql.log" 2>&1; then
    POSTGRES_LOG_TAIL="$(podman logs --tail 120 "$TEST_CONTAINER" 2>&1 || true)"
    write_error "Failed to restore backup to test database

psql output (tail):
$(tail -n 120 "${BACKUP_DIR}/verify_restore_psql.log" 2>/dev/null || true)

postgres logs (tail):
${POSTGRES_LOG_TAIL}"
fi

# Step 5: Validate
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Validating data..."
USER_COUNT=$(podman exec -e PGPASSWORD="$TEST_DB_PASSWORD" "$TEST_CONTAINER" \
    psql -U choresy -d "$TEST_DB_NAME" -t -c "SELECT COUNT(*) FROM users;" 2>/dev/null | tr -d ' \n')

if [[ -z "$USER_COUNT" ]] || [[ "$USER_COUNT" -lt 0 ]]; then
    write_error "Failed to query users table - restore may have failed"
fi

CHORE_COUNT=$(podman exec -e PGPASSWORD="$TEST_DB_PASSWORD" "$TEST_CONTAINER" \
    psql -U choresy -d "$TEST_DB_NAME" -t -c "SELECT COUNT(*) FROM chores;" 2>/dev/null | tr -d ' \n')

echo ""
echo "=========================================="
echo "BACKUP VERIFICATION PASSED"
echo "=========================================="
echo "Backup: ${BACKUP_FILE}"
echo "Database: ${TEST_DB_NAME}"
echo "Users: ${USER_COUNT}"
echo "Chores: ${CHORE_COUNT}"
echo "Verified: $(date '+%Y-%m-%d %H:%M:%S %Z')"
echo "=========================================="

if declare -F notify_email &>/dev/null; then
    if [[ "${BACKUP_NOTIFY_VERIFY_SUCCESS:-0}" == "1" ]]; then
        notify_email "Nabu backup verification SUCCEEDED (${HOSTNAME})" \
            "Timestamp: $(date '+%Y-%m-%d %H:%M:%S %Z')
Hostname: ${HOSTNAME}
Backup: ${BACKUP_FILE}
Test database: ${TEST_DB_NAME}
Users: ${USER_COUNT}
Chores: ${CHORE_COUNT}

Status: OK" || true
    fi
fi
