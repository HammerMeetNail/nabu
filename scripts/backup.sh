#!/bin/bash
# Secondary encrypted logical backup. Continuous WAL recovery is configured
# separately; adopting it must not silently disable this existing daily backup.
set -euo pipefail
umask 077
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-/opt/nabu/.env}"
if [[ -f "$ENV_FILE" ]]; then
    set -a
    source "$ENV_FILE"
    set +a
fi
export RCLONE_CONFIG="${RCLONE_CONFIG:-$HOME/.config/rclone/rclone-nabu.conf}"
if [[ -f "$SCRIPT_DIR/notify-email.sh" ]]; then source "$SCRIPT_DIR/notify-email.sh"; fi
: "${DB_PASSWORD:?DB_PASSWORD is required}"
: "${BACKUP_ENCRYPTION_KEY:?BACKUP_ENCRYPTION_KEY is required}"
DB_USER="${DB_USER:-nabu}"
DB_NAME="${DB_NAME:-nabu}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
# Explicit configured source. Never select the first matching container.
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-nabu_postgres_1}"
BACKUP_REMOTE="${BACKUP_REMOTE:-r2-nabu:${R2_BUCKET:-nabu-app-backups}}"
BACKUP_STATUS_DIR="${BACKUP_STATUS_DIR:-/opt/nabu/state}"
mkdir -p "$BACKUP_STATUS_DIR"
exec 9>"$BACKUP_STATUS_DIR/logical-backup.lock"
flock -n 9 || exit 0
scratch="$(mktemp -d -t nabu-encrypted-backup.XXXXXXXX)"
step=prepare
cleanup() {
    local result=$?
    rm -rf -- "$scratch"
    if [[ $result -ne 0 ]]; then
        printf '{"status":"failed","stage":"%s"}\n' "$step" >&2
        # Preserve the existing configured alert channel, with classified output.
        if declare -F notify_email >/dev/null; then
            notify_email "Nabu backup failed" "Encrypted logical backup failed at stage: $step. Inspect backup freshness and recovery status." >/dev/null 2>&1 || true
        fi
    fi
    exit "$result"
}
trap cleanup EXIT
backup_file="nabu_$(date -u +%Y%m%d_%H%M%S)_$(od -An -N4 -tx1 /dev/urandom | tr -d " \n").sql.gz.gpg"
mkdir -m 700 "$scratch/gnupg"
export PGPASSWORD="$DB_PASSWORD"
step=dump_encrypt
if [[ "$DB_HOST" == localhost || "$DB_HOST" == 127.0.0.1 ]]; then
    dump=(podman exec --env PGPASSWORD "$POSTGRES_CONTAINER" pg_dump -U "$DB_USER" -d "$DB_NAME" --format=plain --no-owner --no-acl)
else
    dump=(pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" --format=plain --no-owner --no-acl)
fi
# No plaintext scratch file, secret command-line argument or raw SQL/provider log.
timeout -k 5 1800 "${dump[@]}" 2>/dev/null \
    | gzip 2>/dev/null \
    | timeout -k 5 1800 gpg --no-options --homedir "$scratch/gnupg" --symmetric --cipher-algo AES256 --batch --no-symkey-cache --pinentry-mode loopback --passphrase-fd 3 3<<<"$BACKUP_ENCRYPTION_KEY" 2>/dev/null \
    > "$scratch/$backup_file"
[[ -s "$scratch/$backup_file" ]]
step=upload
timeout -k 5 1200 rclone copyto "$scratch/$backup_file" "$BACKUP_REMOTE/$backup_file" --contimeout=10s --timeout=60s --retries=2 >/dev/null 2>&1
step=record_success
status_tmp="$(mktemp "$BACKUP_STATUS_DIR/.logical-backup.XXXXXXXX")"
printf '{"status":"passed","completed_at":%s}\n' "$(date -u +%s)" > "$status_tmp"
mv -- "$status_tmp" "$BACKUP_STATUS_DIR/logical-backup.json"
printf '{"status":"passed","format":"encrypted logical dump"}\n'
