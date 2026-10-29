# Nabu iOS App

Native SwiftUI client for Nabu, targeting iPhone. Uses the existing Go backend JSON API.

## Project overview

- **Language**: Swift (SwiftUI), no third-party dependencies in the app target
- **Minimum iOS**: 18.0
- **Project format**: Xcode project (`Nabu.xcodeproj`); there is no `Package.swift`
- **Targets**: Nabu (app), NabuWidgets (widget extension), NabuTests (XCTest unit/contract/snapshot), NabuUITests (XCUITest)

## Directory layout

```text
ios/
  Config/          # Build settings every target shares: versions and the server (see below)
  Nabu.xcodeproj/
  Nabu/
    App/           # App entry point, AppState, AppEnvironment (incl. the -useMockAPI transport), navigation
    API/           # API client, request/response models, per-feature stores and data loaders
    Auth/          # Login, register, magic link, Sign in with Apple, account deletion model
    DesignSystem/  # Colors, typography, reusable components
    Resources/     # Asset catalog, Info.plist, entitlements
    Support/       # Date formatting, offline log queue, duration timers, test hooks
    Views/         # All screens (Home, LogSheet, Activity, Schedule, Stats/, Settings = HouseholdView)
    ContentView.swift
  NabuWidgets/     # Home-screen widget extension
  NabuTests/       # Unit, contract and snapshot tests
  NabuUITests/     # UI tests (XCUITest)
  TestSupport/     # MockAPIServer
```

## Prerequisites

- macOS with Xcode (CI builds with the `macos-15` runner's default Xcode); snapshot tests run only on an iOS 26 simulator and self-skip elsewhere
- Go matching `go.mod` (for the backend)
- Running backend for integration tests (see root `AGENTS.md` for `make local`)

## Test commands

Pick a simulator by UDID (`xcrun simctl list devices available`); a name such
as `iPhone 16` can match several runtimes. Build the app before the test
bundle, as CI does, or `@testable import Nabu` can race the app module. See
`ios/AGENTS.md` for the full sequence.

```bash
export NABU_SIMULATOR_ID="<simulator-udid>"
xcodebuild build -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID"
xcodebuild build-for-testing -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID"

# Unit and contract tests
xcodebuild test-without-building -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" -only-testing:NabuTests

# One test
xcodebuild test-without-building -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" \
  -only-testing:NabuTests/ModelDecodingTests/testDecodeOverview

# UI tests
xcodebuild test-without-building -project ios/Nabu.xcodeproj -scheme Nabu \
  -destination "platform=iOS Simulator,id=$NABU_SIMULATOR_ID" -only-testing:NabuUITests
```

## Versions and the server

`ios/Config/Shared.xcconfig` holds the settings every target shares. The project's Debug and Release configurations are based on `Debug.xcconfig` and `Release.xcconfig`, which include it.

- **`MARKETING_VERSION`** is the App Store version (`1.0.0`). Raise it for each release.
- **`CURRENT_PROJECT_VERSION`** is the build number, which must grow with every upload. An archive passes its own: `xcodebuild archive ... CURRENT_PROJECT_VERSION=42`.
- **`NABU_BASE_URL`** is the server. The app reads it from `NabuBaseURL` in its Info.plist and falls back to production if that's missing or malformed. Both configurations use production.

Don't set any of these on a target. Every target inherits them, which keeps the widget extension's version equal to the app's, as App Store Connect requires (`BuildConfigurationTests` checks it).

To point a debug build at another server, such as the phone preview, put `NABU_BASE_URL = https:/$()/your-host.example` in `ios/Config/Local.xcconfig`. Git ignores that file and release builds never read it. In an xcconfig, `//` starts a comment, hence the `$()`. A Staging configuration is a `Staging.xcconfig` with its own `NABU_BASE_URL`, once a staging server exists.

## Releasing

The web app and the iOS app ship separately, and the web app always goes out first. The **iOS Release** workflow (`.github/workflows/ios-release.yml`) is the release button: in GitHub, open **Actions → iOS Release → Run workflow** and run it on `main`.

1. **Raise the version** for a new App Store version: `MARKETING_VERSION` in `ios/Config/Shared.xcconfig`, through a PR. A TestFlight build of the same version needs no change.
2. **Run the workflow.** It runs three jobs:
   - **Web app is live** (`scripts/ios-release-web-first.sh`): reads the version production runs from `/health` and compares the server code (`cmd/`, `internal/`, `migrations/`, `go.mod`, `go.sum`, not tests) at that release's tag with `main`. If the server changed, it stops and asks for a web deploy first (tag `main`). Changes to the PWA, the app, docs or tests don't hold the app back.
   - **iOS Unit Tests**: `NabuTests`, as in CI.
   - **Archive and upload**: archives the Release configuration with the build number set to the number of commits on `main`, which grows with every release, and uploads it to App Store Connect. The build appears in TestFlight once Apple has processed it; submit it for review from App Store Connect.
3. **The run's summary** gives the version, the build number and the commit.

**Uploading needs an App Store Connect API key.** Create one in App Store Connect under Users and Access → Integrations → App Store Connect API, with the App Manager role. Then save three repository secrets:
- `ASC_KEY_ID`: the key ID
- `ASC_ISSUER_ID`: the issuer ID
- `ASC_KEY_P8`: the contents of the downloaded `.p8` file

Signing is automatic: Xcode uses the key to fetch or create the distribution certificate and profiles for the project's team (`ios/ExportOptions.plist`). Without the secrets, the workflow still checks, tests and builds an unsigned archive, and its summary says the upload was skipped.

## Launch arguments for tests

Only debug builds read these (`Support/TestHooks.swift`): a release build ignores every launch argument and environment override, so it always talks to production with real state.

| Argument | Purpose |
|----------|---------|
| `-nabuBaseURL <url>` | Override backend base URL |
| `-resetState` | Clear persisted state on launch |
| `-disableAnimations` | Disable SwiftUI animations |
| `-useMockAPI` | Use the in-app mock transport instead of a real backend |
| `-seedHomeForUITest` | Seed a signed-in household with fixture chores |

## Related documentation

- [iOS App Store v1 plan](../docs/plans/ios-appstore-v1.md) (active)
- [iOS conversion plan](../docs/plans/ios.md) (superseded, historical)
- [Client parity matrix](../docs/plans/client-parity.md)
- [Repository guidelines](../AGENTS.md)
- [iOS agent instructions](./AGENTS.md)
