#!/usr/bin/env bash
# The iOS app ships after the web app it talks to. Before an iOS release, this
# checks that production already runs every server change in the commit being
# released (HEAD): the server code at production's release tag must equal the
# code at HEAD. Changes to the PWA, the iOS app, docs or tests don't count, so
# a pending web deploy only holds the app back when the server itself changed.
#
# Usage: scripts/ios-release-web-first.sh   (NABU_URL defaults to production)
set -euo pipefail

cd "$(dirname "$0")/.."

URL="${NABU_URL:-https://nabu-app.com}"
# The code the server is built from. Tests are excluded below.
SERVER_PATHS=(cmd internal migrations go.mod go.sum)

health="$(curl -fsS --max-time 20 "$URL/health")" || {
  echo "error: could not read $URL/health" >&2
  exit 1
}
version="$(printf '%s' "$health" | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
if [ -z "$version" ]; then
  echo "error: $URL/health doesn't report a version" >&2
  exit 1
fi

tag="v$version"
if ! git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  git fetch --quiet origin "refs/tags/$tag:refs/tags/$tag" 2>/dev/null || true
fi
if ! git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  echo "error: production runs $version, but there is no tag $tag" >&2
  exit 1
fi
if ! git merge-base --is-ancestor "$tag" HEAD; then
  echo "error: production's $tag is not in the history of the commit being released" >&2
  exit 1
fi

# sed reads all of git's output: head would exit early, and git, still
# writing, would die of SIGPIPE and abort this script under pipefail.
changed="$(git diff --name-only "$tag" HEAD -- "${SERVER_PATHS[@]}" | sed '/_test\.go$/d')"
if [ -n "$changed" ]; then
  echo "The server has changed since $tag, the version in production:" >&2
  printf '%s\n' "$changed" | awk 'NR <= 20 { print "  " $0 }' >&2
  echo "Deploy the web app first (tag main), then release the iOS app." >&2
  exit 1
fi
echo "Production runs $tag, which has all of this commit's server code."
