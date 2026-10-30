# iOS Client Agent Instructions

This file provides guidance to an LLM when working on the Nabu native iOS app.

## Client parity rule

The Nabu project has two clients: a PWA (`web/static/js/`) and a native iOS app (`ios/`). Every feature, bug fix, validation change, security fix, API change, or UI behavior change must be evaluated for both clients.

**Before editing iOS code**, inspect the corresponding PWA module under `web/static/js/` and the corresponding Playwright E2E spec under `tests/e2e/`. The governing rule is **behavior parity, presentation nativeness** (see `docs/plans/ios-appstore-v1.md` §2.1): behavioral semantics — requests sent, `slotHour`/`completedAt` invariants, validation, what an action does — must match the PWA; presentation should use native iOS idiom (native lists, sheets, pickers, SF Symbols) rather than pixel-copying PWA CSS. Record intentional presentation differences in the parity matrix's "Known differences" column.

**Before editing PWA behavior**, inspect the corresponding iOS screen, model, or test under `ios/` to check whether the iOS app needs the same change.

## File placement

- Place all iOS code under `ios/`.
- Do not place iOS files outside `ios/` except:
  - Shared documentation in `docs/`.
  - Backend changes in `internal/`.
  - CI changes in `.github/`.
- The existing Go backend and its API are the single source of business authority. Do not fork business behavior into the iOS app.

## Testing requirements

Every iOS feature must have:

1. **XCTest** unit tests for models, logic, and API contract behavior.
2. **XCUITest** user-flow tests for the corresponding Playwright E2E spec.
3. **Snapshot tests** for visual states where the PWA has visible UI.

Before finishing any iOS work, run the documented test commands and verify all tests pass.

### Running XCUITests against a real server

The E2E test class `NabuHomeEndToEndUITests` in `ios/NabuUITests/NabuUITests.swift` exercises the full register → onboard → seed → log flow against a real (non-mock) server.

**Provision the server (once):**

```bash
# In-memory server (no Postgres needed):
make run
# or: go run ./cmd/server

# Postgres-backed server (via Podman):
make local
```

The server must be listening on `http://localhost:8080`.

**Choose a simulator:**

List installed runtimes and devices, then use a specific simulator UDID. Do
not rely on `name=iPhone 16`: that name may exist in several installed
runtimes and makes the destination ambiguous.

```bash
xcrun simctl list devices available
export NABU_SIMULATOR_ID="<simulator-udid>"
```

Use an iOS 26 runtime when running snapshot coverage. Snapshot tests are
intentionally skipped on other OS major versions because rendering differs.

**Run iOS tests:**

Build the app before building the test bundle. A direct `xcodebuild test` can
race the `NabuTests` target against the app module, causing `@testable import
Nabu` failures. This matches the CI build sequence.

```bash
xcodebuild build \
  -project ios/Nabu.xcodeproj \
  -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID"

xcodebuild build-for-testing \
  -project ios/Nabu.xcodeproj \
  -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID"

xcodebuild test-without-building \
  -project ios/Nabu.xcodeproj \
  -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -only-testing:NabuTests
```

**Run the E2E test:**

```bash
xcodebuild test-without-building \
  -project ios/Nabu.xcodeproj \
  -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -only-testing:NabuUITests/NabuHomeEndToEndUITests
```

`NabuOfflineLaunchUITests` covers offline launch against the same server: it
registers online, relaunches with `-serverUnreachable` (a debug-only hook that
fails every request like a lost connection), logs from the saved Home, then
relaunches online and waits for the queued log to sync. To use a server on
another port, prefix the command with
`TEST_RUNNER_NABU_E2E_BASE_URL=http://localhost:<port>`.

`NabuShortcutsEndToEndUITests` runs the "Log Activity" App Intent from the
Shortcuts app against the same server and checks Siri's answer and Home. It
keeps Nabu running in the background: killing the app right after sign-up
loses the session cookie iOS has not yet written to disk, so a cold
background launch would find no session. iOS applies the app's activity
list about a minute late after another run's household, and a stale tile
logs nothing, so the test retries; a run right after another can take two
minutes. The intents' logic is in
`Nabu/App/AppIntents.swift` (`IntentActions`), unit-tested in
`AppIntentsTests`; a log there must stay the body a Home log sends.

