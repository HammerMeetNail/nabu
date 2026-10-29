#!/usr/bin/env bash
# A web release waits for the iOS tests only when it changes the API contract
# (api/contract: the server's answers the iOS app decodes). This prints
# changed=false when the contract is the one the release in production
# answers with, and changed=true otherwise, including when that release can't
# be found: then the iOS tests run.
#
# Usage: scripts/api-contract-changed.sh >> "$GITHUB_OUTPUT"
#        (NABU_URL defaults to production)
set -euo pipefail

cd "$(dirname "$0")/.."

URL="${NABU_URL:-https://nabu-app.com}"

changed() {
  echo "$1 The iOS tests run." >&2
  echo "changed=true"
  exit 0
}

health="$(curl -fsS --max-time 20 "$URL/health")" || changed "Could not read $URL/health."
version="$(printf '%s' "$health" | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
[ -n "$version" ] || changed "$URL/health doesn't report a version."

tag="v$version"
if ! git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  # CI checks out one commit; the tag's tree is all the diff needs.
  if [ "$(git rev-parse --is-shallow-repository)" = true ]; then
    git fetch --quiet --depth=1 origin "refs/tags/$tag:refs/tags/$tag" 2>/dev/null || true
  else
    git fetch --quiet origin "refs/tags/$tag:refs/tags/$tag" 2>/dev/null || true
  fi
fi
git rev-parse -q --verify "refs/tags/$tag" >/dev/null || changed "Production runs $version, but there is no tag $tag."

if git diff --quiet "$tag" HEAD -- api/contract; then
  echo "The API contract is the one production's $tag answers with: the iOS tests don't run." >&2
  echo "changed=false"
  exit 0
fi
echo "The API contract has changed since $tag, the version in production:" >&2
git diff --name-status "$tag" HEAD -- api/contract | awk 'NR <= 20 { print "  " $0 }' >&2
changed "The release waits for them."
