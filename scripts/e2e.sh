#!/bin/sh
# End-to-end test runner for Choresy
set -e

echo "=== E2E: Starting stack ==="
podman compose up -d --build
./scripts/wait-for-stack.sh

echo "=== E2E: Running tests ==="
go test -race ./...

echo "=== E2E: Done ==="
