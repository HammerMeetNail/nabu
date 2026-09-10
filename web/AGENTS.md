# PWA instructions

Read the [root guide](../AGENTS.md), and evaluate corresponding iOS behavior before editing shared functionality. The frontend is plain ES modules, HTML, and CSS; there is no framework, bundler, or frontend build step.

## Where behavior lives

| File under `web/static/js/` | Responsibility |
|---|---|
| `app.js` | Entry point, rendering, and delegated events on `#app` |
| `state.js` | `createAppState()` and shared mutable UI state |
| `morph.js` | Updates DOM while preserving focus and form state |
| `api.js` | `apiFetch()`, JSON requests and CSRF headers |
| `today.js` | Log API wrappers, including `logChore` and `updateLog` |
| `home.js` | Home grid and logging presentation |
| `calendar.js` | Day/week calendar rendering and log placement |
| `schedule.js` | Schedule CRUD, sheets, and drag/rescheduling |
| `preferences.js` | Chore-order preferences |

Trace the actual event handler, state transition, API call, and render function for a change. `logChore` has several optional positional arguments; read its current signature and search all callers instead of copying an abbreviated signature from documentation. Reuse the existing render/state patterns and shared utilities.

Preserve optimistic updates and rollback on failure. For asynchronous work, account for navigation, logout/household changes, stale responses, and queued work when those paths are affected. Tests should synchronize on observable state, not only on a promise being scheduled.

## Logging and calendar invariants

- `home-tap-chore` currently opens the Home log sheet; it does not immediately create a log. Preserve that interaction unless the task changes it.
- Timed log paths must pass a local-hour integer as `slotHour` to `logChore`, which sends request field `hour`. A missing/null hour means Anytime. For a direct current-time log, send the local current hour and the current ISO timestamp together so relative-time displays remain correct across timezones.
- For a **new** sheet log (`save-log` with empty `logId`), derive `completedAt`, `slotHour`, and `date` from the live `#log-when` input. Use `new Date(whenInput.value).getHours()` for the local hour. Prefill with the current local time, including minutes; do not round to `:00` or read stale button data attributes.
- For **editing** an existing log, preserve the comparison-based guard against `morph.js` corrupting form values during rerenders. Test rerender/focus/time behavior when changing this code.
- In both `renderDayView` and `renderWeekView`, ad-hoc logs with `slotHour === null` go in Anytime; `slotHour === hour` goes in that row. Do not replace stored slot placement with an hour inferred from `completedAt`.

Existing regression examples include [home-when-picker.spec.js](../tests/e2e/home-when-picker.spec.js), [home-time-accuracy.spec.js](../tests/e2e/home-time-accuracy.spec.js), and [home-log-to-calendar.spec.js](../tests/e2e/home-log-to-calendar.spec.js).

## Rendering and request safety

- Use `escapeHTML()` from [static/js/utils.js](static/js/utils.js) for user/database strings interpolated into HTML, SVG text, attributes, titles, and aria labels. Do not duplicate the helper.
- Treat chore icons, names, category, indicator labels/defaults, notes, member names, and other metadata as untrusted. Splitting a label or expecting emoji does not sanitize it. Validate colors server-side and escape attribute values at the rendering sink.
- Keep state-changing requests on `apiFetch()` so `X-CSRF-Token` comes from the `nabu_csrf` cookie. Client validation improves usability but must match backend validation.
- Escaping fixes need a test for the actual render function/sink with hostile data, plus browser coverage for the affected user flow. A helper-only test cannot prove every caller escapes correctly.

## Embedded assets and cache busting

[assets.go](assets.go) embeds templates and static files into the Go binary. After changing runtime files in `web/templates/` or `web/static/`, rebuild/restart the owned server before browser checks. `make local` rebuilds the Compose app without deliberately deleting its database; `make local-fresh` resets data via `down -v` and is only for an intentionally disposable stack.

The server's `buildVersionedJSCache` rewrites relative imports at startup and serves JS with `Cache-Control: no-store`. **Never manually add `?v=...` to a source import**: already-queried imports are skipped and can remain stale after deploy. New modules require no special registration. Read the [README rationale](../README.md#js-static-file-serving-and-cache-busting) before changing serving/caching and verify releases through the [runbook](../docs/deploy-runbook.md).

## Validation and parity

- JS render/unit tests live in [static/js/tests/runner.js](static/js/tests/runner.js), using Node's test runner and jsdom. Run `make test-js` from the worktree root. There is no configured JS linter; `node --check <file>` is a useful focused syntax check.
- Every PWA feature or behavior fix needs a Playwright regression. Read [tests/e2e/AGENTS.md](../tests/e2e/AGENTS.md); cover happy paths and applicable cancellation, errors, reload persistence, and timezone/placement behavior. Run the focused regression, then the full browser suite on the rebuilt server before committing.
- Inspect UI changes in an available real browser tool/Playwright workflow. Screenshots support visual checks; assertions establish behavior. Keep troubleshooting artifacts outside the checkout.
- Complete the root runtime-code gate. Evaluate SwiftUI views, request/response models, native tests, and relevant snapshots for shared changes, and update the parity matrix. PWA-specific presentation can differ when the behavior and reason are documented.
