#!/bin/bash
set -euo pipefail
umask 077
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RECOVERY_ENV_FILE="${RECOVERY_ENV_FILE:-/etc/nabu/recovery.env}"
if [[ -f "$RECOVERY_ENV_FILE" ]]; then
    set -a
    source "$RECOVERY_ENV_FILE"
    set +a
fi
exec python3 "$SCRIPT_DIR/recovery-status.py" "$@"
