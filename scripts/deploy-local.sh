#!/usr/bin/env bash
# Run on the deployment host after copying compose.next.yaml and writing .env.
# Keep the previous digest and environment available for an application rollback.
# Migrations must remain backward compatible; this never restores a database.
set -euo pipefail
deploy_scripts="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
deploy_ready_url="${DEPLOY_READY_URL:-http://127.0.0.1:8082/ready}"
deploy_timeout="${DEPLOY_READY_TIMEOUT:-90}"
deploy_pull_timeout="${DEPLOY_PULL_TIMEOUT:-300}"
deploy_start_timeout="${DEPLOY_START_TIMEOUT:-120}"
deploy_kill_grace="${DEPLOY_KILL_GRACE:-5}"
[[ -f compose.next.yaml ]] || { echo "Missing candidate compose file" >&2; exit 2; }
for deploy_bound in "$deploy_timeout" "$deploy_pull_timeout" "$deploy_start_timeout" "$deploy_kill_grace"; do
    [[ "$deploy_bound" =~ ^[1-9][0-9]*$ ]] && (( deploy_bound <= 600 )) || exit 2
done
umask 077
if [[ -f compose.yaml ]]; then
    cp compose.yaml compose.previous.yaml
else
    rm -f compose.previous.yaml
fi

# A pull failure leaves the running application untouched.
if ! timeout --kill-after="${deploy_kill_grace}s" "${deploy_pull_timeout}s" podman-compose -f compose.next.yaml pull app; then
    if [[ -f .env.previous ]]; then cp .env.previous .env; fi
    echo "Image pull failed; running application retained" >&2
    exit 1
fi
mv compose.next.yaml compose.yaml
if timeout --kill-after="${deploy_kill_grace}s" "${deploy_start_timeout}s" podman-compose -f compose.yaml up -d --no-deps app && \
    "$deploy_scripts/wait-ready.sh" "$deploy_ready_url" "$deploy_timeout"; then
    echo "Deployment passed readiness"
    exit 0
fi

echo "Deployment failed readiness; attempting previous application digest" >&2
if [[ -f compose.previous.yaml ]]; then
    cp compose.previous.yaml compose.yaml
    if [[ -f .env.previous ]]; then cp .env.previous .env; fi
    if timeout --kill-after="${deploy_kill_grace}s" "${deploy_start_timeout}s" podman-compose -f compose.yaml up -d --no-deps app && \
        "$deploy_scripts/wait-ready.sh" "$deploy_ready_url" "$deploy_timeout"; then
        echo "Previous application is ready; deployment remains failed" >&2
        exit 1
    fi
fi
echo "Rollback did not become ready. Escalate using docs/deploy-runbook.md; do not restore over the live database." >&2
exit 2
