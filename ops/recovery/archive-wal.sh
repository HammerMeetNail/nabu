#!/bin/sh
# A segment is acknowledged only after encrypted repository storage succeeds.
# Never turn a full archive queue into successful data loss or print provider
# diagnostics, credentials or signed URLs in the PostgreSQL log.
set -eu
if timeout -s TERM -k 5 120 pgbackrest --stanza=nabu --log-level-console=off --log-level-file=off archive-push "$1" >/dev/null 2>&1; then
    exit 0
else
    archive_status=$?
    echo "nabu WAL archive failed status=$archive_status" >&2
    exit "$archive_status"
fi
