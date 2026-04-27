#!/bin/sh
# Wait for the Choresy stack to be healthy
# polls /health up to 30 times with 1s interval

HOST="${1:-localhost:8080}"
MAX_TRIES=30
ATTEMPT=0

until [ $ATTEMPT -ge $MAX_TRIES ]; do
  if curl -sf "http://${HOST}/health" > /dev/null 2>&1; then
    echo "Stack is ready."
    exit 0
  fi
  ATTEMPT=$((ATTEMPT + 1))
  sleep 1
done

echo "Stack failed to become ready after ${MAX_TRIES} attempts." >&2
exit 1
