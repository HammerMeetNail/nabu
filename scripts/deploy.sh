#!/usr/bin/env bash
# Deploy: determine the next version tag from the latest tag reachable from
# main, create an annotated tag, and push it to trigger the CI deploy
# pipeline (see docs/deploy-runbook.md).
#
# Usage:
#   make deploy        # interactive: confirm, then tag & push
#   DRY_RUN=1 make deploy   # print what would happen without tagging/pushing
set -euo pipefail

cd "$(dirname "$0")/.."

# --- Preconditions ----------------------------------------------------------

BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [ "$BRANCH" != "main" ]; then
  echo "error: deploys must be cut from 'main' (currently on '$BRANCH')" >&2
  exit 1
fi

git fetch origin >/dev/null 2>&1 || true
if [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]; then
  echo "error: local main is out of sync with origin/main; run 'git pull origin main' first" >&2
  exit 1
fi

if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  echo "error: working tree has uncommitted tracked changes; commit or stash them first" >&2
  exit 1
fi

# --- Determine the next tag --------------------------------------------------

LATEST_TAG="$(git describe --tags --abbrev=0 2>/dev/null || true)"
if [ -z "$LATEST_TAG" ]; then
  echo "error: no tags found; cannot determine the next version" >&2
  exit 1
fi
case "$LATEST_TAG" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *)
    echo "error: latest tag '$LATEST_TAG' is not semver vX.Y.Z; bump the version manually" >&2
    exit 1
    ;;
esac

if [ -z "$(git log "${LATEST_TAG}..HEAD" --oneline)" ]; then
  echo "error: nothing new to deploy since $LATEST_TAG" >&2
  exit 1
fi

# v0.1.352 -> v0.1.353 (patch bump, matching repo convention)
NEXT_TAG="$(echo "$LATEST_TAG" | awk -F. '{print $1"."$2"."($3+1)}')"

if git rev-parse -q --verify "refs/tags/$NEXT_TAG" >/dev/null; then
  echo "error: tag $NEXT_TAG already exists — check for orphaned branches: git branch -a --contains $NEXT_TAG" >&2
  exit 1
fi

COUNT="$(git rev-list --count "${LATEST_TAG}..HEAD")"
# sed reads all of git's output. head would exit after one line, and git,
# still writing, would die of SIGPIPE and abort this script under pipefail.
SUBJECT="$(git log --reverse --format=%s "${LATEST_TAG}..HEAD" | sed -n 1p)"

echo "Deploying $COUNT commit(s) since $LATEST_TAG:"
git log --oneline "${LATEST_TAG}..HEAD"
echo
echo "Next tag: $NEXT_TAG"

if [ "${DRY_RUN:-0}" = "1" ]; then
  echo "(dry run) would create annotated tag $NEXT_TAG ('${NEXT_TAG}: ${SUBJECT}') and push it" >&2
  exit 0
fi

read -r -p "Create and push $NEXT_TAG to trigger the production deploy? [y/N] " answer
case "$answer" in
  y|Y) ;;
  *)
    echo "aborted"
    exit 1
    ;;
esac

# --- Tag and push ------------------------------------------------------------

git tag -a "$NEXT_TAG" -m "${NEXT_TAG}: ${SUBJECT}"
git push origin "$NEXT_TAG"

echo
echo "Pushed $NEXT_TAG — CI will build, test, and deploy."
echo "Watch the pipeline to completion and verify production per docs/deploy-runbook.md."
