# Repository Guidelines

This file provides guidance to an LLM when working with code in this repository.

## Agent model setup

Use the Task tool to launch subagents for codebase exploration, CI babysitting, production verification, and other parallelisable work. Subagents are configured to use a less capable, cheaper model than the primary session — this is intentional.

After pushing a `v*` tag, always launch a subagent to watch CI to completion and verify production. Do not wait for the user to ask.

## Git worktrees

Always use a git worktree for any code change — never work directly in the main checkout.

```bash
# Create a worktree inside the repo directory (use a short descriptive name)
git worktree add worktrees/<name> -b <name>

# Work in the worktree
cd worktrees/<name>

# When done (after merging/deploying), remove it
git worktree remove worktrees/<name>
git branch -d <name>
```

The main checkout at the workspace root stays clean and is only used for reference. All edits, commits, and test runs happen inside the worktree.

## Commands

| Task | Command |
|------|---------|
| Run all tests | `make test` |
| Go tests only | `make test-go` |
| JS tests only | `make test-js` |
| Single Go test | `go test ./internal/config/ -run TestLoadDefaults` |
| Format Go | `make fmt` |
| Run server (no DB) | `make run` |
| Local stack (Podman) | `make local` |
| Rebuild stack | `make local-fresh` |
| Seed test user | `make seed` |
| E2E tests | `make e2e` |
| E2E in browser (headed) | `make e2e-watch` |
| E2E debug mode (step-through) | `make e2e-debug` |
| Go coverage | `make coverage` |
| Lint Go | `make lint` |

After changing files in `web/templates/` or `web/static/`, run `make local-fresh` — these assets are embedded into the Go binary via `web/assets.go` (`//go:embed`).

CI runs `go vet ./...` for lint (not golangci-lint). `make lint` uses golangci-lint v2.6.2 and self-bootstraps the binary into `.cache/` on first run.

## Prerequisites

- **Go 1.25+** (CI uses 1.25 to match `go.mod`).
- **JS tests require `pnpm install` first** for `jsdom` (dev dependency). Tests use Node's built-in test runner (`node --test`), not Jest or Mocha.
- **E2E tests require `pnpm exec playwright install chromium`** to download the browser binary.
- **Podman Compose** for local stack (`make local`). Docker Compose may work but is untested.

## Architecture

**Go standard-library HTTP server** with no web framework. Dependencies: `pgx/v5` (Postgres) and `golang.org/x/crypto` (bcrypt). Frontend is **plain ES modules** — no bundler, no framework, no build step.

### Backend (`internal/`)

- **`app/server.go`** — All route registration in one place via `http.ServeMux`. Routes use `method()` helper for HTTP verb enforcement (some with `RequireAuth` wrapper).
- **`handlers/`** — HTTP handlers grouped by feature. Handlers call services; they contain no business logic.
- **`middleware/`** — Applied in order: RequestLogger → SecurityHeaders → Session → CSRF → RateLimiter. RateLimiter only applies to `/api/auth` routes. Session middleware attaches user to context; handlers check with `middleware.CurrentUser(ctx)`.
- **`database/`** — Connection setup + migration runner. Migrations are embedded SQL files from `migrations/` (via `//go:embed` in `migrations/assets.go`), applied at startup. Migration order follows `fs.ReadDir` order, not filename sort — check ordering carefully.
- **`audit/`** — Audit logging interface + std logger implementation.

Session cookie name: `choresy_session`. CSRF cookie name: `choresy_csrf`.

### Frontend (`web/static/js/`)

- **`app.js`** — Entry point. Wires all event listeners on a single `#app` container via event delegation.
- **`state.js`** — `createAppState()` returns the single mutable state object. All UI derives from this.
- **`morph.js`** — DOM morphing: updates existing DOM to match new HTML without destroying focus or form state.
- **`api.js`** — `apiFetch()` wraps `fetch()` adding `Content-Type` and CSRF token for state-changing requests.
- **`today.js`** — Chore grid, tap-to-log, `logChore(choreId, note, date, indicators, slotHour, completedAt)` API wrapper. `slotHour` is an integer (0–23) or `null`.
- **`calendar.js`** — Day and week calendar views. Ad-hoc log placement: `slotHour === null` → Anytime row; `slotHour === hour` → that hour row. Both `renderDayView` and `renderWeekView` follow this rule.
- **`home.js`** — Home-tab grid and quick-log sheet. All log paths from the home tab must pass `slotHour`; nothing from the home tab should ever land in the Anytime row.
- **`schedule.js`** — Schedule CRUD, pick-chore sheet, quick-log sheet, drag-and-drop rescheduling.
- **`preferences.js`** — Chore ordering preferences.

### Key patterns

