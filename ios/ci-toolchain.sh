#!/usr/bin/env bash
# Selects the Xcode that CI builds, tests and archives the iOS app with, and
# prints an iPhone simulator on that Xcode's iOS runtime as udid=<id> for
# $GITHUB_OUTPUT.
#
# App Store Connect takes uploads built with the iOS 26 SDK or later, and the
# snapshot tests were recorded on iOS 26 (they skip on other majors), so this
# picks the newest Xcode 26 on the runner and an iOS 26 simulator. The
# runner's default Xcode can be older. The simulator is an iPhone Pro: the
# snapshots are 3x, and an iPhone SE would render them at 2x.
#
# Usage: ios/ci-toolchain.sh >> "$GITHUB_OUTPUT"   (IOS_MAJOR defaults to 26)
#        ios/ci-toolchain.sh --xcode-only           selects Xcode only
set -euo pipefail

major="${IOS_MAJOR:-26}"

xcode="$(ls -d /Applications/Xcode_"$major"*.app 2>/dev/null | sort -V | tail -1 || true)"
if [ -z "$xcode" ]; then
  echo "No Xcode $major on this runner. Installed:" >&2
  ls -d /Applications/Xcode*.app >&2 || true
  exit 1
fi
sudo xcode-select -s "$xcode/Contents/Developer"
xcodebuild -version >&2
if [ "${1:-}" = --xcode-only ]; then exit 0; fi

runtime="$(xcrun simctl list runtimes available -j | jq -r --arg major "$major" '
  [.runtimes[] | select((.identifier | test("SimRuntime\\.iOS-")) and (.version | startswith($major + ".")))]
  | sort_by(.version | split(".") | map(tonumber)) | last | .identifier // empty')"
if [ -z "$runtime" ]; then
  echo "No iOS $major simulator runtime for $xcode. Installed:" >&2
  xcrun simctl list runtimes >&2
  exit 1
fi

devices() { xcrun simctl list devices available -j | jq -r --arg rt "$runtime" "$1"; }
udid="$(devices '.devices[$rt][]? | select(.name | test("^iPhone [0-9]+ Pro$")) | "\(.name)\t\(.udid)"' |
  sort -V | tail -1 | cut -f2)"
if [ -z "$udid" ]; then
  type="$(xcrun simctl list devicetypes -j | jq -r '.devicetypes[] | select(.name | test("^iPhone [0-9]+ Pro$")) | "\(.name)\t\(.identifier)"' |
    sort -V | tail -1 | cut -f2)"
  [ -n "$type" ] || { echo "No iPhone Pro device type for $runtime" >&2; exit 1; }
  udid="$(xcrun simctl create "CI iPhone" "$type" "$runtime")"
fi
echo "Simulator $udid on $runtime" >&2
echo "udid=$udid"
