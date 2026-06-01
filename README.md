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
make e2e         # End-to-end tests (~31 spec files)
make lint        # golangci-lint
make fmt         # Format Go code
```

## Prerequisites

- **Go 1.25+** (CI uses 1.25, `go.mod` specifies 1.25.10)
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
| `middleware` | RequestLogger → SecurityHeaders → Session → CSRF → RateLimiter |
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
| `SMTP_PORT` | `587` | SMTP port |
| `SMTP_USER` | (empty) | SMTP username |
| `SMTP_PASS` | (empty) | SMTP password |
| `SMTP_FROM` | (empty) | From address for emails |
| `GOOGLE_CLIENT_ID` | (empty) | Google OAuth2 client ID |
| `GOOGLE_CLIENT_SECRET` | (empty) | Google OAuth2 client secret |
| `TRUSTED_PROXY_CIDRS` | (empty) | CIDR list for trusted reverse proxy IPs (rate limiter) |
| `RATE_LIMIT_AUTH_MAX` | `5` | Max auth requests per minute per IP |
| `VAPID_PUBLIC_KEY` | (empty) | VAPID public key (base64-encoded uncompressed EC point) |
| `VAPID_PRIVATE_KEY` | (empty) | VAPID private key (base64-encoded) |
| `VAPID_SUBJECT` | (empty) | VAPID subject (e.g. `mailto:admin@example.com`) |

## API Endpoints

### Auth
`POST /api/auth/register` `POST /api/auth/login` `POST /api/auth/logout` `GET /api/me` `POST /api/auth/email/verification/resend` `GET /api/auth/email/verify` `POST /api/auth/magic-link/request` `GET /api/auth/magic-link/consume` `POST /api/auth/password/forgot` `POST /api/auth/password/reset` `POST /api/auth/password` `GET /api/auth/google/login` `GET /api/auth/google/callback`

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
5. **E2E tests** (Playwright, Chromium, 31 spec files)
6. **Build** — multi-arch container image (linux/amd64, linux/arm64) on Quay.io
7. **Scan & Sign** — Trivy vulnerability scan + Cosign keyless signing
8. **GitHub Release** — auto-generated from conventional commits
9. **Deploy** — verifies tag is on `main`, then deploys via SSH + Cloudflare Tunnel

### Verifying a production deploy

```bash
# Imports must carry the new version tag
curl -s https://nabu-app.com/static/js/calendar.js | grep "^import"
# Expected: import { ... } from "./utils.js?v=0.1.X";

# Cache headers must be no-store / BYPASS (not max-age / HIT)
curl -sI https://nabu-app.com/static/js/app.js | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: BYPASS

# Version endpoint
curl -s https://nabu-app.com/ | grep 'app.js'
# Expected: src="/static/js/app.js?v=0.1.X"
```

See `compose.server.yaml` for full production setup (Cloudflare Tunnel, `/mnt/data` volumes, R2 backups). Server provisioning via `cloud-init.yaml`.

## License

MIT
