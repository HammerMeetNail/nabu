#!/usr/bin/env bash
# Pre-push hook: fast local checks before pushing.
#
#   1. go build ./...        compile errors
#   2. go vet ./...          suspicious code
#   3. make test-go          broken unit tests
#   4. make test-js          broken frontend tests (when node deps are installed)
#   5. parity gate           the same coupling rule the CI `parity` job enforces
#
# Bypasses:
#   SKIP_PARITY=1 git push   skip the parity gate
#   SKIP_FAST=1   git push   skip checks 1-4 (useful for a parity dry run)
#
# This file is the single source of truth: `make hooks` installs a thin wrapper
# that execs it, so an installed hook can never drift behind the repo copy.

set -euo pipefail

RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

PWA_PATHS="web/static/js/ web/static/css/ web/templates/"
IOS_PATHS="ios/"
API_PATHS="internal/handlers/"
MATRIX="docs/plans/client-parity.md"
ZERO="0000000000000000000000000000000000000000"

# ── 5. Parity gate ──────────────────────────────────────────────────────────
#
# Mirrors the CI `parity` job: the matrix must always be well-formed, and a
# push that changes the shared client surface (PWA client code, the iOS app, or
# shared API handlers) must also update the matrix, which forces a conscious
# "does the other client need this?" decision. It deliberately does NOT ask for
# any particular wording in a PR description — CI stopped checking phrases long
# ago, and a local hook cannot read the PR body anyway.

matches_any() {
  local file="$1" prefix
  for prefix in $2; do
    case "$file" in
      "$prefix"*) return 0 ;;
    esac
  done
  return 1
}

# classify_gate <newline-separated changed paths> -> sets GATE_* results
classify_gate() {
  GATE_HAS_CLIENT=0
  GATE_HAS_MATRIX=0
  GATE_CLIENTS=""
  local file
  while IFS= read -r file; do
    [ -z "$file" ] && continue
    if matches_any "$file" "$PWA_PATHS"; then
      GATE_HAS_CLIENT=1
      GATE_CLIENTS="$GATE_CLIENTS pwa"
    elif matches_any "$file" "$IOS_PATHS"; then
      GATE_HAS_CLIENT=1
      GATE_CLIENTS="$GATE_CLIENTS ios"
    elif matches_any "$file" "$API_PATHS"; then
      GATE_HAS_CLIENT=1
      GATE_CLIENTS="$GATE_CLIENTS api"
    fi
    [ "$file" = "$MATRIX" ] && GATE_HAS_MATRIX=1
  done <<<"$1"
  GATE_CLIENTS="${GATE_CLIENTS# }"
  return 0
}

parity_gate() {
  local changed="$1"
  bash "${SCRIPT_DIR}/check-parity.sh" || {
    echo ""
    echo "❌ Parity matrix is malformed (unknown status or bad row)."
    exit 1
  }

  classify_gate "$changed"

  if [ "$GATE_HAS_CLIENT" -eq 0 ]; then
    return 0
  fi

  # Advisory only: touching one client is usually fine; what is actually
  # required is the matrix update that records the decision.
  if [ "$GATE_HAS_CLIENT" -eq 1 ]; then
    echo "  client surface changed:$GATE_CLIENTS"
  fi

  if [ "$GATE_HAS_MATRIX" -eq 0 ]; then
    echo ""
    echo -e "${RED}❌ Shared client surface changed but $MATRIX was not.${NC}"
    echo ""
    echo "   Changed:$(echo "$GATE_CLIENTS" | tr ' ' ' ')"
    echo "   CI's parity job fails on this push. Update the matrix row for the"
    echo "   feature (and use an existing status: Done, Built, iOS pending,"
    echo "   PWA pending, Deferred, Not built, N/A)."
    echo ""
    echo "   Genuinely no matrix impact? Add 'no-parity-update: <reason>' to the"
    echo -e "   PR body, or bypass this hook with ${RED}SKIP_PARITY=1 git push${NC}."
    echo ""
    exit 1
  fi

  echo -e "${YELLOW}⚠ Client changes present — confirm the matrix row reflects both clients.${NC}"
  return 0
}

