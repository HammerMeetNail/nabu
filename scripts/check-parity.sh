#!/usr/bin/env bash
# Check client parity between PWA and iOS app.
#
# Run locally:  bash scripts/check-parity.sh
# In CI:        Parses parity matrix and compares against changed files.
#
# Exit codes:
#   0 - All parity rows are Done or N/A
#   1 - Parity gaps found (rows marked iOS pending or PWA pending)
#   2 - Parity matrix not parseable

set -euo pipefail

MATRIX="docs/plans/client-parity.md"
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m'

if [ ! -f "$MATRIX" ]; then
  echo -e "${RED}Parity matrix not found: $MATRIX${NC}"
  exit 2
fi

echo "=== Client Parity Check ==="
echo ""

# Parse the matrix table: find rows beneath the header separator
# Lines like: | Feature | PWA module | iOS module | Shared API | Parity | Notes |
# The status column is the 6th field
PENDING=0
DONE=0
NA=0

while IFS= read -r line; do
  # Skip non-table rows
  [[ "$line" =~ ^\|.*\|$ ]] || continue
  # Skip header and separator rows
  [[ "$line" == *"---"* ]] && continue
  [[ "$line" == *"Feature"* ]] && continue
  [[ "$line" == *"PWA module"* ]] && continue

  # Extract fields: trim leading/trailing | and whitespace
  trimmed="${line#|}"
  trimmed="${trimmed%|}"

  # Split by | and get the 6th field (Parity status)
  # Use awk for reliable field splitting
  status=$(echo "$trimmed" | awk -F'|' '{gsub(/^[ \t]+|[ \t]+$/, "", $5); print $5}')

  feature=$(echo "$trimmed" | awk -F'|' '{gsub(/^[ \t]+|[ \t]+$/, "", $1); print $1}')

  case "$status" in
    Done|"Done ")
      DONE=$((DONE + 1))
      ;;
    "N/A"|"N/A ")
      NA=$((NA + 1))
      ;;
    "iOS pending"|"iOS pending ")
      echo -e "  ${RED}❗ iOS pending${NC} — $feature"
      PENDING=$((PENDING + 1))
      ;;
    "PWA pending"|"PWA pending ")
      echo -e "  ${RED}❗ PWA pending${NC} — $feature"
      PENDING=$((PENDING + 1))
      ;;
    "")
      ;;
    *)
      echo -e "  ${YELLOW}⚠ Unknown status '${status}'${NC} — $feature"
      ;;
  esac
done < "$MATRIX"

echo ""
echo "---"
echo -e "  Done: ${GREEN}${DONE}${NC}  N/A: ${NA}  Pending: ${RED}${PENDING}${NC}"
echo ""

if [ "$PENDING" -gt 0 ]; then
  echo -e "${RED}${PENDING} parity gap(s) found. Update the corresponding client(s).${NC}"
  exit 1
else
  echo -e "${GREEN}All features at parity.${NC}"
  exit 0
fi
