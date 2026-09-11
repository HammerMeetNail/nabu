#!/bin/bash
# No environment file, live target or container discovery. All source/image/key
# arguments are explicit; recovery.py always creates its own isolated database.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$SCRIPT_DIR/recovery.py" restore-dump "$@"
