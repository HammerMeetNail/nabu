# iOS Client Agent Instructions

This file provides guidance to an LLM when working on the Nabu native iOS app.

## Client parity rule

The Nabu project has two clients: a PWA (`web/static/js/`) and a native iOS app (`ios/`). Every feature, bug fix, validation change, security fix, API change, or UI behavior change must be evaluated for both clients.

**Before editing iOS code**, inspect the corresponding PWA module under `web/static/js/` and the corresponding Playwright E2E spec under `tests/e2e/`. The iOS app must mimic PWA behavior unless `docs/plans/ios.md` or a follow-up product decision says otherwise.

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

**Run the E2E test:**

```bash
xcodebuild test \
  -project ios/Nabu.xcodeproj \
  -scheme Nabu \
  -destination 'platform=iOS Simulator,name=iPhone 16' \
  -only-testing:NabuUITests/NabuHomeEndToEndUITests
```

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

- Root plan: `docs/plans/ios.md`
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
4. Do not invent new behavior because it feels more iOS-like unless `docs/plans/ios.md` explicitly allows it.
5. If PWA behavior appears buggy, stop and ask before intentionally diverging.
6. If a backend endpoint lacks native support, add backend tests before changing the backend.
7. Keep API DTO names close to server JSON names.
8. Use `Int` for Go `int64` IDs unless a test proves overflow risk.
9. Use `Date` for RFC3339 timestamps and `LocalDate` for `YYYY-MM-DD` values.
10. Never update only one client when behavior is shared.

## Parity bookkeeping on PRs

When a PR touches shared client surface — `ios/**`, `web/static/js/**`, or the shared API in `internal/handlers/**` — it must also update the parity matrix (`docs/plans/client-parity.md`) to reflect the change. The CI `parity` job enforces this (it lints the matrix and fails if client/API code changed without a matrix update); the escape hatch is a `no-parity-update: <reason>` line in the PR body.

(This replaced an older rule that required a "PWA-only change…" / "iOS-only change…" phrase in the PR description — that phrase is no longer what CI checks.)

## iOS tests run in CI

The `ios` CI job (`.github/workflows/ci.yaml`, macOS runner) builds the app and runs the `NabuTests` unit/contract suite on every change under `ios/**`. Keep `NabuTests` green — a backend model/API change that breaks the iOS request models (e.g. an added field on `CreateChoreRequest`) will fail this lane. The build sequence is: build the app target alone first (so `@testable import Nabu` resolves on a clean checkout), then `build-for-testing`, then `test-without-building -only-testing:NabuTests`. UI tests are not run in CI.
