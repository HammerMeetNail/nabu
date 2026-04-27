# Choresy

A household chore coordination web app. Designed as a PWA for iPhone homescreen, beautiful enough for a non-technical grandmother.

## Quick Start

```bash
# Development (requires Podman)
make local       # Start stack (app:8080, Mailpit:8025, Postgres:5432, Redis:6379)
make local-fresh # Fresh rebuild with volume wipe
make run         # Run without database (in-memory stores)
make seed        # Seed test user (test@choresy.local / "correct horse battery")

# Testing
make test        # Go + JS tests
make test-go     # Go tests only
make test-js     # JS tests only
make coverage    # Go test coverage report
make e2e         # End-to-end tests
make lint        # golangci-lint
make fmt         # Format Go code
```

## Architecture

**Go standard-library HTTP server** — no web framework. Dependencies: `pgx/v5` (Postgres) and `bcrypt` (auth). Frontend is vanilla JS ES modules — no bundler, no framework.

### Backend (`internal/`)

| Package | Purpose |
|---------|---------|
| `app` | Route registration, middleware chain, dependency wiring |
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
| `app.js` | SPA routing, event delegation, all views |
| `auth.js` | Login, register, magic link, password reset views |
| `household.js` | Household create/join, member management |
| `today.js` | Chore grid tap-to-log/undo, date navigator, progress bar |
| `stats.js` | Leaderboard, streaks, category bars, weekly recap |

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

See `compose.server.yaml` for production setup. Uses Cloudflare Tunnel, persistent `/mnt/data` volumes, and encrypted R2 backups. Server provisioning via `cloud-init.yaml`.

```bash
# Production deployment
podman compose -f compose.server.yaml up -d

# Backup
./scripts/backup.sh

# Restore
./scripts/restore.sh --latest
```

## License

MIT
