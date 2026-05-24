#!/bin/sh
# End-to-end test runner for Choresy
set -e

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

echo "=== E2E: Starting stack ==="
if [ "${CI}" = "true" ]; then
  # CI: run app with in-memory stores, Mailpit as service container
  SMTP_HOST=localhost SMTP_PORT=1025 RATE_LIMIT_AUTH_MAX=1000 go run ./cmd/server &
  APP_PID=$!
  trap "kill ${APP_PID} 2>/dev/null" EXIT
else
  if command -v podman > /dev/null 2>&1; then
    podman compose up -d --build
  else
    docker compose up -d --build
  fi
fi
./scripts/wait-for-stack.sh

echo "=== E2E: Running Playwright tests ==="
if [ -x "$ROOT_DIR/node_modules/.bin/playwright" ]; then
  "$ROOT_DIR/node_modules/.bin/playwright" test --config playwright.config.js "$@"
elif [ -x "$ROOT_DIR/../../node_modules/.bin/playwright" ]; then
  "$ROOT_DIR/../../node_modules/.bin/playwright" test --config playwright.config.js "$@"
else
  pnpm exec playwright test --config playwright.config.js "$@"
fi

EXIT=$?
echo "=== E2E: Done (exit $EXIT) ==="
exit $EXIT
