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

## PR description requirements

When submitting a PR that touches iOS or PWA code, the description must state one of:

- "PWA and iOS both updated."
- "PWA-only change; iOS not affected because \<reason\>."
- "iOS-only change; PWA not affected because \<reason\>."
