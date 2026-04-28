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

After changing files in `web/templates/` or `web/static/`, run `make local-fresh` — these assets are embedded into the Go binary.

## Architecture

**Go standard-library HTTP server** with no web framework. Single external dependency: `jackc/pgx` (Postgres driver) and `golang.org/x/crypto` (bcrypt). Frontend is **plain ES modules** — no bundler, no framework, no build step.

### Backend (`internal/`)

- **`app/server.go`** — All route registration in one place via `http.ServeMux`. Routes are explicit `mux.HandleFunc()` calls wrapped with `method()` for HTTP verb enforcement.
- **`handlers/`** — HTTP handlers grouped by feature (auth, chores, logs, schedule, notifications, stats). Handlers call services; they don't contain business logic.
- **`middleware/`** — Applied in order: RequestLogger → SecurityHeaders → Session → CSRF → RateLimiter. Session middleware attaches user to context; handlers check with `middleware.CurrentUser(ctx)`.
- **`database/`** — Connection setup and migration runner. Migrations are embedded SQL files from `migrations/` applied at startup.
- **`audit/`** — Audit logging interface + std logger implementation.

### Frontend (`web/static/js/`)

- **`app.js`** — Entry point. Wires all event listeners on a single `#app` container. Delegates to imported handler functions.
- **`state.js`** — `createAppState()` returns the single mutable state object. All UI derives from this state.
- **`morph.js`** — DOM morphing: updates existing DOM to match new HTML without destroying focus or form state.
- **`api.js`** — `apiFetch()` wraps `fetch()` adding `Content-Type` and CSRF token for state-changing requests.

### Key patterns

- **Service/Store separation**: Services hold business logic, stores hold persistence. Both have memory and Postgres implementations.
- **Dependency injection via function args**: Handlers receive `{ state, apiFetch, render }` — no globals, easy to test.
- **Optimistic UI**: Frontend updates state before server confirms; rolls back on error.
- **`apiFetch()`** wraps `fetch()` adding `Content-Type` and CSRF token (`X-CSRF-Token` header from `choresy_csrf` cookie) for state-changing requests.

## Local dev stack

`make local` starts via Podman Compose: app on `:8080`, Mailpit on `:8025`, Postgres on `:5432`, Redis on `:6379`.

When `DATABASE_URL` is empty, the server falls back to in-memory stores (useful for `make run` without Podman).

## Style

- `go fmt` for Go. No configured JS linter.
- Keep packages focused by domain; HTTP-only logic in `handlers/`.
- Frontend: clear DOM-oriented functions over framework abstractions. Render functions return HTML strings.
- 80% minimum Go statement coverage target.