**How it works:**

The test generates a unique email, then launches the app with these launch arguments:

```
-disableAnimations -resetState -nabuBaseURL http://localhost:8080 -nabuAutoRegister <email> <password>
```

`ContentView.task` in `ios/Nabu/ContentView.swift` parses these arguments via `parseTestCreds()` and programmatically calls `auth.register()` → `auth.createHousehold()` → `auth.seedDefaults()` → `loadAppData()` — bypassing the registration/onboarding UI entirely.

A CSRF pre-flight (`GET /api/me`) runs before the `POST /api/auth/register` to obtain a `nabu_csrf` cookie; the native app has no HTML page load to set it the way the PWA does.

**Key encoding rules enforced by this test:**

- The Go server uses camelCase JSON tags (`choreId`, `volumeML`, `completedAt`). The iOS `apiEncoder` in `ios/Nabu/API/Models.swift` must use `.useDefaultKeys` — never `.convertToSnakeCase`.
- Go's `time.Time` marshals to RFC3339 with fractional seconds (e.g. `2026-06-06T16:23:47.081048Z`). The iOS `apiDecoder` uses a custom `ISO8601DateFormatter` with `.withFractionalSeconds` and a fallback without fractions.
- Server JSON fields that may be `null` (like `indicatorLabels` on a chore with no indicators) must use `decodeIfPresent(… ) ?? []` in custom `init(from:)` — Swift will not coerce `null` to `[String]`.
- The log handler in `internal/handlers/log.go` validates that the chore exists and belongs to the user's household before creating a log (defense in depth against FK violations from stale client-side chore lists).

## Reference files

- Active plan: `docs/plans/ios-appstore-v1.md` (App Store v1 parity & polish)
- Historical plan: `docs/plans/ios.md` (superseded conversion plan)
- Parity matrix: `docs/plans/client-parity.md`
- Backend architecture: root `AGENTS.md`
- PWA source: `web/static/js/`
- PWA E2E specs: `tests/e2e/`

## Key invariants from the PWA

These PWA behaviors must be preserved in the native iOS app:

1. **Home-tab direct log**: must send `hour = Calendar.current.component(.hour, from: now)` and `completedAt = now`.
2. **Home-tab sheet log**: must derive `completedAt`, `date`, and `hour` from the selected When picker value. The When picker must preserve minutes and must not round to `:00`.
3. **Calendar ad-hoc log placement**: `slotHour == nil` → Anytime row. `slotHour == hour` → that hour row.
4. **Never drop `slotHour` or replace it with `completedAt.hour`**. They are related but distinct behavior.
5. **User-controlled metadata must be rendered as plain text**. Avoid `AttributedString(markdown:)` for user content.

## Implementation rules

1. Before implementing a feature, read the corresponding PWA files and Playwright specs.
2. Write or port tests first.
3. Implement the smallest native code that makes the tests pass.
4. Do not invent new *behavior* (data sent, invariants, validation, feature semantics) because it feels more iOS-like. Native *presentation* is expected and encouraged per `docs/plans/ios-appstore-v1.md` §2.1.
5. If PWA behavior appears buggy, stop and ask before intentionally diverging.
6. If a backend endpoint lacks native support, add backend tests before changing the backend.
7. Keep API DTO names close to server JSON names.
8. Use `Int` for Go `int64` IDs unless a test proves overflow risk.
9. Use `Date` for RFC3339 timestamps and `LocalDate` for `YYYY-MM-DD` values.
10. Never update only one client when behavior is shared.
11. Versions and the server live in `ios/Config` (see `ios/README.md`). Never set `MARKETING_VERSION`, `CURRENT_PROJECT_VERSION` or a server URL on a target, and never hard-code a server in Swift.

