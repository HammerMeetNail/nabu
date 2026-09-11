#!/bin/sh
# Wait for the Choresy stack to be healthy
# Wait on the same bounded dependency readiness probe used for deployment.

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
exec bash "$SCRIPT_DIR/wait-ready.sh" "http://${1:-localhost:8080}/ready" 30
