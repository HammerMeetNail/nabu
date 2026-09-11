# Mac validation handoff

Mac follow-up completed: see the [validation record](review-2026-09-10-mac-validation.md)
for fixes, exact source evidence and passing local Xcode/browser results. The
physical-device release checks remain open. The original handoff below is kept
as the verification checklist.

The user deferred Xcode testing to a separate Mac session after this branch is
committed and pushed. Use `pwa-review-20260910` and record the exact fetched commit
before running these checks. The [implementation record](review-2026-09-10-implementation.md)
contains the backend/PWA results and local operational evidence.

The Linux session parsed all Swift sources and ran 172 portable Foundation
XCTest cases. It did not typecheck SwiftUI, run Xcode, exercise iOS cookie/APNs/
protected-file behavior, or record new snapshots. Treat the following as required
native validation and correction work, not an already-passing release gate.

## Build and automated tests

Follow [ios/AGENTS.md](../ios/AGENTS.md). Work in a git worktree, keep result bundles
outside the repository, and have one owner coordinate builds and the simulator.
Use an installed iOS 26 simulator by UDID; existing snapshots deliberately skip
on other major versions.

```bash
xcodebuild -version
xcrun simctl list devices available
export NABU_SIMULATOR_ID="<iOS-26-simulator-udid>"
export NABU_MAC_RESULTS="$(mktemp -d /tmp/nabu-review-xcode.XXXXXX)"

xcodebuild build \
  -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -derivedDataPath "$NABU_MAC_RESULTS/DerivedData" \
  -resultBundlePath "$NABU_MAC_RESULTS/build.xcresult"

xcodebuild build-for-testing \
  -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -derivedDataPath "$NABU_MAC_RESULTS/DerivedData" \
  -resultBundlePath "$NABU_MAC_RESULTS/build-tests.xcresult"

xcodebuild test-without-building \
  -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -derivedDataPath "$NABU_MAC_RESULTS/DerivedData" \
  -resultBundlePath "$NABU_MAC_RESULTS/unit.xcresult" \
  -only-testing:NabuTests
```

Provision the local backend at `http://localhost:8080` for the real-server UI
class. Prefer PostgreSQL (`make local-fresh`) so migrations and isolation are
exercised. Use `make run` only when explicitly recording that the run used memory
storage. UI launches now supply a local base URL; the review's mock scenarios use
`http://localhost:9998` and return errors for unhandled routes. Do not point tests
at production or reuse production accounts.

```bash
xcodebuild test-without-building \
  -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -derivedDataPath "$NABU_MAC_RESULTS/DerivedData" \
  -resultBundlePath "$NABU_MAC_RESULTS/ui.xcresult" \
  -only-testing:NabuUITests
```

The sequence builds the app before the test bundle to avoid the known test-target
module race. Verify numeric exits and result-bundle test/failure/skip/crash counts;
passing test-case lines alone do not establish success. Record simulator runtime,
Xcode version, exact commit, bundle paths and any expected skips. A feature-branch
push alone does not trigger this repository's CI; the iOS job runs on eligible
PRs/release tags and covers `NabuTests`, not the full UI suite.

## Changed flows to verify

| Area | Existing/new source and required checks |
| --- | --- |
| Recent amounts and units | `NabuReviewRecoveryUITests.testGramEntryRecentChipAndFailedSaveRetainValuesForRetry` exercises chips, a failed save, unchanged retry and Activity roundtrip. Verify gram/count/volume/duration controls, type-specific volume prefill, midnight/reload and empty/error recent reads. |
| Activity and Stats | The new Activity UI case covers error/retry and retained search across Home. Model tests cover response reversal, deletion/pagination and all 20 configured widgets. Add screen assertions for delayed household changes and visible Stats loading/error/retry states; retain empty-section controls. |
| Notifications | The new UI case covers an all-read first page, failed older page, retry, mark-read/delete and enlarged text. Confirm open-panel refresh behavior, pagination position and unread counts after mutations. |
| Exports | The new UI case covers date controls, an actionable error, delayed cancellation, retry and the native share sheet. Verify canceled/stale exports create no shareable file and dismissed files are removed. |
| Claims, logout and identity | Password setup has an open/cancel UI case and API contracts. Verify successful setup, old credential/session revocation, failed logout through relaunch, and pending reads during account/household changes. Test real URLSession cookies, not only mock responses. |
| Journal and timers | Core tests cover immutable payloads, disk/corruption failures, stale identity and stopped-timer recovery. Add/review screen-level rejection/retry/discard and relaunch tests. Verify protected files on a device and both successful and failed stop/save. |
| Schedule and deletion | The new empty-Schedule UI case opens the creation picker and cancels. Verify save/reload, recipient-local recurrence/end dates, ownership-transfer guidance, and canceled deletion recovery. |
| Accessibility | Run the existing accessibility audit. Verify changed LogSheet, notification and export controls at large Dynamic Type, keyboard focus where applicable, labels and real VoiceOver navigation. |

The screen fixtures live in `App/AppEnvironment.swift` under DEBUG and are selected
with `-reviewScenario`. Their API contracts compile in the portable harness, but
the new XCUITest assertions have not yet executed. Fix source/selector issues
against actual UI behavior; do not weaken recovery or identity assertions to make
an unverified flow green.

## Snapshots and device boundaries

Existing `StatsSnapshotTests`, `StateQualitySnapshotTests` and
`PushPrePromptSnapshotTests` have real iOS 26 baselines. Run them before updating
anything. Add actual-screen coverage and reviewed baselines for changed
LogSheet/unit/error states, notification history/error/paging, export date/error/
cancel states and the empty Schedule CTA. The Linux session did not fabricate or
record these pixels.

Physical-device checks remain necessary for session-cookie/logout behavior,
protected-file availability, APNs registration reassignment and stale notification
actions across accounts, multi-device delivery, background/foreground recovery,
and VoiceOver. The backend's fake providers and portable UIKit shims cannot
establish those results. Keep production access outside this handoff unless the
user separately authorizes it.
