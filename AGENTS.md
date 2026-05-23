# Repository Guidelines

This file provides guidance to an LLM when working with code in this repository.

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

## Style

- `go fmt` for Go. No configured JS linter.
- Keep packages focused by domain; HTTP-only logic in `handlers/`.
- Frontend: clear DOM-oriented functions over framework abstractions. Render functions return HTML strings.
- 80% minimum Go statement coverage target.

## Key invariants — do not break

These caused hard-to-diagnose production bugs and are covered by `tests/e2e/home-log-to-calendar.spec.js`:

1. **Home-tab direct tap** (`home-tap-chore` event in `app.js`): must call `logChore(..., new Date().getHours(), ...)`.
2. **Home-tab sheet log** (`save-home-log` event in `app.js`): must extract `new Date(whenInput.value).getHours()` and pass as `slotHour`.
3. **`renderWeekView` in `calendar.js`**: ad-hoc logs (those not matching a scheduled slot) must be placed in their `slotHour` row, not forced into the Anytime row — mirrors the `adHocCells` pattern in `renderDayView`.
4. **No hard-coded `?v=N` in JS import paths** — the server rewrites them all at startup.
