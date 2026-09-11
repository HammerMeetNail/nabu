# Nabu

A household chore coordination web app. Designed as a PWA for iPhone homescreen, beautiful enough for a non-technical grandmother.

## Quick Start

```bash
# Development (requires Podman)
make local       # Start stack (app:8080, Mailpit:8025, Postgres:5432)
make local-fresh # Fresh rebuild with volume wipe (required after any JS/template change)
make run         # Run without database (in-memory stores)
make seed        # Seed test user (test@nabu.local / "correct horse battery")

# Testing
make test        # Go + JS tests
make test-go     # Go tests only
make test-js     # JS tests only
make coverage    # Go test coverage report
make e2e         # End-to-end tests (~63 spec files)
make lint        # golangci-lint
make fmt         # Format Go code
```

## Prerequisites

- **Go 1.26.8** (local modules, CI and the release image use this compiler)
- **JS tests require `pnpm install` first** for `jsdom` (dev dependency). Tests use Node's built-in test runner.
- **E2E tests require `pnpm exec playwright install chromium`** to download the browser binary.
- **Podman Compose** for local stack (`make local`). Docker Compose may work but is untested.

## Architecture

**Go standard-library HTTP server** — no web framework. Dependencies: `pgx/v5` (Postgres) and `golang.org/x/crypto` (bcrypt). Frontend is **plain ES modules** — no bundler, no framework, no build step.

### Backend (`internal/`)

| Package | Purpose |
|---------|---------|
| `app` | Route registration, middleware chain, dependency wiring, JS/SW cache-busting |
| `auth` | Registration, login, sessions, magic link, password reset, Google OIDC |
| `household` | Household creation, invite system, role management, multi-household switching |
| `chore` | Chore CRUD, reorder, restore defaults, 12 predefined system chores |
| `log` | Chore logging (idempotent per chore/day), today/week/month/history queries |
| `schedule` | Recurring schedules with time-slot matching engine, drag-and-drop rescheduling |
| `notification` | In-app notification store, per-type notification preferences, push dispatch |
| `push` | Web Push (RFC 8291) — VAPID JWT signing, aes128gcm encryption, subscription store |
| `stats` | Leaderboard, streaks, heatmap, category breakdown, busy hours, weekly recap |
| `userprefs` | Chore ordering, hidden home chores, timezone preferences |
| `handlers` | HTTP handlers grouped by feature — no business logic, call services directly |
| `middleware` | RequestLogger → SecurityHeaders → Session → CSRF → RateLimiter (strict `/api/auth` + optional global `/api/*` backstop) |
| `database` | pgx connection pool + embedded SQL migration runner (26 migrations) |
| `config` | Environment variable loader with defaults |
| `audit` | Audit logging interface + std logger implementation |
| `mail` | SMTP mailer for verification, magic link, password reset emails |
| `version` | Build-time version string (injected via `-ldflags`) |

### Frontend (`web/static/js/`)

| Module | Purpose |
|--------|---------|
| `app.js` | SPA entry point; event delegation for all user interactions |
| `state.js` | Single mutable app state object |
| `morph.js` | DOM morphing — updates DOM to match new HTML without losing focus/form state |
| `api.js` | `apiFetch()` wrapper; injects CSRF token on state-changing requests |
| `utils.js` | Shared helpers (`escapeHTML`, date formatting, etc.) |
| `head-init.js` | Inline `<head>` script — theme, timezone, and initial state bootstrap |
| `auth.js` | Login, register, magic link, password reset, OIDC views |
| `household.js` | Household create/join, member management, multi-household switching |
| `today.js` | Chore grid, tap-to-log/undo, `logChore()` API wrapper |
| `calendar.js` | Day and week calendar views; ad-hoc log placement by `slotHour` |
| `home.js` | Home-tab grid and quick-log sheet |
| `schedule.js` | Schedule CRUD, time-slot matching engine |
| `schedule-tab.js` | Schedule tab UI, pick-chore sheet, drag-and-drop rescheduling |
| `preferences.js` | Chore ordering preferences |
| `stats.js` | Leaderboard, streaks, category bars, weekly recap |
| `chores.js` | Chores management tab — CRUD, indicator defaults, volume settings |
| `profile.js` | Profile / settings tab |
| `notifications.js` | Notification bell, dropdown, and preferences |

### `slotHour` — calendar placement

`POST /api/logs` accepts an optional `hour` integer (0–23) in the request body, stored as `slot_hour`. This drives where a log appears in the calendar:

- `slot_hour IS NULL` → **Anytime** row
- `slot_hour = N` → **N:00** hour row

