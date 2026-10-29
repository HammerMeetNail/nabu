#!/usr/bin/env bash
# Signed-in smoke test of a deployed Nabu, with a throwaway account.
#
# Signs up a new account, sets up a household with the default activities,
# reads the main endpoints, logs an activity and deletes the log, signs out
# and back in, then deletes the account. If a step fails, the account is
# still deleted. Every run signs up a new address, so a run whose cleanup
# failed never blocks the next one. The sign-up sends that address one
# verification email.
#
#   scripts/prod-smoke.sh [base-url]      default https://nabu-app.com
#   SMOKE_EMAIL=me+x@example.org ...      choose the address
set -euo pipefail

BASE="${1:-https://nabu-app.com}"
EMAIL="${SMOKE_EMAIL:-verify+smoke-$(date -u +%Y%m%d%H%M%S)@yearofbingo.com}"
PASSWORD="$(python3 -I -c 'import secrets; print(secrets.token_hex(16))')"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/nabu-smoke.XXXXXX")"
JAR="$WORK/cookies"
BODY="$WORK/body"
CODE=""
CREATED=0
DELETED=0

csrf() { awk '$6 == "nabu_csrf" { v = $7 } END { print v }' "$JAR" 2>/dev/null; }
credentials() { printf '{"email":"%s","password":"%s"}' "$EMAIL" "$PASSWORD"; }
field() { python3 -I -c 'import json, sys; d = json.load(open(sys.argv[2])); print(eval(sys.argv[1], {"d": d}))' "$1" "$BODY"; }

# req METHOD PATH [JSON]: sets CODE and leaves the response body in $BODY.
req() {
  local args=(-sS -b "$JAR" -c "$JAR" -o "$BODY" -w '%{http_code}' -X "$1")
  if [ "$1" != GET ]; then args+=(-H "X-CSRF-Token: $(csrf)"); fi
  if [ -n "${3:-}" ]; then args+=(-H 'Content-Type: application/json' --data "$3"); fi
  CODE="$(curl "${args[@]}" "$BASE$2")"
}

# expect LABEL CODE...: fails the run unless CODE is one of those given.
expect() {
  local label="$1" ok
  shift
  for ok in "$@"; do
    if [ "$CODE" = "$ok" ]; then
      echo "ok    $label: $CODE"
      return 0
    fi
  done
  echo "FAIL  $label: $CODE, expected $*"
  head -c 500 "$BODY" 2>/dev/null || true
  echo
  exit 1
}

cleanup() {
  local status=$?
  if [ "$CREATED" = 1 ] && [ "$DELETED" = 0 ]; then
    req GET /login || true
    req POST /api/auth/login "$(credentials)" || true
    req DELETE /api/me '{"confirm":"DELETE","deleteLogs":true,"deleteNotes":true}' || true
    if [ "$CODE" = 200 ]; then
      echo "ok    cleanup: deleted $EMAIL"
    else
      echo "FAIL  cleanup: DELETE /api/me returned $CODE; delete $EMAIL by hand"
      status=1
    fi
  fi
  rm -rf "$WORK"
  exit "$status"
}
trap cleanup EXIT

echo "Smoke test of $BASE as $EMAIL"
req GET /login
expect "GET /login" 200

req POST /api/auth/register "$(credentials)"
expect "sign up" 201
CREATED=1

req GET /api/me
expect "GET /api/me" 200
req POST /api/household '{"name":"Smoke test"}'
expect "create household" 200 201
req POST /api/chores/seed-defaults
expect "seed activities" 200 201

req GET /api/chores
expect "GET /api/chores" 200
chore="$(field 'next((c["id"] for c in d["chores"] if c.get("metricType") in ("", "none") and not c.get("subjects") and c.get("visibility") != "admins"), "")')"
[ -n "$chore" ] || { echo "FAIL  no activity without details to log"; exit 1; }

hour="$(date +%H)"
req POST /api/logs "{\"choreId\":$chore,\"note\":\"smoke test\",\"hour\":$((10#$hour)),\"completedAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
expect "log an activity" 201
log="$(field 'd["log"]["id"]')"

for path in /api/household /api/logs/latest-per-chore /api/logs/today /api/notifications \
  '/api/stats/dashboard?read=overview&read=chores'; do
  req GET "$path"
  expect "GET $path" 200
done

req DELETE "/api/logs/$log"
expect "delete the log" 200 204

req POST /api/auth/logout
expect "sign out" 200 204
req GET /login
req POST /api/auth/login "$(credentials)"
expect "sign in" 200

req DELETE /api/me '{"confirm":"DELETE","deleteLogs":true,"deleteNotes":true}'
expect "delete the account" 200
DELETED=1

req GET /login
req POST /api/auth/login "$(credentials)"
expect "sign-in after deletion is refused" 401

echo "OK"
