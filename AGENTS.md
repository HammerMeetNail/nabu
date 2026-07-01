# Repository Guidelines

This file provides guidance to an LLM when working with code in this repository.

## Agent model setup

Use the Task tool to launch subagents for codebase exploration, CI babysitting, production verification, and other parallelisable work. Subagents are configured to use a less capable, cheaper model than the primary session — this is intentional.

**Subagent scope limits**: Subagents handle well-defined, standalone tasks: monitoring CI, verifying production deploys, searching the codebase for patterns, or reading files in bulk. If a subagent encounters complexity, ambiguity, or a task that requires design decisions, it must stop and report back to the primary agent — never re-implement features, make design choices, or produce code changes. Kick the work back.

**Mandatory delegation to the `git-ops` subagent**: commit, push, and `gh pr create` are mechanical, no-design-decision tasks and MUST be delegated to the `git-ops` subagent once the primary session has staged-ready changes. Do not run `git commit`, `git push`, or `gh pr create` inline in the primary session. The primary session is still responsible for the worktree setup, the pre-push build/test/lint checklist, deciding the commit message intent, and choosing the correct client-parity statement — hand those to the subagent as input. The subagent refuses to edit code or make design decisions; if anything is ambiguous it reports back.

After pushing a `v*` tag, always launch a subagent to watch CI to completion and verify production. Do not wait for the user to ask.

## Git worktrees

Always use a git worktree for any code change — never work directly in the main checkout.

```bash
# First, pull latest main so your worktree starts from the current state
git fetch origin && git checkout main && git pull origin main

# Create a worktree inside the repo directory (use a short descriptive name)
git worktree add worktrees/<name> -b <name>

# Work in the worktree
cd worktrees/<name>

# When done (after merging/deploying), remove it
git worktree remove worktrees/<name>
git branch -d <name>
```

The main checkout at the workspace root stays clean and is only used for reference. All edits, commits, and test runs happen inside the worktree.

### Worktree branching and deploy safety

**All deploys must happen after a merge to `main`.** Never deploy from a branch. Deploys are triggered by pushing a `v*` tag, and CI enforces that the tagged commit is reachable from `main` (see `deploy` job in `.github/workflows/ci.yaml`). Branch tags are rejected.

**Standard deploy flow:**

1. Create a worktree branch, make changes, run tests.
2. Commit and push the branch.
3. Open a PR and merge it to `main` (or merge locally and push `main`).
4. After the merge lands on `origin/main`, fetch and tag on `main`:
   ```bash
   git fetch origin
   git checkout main && git pull origin main
   git tag v0.1.X
   git push origin v0.1.X
   ```
5. CI builds, tests, and deploys. The deploy job verifies the tag's commit is an ancestor of `origin/main` before proceeding.

**Rules:**