- **Service/Store separation**: Services hold business logic, stores hold persistence. Both have memory and Postgres implementations. When `DATABASE_URL` is empty, everything uses in-memory stores.
- **Dependency injection via function args**: `BuildServer()` in `app/server.go` wires all dependencies; there are no global singletons.
- **Optimistic UI**: Frontend updates state before server confirms; rolls back on error.
- **`apiFetch()`** adds `X-CSRF-Token` header read from `choresy_csrf` cookie for all state-changing requests.
- **`slotHour` in logs**: `POST /api/logs` accepts `hour` (integer) in the JSON body → stored as `slot_hour` in the DB → drives calendar placement. A missing or null `hour` puts the log in the Anytime row. Always pass `hour` from timed UI paths.

### JS static file serving and cache busting

**Do not change this mechanism without understanding it fully.**

At startup, `buildVersionedJSCache` in `internal/app/server.go` walks every `.js` file in the embedded FS and rewrites all relative ES module import paths to include `?v=<version>` (e.g. `from './calendar.js'` → `from './calendar.js?v=0.1.6'`). The rewritten content is held in memory and served with `Cache-Control: no-store`.

Why this exists: Cloudflare sits in front of production and overrides `Cache-Control: no-cache` with `max-age=14400` (4 hours). `no-store` is stronger — Cloudflare responds with `cf-cache-status: BYPASS` and does not cache at all. The versioned import paths additionally bust the browser module cache on every deploy, since each new version produces new URLs for every module in the import graph.

Rules that follow from this:
- **Never add `?v=anything` manually to a relative import in JS source.** The rewriter skips paths that already contain `?`, so a hard-coded version will not be updated on deploy and will serve stale code.
- **Always verify after a deploy** (see Production section below).
- If you add a new JS module that itself imports other modules, the rewriter handles it automatically — no extra work needed.

## Local dev stack

`make local` starts via Podman Compose: app on `:8080`, Mailpit on `:8025`, Postgres on `:5432`.

When `DATABASE_URL` is empty, the server falls back to in-memory stores (useful for `make run` without Podman).

## Test credentials

Local seed: `test@choresy.local` / `correct horse battery`. Stack must be running (`make local`) before `make seed`.

Production test account: `verify@yearofbingo.com` / `test123456` (household and seeded chores already set up).

## Production

- **URL**: `https://choresy.yearofbingo.com`
- **Deploy trigger**: push a `v*` tag (e.g. `git tag v0.1.7 && git push origin v0.1.7`). CI builds, tests, and deploys automatically.
- **CI**: `.github/workflows/ci.yaml` — runs secret scan, JS tests, lint, Go tests (with coverage), and E2E tests before deploying.

### Verifying a production deploy

After CI goes green, confirm the correct version is serving versioned imports:

```bash
# Check that JS imports carry the new version tag
curl -s https://choresy.yearofbingo.com/static/js/calendar.js | grep "^import"
# Expected: import { ... } from "./utils.js?v=0.1.X";

# Check cache headers — must be no-store, must NOT be max-age
curl -sI https://choresy.yearofbingo.com/static/js/app.js | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: BYPASS
```

If `cf-cache-status` is `HIT` or `MISS` (not `BYPASS`), the `no-store` header is not reaching Cloudflare — investigate `server.go`.

If imports still show the old version number, the binary was not rebuilt with the new tag — check that `internal/version/version.go` (or equivalent) is populated at build time via `-ldflags`.

### Checking the version endpoint

```bash
# The index page embeds the version; check it with:
curl -s https://choresy.yearofbingo.com/ | grep 'app.js'
# Expected: <script ... src="/static/js/app.js?v=0.1.X" ...>
```

## CI / Deploy babysitting

After pushing a `v*` tag, an agent should monitor the pipeline to completion and verify production. Use this process:

### 1. Watch the CI run

```bash
# Find the run ID for the tag
gh run list --limit 5

# Stream logs until the run completes (blocks until done)
gh run watch <run-id>

# If a job fails, check which step failed
gh run view <run-id> --json jobs \
  --jq '.jobs[] | {name: .name, conclusion: .conclusion, steps: [.steps[] | select(.conclusion == "failure") | .name]}'

# Re-run only failed jobs (for transient infra errors)
gh run rerun <run-id> --failed
```

### 2. Distinguish transient vs. real failures

- If **only** the checkout/setup step failed and all test jobs passed → transient GitHub Actions infra error → re-run with `gh run rerun <run-id> --failed`.
- If a test job (Go Tests, JS Tests, E2E, Lint) failed → real failure → read the full log, fix the code, commit, re-tag, and push a new `v*` tag.

### 3. Verify production after deploy

Once the `Deploy to Production` job goes green:

```bash
# Confirm versioned imports carry the new tag
curl -s https://choresy.yearofbingo.com/static/js/calendar.js | grep "^import"
# Expected: import { ... } from "./utils.js?v=0.1.X";

# Confirm cache headers
curl -sI https://choresy.yearofbingo.com/static/js/app.js | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: BYPASS

# Confirm correct version in index page
curl -s https://choresy.yearofbingo.com/ | grep 'app.js'
# Expected: src="/static/js/app.js?v=0.1.X"
```

If `cf-cache-status` is `HIT` or imports show the old version, the deploy did not take — investigate `server.go` and the CI build logs.

## E2E tests

