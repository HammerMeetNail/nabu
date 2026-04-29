#!/bin/sh
# End-to-end test runner for Choresy
set -e

echo "=== E2E: Starting stack ==="
if command -v podman > /dev/null 2>&1; then
  podman compose up -d --build
else
  docker compose up -d --build
fi
./scripts/wait-for-stack.sh

echo "=== E2E: Running Playwright tests ==="
pnpm exec playwright test --config playwright.config.js "$@"

EXIT=$?
echo "=== E2E: Done (exit $EXIT) ==="
exit $EXIT