# ── Self test: `bash scripts/pre-push-hook.sh --self-test` ───────────────────
# Runs the classification rules without touching git, so the gate itself is
# checkable. CI never installs the hook, so this is the only automated check it
# has.

self_test() {
  local failures=0
  check() {
    local label="$1" changed="$2" want_client="$3" want_matrix="$4"
    classify_gate "$changed"
    if [ "$GATE_HAS_CLIENT" != "$want_client" ] || [ "$GATE_HAS_MATRIX" != "$want_matrix" ]; then
      echo "  FAIL $label: got client=$GATE_HAS_CLIENT matrix=$GATE_HAS_MATRIX, want client=$want_client matrix=$want_matrix"
      failures=$((failures + 1))
    else
      echo "  ok   $label"
    fi
  }
  echo "→ pre-push parity gate self test"
  check "iOS only, no matrix            " $'ios/Nabu/Views/X.swift' 1 0
  check "iOS only, matrix updated       " $'ios/Nabu/Views/X.swift\ndocs/plans/client-parity.md' 1 1
  check "PWA only, no matrix            " $'web/static/js/home.js' 1 0
  check "both clients, matrix updated   " $'web/static/js/home.js\nios/Nabu/X.swift\ndocs/plans/client-parity.md' 1 1
  check "shared handler, no matrix      " $'internal/handlers/log.go' 1 0
  check "docs only                      " $'README.md\ndocs/plans/other.md' 0 0
  check "css templates count as PWA     " $'web/templates/home.html' 1 0
  check "matrix row alone               " $'docs/plans/client-parity.md' 0 1
  if [ "$failures" -gt 0 ]; then
    echo ""
    echo "❌ $failures self-test case(s) failed."
    exit 1
  fi
  echo "All parity gate cases behaved as expected."
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# `--self-test` checks the parity gate's classification rules without git or the
# build, so it is usable as a quick unit check (and the only automated one, since
# CI never installs hooks).
if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit 0
fi

# ── 1-4. Fast checks ────────────────────────────────────────────────────────

if [ "${SKIP_FAST:-}" = "1" ]; then
  echo "→ build/vet/go tests/js tests skipped (SKIP_FAST=1)"
else
  echo "→ go build ./..."
  if ! go build ./... 2>&1; then
    echo ""
    echo "❌ Build failed. Fix compile errors before pushing."
    exit 1
  fi

  echo "→ go vet ./..."
  if ! go vet ./... 2>&1; then
    echo ""
    echo "❌ go vet failed. Fix issues before pushing."
    exit 1
  fi

  echo "→ make test-go"
  if ! make test-go 2>&1; then
    echo ""
    echo "❌ Go tests failed. Fix tests before pushing."
    exit 1
  fi

  if command -v node &>/dev/null && [ -d node_modules ]; then
    echo "→ make test-js"
    if ! make test-js 2>&1; then
      echo ""
      echo "❌ JS tests failed. Fix tests before pushing."
      exit 1
    fi
  else
    echo "→ make test-js (skipped: node or node_modules not found)"
  fi
fi

if [ "${SKIP_PARITY:-}" = "1" ]; then
  echo "→ parity gate skipped (SKIP_PARITY=1)"
  exit 0
fi

changed_all=""
refs_seen=0
while IFS=' ' read -r local_ref local_sha remote_ref remote_sha; do
  [ "$local_sha" = "$ZERO" ] && continue
  refs_seen=$((refs_seen + 1))
  if [ "$remote_sha" = "$ZERO" ]; then
    base=$(git merge-base origin/main "$local_sha" 2>/dev/null || echo "")
    [ -z "$base" ] && base=$(git rev-list --max-parents=0 "$local_sha")
  else
    base="$remote_sha"
  fi
  changed=$(git diff --name-only "$base".."$local_sha" 2>/dev/null || echo "")
  changed_all="${changed_all}${changed}
"
done

if [ "$refs_seen" -eq 0 ]; then
  exit 0
fi

parity_gate "$(printf '%s' "$changed_all" | sed '/^$/d')"