**Every new feature and every bug fix must include a Playwright E2E test.**  Do not skip this step; do not wait for the user to ask.

### What to cover

- **Bug fix**: write a test that reproduces the bug (it should fail on the old code), then verify the fix makes it pass.
- **New feature**: write tests that exercise the happy path, the sad path (cancel / error), and any persistence guarantees (e.g. reload the page and confirm state survived).

### Workflow

1. Add the spec file to `tests/e2e/` alongside the existing specs.
2. Run `make local-fresh` to rebuild the app binary with any template/asset changes, then `make e2e` to run the full suite.  All tests — old and new — must pass before committing.
3. If the local stack (`make local`) is already running with the old binary, run `make local-fresh` first; otherwise the new code won't be loaded.

### Patterns

Follow the conventions in the existing specs:

- **`uniqueEmail()`** — generate a unique address per test to avoid cross-test contamination.
- **`setupWithChores(page)`** — register a user, create a household, seed defaults, reload, wait for `.home-grid`.  Copy and adapt this helper for each spec file that needs it.
- **`longPress(page, locator)`** — simulate a 650 ms mousedown to trigger the 500 ms long-press threshold.
- Use `page.request.post/patch/get` for direct API calls (bypasses the UI where appropriate).
- Wait for DOM changes with `expect(...).toBeVisible()` / `toHaveCount()` rather than fixed `waitForTimeout` calls wherever possible.  Use `waitForTimeout` only when an animation or async side-effect has no observable DOM signal.

### Spec file naming

Name spec files after the feature/area: `<area>-<feature>.spec.js` (e.g. `home-remove-chore.spec.js`, `schedule-drag.spec.js`).

## Style

- `go fmt` for Go. No configured JS linter.
- Keep packages focused by domain; HTTP-only logic in `handlers/`.
- Frontend: clear DOM-oriented functions over framework abstractions. Render functions return HTML strings.
- 80% minimum Go statement coverage target.

## Key invariants — do not break

These caused hard-to-diagnose production bugs and are covered by E2E tests:

1. **Home-tab direct tap** (`home-tap-chore` event in `app.js`): must call `logChore(..., new Date().getHours(), new Date().toISOString())` — the `slotHour` from `getHours()` drives calendar placement, and `completedAt` as the current time prevents the home-tab "time ago" from being wrong by the UTC offset. 
2. **Home-tab sheet log** (`save-home-log` event in `app.js`): must extract `new Date(whenInput.value).getHours()` and pass as `slotHour`.
3. **`renderWeekView` in `calendar.js`**: ad-hoc logs (those not matching a scheduled slot) must be placed in their `slotHour` row, not forced into the Anytime row — mirrors the `adHocCells` pattern in `renderDayView`.
4. **No hard-coded `?v=N` in JS import paths** — the server rewrites them all at startup.

## Push notification troubleshooting

See `PUSH_DEBUG.md` for the diagnostic playbook. The key gotcha: the HKDF chain in `internal/push/encrypt.go` must match `http_ece` (npm) exactly — Apple returns 201 even when the encryption keys are wrong, so there is no error signal at the gateway. The only way to know the push arrived is to check `self.lastPush` or `self.__diag` in the service worker.

### Push architecture (`internal/push/`)

- `encrypt.go` — RFC 8291 aes128gcm encryption. HKDF chain: HKDF-Extract(auth, DH) → HKDF-Expand("WebPush: info\0" + clientPub + ephemeralPub, 32) → HKDF-Extract(randomSalt, secret) → HKDF-Expand("Content-Encoding: aes128gcm\0", 16) for CEK, HKDF-Expand("Content-Encoding: nonce\0", 12) for nonce.
- `vapid.go` — VAPID JWT signing (ES256). JWT header must contain ONLY `typ` and `alg` — no `kty`/`crv`.
- `service.go` — HTTP POST to push endpoint. Uses `Content-Encoding: aes128gcm`, `TTL: 60`, VAPID `Authorization` header.

### iOS PWA debugging

Use `remotedebug-ios-webkit-adapter` (npm) to connect Chrome DevTools Protocol to an iPhone PWA. Run it on the Mac, then connect via WebSocket to evaluate JS in the main page:

```bash
remotedebug_ios_webkit_adapter --port 9222
# Browse to http://localhost:9222/json to list pages
# WebSocket: ws://localhost:9222/ios_<UDID>/ws://127.0.0.1:<port>/devtools/page/<n>
```

The service worker debugging page is listed but often unresponsive. Use the main page to relay diagnostics via `MessageChannel`:

```js
// The SW (v0.1.37+) responds to "push-diag" message:
sw.postMessage("push-diag", [channel.port2])
// Returns: { lastPush, diag: [...], registration }
```

`self.lastPush` being `{}` means no push event ever fired. The `diag` array logs `push-received`, `push-decode-error`, and `subscriptionchange` events.

`showNotification()` from the main page tests the notification display path independently of push delivery. If it works but pushes don't arrive, the issue is in the server-side encryption or VAPID headers.
