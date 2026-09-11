#!/bin/bash
# Opt-in recovery configuration survives application deployments. This command
# never restores into an existing database or reads the application .env file.
set -euo pipefail
umask 077
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RECOVERY_ENV_FILE="${RECOVERY_ENV_FILE:-/etc/nabu/recovery.env}"
if [[ -f "$RECOVERY_ENV_FILE" ]]; then
    set -a
    source "$RECOVERY_ENV_FILE"
    set +a
fi
: "${RECOVERY_POSTGRES_IMAGE:?Set the verified source-compatible PostgreSQL image in recovery.env}"
: "${RECOVERY_APP_IMAGE:?Set the application image digest in recovery.env}"
: "${RECOVERY_BACKUP_REMOTE:?Set the encrypted backup remote in recovery.env}"
: "${RECOVERY_KEY_FILE:?Set the private off-site escrowed key file in recovery.env}"
RECOVERY_STATUS_DIR="${RECOVERY_STATUS_DIR:-/opt/nabu/state}"
if [[ "${1:-}" == --check-config ]]; then
    : "${RECOVERY_HEARTBEAT_URL:?Configure the recovery alert/deadman receiver}"
    python3 - "$RECOVERY_KEY_FILE" "$RECOVERY_STATUS_DIR/recovery-status.json" <<'PY'
import datetime as dt, json, pathlib, sys
key, path = map(pathlib.Path, sys.argv[1:])
try:
    state = json.loads(path.read_text())
    age = (dt.datetime.now(dt.timezone.utc) - dt.datetime.fromisoformat(state['checked_at'])).total_seconds()
    valid = (key.is_file() and not key.is_symlink() and key.stat().st_mode & 0o077 == 0
             and state['status'] == 'passed' and state.get('heartbeat_delivered') is True and 0 <= age < 300)
except (OSError, ValueError, KeyError, TypeError):
    valid = False
raise SystemExit(0 if valid else 1)
PY
    exit
fi
mkdir -p "$RECOVERY_STATUS_DIR"
exec 9>"$RECOVERY_STATUS_DIR/verify-backup.lock"
flock -n 9 || exit 0
report="$RECOVERY_STATUS_DIR/restore-$(date -u +%Y%m%d_%H%M%S)-$$.json"
exec python3 "$SCRIPT_DIR/recovery.py" restore-dump \
    --remote "$RECOVERY_BACKUP_REMOTE" --key-file "$RECOVERY_KEY_FILE" \
    --image "$RECOVERY_POSTGRES_IMAGE" --app-image "$RECOVERY_APP_IMAGE" \
    --max-age-hours 26 --report "$report"
