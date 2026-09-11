#!/usr/bin/env bash
# Bounded local/deployment readiness assertion. Never print response bodies.
set -euo pipefail
ready_url="${1:-http://127.0.0.1:8080/ready}"
ready_seconds="${2:-90}"
if [[ ! "$ready_seconds" =~ ^[1-9][0-9]*$ ]] || (( ready_seconds > 600 )); then
    echo "Readiness timeout must be 1..600 seconds" >&2
    exit 2
fi
ready_deadline=$((SECONDS + ready_seconds))
while (( SECONDS < ready_deadline )); do
    if curl --fail --silent --output /dev/null --connect-timeout 1 --max-time 2 "$ready_url"; then
        echo "Application is ready"
        exit 0
    fi
    sleep 1
done
echo "Application readiness deadline exceeded" >&2
exit 1