## Swift 6

The app, the widget extension and the unit tests build in the Swift 6 language mode with Approachable Concurrency (`SWIFT_APPROACHABLE_CONCURRENCY`): a nonisolated `async` function runs on its caller's actor, so a `@MainActor` store calling `APIClient` sends nothing across actors. Building needs Xcode 26 or later, which CI selects (`ios/ci-toolchain.sh`). The UI tests stay in Swift 5 mode: `XCUIElement` is main-actor isolated and XCTest's methods are not, so their 19 classes would need restructuring for no runtime gain. A shared static that a lock or immutability makes safe is `nonisolated(unsafe)`, with a comment saying why.

## Errors

No failure ends in an empty `catch`. `APIClient`'s requests are `throws(APIError)`: a request that got no answer is `networkError` (which `LogStore.isNetworkFailure` queues), and an answer the app can't read is `decodingError`.

- **A failed action tells the person**, in the PWA's words: the server's message when it sent one, otherwise the action's own ("Failed to remove member"), as `APIError.message(for:fallback:)` decides. `AppState.reportFailure` shows it above the tabs (`ErrorToast`). A sheet would hide that toast, so a sheet shows its own over itself with `.errorToast($failure)`. Pass `reportFailure` the revision the action started with, so another identity never sees it, except for a request that changes the identity (`APIClient.changesIdentity`, which includes renaming the household): its screens are rebuilt for the identity after it, so its failure belongs there.
- **A failure the screen already shows**, such as a switch or a preference that goes back, and **a background read**, which keeps the last good data, only log.
- **Catch the rest.** A `do` block around `APIClient` calls throws `APIError`. If it catches only some cases (`catch APIError.contextChanged { }`), end it with a plain `catch` as well. Rethrowing what's left as `any Error` crashed Xcode 26.3's Swift 6.2 compiler in a unit test that Xcode 27 builds.
- **Logging** goes through `Log` (`os.Logger`, one per area) and `failure(_:_:)`: what failed and the error's kind are public, and the error's text is private, since a server message or a decoding error can carry what someone typed. Never log a token, a cookie, an invite code or a URL with its query.

## Strings

The app's user-facing text lives in a String Catalog, `Nabu/Resources/Localizable.xcstrings` (English only, until localization after launch). The compiler extracts the literals in `Text`, `Label`, `Button` and the like (`SWIFT_EMIT_LOC_STRINGS`), and building in Xcode adds new ones to the catalog; a command-line build does not, so after adding UI text from the command line, build once in Xcode or add the key to the catalog. Text built as a plain `String` (an error message, a toast) is only extracted through `String(localized:)`.

## Parity bookkeeping on PRs

When a PR touches shared client surface — `ios/**`, `web/static/js/**`, or the shared API in `internal/handlers/**` — it must also update the parity matrix (`docs/plans/client-parity.md`) to reflect the change. The CI `parity` job enforces this (it lints the matrix and fails if client/API code changed without a matrix update); the escape hatch is a `no-parity-update: <reason>` line in the PR body.

(This replaced an older rule that required a "PWA-only change…" / "iOS-only change…" phrase in the PR description — that phrase is no longer what CI checks.)

## iOS tests run in CI

The `ios` CI job (`.github/workflows/ci.yaml`, macOS runner) builds the app and runs the `NabuTests` unit/contract suite on every change under `ios/**` or `api/contract/**`, and before a web release whose API contract changed. `APIContractTests` decodes every server answer in `api/contract` (recorded by `TestAPIContract` in `internal/app`) with the app's models: a new request the app makes needs a step in that Go scenario and its answer's type in `contractChecks`. Keep `NabuTests` green — a backend model/API change that breaks the iOS request models (e.g. an added field on `CreateChoreRequest`) will fail this lane. The build sequence is: build the app target alone first (so `@testable import Nabu` resolves on a clean checkout), then `build-for-testing`, then `test-without-building -only-testing:NabuTests`. UI tests are not run in CI.
