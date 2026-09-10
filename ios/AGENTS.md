# iOS client instructions

Read the [root guide](../AGENTS.md) first. Commands below run from the task worktree root. The native app uses SwiftUI and the existing Go JSON API; the backend remains the authority for business rules and authorization.

## Locate the change

| Area | Files |
|---|---|
| Project, targets, dependencies | [Nabu.xcodeproj/project.pbxproj](Nabu.xcodeproj/project.pbxproj) |
| App lifecycle, state, navigation | `Nabu/App/`, [Nabu/ContentView.swift](Nabu/ContentView.swift) |
| HTTP, cookies/CSRF, models, stores | `Nabu/API/`, especially `APIClient.swift`, `Models.swift`, `RequestModels.swift`, and `Data/` |
| Authentication | `Nabu/Auth/` |
| Screens and charts | `Nabu/Views/`, including `Views/Stats/` |
| Shared presentation / helpers | `Nabu/DesignSystem/`, `Nabu/Support/` |
| Unit, contract, snapshot tests | `NabuTests/` and `NabuTests/__Snapshots__/` |
| Native UI tests / test support | `NabuUITests/`, `TestSupport/` |

The project is an Xcode project with Swift package dependencies; there is no root `ios/Package.swift`. Check target membership in `project.pbxproj` when adding source, test, or fixture files. Use the project build settings and installed toolchain as the source for deployment/runtime requirements.

Before changing shared behavior, inspect the corresponding PWA module and actual Playwright spec. The [active iOS plan](../docs/plans/ios-appstore-v1.md) calls for **behavior parity, native presentation**: match validation, data sent, action semantics, and persistence while using native lists, sheets, pickers, and symbols. Record intentional presentation differences in the [parity matrix](../docs/plans/client-parity.md).

The [conversion plan](../docs/plans/ios.md) is historical. Plans and the parity matrix can contain aspirational or stale test inventories; use `rg --files ios/NabuTests ios/NabuUITests` to locate real coverage before choosing tests.

## API and state invariants

- Keep DTO field names aligned with Go's camelCase JSON (`choreId`, `volumeML`, `completedAt`). `apiEncoder` in `Nabu/API/Models.swift` uses `.useDefaultKeys`; do not switch it to `.convertToSnakeCase`.
- Decode RFC3339 timestamps with and without fractional seconds using the existing custom decoder. Use `Date` for timestamps and `LocalDate` for `YYYY-MM-DD`; do not convert calendar-only values through UTC timestamps.
- For arrays the server may return as null, use `decodeIfPresent(...) ?? []` where the API contract treats null as empty. Keep fixtures and custom decoders aligned with server responses.
- Preserve native CSRF preflight/cookie handling. Before registration has a session, `GET /api/me` obtains a `nabu_csrf` cookie; native clients do not get it from an HTML page load.
- Use `Int` for existing Go `int64` identifiers. Preserve ownership checks on the server when sending foreign resource IDs; stale client state is not authorization.
- For direct current-time Home logs, send `hour = Calendar.current.component(.hour, from: now)` and `completedAt = now`. Sheet logs derive `date`, `hour`, and `completedAt` from the selected When value, including minutes.
- Preserve `slotHour` separately from `completedAt`: nil means Anytime, an integer means the corresponding calendar row. Do not infer placement from the timestamp or round picker times to `:00`.
- Render user-controlled metadata as plain text; do not interpret names, labels, or notes through `AttributedString(markdown:)`.
- For asynchronous state changes, preserve actor isolation, cancellation, and stale-result rejection on navigation, logout, and household changes. Test the completion conditions and ownership as well as the successful response.

If the PWA has a bug within the authorized task, investigate and fix the shared behavior in both clients. Clarify only unresolved product decisions that materially change the task; do not copy a known bug merely to claim parity or stop solely because the clients differ.

## Validation

New native behavior needs XCTest coverage for affected logic/contracts and XCUITest coverage for user flows. Visual changes need relevant snapshot coverage, including existing supported appearances/states. Bugs need a regression at the affected layer. Documentation and mechanical changes do not need artificial UI tests.

