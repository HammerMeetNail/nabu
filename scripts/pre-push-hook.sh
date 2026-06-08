#!/usr/bin/env bash
# Pre-push hook: checks client parity before pushing.
#
# If the push touches PWA and/or iOS files, it checks that both clients
# were updated (or warns if only one was touched). The CI parity job
# enforces the PR description requirement as a backstop.
#
# Skip with:  SKIP_PARITY=1 git push

set -euo pipefail

if [ "${SKIP_PARITY:-}" = "1" ]; then
  exit 0
fi

RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

PWA_PATHS="web/static/js/ web/static/css/ web/templates/"
IOS_PATHS="ios/"

while IFS=' ' read -r local_ref local_sha remote_ref remote_sha; do
  # Skip deleted refs
  [ "$local_sha" = "0000000000000000000000000000000000000000" ] && continue

  # Determine the base commit
  if [ "$remote_sha" = "0000000000000000000000000000000000000000" ]; then
    # New branch — diff against the merge base with origin/main
    base=$(git merge-base origin/main "$local_sha" 2>/dev/null || echo "")
    if [ -z "$base" ]; then
      base=$(git rev-list --max-parents=0 "$local_sha")
    fi
  else
    base="$remote_sha"
  fi

  # Get list of changed files
  changed=$(git diff --name-only "$base".."$local_sha" 2>/dev/null || echo "")

  has_pwa=0
  has_ios=0

  for prefix in $PWA_PATHS; do
    if echo "$changed" | grep -q "^${prefix}"; then
      has_pwa=1
      break
    fi
  done

  if echo "$changed" | grep -q "^${IOS_PATHS}"; then
    has_ios=1
  fi

  # Only check if client files changed
  if [ "$has_pwa" -eq 0 ] && [ "$has_ios" -eq 0 ]; then
    exit 0
  fi

  # Both clients touched — likely intentional
  if [ "$has_pwa" -eq 1 ] && [ "$has_ios" -eq 1 ]; then
    echo -e "${YELLOW}⚠ Pre-push: both PWA and iOS files changed — verify parity.${NC}"
    exit 0
  fi

  # Only one client touched — warn and check the parity matrix
  if [ "$has_pwa" -eq 1 ] && [ "$has_ios" -eq 0 ]; then
    echo ""
    echo -e "${YELLOW}┌──────────────────────────────────────────────────────┐${NC}"
    echo -e "${YELLOW}│ ⚠  PWA files changed but no iOS files changed       │${NC}"
    echo -e "${YELLOW}│                                                      │${NC}"
    echo -e "${YELLOW}│ If this is a PWA-only change, your PR description    │${NC}"
    echo -e "${YELLOW}│ must say:                                            │${NC}"
    echo -e "${YELLOW}│   'PWA-only change; iOS not affected because ...'    │${NC}"
    echo -e "${YELLOW}│                                                      │${NC}"
    echo -e "${YELLOW}│ Otherwise, update the iOS app too (ios/Nabu/).       │${NC}"
    echo -e "${YELLOW}└──────────────────────────────────────────────────────┘${NC}"
    echo ""
    echo -e "Re-run with ${RED}SKIP_PARITY=1 git push${NC} to bypass."
    echo ""

    # Run full parity check for information
    echo "Parity matrix status:"
    bash scripts/check-parity.sh || true
    echo ""
    exit 1
  fi

  if [ "$has_ios" -eq 1 ] && [ "$has_pwa" -eq 0 ]; then
    echo ""
    echo -e "${YELLOW}┌──────────────────────────────────────────────────────┐${NC}"
    echo -e "${YELLOW}│ ⚠  iOS files changed but no PWA files changed       │${NC}"
    echo -e "${YELLOW}│                                                      │${NC}"
    echo -e "${YELLOW}│ If this is an iOS-only change, your PR description   │${NC}"
    echo -e "${YELLOW}│ must say:                                            │${NC}"
    echo -e "${YELLOW}│   'iOS-only change; PWA not affected because ...'    │${NC}"
    echo -e "${YELLOW}│                                                      │${NC}"
    echo -e "${YELLOW}│ Otherwise, update the PWA too (web/static/js/).      │${NC}"
    echo -e "${YELLOW}└──────────────────────────────────────────────────────┘${NC}"
    echo ""
    echo -e "Re-run with ${RED}SKIP_PARITY=1 git push${NC} to bypass."
    echo ""

    echo "Parity matrix status:"
    bash scripts/check-parity.sh || true
    echo ""
    exit 1
  fi

done
