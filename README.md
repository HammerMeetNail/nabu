# Choresy

A household chore coordination web app. Designed as a PWA for iPhone homescreen, beautiful enough for a non-technical grandmother.

## Quick Start

```bash
# Development (requires Podman)
make local       # Start stack (app:8080, Mailpit:8025, Postgres:5432)
make local-fresh # Fresh rebuild with volume wipe (required after any JS/template change)
make run         # Run without database (in-memory stores)
make seed        # Seed test user (test@choresy.local / "correct horse battery")

# Testing
make test        # Go + JS tests
make test-go     # Go tests only
make test-js     # JS tests only
make coverage    # Go test coverage report
make e2e         # End-to-end tests (156 tests, ~2.5 min)
make lint        # golangci-lint
make fmt         # Format Go code
```

## Architecture

**Go standard-library HTTP server** — no web framework. Dependencies: `pgx/v5` (Postgres) and `bcrypt` (auth). Frontend is vanilla JS ES modules — no bundler, no framework.

### Backend (`internal/`)

| Package | Purpose |
|---------|---------|
| `app` | Route registration, middleware chain, dependency wiring, JS cache-busting |
| `auth` | Registration, login, sessions, magic link, password reset, Google OIDC |
| `household` | Household creation, invite system, role management |
| `chore` | Chore CRUD, reorder, 12 predefined system chores |
| `log` | Chore logging (idempotent per chore/day), today/week/month queries |
| `schedule` | Recurring schedules with time-slot matching engine |
| `notification` | Push/email notification store, reminder preferences |
| `stats` | Leaderboard, streaks, heatmap, category breakdown, weekly recap |
| `middleware` | RequestLogger → SecurityHeaders → Session → CSRF → RateLimiter |
| `database` | pgx connection pool + embedded SQL migration runner |
| `config` | Environment variable loader with defaults |

### Frontend (`web/static/js/`)

| Module | Purpose |
|--------|---------|
| `app.js` | SPA entry point; event delegation for all user interactions |
| `state.js` | Single mutable app state object |
| `morph.js` | DOM morphing — updates DOM to match new HTML without losing focus/form state |
| `api.js` | `apiFetch()` wrapper; injects CSRF token on state-changing requests |
| `utils.js` | Shared helpers (escapeHTML, date formatting, etc.) |
| `auth.js` | Login, register, magic link, password reset views |
| `household.js` | Household create/join, member management |
| `today.js` | Chore grid, tap-to-log/undo, `logChore()` API wrapper |
| `calendar.js` | Day and week calendar views; ad-hoc log placement by `slotHour` |
| `home.js` | Home-tab grid and quick-log sheet |
| `schedule.js` | Schedule CRUD, pick-chore sheet, drag-and-drop rescheduling |
| `preferences.js` | Chore ordering preferences |
| `stats.js` | Leaderboard, streaks, category bars, weekly recap |

### `slotHour` — calendar placement

`POST /api/logs` accepts an optional `hour` integer (0–23) in the request body, stored as `slot_hour`. This drives where a log appears in the calendar:

- `slot_hour IS NULL` → **Anytime** row
- `slot_hour = N` → **N:00** hour row

**All logs created from the home tab must pass a non-null `slotHour`.** Logs must never land in Anytime unless they were explicitly unscheduled. This is enforced by E2E tests in `tests/e2e/home-log-to-calendar.spec.js`.

### JS static file serving and cache busting

At startup, `buildVersionedJSCache` in `internal/app/server.go` walks every `.js` file in the embedded FS and rewrites all relative ES module import paths to include `?v=<version>` (e.g. `from './calendar.js'` → `from './calendar.js?v=0.1.6'`). All JS files are then served from memory with `Cache-Control: no-store`.

**Why:** Cloudflare overrides `Cache-Control: no-cache` with `max-age=14400` (4 hours). `no-store` bypasses this entirely (`cf-cache-status: BYPASS`). The versioned import paths bust browser module caches on every deploy.

**Rule: never add `?v=anything` manually to a JS import path.** The rewriter skips paths that already contain `?`, so a hard-coded version won't be updated on deploy.

## API Endpoints

### Auth
`POST /api/auth/register` `POST /api/auth/login` `POST /api/auth/logout` `GET /api/me` `POST /api/auth/email/verification/resend` `GET /api/auth/email/verify` `POST /api/auth/magic-link/request` `GET /api/auth/magic-link/consume` `POST /api/auth/password/forgot` `POST /api/auth/password/reset` `GET /api/auth/google/login` `GET /api/auth/google/callback`

### Household
`GET /api/household` `POST /api/household` `PATCH /api/household` `POST /api/household/invites` `GET /api/household/invites` `DELETE /api/household/invites/{id}` `POST /api/household/join` `PATCH /api/household/members/{id}` `DELETE /api/household/members/{id}` `POST /api/household/leave` `POST /api/household/transfer`

### Chores
`GET /api/chores` `POST /api/chores` `GET /api/chores/{id}` `PATCH /api/chores/{id}` `DELETE /api/chores/{id}` `POST /api/chores/reorder` `GET /api/chores/defaults` `POST /api/chores/seed-defaults`

### Logging
`POST /api/logs` `DELETE /api/logs/{id}` `GET /api/logs/today` `GET /api/logs/week` `GET /api/logs/month`

### Stats
`GET /api/stats/leaderboard` `GET /api/stats/streaks` `GET /api/stats/heatmap` `GET /api/stats/breakdown` `GET /api/stats/recap`

## Deployment

- **Production URL**: `https://choresy.yearofbingo.com`
- **Deploy**: push a `v*` tag — CI builds, runs all tests, and deploys automatically
- **Test account**: `verify@yearofbingo.com` / `test123456`

### Verifying a production deploy

```bash
# Imports must carry the new version tag
curl -s https://choresy.yearofbingo.com/static/js/calendar.js | grep "^import"
# Expected: import { ... } from "./utils.js?v=0.1.X";

# Cache headers must be no-store / BYPASS (not max-age / HIT)
curl -sI https://choresy.yearofbingo.com/static/js/app.js | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: BYPASS
```

See `compose.server.yaml` for full production setup (Cloudflare Tunnel, `/mnt/data` volumes, R2 backups). Server provisioning via `cloud-init.yaml`.

## License

MIT