Use deterministic fixtures and observable completion rather than sleeps. Concurrency/callback changes require independent behavior and test-synchronization review before the full gate. Keep one owner for the simulator and DerivedData; unrelated agents must not start competing builds.

### Platform availability

iOS builds and simulator tests require macOS with Xcode. On Linux or a host without Xcode, continue source, fixture, parity, and portable backend/PWA validation, then report the exact native checks that remain unrun. Do not treat a skipped snapshot suite or an unobserved CI lane as a pass.

### Choose a simulator and artifact directory

List installed runtimes/devices and choose a concrete simulator UDID. Device names can be duplicated across runtimes. Snapshot suites currently use baselines recorded on iOS major 26 and skip other majors; inspect `recordedOSMajor` in the affected suite before choosing a runtime or updating baselines.

```bash
xcodebuild -version
xcrun simctl list runtimes
xcrun simctl list devices available
export NABU_SIMULATOR_ID="<available-simulator-udid>"
export NABU_IOS_ARTIFACTS="$(mktemp -d "${TMPDIR:-/tmp}/nabu-ios.XXXXXX")"
```

### Build and run unit/contract tests

Build the app before building tests. A clean `build-for-testing` can otherwise race the test target's `@testable import Nabu` against emission of the app module. This follows the CI sequence, with task-specific artifacts:

```bash
xcodebuild build \
  -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -derivedDataPath "$NABU_IOS_ARTIFACTS/DerivedData"

xcodebuild build-for-testing \
  -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -derivedDataPath "$NABU_IOS_ARTIFACTS/DerivedData"

xcodebuild test-without-building \
  -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -derivedDataPath "$NABU_IOS_ARTIFACTS/DerivedData" \
  -resultBundlePath "$NABU_IOS_ARTIFACTS/unit.xcresult" \
  -only-testing:NabuTests
```

For a focused run, narrow `-only-testing:` to an existing test class/method. Rebuild after source/test edits before using `test-without-building`. Use a fresh result-bundle path for each run; when piping logs through another command, retain `xcodebuild`'s exit status (for example with Bash `pipefail`). Inspect final counts, failures, crashes, and skips in the result bundle, including snapshot runtime skips.

### Native UI flows

Most UI tests use mock support. [NabuHomeEndToEndUITests](NabuUITests/NabuUITests.swift) instead targets a real backend at `http://localhost:8080`. Start an owned backend with `make run` (development, empty `DATABASE_URL`) or `make local`; coordinate ports before starting it.

After the build sequence above, run that real-server test class:

```bash
xcodebuild test-without-building \
  -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -derivedDataPath "$NABU_IOS_ARTIFACTS/DerivedData" \
  -resultBundlePath "$NABU_IOS_ARTIFACTS/home-e2e.xcresult" \
  -only-testing:NabuUITests/NabuHomeEndToEndUITests
```

Use `-only-testing:NabuUITests` with a new result path for the full native flow suite. The real-server class creates a unique email and uses `-nabuAutoRegister` to register, onboard, and seed programmatically; it does **not** cover the registration/onboarding UI. Test those screens through their own UI interactions when changing them.

### CI and parity

The [CI iOS job](../.github/workflows/ci.yaml) runs the app build, `build-for-testing`, and `test-without-building -only-testing:NabuTests` for `ios/**` changes and every `v*` release tag. UI tests are not run there. Snapshot tests inside `NabuTests` may skip on the selected runtime.

Backend-only PR changes do not automatically select the iOS lane, and fixture-based contracts do not prove compatibility with a changed live server. Check native models/fixtures explicitly for API changes and run the relevant contracts/integration coverage on an available Mac. The image/deploy job dependencies currently omit the iOS job; release verification must inspect its result separately.

Update the parity matrix for affected behavior and known differences. CI filters include all `ios/**` files, so documentation-only changes here can use `no-parity-update: <reason>` in the PR body when no feature row needs changing. Read the [parity workflow](../.opencode/skills/client-parity/SKILL.md) directly if it is not exposed as a skill in the current session. Complete the root checks applicable to the changed files.
