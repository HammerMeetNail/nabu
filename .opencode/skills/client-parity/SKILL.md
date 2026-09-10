---
name: client-parity
description: Evaluate Nabu PWA and native iOS behavior, API contracts, tests, and parity bookkeeping when a change affects either client or shared backend behavior.
license: MIT
metadata:
  audience: all
  workflow: check-before-commit
---

# Client parity

This repository-local workflow can be read directly by any coding agent; an installed skill or `/client-parity` command is not required. Paths below are relative to the task worktree root. Follow the user's task and the root/scoped `AGENTS.md` instructions.

## Evaluate the affected behavior

1. Read the actual implementation and tests for the requested change. Identify whether it affects requests/responses, validation, authorization, persistence, logging times, or presentation only.
2. Inspect the corresponding PWA and iOS code. Shared behavior must match; SwiftUI presentation should use native idioms. Make matching changes within the task, and record intentional platform differences with a concrete reason.
3. Check [the parity matrix](../../../docs/plans/client-parity.md) for the affected rows. Update behavior, status, test references, and known differences accurately. A pre-existing pending row calls for a scope decision; it does not require completing unrelated backlog. Do not mark work Done just because source exists or a linter passes.
4. For backend/API changes, check Swift `Models.swift` / `RequestModels.swift`, native store/data-loader calls, JS API consumers, and fixtures in `ios/NabuTests/Fixtures/`. A backend-only PR does not automatically run the native CI lane.
5. Add/run coverage at the changed layer following the root and platform guides. PWA flows use Playwright; native flows use XCTest/XCUITest and relevant snapshots. Report unavailable platform checks and runtime skips explicitly.

If a known bug is within the task, correct the shared behavior rather than reproducing it in the other client. Ask only when an unresolved product or scope decision needs the user; continue independent work.

## Matrix lint and PR requirements

Run from the task worktree root:

```bash
make check-parity
```

The script validates status names and reports a tally. Exit 0 means the matrix is well-formed; it does not prove feature parity or test coverage. Known statuses are `Done`, `Built`, `iOS pending`, `PWA pending`, `Deferred`, `Not built`, and `N/A`. `bash scripts/check-parity.sh --strict` additionally fails on pending rows when a release/task requires that check; it is not the default per-PR gate and still does not verify implementation.

The [CI parity job](../../../.github/workflows/ci.yaml) checks changes under `web/static/js/**`, `web/static/css/**`, `web/templates/**`, `ios/**`, and `internal/handlers/**`. A matching PR must update `docs/plans/client-parity.md`, or include `no-parity-update: <reason>` when no matrix change is appropriate. Documentation-only changes under `ios/` are one example. The older three prescribed “both updated / PWA-only / iOS-only” phrases are not required by this CI job.

Describe the actual parity outcome and validation in the PR. The local pre-push hook has a separate legacy client-path check; follow the root guide's documented handling of that check, without skipping its build/test stages. An unrelated pending feature is not a reason to block a correctly scoped change.

## Useful counterparts

| PWA | iOS |
|---|---|
| `web/static/js/app.js`, `state.js` | `ios/Nabu/ContentView.swift`, `App/AppState.swift` |
| `web/static/js/api.js` and feature API wrappers | `ios/Nabu/API/APIClient.swift`, `Models.swift`, `RequestModels.swift` |
| `web/static/js/home.js`, `today.js` | `ios/Nabu/Views/HomeView.swift`, `LogSheet.swift`, `API/LogStore.swift` |
| `web/static/js/calendar.js` | `ios/Nabu/Views/ActivityView.swift`, `API/ActivityStore.swift` |
| `web/static/js/schedule.js` | `ios/Nabu/Views/ScheduleView.swift`, `API/ScheduleStore.swift` |
| `web/static/js/stats.js` | `ios/Nabu/Views/StatsView.swift`, `Views/Stats/` |
| `web/static/js/chores.js` | `ios/Nabu/Views/ChoreEditView.swift`, `API/ChoreStore.swift` |
| `web/static/js/household.js` | `ios/Nabu/Views/HouseholdView.swift` |
| `web/static/js/notifications.js` | `ios/Nabu/Views/NotificationPreferencesView.swift`, `NotificationsView.swift` |
| `web/static/js/preferences.js` | `ios/Nabu/API/Data/PreferencesDataLoader.swift` |

Use `rg` to locate the current caller and real test coverage; this table is a starting map, not a complete inventory.
