# Nabu iOS App

Native SwiftUI client for Nabu, targeting iPhone. Uses the existing Go backend JSON API.

## Project overview

- **Language**: Swift (SwiftUI)
- **Minimum iOS**: TBD (likely iOS 17+)
- **Project format**: Xcode project + Swift Package Manager
- **Test targets**: NabuTests (XCTest unit), NabuUITests (XCUITest)

## Directory layout

```text
ios/
  Nabu.xcodeproj/
  Package.swift
  Nabu/
    App/          # App entry point, state, environment, navigation
    API/          # API client, error handling, cookie/CSRF, models
    Auth/         # Login, register, magic link, OAuth
    Household/    # Household CRUD, members, invites
    Home/         # Home grid, log sheet, chore management
    Activity/     # History, day calendar, week calendar
    Schedule/     # Schedule list and CRUD
    Chores/       # Chore management
    Notifications/# In-app notifications and push
    Stats/        # All stats views and charts
    Settings/     # Account, household, preferences
    DesignSystem/ # Colors, typography, reusable components
    Support/      # Date formatting, recurrence logic, timezone sync
  NabuTests/      # Unit and contract tests
  NabuUITests/    # UI tests (XCUITest)
  TestSupport/    # Mock API server, fixture factory
```

## Prerequisites

- macOS with Xcode (version TBD)
- Go 1.25+ (for backend)
- Running backend for integration tests (see root `AGENTS.md` for `make local`)

## Test commands

### Unit and contract tests

```bash
xcodebuild test \
  -project ios/Nabu.xcodeproj \
  -scheme Nabu \
  -destination 'platform=iOS Simulator,name=iPhone 16'
```

### UI tests

```bash
xcodebuild test \
  -project ios/Nabu.xcodeproj \
  -scheme NabuUITests \
  -destination 'platform=iOS Simulator,name=iPhone 16'
```

### Single test

```bash
xcodebuild test \
  -project ios/Nabu.xcodeproj \
  -scheme Nabu \
  -destination 'platform=iOS Simulator,name=iPhone 16' \
  -only-testing:NabuTests/APIClientTests/testGETSuccess
```

## Launch arguments for tests

| Argument | Purpose |
|----------|---------|
| `-nabuBaseURL <url>` | Override backend base URL |
| `-resetState` | Clear persisted state on launch |
| `-disableAnimations` | Disable SwiftUI animations |
| `-useMockAPI` | Use `MockAPIServer` instead of real backend |

## Related documentation

- [iOS conversion plan](../docs/plans/ios.md)
- [Client parity matrix](../docs/plans/client-parity.md)
- [Repository guidelines](../AGENTS.md)
- [iOS agent instructions](./AGENTS.md)
