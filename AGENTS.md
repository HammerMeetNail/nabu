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

- **Go 1.25+** (CI uses 1.24 in build job, but `go.mod` declares 1.25).
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

### Key patterns

- **Service/Store separation**: Services hold business logic, stores hold persistence. Both have memory and Postgres implementations. When `DATABASE_URL` is empty, everything uses in-memory stores.
- **Dependency injection via function args**: `BuildServer()` in `app/server.go` wires all dependencies; there are no global singletons.
- **Optimistic UI**: Frontend updates state before server confirms; rolls back on error.
- **`apiFetch()`** adds `X-CSRF-Token` header read from `choresy_csrf` cookie for all state-changing requests.

## Local dev stack

`make local` starts via Podman Compose: app on `:8080`, Mailpit on `:8025`, Postgres on `:5432`.

When `DATABASE_URL` is empty, the server falls back to in-memory stores (useful for `make run` without Podman).

## Test credentials

Seed creates: `test@choresy.local` / `correct horse battery`. Stack must be running (`make local`) before `make seed`.

## Style

- `go fmt` for Go. No configured JS linter.
- Keep packages focused by domain; HTTP-only logic in `handlers/`.
- Frontend: clear DOM-oriented functions over framework abstractions. Render functions return HTML strings.
- 80% minimum Go statement coverage target.
