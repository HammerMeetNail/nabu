#!/usr/bin/env bash
# Lint the client parity matrix and report PWA<->iOS parity status.
#
# Run locally:  bash scripts/check-parity.sh          # report + lint (default)
#               bash scripts/check-parity.sh --strict  # also fail on any gap
#
# Default mode is a MATRIX LINT: it verifies every row uses a known status and
# prints a tally. It does NOT fail merely because some features are still "iOS
# pending" — during active development that is expected, and a permanently-red
# gate just gets ignored (which is how the matrix drifted in the first place).
# Enforcement that the *right* PRs touch the matrix lives in the CI "parity"
# coupling check; mechanical model/API drift is caught by the iOS test lane.
#
# Strict mode (--strict or CHECK_PARITY_STRICT=1) restores the old behavior:
# exit 1 if any row is "iOS pending" or "PWA pending". Useful as a release gate.
#
# Exit codes:
#   0 - Matrix is well-formed (and, in --strict, fully at parity)
#   1 - (--strict only) one or more parity gaps remain
#   2 - Matrix missing or contains an unknown/typo status

set -euo pipefail

MATRIX="docs/plans/client-parity.md"
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m'

STRICT="${CHECK_PARITY_STRICT:-0}"
for arg in "$@"; do
  [ "$arg" = "--strict" ] && STRICT=1
done

if [ ! -f "$MATRIX" ]; then
  echo -e "${RED}Parity matrix not found: $MATRIX${NC}"
  exit 2
fi

echo "=== Client Parity Check ==="
echo ""

DONE=0; NA=0; BUILT=0; DEFERRED=0; NOTBUILT=0; PENDING=0; UNKNOWN=0

while IFS= read -r line; do
  [[ "$line" =~ ^\|.*\|$ ]] || continue
  [[ "$line" == *"---"* ]] && continue
  [[ "$line" == *"Feature"* ]] && continue
  [[ "$line" == *"PWA module"* ]] && continue
  [[ "$line" == *"PWA spec"* ]] && continue        # skip the test-mapping table
  [[ "$line" == *"| Target |"* ]] && continue       # skip the test-inventory table

  trimmed="${line#|}"
  trimmed="${trimmed%|}"

  # Status is the 5th cell of the feature table; group header rows (e.g.
  # "| **Auth** |") have no 5th cell and yield an empty status -> skipped.
  status=$(echo "$trimmed" | awk -F'|' '{gsub(/^[ \t]+|[ \t]+$/, "", $5); gsub(/\*\*/, "", $5); print $5}')
  feature=$(echo "$trimmed" | awk -F'|' '{gsub(/^[ \t]+|[ \t]+$/, "", $1); gsub(/\*\*/, "", $1); print $1}')

  case "$status" in
    Done)          DONE=$((DONE + 1)) ;;
    "N/A")         NA=$((NA + 1)) ;;
    Built)         BUILT=$((BUILT + 1)) ;;
    Deferred)      DEFERRED=$((DEFERRED + 1)) ;;
    "Not built")   NOTBUILT=$((NOTBUILT + 1)); echo -e "  ${YELLOW}● Not built${NC} — $feature" ;;
    "iOS pending") echo -e "  ${RED}❗ iOS pending${NC} — $feature"; PENDING=$((PENDING + 1)) ;;
    "PWA pending") echo -e "  ${RED}❗ PWA pending${NC} — $feature"; PENDING=$((PENDING + 1)) ;;
    "")            ;;  # group header / non-feature row
    *)             echo -e "  ${RED}✘ Unknown status '${status}'${NC} — $feature"; UNKNOWN=$((UNKNOWN + 1)) ;;
  esac
done < "$MATRIX"

echo ""
echo "---"
echo -e "  Done: ${GREEN}${DONE}${NC}  Built: ${BUILT}  N/A: ${NA}  Deferred: ${DEFERRED}  Not built: ${YELLOW}${NOTBUILT}${NC}  Pending: ${RED}${PENDING}${NC}"
echo ""

if [ "$UNKNOWN" -gt 0 ]; then
  echo -e "${RED}${UNKNOWN} row(s) use an unknown status. Use one of: Done, Built, iOS pending, PWA pending, Deferred, Not built, N/A.${NC}"
  exit 2
fi

if [ "$STRICT" = "1" ] && [ "$PENDING" -gt 0 ]; then
  echo -e "${RED}--strict: ${PENDING} parity gap(s) remain. Update the corresponding client(s).${NC}"
  exit 1
fi

echo -e "${GREEN}Matrix is well-formed.${NC}"
exit 0