**All logs created from the home tab must pass a non-null `slotHour`.** Logs must never land in Anytime unless they were explicitly unscheduled. This is enforced by E2E tests in `tests/e2e/home-log-to-calendar.spec.js`.

## Push Notifications

Web Push (RFC 8291) for chore reminders on iOS and Android PWAs. Uses VAPID (voluntary application server identification) with ES256-signed JWTs and aes128gcm content encryption. Push subscriptions are stored per-device; users configure which chore events trigger pushes via notification preferences.

**Required env vars** for push: `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, `VAPID_SUBJECT` (e.g. `mailto:admin@example.com`).

The service worker (`web/static/service-worker.js`) handles incoming pushes and displays notifications. On new deploys the SW version is bumped automatically via injected build version, triggering the browser's update flow and showing an "App updated" toast.

See `PUSH_DEBUG.md` for the diagnostic playbook.

## PWA

Nabu is a Progressive Web App with a service worker caching strategy. Key PWA features:

- **Offline-ready**: SW caches static assets and serves stale-while-revalidate
- **iOS homescreen**: Manifest, apple-touch-icon, standalone display mode
- **Update detection**: Version-aware SW triggers update toast on new deploys
- **Push notifications**: Native browser/web push integration (see above)

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `8080` | HTTP listen port |
| `APP_ENV` | `development` | Environment name |
| `APP_BASE_URL` | `http://localhost:8080` | Public base URL (for magic links, OIDC redirects) |
| `SERVER_SECURE` | `false` | Set to `true` when behind TLS (enables Secure cookies, HSTS) |
| `DATABASE_URL` | (empty) | Postgres connection string; empty = in-memory stores |
| `SMTP_HOST` | (empty) | SMTP server hostname |
| `DB_MAX_OPEN_CONNS` | `25` | Total PostgreSQL connection budget (10..100): 8 reserved for delivery, remainder for HTTP; size from workload measurements |
| `DB_MAX_IDLE_CONNS` | `5` | Idle HTTP connections (capped to HTTP share); delivery keeps at most 2 idle |
| `SMTP_PORT` | `587` | SMTP port |
| `SMTP_USER` | (empty) | SMTP username |
| `SMTP_PASS` | (empty) | SMTP password |
| `SMTP_FROM` | (empty) | From address for emails |
| `GOOGLE_CLIENT_ID` | (empty) | Google OAuth2 client ID |
| `GOOGLE_CLIENT_SECRET` | (empty) | Google OAuth2 client secret |
| `APPLE_CLIENT_IDS` | (empty) | Comma-separated audiences accepted on Sign in with Apple identity tokens (the iOS bundle ID; empty disables the native endpoint) |
| `APPLE_WEB_CLIENT_ID` | (empty) | Services ID for web Sign in with Apple (must have `APP_BASE_URL`'s domain + `/api/auth/apple/web/callback` registered in the Apple portal; empty hides the web button). Automatically accepted as a token audience |
| `APNS_AUTH_KEY_P8` | (empty) | APNs .p8 auth key contents (PEM, PEM with `\n` escapes, or base64 of the PEM). All four `APNS_*` must be set to enable native push |
| `APNS_KEY_ID` | (empty) | APNs auth key ID |
| `APNS_TEAM_ID` | (empty) | Apple developer team ID (also enables the universal-links AASA with `APNS_BUNDLE_ID`) |
| `APNS_BUNDLE_ID` | (empty) | iOS app bundle ID (APNs topic) |
| `TRUSTED_PROXY_CIDRS` | (empty) | CIDR list for trusted reverse-proxy IPs. Enables real client-IP attribution (X-Forwarded-For) for rate limiting and audit logs, and gates the global rate-limit backstop. Required in production; malformed/universal ranges are rejected at startup. |
| `RATE_LIMIT_AUTH_MAX` | `30` | Max `/api/auth` requests per minute per IP |
| `RATE_LIMIT_GLOBAL_MAX` | `600` | Aggregate backstop across all `/api/*` paths per IP. Only active when `TRUSTED_PROXY_CIDRS` is set. |
| `RATE_LIMIT_JOIN_MAX` | `30` | Household-join requests per minute per IP |
| `RATE_LIMIT_MAX_CLIENTS` | `4096` | Maximum tracked IPs per limiter; new clients receive 429 until a window expires when full |
| `VAPID_PUBLIC_KEY` | (empty) | VAPID public key (base64-encoded uncompressed EC point) |
| `VAPID_PRIVATE_KEY` | (empty) | VAPID private key (base64-encoded) |
| `VAPID_SUBJECT` | (empty) | VAPID subject (e.g. `mailto:admin@example.com`) |

Rate budgets apply independently to each server replica; they are not a distributed global quota. Auth, join and aggregate API scopes compose, so changing resource IDs cannot reset an allowance. Production requires a database, HTTPS `APP_BASE_URL`, `SERVER_SECURE=true`, and explicit trusted proxy ranges. The local release-image compose file uses development mode for plain HTTP.

## API Endpoints

### Auth
`POST /api/auth/register` `POST /api/auth/login` `POST /api/auth/logout` `GET /api/me` `POST /api/auth/email/verification/resend` `GET /api/auth/email/verify` `POST /api/auth/magic-link/request` `GET /api/auth/magic-link/consume` `POST /api/auth/password/forgot` `POST /api/auth/password/reset` `POST /api/auth/password` `GET /api/auth/google/login` `GET /api/auth/google/callback` `POST /api/auth/apple/native` `GET /api/auth/apple/web/login` `POST /api/auth/apple/web/callback`

### Household
`GET /api/household` `POST /api/household` `PATCH /api/household` `POST /api/household/invites` `GET /api/household/invites` `DELETE /api/household/invites/{id}` `POST /api/household/join` `PATCH /api/household/members/{id}` `DELETE /api/household/members/{id}` `POST /api/household/leave` `POST /api/household/transfer`
`GET /api/households` `POST /api/households/{id}/activate`

### Chores
`GET /api/chores` `POST /api/chores` `GET /api/chores/{id}` `PATCH /api/chores/{id}` `DELETE /api/chores/{id}` `POST /api/chores/reorder` `GET /api/chores/defaults` `POST /api/chores/seed-defaults` `POST /api/chores/{id}/restore-default`

### Logging
`POST /api/logs` `PATCH /api/logs/{id}` `DELETE /api/logs/{id}` `GET /api/logs/today` `GET /api/logs/week` `GET /api/logs/month` `GET /api/logs/history` `GET /api/logs/latest-per-chore`

### Schedules
`GET /api/schedules` `POST /api/schedules` `PATCH /api/schedules/{id}` `DELETE /api/schedules/{id}` `GET /api/schedules/for-date`

### Notifications & Push
`GET /api/notifications` `POST /api/notifications/read-all` `POST /api/notifications/{id}/read` `DELETE /api/notifications/{id}`
`GET /api/notification-preferences` `PATCH /api/notification-preferences`
`POST /api/push/subscribe` `POST /api/push/unsubscribe`

### Preferences
`GET /api/preferences` `PATCH /api/preferences`

### Stats
`GET /api/stats/leaderboard` `GET /api/stats/streaks` `GET /api/stats/heatmap` `GET /api/stats/breakdown` `GET /api/stats/recap` `GET /api/stats/overview` `GET /api/stats/busy-hours` `GET /api/stats/chores` `GET /api/stats/chores/{id}` `GET /api/stats/chores/{id}/time-series`

### Health
`GET /health` `GET /ready`

## JS static file serving and cache busting

At startup, `buildVersionedJSCache` in `internal/app/server.go` walks every `.js` file in the embedded FS and rewrites all relative ES module import paths to include `?v=<version>` (e.g. `from './calendar.js'` → `from './calendar.js?v=0.1.187'`). All JS files are then served from memory with `Cache-Control: no-store`.

**Why:** Cloudflare overrides `Cache-Control: no-cache` with `max-age=14400` (4 hours). `no-store` bypasses this entirely (`cf-cache-status: BYPASS`). The versioned import paths bust browser module caches on every deploy.

**Rule: never add `?v=anything` manually to a JS import path.** The rewriter skips paths that already contain `?`, so a hard-coded version won't be updated on deploy.

The service worker file (`/service-worker.js`) is also version-injected at startup so the browser detects a new SW on every deploy.

**HTML pages ship `no-store` too.** The root `/` serves server-rendered marketing HTML (`home.html`) to anonymous visitors and the SPA app shell to signed-in users, so it can never be cached by Cloudflare — a cached copy would serve the wrong page version to one of the two audiences. The same `Cache-Control: no-store` is applied to every HTML page the server renders (`/`, `/login`, `/register`, `/today`, static pages like `/privacy`, …) so each visit reflects the latest session state.

## Deployment

- **Production URL**: `https://nabu-app.com`
- **Container image**: `quay.io/nabu/nabu` (multi-arch: amd64 + arm64)
- **Deploy**: push a `v*` tag — CI builds, runs all tests, scans for vulnerabilities, signs the image, and deploys automatically
- **Test account**: `verify@yearofbingo.com` / `test123456`

### CI Pipeline

Pushes to `main` and `v*` tags trigger:

1. **Secret scan** (Gitleaks)
2. **Lint** (golangci-lint)
3. **Go tests** with race detector and coverage (reported to Codecov)
4. **JS tests** (Node built-in test runner)
5. **E2E tests** (Playwright, Chromium, 63 spec files)
6. **iOS unit tests** (`xcodebuild test` of `NabuTests` on macOS, when `ios/**` changes)
7. **Client parity** (PR only) — lints the parity matrix and requires it to be updated when client/API code changes
8. **Build** — multi-arch container image (linux/amd64, linux/arm64) on Quay.io
9. **Scan & Sign** — Trivy vulnerability scan + Cosign keyless signing
10. **GitHub Release** — auto-generated from conventional commits
11. **Deploy** — verifies tag is on `main`, then deploys via SSH + Cloudflare Tunnel

### Verifying a production deploy

```bash
# Imports must carry the new version tag
curl -s https://nabu-app.com/static/js/calendar.js | grep "^import"
# Expected: import { ... } from "./utils.js?v=0.1.X";

# Cache headers must be no-store / BYPASS (not max-age / HIT)
curl -sI https://nabu-app.com/static/js/app.js | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: BYPASS

# Version endpoint — the app shell lives at SPA routes like /login; anonymous
# GET / serves the marketing page, not the shell
curl -s https://nabu-app.com/login | grep 'app.js'
# Expected: src="/static/js/app.js?v=0.1.X"

# Anonymous root serves server-rendered marketing HTML, canonicalized to /
curl -s https://nabu-app.com/ | grep -o 'rel="canonical" href="[^"]*"'
# Expected: rel="canonical" href="https://nabu-app.com/"

# HTML cache headers — marketing page and app shell must both be no-store/BYPASS
curl -sI https://nabu-app.com/ | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: BYPASS
```

The application deployment is defined in `compose.server.yaml`; the standalone PostgreSQL cluster must be inventoried separately. Server provisioning is in `cloud-init.yaml`. The [recovery runbook](docs/recovery-runbook.md) defines a five-minute data-loss target, one-hour recovery target, prepared encrypted WAL backups, freshness/restore alerts, and isolated restore/cutover procedures. Daily encrypted logical backups remain secondary protection; production archive health and timing require operational verification.

## License

MIT

Database diagnostics emit aggregate `database.pool` records once per minute.
They report connection use/waits and cumulative query counts, failures, total/max
latency and histogram bins (<=1/5/10/25/50/100/500/1000ms, then >1000ms). Query
latency includes receiving rows; pool acquisition waits are separate. No SQL,
arguments, connection strings or provider error bodies are retained. Compare
these measurements with HTTP latency and DB resources before increasing pool
limits or supported household counts. The monitor stops with the server.

### PostgreSQL query regression workload

`TEST_DATABASE_URL` must point to a disposable local/CI PostgreSQL service whose
role can create databases. Integration tests create a random database per test,
then close and drop only that database; extension migrations stay isolated too.
They never use `DATABASE_URL` as a fallback.

Run the opt-in history workload with `NABU_PERF=1 TEST_DATABASE_URL=… go test -v
-timeout 360s ./internal/log -run TestLocalHistoryWorkload`. Set
`NABU_PERF_REPORT` to an absolute path outside the checkout (the default is
`/tmp/nabu-history-workload.json`). It compares query plans, allocations and index
write cost across 10 synthetic households with 500,000 logs, then exercises both
10 and 24 concurrent workers against a 12-connection pool. This is a local
regression workload, not a supported household-count claim.

### Notification retention and paging

In-app notifications are recipient-owned receipts: read and unread notifications
remain until the recipient deletes them or deletes the account. Marking read does
not delete a receipt, and there is no automatic age purge. This policy deliberately
avoids silent age-based data loss. Historical receipt text is not retroactively
redacted when a household role or chore visibility changes; new delivery always
rechecks current access. Activity logs have their own lifetime and are unaffected
by deleting notifications.

Both clients list newest first in pages of at most 50, ordered by creation time
and ID. `GET /api/notifications?cursor=...` returns an optional `nextCursor`; clients
must treat it as an opaque position. Each query still restricts rows to the
signed-in recipient. New arrivals do not shift an older page, and deleting a
cursor boundary does not create gaps. Open notification panels keep their loaded
pages until an explicit refresh; background refresh resumes after closing.
