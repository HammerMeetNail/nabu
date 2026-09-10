# Nabu iOS app

Native SwiftUI client for Nabu, using the existing Go backend JSON API.

## Project and layout

Open `ios/Nabu.xcodeproj`. It contains the app and test targets and manages Swift package dependencies, including SnapshotTesting. The current deployment target is iOS 18; [project.pbxproj](Nabu.xcodeproj/project.pbxproj) is the source of truth for build settings. There is no standalone `ios/Package.swift`.

```text
ios/
  Nabu.xcodeproj/   # Project, schemes, dependencies, target membership
  Nabu/
    App/           # App lifecycle, environment, state, navigation
    API/           # HTTP, cookie/CSRF handling, models, stores, data loaders
    Auth/          # Authentication and onboarding
    Views/         # Screens, including Views/Stats/ for charts
    DesignSystem/  # Shared colors and components
    Support/       # Dates, timers, offline queue, push, other helpers
    Resources/     # Assets, app metadata, entitlements
    ContentView.swift
  NabuTests/       # XCTest contracts/logic and snapshot baselines
  NabuUITests/     # XCUITest flows
  TestSupport/    # Mock API and fixture support
```

## Build and test

Use macOS with Xcode and a compatible installed simulator. The backend toolchain is specified by [go.mod](../go.mod); real-server flow tests also need the local backend on port 8080.

[ios/AGENTS.md](AGENTS.md#validation) contains the maintained build and test commands. Select a simulator by UDID, build the app first, then build the tests and use `test-without-building`. Both test targets use the `Nabu` scheme. Snapshot suites have runtime-specific baselines; record skips separately from passing tests.

CI runs `NabuTests` for iOS changes and release tags. It does not run XCUITest flows or automatically validate native contracts on backend-only PRs.

## Test launch arguments

These arguments are consumed by the app's test support; inspect [TestHooks.swift](Nabu/Support/TestHooks.swift) and the relevant UI test when changing them.

| Argument | Purpose |
|---|---|
| `-nabuBaseURL <url>` | Override backend URL |
| `-resetState` | Clear persisted state on launch |
| `-disableAnimations` | Disable animations for tests |
| `-useMockAPI` | Use mock API support |
| `-nabuAutoRegister <email> <password>` | Provision a test account/household without driving onboarding UI |

## Related documentation

- [iOS agent instructions](AGENTS.md)
- [Repository workflow](../AGENTS.md)
- [Active iOS App Store plan](../docs/plans/ios-appstore-v1.md)
- [Client parity matrix](../docs/plans/client-parity.md)
- [Historical conversion plan](../docs/plans/ios.md)