- **Never tag on a branch.** Always merge to `main` first, then tag.
- **Before starting new work**, always `git fetch origin && git pull origin main` so your worktree is based on the latest state.
- **Never re-implement a feature that already exists.** If you need a feature from another branch or tag, cherry-pick or merge it — do not write it again from scratch. Re-implementations drop edge-case fixes, tests, and accompanying infrastructure (e.g. migrations, Cancel buttons).
- **Check for orphaned branches** after a deploy: `git branch -a --contains <tag>` will show whether the tag's commit is reachable from `main`. If it is not, the changes live on an orphan branch and are at risk of being lost.

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
| Install git hooks | `make hooks` |
| Check client parity | `make check-parity` |

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
- **`middleware/`** — Applied in order: RequestLogger → SecurityHeaders → Session → CSRF → RateLimiter. There are two rate limiters: a strict per-IP limiter on `/api/auth` (`RATE_LIMIT_AUTH_MAX`, default 5/min) and a permissive global backstop on all `/api/*` (`RATE_LIMIT_GLOBAL_MAX`, default 120/min). The global backstop is only wired up when `TRUSTED_PROXY_CIDRS` is set, so that per-client IPs are reliably attributable (otherwise it would key every request to a proxy's single IP and could 429 the whole user base). Session middleware attaches user to context; handlers check with `middleware.CurrentUser(ctx)`. `SecurityHeaders` and `CSRF` take a `secure` bool (from `SERVER_SECURE`) that drives HSTS and the cookie `Secure` flag — not the spoofable `X-Forwarded-Proto`.
- **`database/`** — Connection setup + migration runner. Migrations are embedded SQL files from `migrations/` (via `//go:embed` in `migrations/assets.go`), applied at startup. Migration order follows `fs.ReadDir` order, not filename sort — check ordering carefully.
- **`audit/`** — Audit logging interface + std logger implementation.

Session cookie name: `nabu_session`. CSRF cookie name: `nabu_csrf`.

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

### When adding or changing predefined chores

**Adding a new predefined chore or changing an existing one's fields (icon, color, indicatorLabels, hasVolumeML, hasRating, etc.) requires two changes — never skip either one:**

1. **Update `PredefinedChores`** in `internal/chore/service.go` — this defines what new households get.
2. **Add a migration** that upserts the change for every existing household. The migration must use `WHERE NOT EXISTS` (for new chores) or `UPDATE` (for field changes) keyed on `predefined_key`, so it is idempotent on re-run. Without this step, existing users never see the new or updated defaults.

### Key patterns

- **Service/Store separation**: Services hold business logic, stores hold persistence. Both have memory and Postgres implementations. When `DATABASE_URL` is empty, everything uses in-memory stores.
- **Dependency injection via function args**: `BuildServer()` in `app/server.go` wires all dependencies; there are no global singletons.
- **Reminder scheduler is leader-gated**: the background reminder scheduler (`internal/reminder`) runs ticks only while it holds a Postgres session-level advisory lock (`reminder.PostgresAdvisoryLock`), so running multiple app instances does not emit duplicate reminders. In-memory/single-node mode passes no lock and always ticks. Don't add a second always-on scheduler without the same guard.
- **Optimistic UI**: Frontend updates state before server confirms; rolls back on error.
- **`apiFetch()`** adds `X-CSRF-Token` header read from `nabu_csrf` cookie for all state-changing requests.
- **`slotHour` in logs**: `POST /api/logs` accepts `hour` (integer) in the JSON body → stored as `slot_hour` in the DB → drives calendar placement. A missing or null `hour` puts the log in the Anytime row. Always pass `hour` from timed UI paths.

### Concurrency, config, shutdown, and logging (do not regress)

These four rules cover classes of bug found in past audits. Apply them to every new store, config key, background goroutine, and log line.

- **In-memory stores must be goroutine-safe.** The reminder scheduler runs as a background goroutine (`go reminderSched.Start(ctx)` in `NewServerWithDB`) that reads notification/push/schedule state *concurrently with HTTP handlers* — this is true even in `DATABASE_URL`-empty mode, where all stores are in-memory. Every `MemoryStore` therefore must guard its maps/slices with a `sync.Mutex`/`RWMutex` and lock in every method (a concurrent map write is a fatal Go panic, not just a race). When adding a memory store, copy the locking discipline from `internal/chore/store_memory.go`; do not ship an unsynchronized one. Run `go test -race ./...` for anything touching stores or the scheduler.
- **Config fails fast in production; no silent insecure fallback.** `config.Load` rejects a missing `DATABASE_URL` when `APP_ENV=production` rather than silently booting on the ephemeral in-memory store. When you add a REQUIRED-in-prod env var (a real database, OAuth, proxy CIDRs), validate it in `config.Load` and return an error — never degrade silently.
- **Background goroutines must be cancelable and torn down on shutdown.** The scheduler takes the context from `NewServerWithDB`, and `Server.Close()` (invoked by `BuildServer`'s returned `io.Closer`) cancels it and stops the rate-limiter cleanup goroutines. `cmd/server` shuts the HTTP server down gracefully on SIGINT/SIGTERM. Don't start a goroutine with `context.Background()` and no stop path; wire new long-lived goroutines into `Server.Close()`.
- **Never log secrets, tokens, capability URLs, or raw PII.** Request logs emit a *hashed* client IP (`hashClientIP`), not the raw address; push subscribe logs only the endpoint *host* (`endpointHost`), never the full endpoint (its path is a bearer-style capability). Auth flows log `user_id` and hashed-email fingerprints only. Follow the same discipline for any new log line.

### JS static file serving and cache busting

**Do not change this mechanism without understanding it fully** — the full rationale (Cloudflare's cache override, why `no-store`/`cf-cache-status: BYPASS`) is in the README's "JS static file serving and cache busting" section.

`buildVersionedJSCache` in `internal/app/server.go` rewrites every relative ES-module import to `?v=<version>` at startup and serves all `.js` from memory with `Cache-Control: no-store`, busting both the CDN and browser module caches on each deploy. Rules:

- **Never add `?v=anything` manually to a relative import in JS source.** The rewriter skips paths that already contain `?`, so a hard-coded version will not update on deploy and serves stale code.
- New JS modules that import other modules are handled automatically — no extra work.
- Verify after a deploy (see `docs/deploy-runbook.md`).

## Client parity

This repository has two first-class clients:

- **PWA** — `web/static/js/` (plain ES modules)
- **iOS** — `ios/` (native SwiftUI)

Every feature, bug fix, validation change, security fix, API change, or UI behavior change must be evaluated for both clients. See `docs/plans/ios.md` for the full conversion plan and `docs/plans/client-parity.md` for the feature matrix.

**When changing the PWA**, check whether the iOS app needs:
- A matching SwiftUI UI change.
- A matching API model change.
- A matching XCTest or XCUITest update.
- A matching snapshot update.

**When changing the iOS app**, check whether the PWA needs:
- A matching JavaScript UI/state change.
- A matching Playwright E2E test.
- A matching JS render/unit test.
- A matching backend handler/service/store change.

**Keep `docs/plans/client-parity.md` updated.** When a PR changes shared client surface — `web/static/js/**`, `ios/**`, or the shared API in `internal/handlers/**` — it must also update the parity matrix to reflect the change. The CI `parity` job (`.github/workflows/ci.yaml`) enforces this: it (a) lints the matrix (every row must use a known status: `Done`, `Built`, `iOS pending`, `PWA pending`, `Deferred`, `Not built`, `N/A`) and (b) fails if client/API code changed but the matrix wasn't touched. Escape hatch: include `no-parity-update: <reason>` in the PR body for changes that genuinely need no matrix update.

> Note: this replaced an older check that only grepped the PR description for phrases like "PWA-only change". The phrase is no longer what CI checks — update the matrix instead.

`scripts/check-parity.sh` is now a **matrix linter** by default (exit 0 if well-formed, exit 2 on an unknown status); pass `--strict` to also fail when any row is still `iOS pending`/`PWA pending` (useful as a release gate, not a per-PR gate). `make check-parity` runs it.

**iOS is built and tested in CI.** The `ios` job (`.github/workflows/ci.yaml`, `macos` runner) runs `xcodebuild test` of the `NabuTests` unit/contract suite whenever `ios/**` changes, so a backend model/API change that breaks the iOS request models fails at PR time. Keep `NabuTests` green. Use the `client-parity` skill (`/client-parity`) for parity-aware guidance during development.

See `ios/AGENTS.md` for iOS-specific agent instructions.

## Local dev stack

`make local` starts via Podman Compose: app on `:8080`, Mailpit on `:8025`, Postgres on `:5432`.

When `DATABASE_URL` is empty, the server falls back to in-memory stores (useful for `make run` without Podman).

### Coordinating multiple local stacks

The compose file binds to fixed host ports (8080, 5432, 8025, 1025), so **only one stack runs at a time.** Before starting a new one, `make down` in the worktree that owns the running containers (find it with `podman ps --format '{{.Names}}' | grep nabu`). E2E tests respect `BASE_URL` (default `http://localhost:8080`).

To run **two stacks simultaneously** (project-name + port overrides), see **`docs/dev-multistack.md`**.

## Test credentials

Local seed: `test@nabu.local` / `correct horse battery`. Stack must be running (`make local`) before `make seed`.

Production test account: `verify@yearofbingo.com` / `test123456` (household and seeded chores already set up).

## Production

- **URL**: `https://nabu-app.com`
- **Deploy trigger**: push a `v*` tag (e.g. `git tag v0.1.7 && git push origin v0.1.7`). CI builds, tests, and deploys automatically.
- **CI**: `.github/workflows/ci.yaml` — runs secret scan, JS tests, lint, Go tests (with coverage), E2E tests, the client-parity gate (PR only), and the iOS `NabuTests` lane (macOS, when `ios/**` changes) before deploying.

**After pushing a `v*` tag, watch the pipeline to completion and verify production** (don't wait to be asked; a cheaper subagent may be delegated to it). The full runbook — `gh run watch`, transient-vs-real failure triage, and the production verification commands (`/health`, versioned imports, `no-store`/`BYPASS` cache headers) — lives in **`docs/deploy-runbook.md`**.

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

## Pre-push checklist — never skip these

**Before pushing a branch for review or tagging for deploy, run all of these locally in the worktree.** CI runs the same checks; failures here mean failures there.

| Step | Command | Catches |
|------|---------|---------|
| Build | `go build ./...` | Compile errors from signature changes |
| Vet | `go vet ./...` | Suspicious code |
| Go tests | `make test-go` | Broken unit tests, missing DB column references |
| JS tests | `make test-js` | Broken frontend tests |

**Additionally, when your change affects counts, constants, schemas, or function signatures, grep the codebase for all references that may need updating.** Examples:

- Adding a new default chore → grep `tests/e2e/` for the old count (e.g. `13`, `toHaveCount(13)`)
- Adding a DB column → grep `test.go` files for SQL regex patterns and Scan column lists that need the new column
- Changing a function signature → grep for all callers of that function
- Adding a new JSON field → ensure both `json` tags and JS code handle it

**The pre-push hook** (`scripts/pre-push-hook.sh`, installed via `make hooks`) enforces `go build`, `go vet`, and `make test-go` automatically. It does not run JS or E2E tests (those need Node/Playwright), so you must run `make test-js` manually.

**Run one Go test invocation at a time.** `go test ./...` runs package test binaries in parallel, and the `auth` package spends ~25s pegging a core on bcrypt (cost 13). Launching overlapping `go test`/`go build`/`go vet` processes (or two `./...` runs at once) starves the cheap packages and can trip their timeout — this looks like a hang or a spurious `stats`/`userprefs` failure but is pure contention. If you see a package time out, re-run it in isolation (`go test ./internal/<pkg>/`) before believing it's a real failure. `make test-go` and CI pin an explicit `-timeout` so a genuine hang fails fast and legibly instead of silently consuming the default 10 minutes.

## Key invariants — do not break

These caused hard-to-diagnose production bugs and are covered by E2E tests:

1. **Home-tab direct tap** (`home-tap-chore` event in `app.js`): must call `logChore(..., new Date().getHours(), new Date().toISOString())` — the `slotHour` from `getHours()` drives calendar placement, and `completedAt` as the current time prevents the home-tab "time ago" from being wrong by the UTC offset. 
2. **Home-tab sheet log** (`save-log` event in `app.js`): for new logs (empty `logId`), the handler must extract `completedAt`, `slotHour`, and `date` from the `#log-when` input value — not from data attributes on the button. `slotHour` is derived via `new Date(whenInput.value).getHours()`. The when input is pre-filled with the current local time (minutes included), not rounded to `:00`. For editing existing logs, the comparison-based guard is retained to protect against morph.js corruption during re-renders.
3. **`renderWeekView` in `calendar.js`**: ad-hoc logs (those not matching a scheduled slot) must be placed in their `slotHour` row, not forced into the Anytime row — mirrors the `adHocCells` pattern in `renderDayView`.
4. **No hard-coded `?v=N` in JS import paths** — the server rewrites them all at startup.

## Security

**Every code change must consider these rules.**  Security regressions are as important as functional regressions — write tests and run `go vet` / `make lint` before committing.

### Principle of least authority

Every operation that reads or mutates data must verify the actor is authorized for that data.

- **Service-layer ownership checks (defense in depth).**  Handlers extract the user from the request context; services re-verify that the requested resource belongs to the user's household.  Never assume the handler's guard is sufficient — add the check in the service too.
- **Cross-resource isolation.**  When two users interact (e.g. one member changes another's role), verify they belong to the same household.  Compare both household IDs; a mismatch is an immediate reject.
- **Resource deletion and mutation must pass the owning household ID.**  Methods like `DeleteChore`, `UpdateChore`, `UpdateLog` accept a `householdID` parameter and the service verifies `resource.HouseholdID == householdID`.
- **Do not trust foreign keys from the client.**  If a request body contains `choreId`, `inviteId`, `userId`, `scheduleId`, or any other resource reference, verify that the referenced record belongs to the actor's household before using it.  This applies even when the write itself stores the current `householdID` separately.
- **Read endpoints need ownership checks too.**  Stats, detail pages, exports, and lookup endpoints can leak cross-household metadata even when they do not mutate anything.  Treat reads by ID the same way as writes by ID.

### Authentication and session safety

- **Constant-time comparison for all security tokens.**  CSRF cookie vs header, OIDC state parameter — use `crypto/subtle.ConstantTimeCompare`.  Never use `==` or `!=`.
- **Session cookies must set `HttpOnly`, `Secure` (when behind TLS), and `SameSite=Lax`.**  This applies to the set-cookie on login/register *and* the clear-cookie on logout.
- **Sessions expire.**  Hard expiry is set at creation; an idle timeout automatically deletes sessions that haven't been touched within a sliding window.
- **Password minimum length is 8 characters; maximum is 72 (bcrypt limit).**  Reject anything outside this range before hashing.
- **bcrypt cost factor: 13** for new hashes.  Pre-compute hashes in tests with `bcrypt.MinCost` to keep tests fast.

### OIDC / JWT verification

- **Verify JWT signatures.**  Fetch the provider's JWKS, validate `alg: RS256`, rebuild the RSA public key from `n`/`e`, verify the signature, then check `iss`, `aud`, `exp`, and `nonce`.
- **The `nonce` claim is mandatory.**  An absent or empty nonce is a rejection.
- **VAPID JWTs (ES256) use raw r∥s format (64 bytes).**  DER encoding is rejected by push services.  Pad each component to exactly 32 bytes.

### Input validation and output escaping

- **Server-side validation is mandatory.**  Client-side validation is a UX convenience only.  Check field lengths, required-ness, and format (e.g. hex colour regex) on every create and update handler.
- **Escape all user-controlled strings in HTML templates.**  Emoji, names, notes, display names — anything that came from user input or the database.  Use `escapeHTML()` from `utils.js` (not a local copy).  Never duplicate the function.
- **Colour fields must match `^#[0-9A-Fa-f]{6}$`.**  Reject any other value at the handler level.
- **Treat all chore metadata as untrusted.**  `icon`, `name`, `category`, `indicatorLabels`, and `indicatorDefaults` all come from users or the database.  Escape every one of them at render time, including inside history rows, SVG text, `aria-label`, `title`, and `style` attributes.
- **Do not treat string splitting or emoji-only conventions as sanitization.**  A value like `label.split(' ')[0]` is still untrusted text and must be escaped before interpolation.
- **Validate arrays, not just scalars.**  For list fields like `indicatorLabels` and `indicatorDefaults`, validate maximum item count, per-item length, and cross-field relationships such as "defaults must be a subset of labels".
- **Prefer explicit validation rules for free-text metadata.**  Fields like `category`, `initials`, and indicator labels must have server-side maximum lengths and reject control characters even if the UI currently constrains them.

### Security regression tests

- **Every security bug fix must add a regression test at the vulnerable layer.**  If the bug is in a handler or service, add a Go test.  If the bug is in rendered HTML, add a JS render test or E2E test.  Many fixes need both.
- **For escaping bugs, test the exact sink.**  Add tests for the concrete render function that previously interpolated unescaped data, not just a generic helper test.
- **For authorization bugs, add a cross-household negative test.**  The test should prove that a user from household A cannot read, mutate, or delete a record from household B.

### HTTP security

- **Strict-Transport-Security header** must be set when the deployment is configured secure (`SERVER_SECURE=true`) or the request arrived over direct TLS. Drive HSTS and the CSRF/session cookie `Secure` flag from `SERVER_SECURE`, **not** from the client-supplied `X-Forwarded-Proto` header (any client can spoof it).
- **Rate-limit authentication endpoints.**  `/api/auth` has a strict limiter (`RATE_LIMIT_AUTH_MAX`, default 5/min/IP). A permissive global backstop on all `/api/*` (`RATE_LIMIT_GLOBAL_MAX`, default 120/min/IP) is enabled only when `TRUSTED_PROXY_CIDRS` is set, so it can't 429 the whole user base behind an unconfigured proxy. Behind a proxy/tunnel, set `TRUSTED_PROXY_CIDRS` so client IPs (and audit-log IPs) are real.
- **`429 Too Many Requests` responses must carry a `Retry-After` header.**

### Error handling prevents enumeration

- **Never reveal whether an email address is registered.**  Registration, password-reset, and magic-link endpoints return the same HTTP status and the same phrasing regardless.
- **Avoid leaking internal details.**  `writeError` messages should be user-facing and not expose stack traces, SQL errors, or library internals.

## Push notification troubleshooting

Web Push (RFC 8291 aes128gcm + VAPID ES256) lives in `internal/push/` (`encrypt.go`, `vapid.go`, `service.go`). The key gotcha: **Apple returns 201 even when the encryption keys are wrong**, so there is no error signal at the gateway — the only way to know a push arrived is `self.lastPush` / `self.__diag` in the service worker.

The full diagnostic playbook — the exact HKDF chain, the VAPID JWT header rules (`typ`/`alg` only, no `kty`/`crv`), `remotedebug-ios-webkit-adapter` setup, and the `push-diag` `MessageChannel` flow — is in **`docs/plans/PUSH_DEBUG.md`**.
