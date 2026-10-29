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
