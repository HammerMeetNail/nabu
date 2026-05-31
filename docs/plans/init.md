# Choresy — Implementation Plan

A household chore coordination web app. Husband, wife, and other household members log and coordinate chores throughout the day, week, month, and year. Designed as a PWA for iPhone homescreen, beautiful enough for a non-technical grandmother.

**Core references:**
- Architecture, auth, UI, and testing patterns: `~/git/CalTrack`
- Deployment, CI/CD, and self-hosting patterns: `~/git/yearofbingo`

---

## Architecture Overview

```
[Cloudflare Tunnel (cloudflared)]   ← TLS termination, DDoS protection
         |
   [Host port 80]
         |
   [Go binary :8080]               ← Single binary, serves API + embedded SPA
    |          |
[PostgreSQL] [Redis]              ← Redis for sessions + real-time pub/sub
```

**Key architectural decisions (inherited from CalTrack/YearOfBingo):**

| Concern | Decision |
|---|---|
| Backend framework | Go stdlib `net/http` — zero dependencies beyond pgx and crypto |
| Frontend framework | Vanilla JavaScript ES modules — no React/Vue/Svelte |
| CSS | Single plain CSS file with custom properties (design tokens) |
| Database | PostgreSQL, raw SQL via `database/sql` + `pgx/v5` — no ORM |
| Auth | Session-based (HttpOnly cookies), email+password, magic link, Google OIDC |
| CSRF | Double-submit cookie pattern (`choresy_csrf` cookie + `X-CSRF-Token` header) |
| Asset pipeline | Content-hashed CSS/JS at build time, embedded via `//go:embed`, 1-year immutable cache |
| Migrations | Numbered SQL files applied at startup via `golang-migrate` |
| Real-time | Server-Sent Events (SSE) with Redis Pub/Sub for cross-instance broadcasting |
| Deployment | Single `podman-compose` service, Cloudflare Tunnel ingress |
| Testing | Go stdlib `testing`, Node built-in `node:test`, Playwright for E2E |
| Coverage target | ≥75% automated (Go + JS combined) |

---

## Domain Model

```
Household
├── id, name, invite_code, created_at
│
├── User (belongs to Household)
│   ├── id, household_id, email, password_hash, display_name, avatar_color
│   ├── email_verified, created_at
│   └── Roles: owner, admin, member
│
├── Chore (belongs to Household)
│   ├── id, household_id, name, icon (emoji), color, sort_order
│   ├── category (feeding, cleaning, care, plants, custom)
│   ├── is_predefined (true for system defaults, false for custom)
│   └── created_by, created_at
│
├── ChoreLog (belongs to Household, User, Chore)
│   ├── id, household_id, user_id, chore_id
│   ├── completed_at (timestamp)
│   ├── note (optional text)
│   └── created_at
│
├── ChoreSchedule (belongs to Household, Chore)
│   ├── id, household_id, chore_id
│   ├── cron_expression or time_slots (JSON array of HH:MM)
│   ├── days_of_week (bitmask or JSON array)
│   ├── is_active, assigned_to_user_id (optional)
│   └── created_at, updated_at
│
├── ReminderPreference (belongs to User)
│   ├── id, user_id
│   ├── push_enabled, email_enabled
│   ├── quiet_hours_start, quiet_hours_end
│   └── timezone
│
└── Notification (belongs to User)
    ├── id, user_id, type, title, body
    ├── is_read, created_at
    └── metadata (JSON)
```

---

## Pre-defined System Chores

Shipped as seed data in a migration (migration 003). These are automatically created when a household is created.

| Name | Icon | Category | Color |
|---|---|---|---|
| Feed Cats (Morning) | 🐱 | feeding | #F59E0B |
| Feed Cats (Evening) | 🐱 | feeding | #F59E0B |
| Feed Baby | 🍼 | feeding | #EC4899 |
| Change Baby | 👶 | care | #8B5CF6 |
| Water Plants | 🌱 | plants | #10B981 |
| Clean Litter Box | 🧹 | cleaning | #6366F1 |
| Take Out Trash | 🗑️ | cleaning | #6B7280 |
| Wash Dishes | 🍽️ | cleaning | #3B82F6 |
| Vacuum | 🧹 | cleaning | #06B6D4 |
| Laundry | 👕 | cleaning | #F97316 |
| Walk Dog | 🐕 | care | #EF4444 |
| Make Bed | 🛏️ | cleaning | #14B8A6 |

---

## Color Palette & Design Tokens

```
Brand:         #19323C (dark navy)
Primary:       #2E86AB (teal blue)
Secondary:     #A23B72 (berry)
Accent:        #F18F01 (warm amber)
Success:       #386641 (forest green)
Danger:        #BC4742 (brick red)
Warning:       #F2E863 (soft yellow)
Background:    #F4EFE7 (warm cream)
Surface:       #FFFFFF
Text Primary:  #1A1A2E
Text Secondary:#6B7280
Border:        #D1D5DB
Radius:        12px (cards), 8px (buttons), 9999px (pills/chips)
Shadow:        0 1px 3px rgba(0,0,0,0.08), 0 1px 2px rgba(0,0,0,0.06)
```

---

## Implementation Stages

Each stage is designed to be implemented autonomously. Stages are sequential — each depends on the previous. Within a stage, items labeled **[P]** can run in parallel.

**Testing mandate:** Every Go package must have `_test.go` files targeting ≥80% statement coverage. Every JS module must have corresponding tests in `web/static/js/tests/`. E2E tests cover critical user flows.

---

### Stage 0: Project Scaffolding & CI/CD (Day 1)

**Goal:** Empty project that builds, tests pass, CI runs green.

#### 0.1 Project Initialization **[P]**

- [ ] `go mod init github.com/dave/choresy`
- [ ] Create directory structure:
  ```
  cmd/server/
  internal/
    app/
    audit/
    auth/
    config/
    database/
    handlers/
    middleware/
    models/         (replaces scattered domain types — CalTrack improvement)
  migrations/
  web/
    templates/
    static/
      css/
      js/
        tests/
      icons/
  tests/
    e2e/
  scripts/
  docs/
    plans/
  ```
- [ ] Create `.gitignore` (copy from CalTrack)
- [ ] Create `AGENTS.md` with project conventions
- [ ] Create `.env.example` with all config vars documented
- [ ] Create `LICENSE`

#### 0.2 Container Build **[P]**

- [ ] Create `Containerfile` — multi-stage (golang:1.24-alpine → alpine:3.19), non-root `appuser`, HEALTHCHECK
- [ ] Create `.dockerignore`
- [ ] Create `compose.yaml` — services: app (build from Containerfile), postgres (17-alpine), redis (7-alpine), mailpit
- [ ] Create `compose.server.yaml` — production variant (pulls image, persistent volumes at `/mnt/data/`, port 80:8080, restart unless-stopped)
- [ ] Create `compose.prod.yaml` — local production simulation (pulls from registry)

#### 0.3 Makefile

- [ ] `make local` — down → build → up (podman compose)
- [ ] `make local-fresh` — down -v → local
- [ ] `make down` — podman compose down -v
- [ ] `make test` — go test + js test
- [ ] `make test-go` — `go test -race ./...`
- [ ] `make test-js` — `node --test web/static/js/tests/runner.js`
- [ ] `make coverage` — `go test -race -coverprofile=coverage.out ./...`
- [ ] `make lint` — golangci-lint (auto-installed to `.cache/`)
- [ ] `make fmt` — `go fmt ./...`
- [ ] `make e2e` — runs `./scripts/e2e.sh`
- [ ] `make release` — version bump + tag + push

#### 0.4 Config Package

- [ ] `internal/config/config.go` — `Config` struct with all env vars, `Load()` function
- [ ] `internal/config/config_test.go` — test defaults + env overrides
- [ ] Config vars:
  ```
  PORT, APP_ENV, APP_BASE_URL, SERVER_SECURE,
  DATABASE_URL, REDIS_URL,
  SESSION_SECRET, CSRF_SECRET,
  SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS, SMTP_FROM,
  GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET,
  TRUSTED_PROXY_CIDRS,
  ```

#### 0.5 Database Package

- [ ] `internal/database/open.go` — `sql.Open("pgx", url)` + ping + pool settings (MaxOpenConns=25, MaxIdleConns=5)
- [ ] `internal/database/migrate.go` — `golang-migrate` runner, auto-migrate on startup
- [ ] `internal/database/migrate_test.go`
- [ ] `migrations/assets.go` — `//go:embed *.sql` for embedding migration files

#### 0.6 Hello World Server

- [ ] `cmd/server/main.go` — `config.Load()` → `database.Open()` → `database.Migrate()` → `app.BuildServer()` → `http.ListenAndServe()` → graceful shutdown
- [ ] `internal/app/server.go` — route registration with middleware chain: `RequestLogger → SecurityHeaders → Session → CSRF → RateLimiter`
- [ ] `internal/app/server_test.go` — test middleware ordering, health endpoints
- [ ] `internal/handlers/health.go` — `/health` and `/ready` endpoints
- [ ] `internal/handlers/health_test.go`
- [ ] `internal/handlers/json.go` — JSON response helpers (`writeJSON`, `writeError`, `readJSON`)
- [ ] `internal/middleware/requestlog.go` — structured request logging
- [ ] `internal/middleware/security.go` — security headers (CSP, HSTS, X-Frame-Options, etc.)
- [ ] `internal/middleware/csrf.go` — double-submit cookie CSRF
- [ ] `internal/middleware/ratelimit.go` — IP+path rate limiter on auth endpoints
- [ ] `internal/middleware/middleware_test.go` — test all middleware
- [ ] `internal/audit/` — audit logging interface + std logger implementation

#### 0.7 Frontend Skeleton

- [ ] `web/assets.go` — `//go:embed templates static` for embedding all frontend assets
- [ ] `web/templates/index.html` — SPA shell: `<!DOCTYPE html>`, meta viewport, CSP meta, `<main id="app">`, bottom tabs nav, footer
- [ ] `web/static/manifest.webmanifest` — PWA manifest (name: "Choresy", short_name: "Choresy", theme_color: "#19323C", background_color: "#F4EFE7")
- [ ] `web/static/service-worker.js` — cache-first for static, network-first for nav, offline fallback
- [ ] `web/static/offline.html` — "You're offline" page
- [ ] `web/static/icons/icon.svg` — app icon (choresy brand mark)
- [ ] `web/static/css/app.css` — CSS reset + design tokens (custom properties) + base typography + layout skeleton (mobile-first, max-width 480px)
- [ ] `web/static/js/app.js` — main entry: event delegation on `#app`, SPA routing, `render()` orchestrator
- [ ] `web/static/js/state.js` — single mutable state object factory (`createAppState()`)
- [ ] `web/static/js/morph.js` — lightweight DOM morphing (copy from CalTrack pattern)
- [ ] `web/static/js/api.js` — `apiFetch()` wrapper with CSRF header injection
- [ ] `web/static/js/tests/runner.js` — empty test runner with at least 1 placeholder test

#### 0.8 CI Pipeline

- [ ] `.github/workflows/ci.yaml` — trigger on push to main, PRs, version tags
- [ ] Jobs: changes-detection → lint → go-test → js-test → e2e → build-image → (on tag) scan-push → deploy
- [ ] `scripts/e2e.sh` — full E2E orchestration (build assets, start stack, seed, run Playwright)
- [ ] `scripts/seed.sh` — create test household + users
- [ ] `scripts/build-assets.sh` — content-hash CSS/JS, generate `web/static/dist/manifest.json`

#### 0.9 Server Provisioning

- [ ] `cloud-init.yaml` — Hetzner VPS provisioning (podman-compose, cloudflared, firewalld, systemd units, backup timers)
- [ ] `scripts/backup.sh` — `pg_dump` → gzip → GPG encrypt → upload to R2
- [ ] `scripts/restore.sh` — download, decrypt, restore
- [ ] `scripts/verify-backup.sh` — backup integrity check

**Stage 0 Verification:**
```bash
make test        # All tests pass (minimal but real)
make lint        # No lint errors
make local       # App starts, /health returns 200
curl localhost:8080/health   # {"status":"ok"}
```

---

### Stage 1: Auth System (Day 2)

**Goal:** Users can register, login, logout, verify email, use magic links, reset passwords, and sign in with Google. Session persists across page loads.

#### 1.1 Auth Domain & Store Interface

- [ ] `internal/models/user.go` — `User` struct, `Session` struct, `AuthToken` struct
- [ ] `internal/auth/store.go` — `Store` interface:
  ```go
  type Store interface {
      CreateUser(ctx, email, passwordHash) (*User, error)
      GetUserByEmail(ctx, email) (*User, error)
      GetUserByID(ctx, id) (*User, error)
      VerifyEmail(ctx, userID) error
      CreateSession(ctx, userID, tokenHash, expiresAt) (*Session, error)
      GetSession(ctx, tokenHash) (*Session, error)
      DeleteSession(ctx, tokenHash) error
      DeleteUserSessions(ctx, userID) error
      CreateAuthToken(ctx, userID, tokenHash, kind, expiresAt) (*AuthToken, error)
      ConsumeAuthToken(ctx, tokenHash, kind) (*AuthToken, error)
  }
  ```
- [ ] `internal/auth/memory_store.go` — in-memory implementation for testing/dev
- [ ] `internal/auth/postgres_store.go` — Postgres implementation
- [ ] `internal/auth/postgres_store_test.go` — tests with `go-sqlmock`

#### 1.2 Auth Service

- [ ] `internal/auth/service.go` — `Service` struct with methods:
  - `Register(ctx, email, password)` — validate email format, enforce 8+ char password, bcrypt hash, create user, send verification email
  - `Login(ctx, email, password)` — verify credentials, create session, return session token
  - `Logout(ctx, sessionToken)` — delete session
  - `Authenticate(ctx, sessionToken)` — validate session, return user
  - `VerifyEmail(ctx, token)` — consume verification token
  - `ResendVerification(ctx, userID)` — generate new token, send email
  - `RequestMagicLink(ctx, email)` — generate token, send magic link email
  - `ConsumeMagicLink(ctx, token)` — consume token, create session
  - `RequestPasswordReset(ctx, email)` — generate token, send reset email
  - `ResetPassword(ctx, token, newPassword)` — consume token, update password, delete all sessions
- [ ] `internal/auth/service_test.go` — comprehensive tests with MemoryStore and sqlmock
- [ ] `internal/auth/password.go` — bcrypt hash/verify helpers (cost=12, same as CalTrack)

#### 1.3 Google OIDC

- [ ] `internal/auth/oidc.go` — `OIDCProvider` interface + `GoogleOIDCProvider`
- [ ] `internal/auth/oidc_test.go` — test auth URL generation, nonce validation
- [ ] `GET /api/auth/google/login` — redirect to Google consent screen
- [ ] `GET /api/auth/google/callback` — exchange code, verify nonce, find-or-create user, create session

#### 1.4 Email Service

- [ ] `internal/mail/sender.go` — `Sender` interface: `Send(ctx, to, subject, htmlBody) error`
- [ ] `internal/mail/smtp_sender.go` — SMTP implementation (used in production)
- [ ] `internal/mail/memory_sender.go` — in-memory for testing (stores sent emails)
- [ ] `internal/mail/log_sender.go` — logs to stdout for dev
- [ ] `internal/mail/sender_test.go`
- [ ] `internal/mail/templates.go` — email HTML templates: verification, magic link, password reset, welcome

#### 1.5 Auth Handlers

- [ ] `internal/handlers/auth.go` — HTTP handlers:
  - `POST /api/auth/register` — body: `{email, password}` → `{user, session}`
  - `POST /api/auth/login` — body: `{email, password}` → `{user, session}`
  - `POST /api/auth/logout` — clears session cookie
  - `GET /api/me` — returns current user
  - `POST /api/auth/email/verification/resend`
  - `GET /api/auth/email/verify?token=`
  - `POST /api/auth/magic-link/request`
  - `GET /api/auth/magic-link/consume?token=`
  - `POST /api/auth/password/forgot`
  - `POST /api/auth/password/reset` — body: `{token, password}`
- [ ] `internal/handlers/auth_test.go` — test all endpoints with mock service

#### 1.6 Session Middleware

- [ ] `internal/middleware/auth.go` — session middleware: extract `choresy_session` cookie, call `auth.Authenticate()`, inject `User` into context
- [ ] `internal/middleware/auth.go` — `CurrentUser(ctx)` helper, `RequireAuth` wrapper

#### 1.7 Frontend Auth

- [ ] `web/static/js/auth.js` — render functions + handlers:
  - `renderLoginView(state)` — email + password form, "Sign in with Google" button, "Forgot password?", "Create account" link
  - `renderRegisterView(state)` — email + password + confirm password form, Google button, "Already have an account?" link
  - `renderMagicLinkRequest(state)` — email input, "Send Magic Link" button
  - `renderMagicLinkNotice(state)` — "Check your email" message
  - `renderVerifyEmail(state)` — "Check your email" / "Email verified!" messages
  - `renderForgotPassword(state)` — email input
  - `renderResetPassword(state)` — new password + confirm
  - Auth event handlers: `login`, `register`, `logout`, `magic-link-request`, `password-forgot`, `password-reset`
- [ ] `web/static/js/api.js` — auth API functions: `apiLogin`, `apiRegister`, `apiLogout`, `apiMe`, etc.
- [ ] Auth CSS — form styles, auth card, Google button styling, error/success states

#### 1.8 Frontend Auth Tests

- [ ] `web/static/js/tests/runner.js` — add auth tests:
  - Render login view produces expected HTML
  - Render register view produces expected HTML
  - Invalid email rejected by validation
  - Short password rejected by validation
  - Login handler calls apiFetch with correct payload
  - CSRF token included in API calls

**Stage 1 Verification:**
```bash
make test        # All tests pass (Go + JS)
make e2e         # E2E: register, verify, login, logout flows pass
# Manual: Register user, receive verification email in Mailpit, verify, login, see empty dashboard
```

---

### Stage 2: Households & Multi-User (Day 3)

**Goal:** Users belong to households. Household creation on registration. Invite system for adding members. Role management.

#### 2.1 Household Domain

- [ ] `internal/models/household.go` — `Household` struct, `HouseholdMember` struct (with role enum: owner/admin/member)
- [ ] `internal/models/household.go` — `Invite` struct (code, created_by, expires_at, max_uses)

#### 2.2 Household Store & Service

- [ ] `internal/household/store.go` — `Store` interface
- [ ] `internal/household/postgres_store.go` — Postgres implementation
- [ ] `internal/household/postgres_store_test.go`
- [ ] `internal/household/service.go` — `Service`:
  - `CreateHousehold(ctx, name, ownerID)` — create household, add owner as member, seed pre-defined chores
  - `GetHousehold(ctx, householdID)` — get household with member list
  - `CreateInvite(ctx, householdID, createdBy)` — generate invite code (6-char alphanumeric)
  - `JoinHousehold(ctx, inviteCode, userID)` — add user as member, consume invite use
  - `UpdateMemberRole(ctx, householdID, userID, newRole)` — admin/owner only
  - `RemoveMember(ctx, householdID, userID)` — admin/owner only, can't remove last owner
  - `LeaveHousehold(ctx, householdID, userID)` — member can leave (owner must transfer first)
  - `TransferOwnership(ctx, householdID, currentOwnerID, newOwnerID)`
- [ ] `internal/household/service_test.go`

#### 2.3 Household Handlers

- [ ] `internal/handlers/household.go`:
  - `GET /api/household` — current user's household
  - `POST /api/household` — create household (only if user has none)
  - `PATCH /api/household` — update household name
  - `POST /api/household/invites` — create invite
  - `GET /api/household/invites` — list active invites
  - `DELETE /api/household/invites/{id}` — revoke invite
  - `POST /api/household/join` — body: `{invite_code}`
  - `PATCH /api/household/members/{id}` — body: `{role}`
  - `DELETE /api/household/members/{id}` — remove member
  - `POST /api/household/leave` — leave household
  - `POST /api/household/transfer` — body: `{new_owner_id}`
- [ ] `internal/handlers/household_test.go`

#### 2.4 Post-Registration Flow

- [ ] Modify `POST /api/auth/register` — auto-create household named "{User}'s Home", seed pre-defined chores
- [ ] Modify `POST /api/auth/register` — return `{user, session, household}`

#### 2.5 Frontend Household Management

- [ ] `web/static/js/household.js`:
  - `renderHouseholdView(state)` — household name, member list (avatars + names + roles), invite code display, "Copy Invite Link" button
  - `renderJoinHousehold(state)` — invite code input
  - `renderMemberManagement(state)` — role dropdowns, remove buttons
  - Event handlers: `create-household`, `join-household`, `copy-invite`, `change-role`, `remove-member`, `leave-household`
- [ ] Household CSS — member pills, invite code display, role badges

#### 2.6 First-Run / Setup Wizard

- [ ] On first login (user has no household), show setup wizard:
  - Step 1: "Welcome to Choresy! Let's set up your home."
  - Step 2: "Invite your partner" — shows invite code + "Copy" button, optional "Send via email" input
  - Step 3: "Choose your chores" — checklist of pre-defined chores to enable/disable
- [ ] `web/static/js/setup.js` — wizard render + handlers

**Stage 2 Verification:**
```bash
make test        # All tests pass
make e2e         # E2E: register → household auto-created → invite partner → partner joins → roles work
```

---

### Stage 3: Chore Management (Day 4)

**Goal:** Household members can view, create, edit, delete, and organize chores.

#### 3.1 Chore Domain & Store

- [ ] `internal/models/chore.go` — `Chore` struct (id, household_id, name, icon, color, category, sort_order, is_predefined, created_by, created_at)
- [ ] `internal/chore/store.go` — `Store` interface:
  ```go
  type Store interface {
      CreateChore(ctx, chore) (*Chore, error)
      GetChore(ctx, id) (*Chore, error)
      ListChores(ctx, householdID) ([]*Chore, error)
      UpdateChore(ctx, chore) error
      DeleteChore(ctx, id) error
      ReorderChores(ctx, householdID, choreIDs []int64) error
      SeedPredefinedChores(ctx, householdID) error
  }
  ```
- [ ] `internal/chore/postgres_store.go` — Postgres implementation
- [ ] `internal/chore/postgres_store_test.go`
- [ ] `internal/chore/memory_store.go` — in-memory for tests

#### 3.2 Chore Service

- [ ] `internal/chore/service.go` — `Service`:
  - `CreateChore(ctx, householdID, userID, name, icon, color, category)` — validate unique name per household
  - `ListChores(ctx, householdID)` — return sorted by sort_order
  - `UpdateChore(ctx, choreID, name, icon, color, category)`
  - `DeleteChore(ctx, choreID)` — soft-delete (or hard-delete if no logs exist)
  - `ReorderChores(ctx, householdID, choreIDs[])`
  - `GetSystemDefaults()` — return list of pre-defined chores (the 12 listed above)
  - `EnableDefaultChores(ctx, householdID, selectedNames[])` — seed only selected defaults
- [ ] `internal/chore/service_test.go`

#### 3.3 Chore Handlers

- [ ] `internal/handlers/chore.go`:
  - `GET /api/chores` — list household chores
  - `POST /api/chores` — create custom chore `{name, icon, color, category}`
  - `GET /api/chores/{id}` — get single chore
  - `PATCH /api/chores/{id}` — update chore
  - `DELETE /api/chores/{id}` — delete chore (only custom chores, not pre-defined)
  - `POST /api/chores/reorder` — `{chore_ids: [1, 3, 2]}`
  - `GET /api/chores/defaults` — list system default chores
  - `POST /api/chores/seed-defaults` — `{names: ["Feed Cats (Morning)", ...]}`
- [ ] `internal/handlers/chore_test.go`

#### 3.4 Frontend Chore Management

- [ ] `web/static/js/chores.js`:
  - `renderChoreList(state)` — grid of chore cards, each with icon, name, color indicator, category pill
  - `renderChoreCard(chore)` — single chore card with color accent
  - `renderChoreForm(state, chore?)` — modal/drawer for create/edit: name input, icon picker (emoji grid), color picker, category dropdown
  - `renderIconPicker()` — grid of common emojis
  - `renderColorPicker()` — row of color swatches
  - `renderCategoryFilter(state)` — horizontal scrollable pill filter (All, Feeding, Cleaning, Care, Plants, Custom)
  - Event handlers: `create-chore`, `edit-chore`, `delete-chore`, `reorder-chores`, `filter-category`
  - Drag-to-reorder on desktop (optional — use up/down buttons as simpler fallback)
- [ ] Chore CSS — card grid (2 columns), color accent bars, category pills, icon display, empty state

#### 3.5 Navigation Shell

- [ ] Update `render()` in `app.js` to render full layout:
  - Top bar: app name + current user avatar (color circle with initial)
  - Main content: `#app` renders current route view
  - Bottom tabs: 🏠 Home | 📋 Chores | 📊 History | ⚙️ Settings
  - Active tab highlighted with primary color
- [ ] Routes:
  - `/` or `/today` → today's chore log view
  - `/chores` → chore management list
  - `/history` → history view
  - `/settings` → settings (household, members, preferences)
- [ ] Unauthenticated → redirect to `/login`

**Stage 3 Verification:**
```bash
make test                          # All tests pass
make e2e                           # E2E: create household, view defaults, create custom chore, edit, reorder, delete
```

---

### Stage 4: Chore Logging — Core Loop (Day 5–6)

**Goal:** The main UX — one-tap logging of completed chores. Daily view showing who did what today. This is the killer feature. Must be grandma-simple.

#### 4.1 ChoreLog Domain & Store

- [ ] `internal/models/chore_log.go` — `ChoreLog` struct (id, household_id, user_id, chore_id, completed_at, note, created_at)
- [ ] `internal/log/store.go` — `Store` interface:
  ```go
  type Store interface {
      CreateLog(ctx, log) (*ChoreLog, error)
      GetLog(ctx, id) (*ChoreLog, error)
      DeleteLog(ctx, id) error
      ListLogs(ctx, householdID, date time.Time) ([]*ChoreLog, error)
      ListLogsRange(ctx, householdID, start, end time.Time) ([]*ChoreLog, error)
      GetDailySummary(ctx, householdID, date time.Time) (*DailySummary, error)
      GetUserStats(ctx, userID, start, end time.Time) (*UserStats, error)
      GetHouseholdStats(ctx, householdID, start, end time.Time) (*HouseholdStats, error)
  }
  ```
- [ ] `internal/log/postgres_store.go` — Postgres implementation
- [ ] `internal/log/postgres_store_test.go`

#### 4.2 ChoreLog Service

- [ ] `internal/log/service.go` — `Service`:
  - `LogChore(ctx, householdID, userID, choreID, completedAt, note)` — create log entry
  - `DeleteLog(ctx, logID, userID)` — delete own log (or admin delete any in household)
  - `GetTodayLogs(ctx, householdID)` — get all logs for today (by household timezone)
  - `GetDayLogs(ctx, householdID, date)` — get logs for specific date
  - `GetWeekLogs(ctx, householdID, weekStart)` — get logs for 7-day window
  - `GetMonthLogs(ctx, householdID, year, month)` — get logs for calendar month
  - `GetDailySummary(ctx, householdID, date)` — aggregated: total chores, by user, by category
  - `GetUserStats(ctx, userID, start, end)` — count of chores done, streaks
  - `GetHouseholdStats(ctx, householdID, start, end)` — leaderboard-style stats
  - `QuickLog(ctx, householdID, userID, choreID)` — log at current time (most common action)
- [ ] `internal/log/service_test.go`

#### 4.3 ChoreLog Handlers

- [ ] `internal/handlers/log.go`:
  - `POST /api/logs` — body: `{chore_id, completed_at?, note?}` → returns ChoreLog
  - `DELETE /api/logs/{id}` — delete log entry
  - `GET /api/logs/today` — today's logs with user and chore details
  - `GET /api/logs/day?date=2026-04-27` — specific day
  - `GET /api/logs/week?start=2026-04-27` — week of logs
  - `GET /api/logs/month?year=2026&month=4` — month of logs
  - `GET /api/logs/summary?date=2026-04-27` — daily summary
  - `GET /api/logs/stats?start=2026-04-01&end=2026-04-30` — user stats
- [ ] `internal/handlers/log_test.go`

#### 4.4 Frontend — Today View (The Core UX)

**Design philosophy:** The user opens the app and immediately sees a list of chores. Each chore is a big, tappable button. Tapping it logs that the user completed it. Done. Nothing else required. A logged chore shows a checkmark and who did it. If someone else already did it, it shows their name/color and is grayed out.

- [ ] `web/static/js/today.js`:
  - `renderTodayView(state)` — the main view:
    1. **Date header**: "Monday, April 27" with `<` `>` arrows to navigate days
    2. **Chore grid**: 2-column grid of big, colorful chore buttons
       - Each button: icon (large emoji), chore name, category pill
       - Not yet done today: full color, tappable
       - Done by me today: show checkmark overlay + "You" label, slightly muted
       - Done by someone else today: show checkmark + their name/initial, fully muted/grayed
    3. **Summary bar**: "4 of 8 chores done today" with progress indicator
    4. **FAB (floating action button)**: "+" to open quick-add overlay for custom/rare chores
  - `renderChoreButton(chore, logEntry)` — single chore tile
  - `renderDayNavigator(state)` — date picker with arrows
  - `renderProgressBar(done, total)` — colored progress bar
  - `renderQuickAddModal(state)` — modal with chore list to log any chore at custom time
  - Event handlers:
    - `log-chore` — tap chore button → POST /api/logs → update state → re-render
    - `undo-log` — tap logged chore → undo (DELETE /api/logs/{id})
    - `navigate-day` — change date, fetch logs for new date
    - `quick-add` — open modal, select chore, optional time, submit
    - `add-note` — add optional note to a log entry

#### 4.5 Frontend — History View

- [ ] `web/static/js/history.js`:
  - `renderHistoryView(state)`:
    1. **Week summary bar**: horizontal stripe of 7 days with completion dots
    2. **Day detail**: selected day shows list of done chores with times and who did them
    3. **User filter**: "All" | "Alice" | "Bob" filter buttons
    4. **Month calendar mini-view**: tap a day to see its logs
  - Event handlers: `select-history-day`, `filter-history-user`, `view-week`, `view-month`

#### 4.6 Today View CSS

This is the most important visual design. It must be beautiful and tactile.

- Chore button: `min-height: 80px`, `border-radius: 16px`, color accent on left border, large emoji (2rem), chore name in bold, category in small pill
- Logged state: `opacity: 0.5`, checkmark overlay using `::after` pseudo-element
- Progress bar: `height: 6px`, `border-radius: 3px`, animated width transition
- Date header: large serif-style date, arrow buttons
- FAB: `width: 56px`, `height: 56px`, `border-radius: 28px`, `position: fixed`, `bottom: 80px`, `right: 20px`, shadow, primary color
- Bottom tabs: `position: fixed`, `bottom: 0`, `width: 100%`, `max-width: 480px`, `height: 64px`, safe-area-inset-bottom padding

#### 4.7 Real-time Log Sync (SSE)

- [ ] `internal/sse/broker.go` — SSE broker pattern:
  - Maintains map of `householdID → []chan Event`
  - `Subscribe(householdID) → chan Event`
  - `Unsubscribe(householdID, chan)`
  - `Publish(householdID, Event)` — sends to all subscribers
- [ ] `internal/sse/handler.go` — `GET /api/events` handler:
  - Sets SSE headers (`text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`)
  - Subscribes to household's event channel
  - Writes events as they arrive
  - Closes on client disconnect (context cancellation)
- [ ] `internal/handlers/log.go` — on every `CreateLog`/`DeleteLog`, publish SSE event: `{type: "log_created"|"log_deleted", payload: ChoreLog}`
- [ ] `web/static/js/events.js` — `connectSSE(state)`:
  - Creates `EventSource("/api/events")`
  - On `log_created` → update `state.todayLogs`, re-render today view
  - On `log_deleted` → remove from `state.todayLogs`, re-render
  - Auto-reconnect on connection loss

#### 4.8 Stats Endpoint (for leaderboard)

- [ ] `GET /api/stats/weekly` — `{start, end}` → per-user counts
- [ ] `GET /api/stats/streaks` — current streaks per user per chore

**Stage 4 Verification:**
```bash
make test        # All tests pass
make e2e         # E2E: open today view → see chores → tap chore → log appears with checkmark → 
                 #       other user logs same chore → it updates in real-time →
                 #       navigate to yesterday → see yesterday's logs →
                 #       view history → week summary → month view
```

---

### Stage 5: Scheduling & Recurring Chores (Day 7)

**Goal:** Define schedules for chores (e.g., "Feed Cats every day at 8am and 5pm"). Chores only appear as "pending" at their scheduled times. Supports "N times per day" and "N times per week" patterns.

#### 5.1 Schedule Domain & Store

- [ ] `internal/models/schedule.go` — `ChoreSchedule` struct:
  ```go
  type ChoreSchedule struct {
      ID              int64
      HouseholdID     int64
      ChoreID         int64
      FrequencyType   string   // "daily", "weekly", "interval_days"
      TimesOfDay      []string // ["08:00", "17:00"] — HH:MM in household timezone
      DaysOfWeek      []int    // [0=Sun...6=Sat] for weekly schedules
      IntervalDays    int      // for "every N days" schedules
      TargetCount     int      // expected completions per period (nil = once per time slot)
      IsActive        bool
      AssignedUserID  *int64   // optional: suggests who should do it
      CreatedAt       time.Time
      UpdatedAt       time.Time
  }
  ```
- [ ] `internal/schedule/store.go` — `Store` interface + Postgres implementation
- [ ] `internal/schedule/postgres_store.go`
- [ ] `internal/schedule/postgres_store_test.go`

#### 5.2 Schedule Service

- [ ] `internal/schedule/service.go` — `Service`:
  - `CreateSchedule(ctx, schedule)` — create schedule for a chore
  - `UpdateSchedule(ctx, schedule)` — modify schedule
  - `DeleteSchedule(ctx, scheduleID)` — remove schedule
  - `GetSchedules(ctx, householdID)` — all active schedules
  - `GetSchedulesForChore(ctx, choreID)` — schedules for a specific chore
  - `GetPendingChores(ctx, householdID, date)` — chores that should be done at current time (not yet logged today)
  - `IsChorePending(ctx, choreID, householdID, date)` — check if chore needs doing now
  - `GetNextOccurrence(ctx, scheduleID)` — calculate next time this chore is due
- [ ] `internal/schedule/service_test.go`

#### 5.3 Schedule Engine (Time-based Logic)

- [ ] `internal/schedule/engine.go` — pure function package:
  - `IsSlotActive(schedule, timeOfDay, weekday time.Time) bool` — is this schedule active right now?
  - `GetActiveSlots(schedules, now time.Time) []ScheduleSlot` — which slots should be visible now?
  - `GetTodaysSlots(schedules, date time.Time) []ScheduleSlot` — all slots for a full day
  - `GetPendingCount(schedules, logs, date) int` — how many scheduled chores are not yet done today
  - `GetNextUp(schedules, logs, now) *ScheduleSlot` — what's the next pending chore?
- [ ] `internal/schedule/engine_test.go` — extensive table-driven tests for edge cases:
  - Daily at 8am, 5pm → 2 slots per day
  - Weekly Mon/Wed/Fri at 9am → 3 slots per week
  - Every 3 days at 10am → slot every 3rd day
  - Multiple times per day (feed cats 3x) → 3 slots
  - Timezone handling
  - DST transitions

#### 5.4 Schedule Handlers

- [ ] `internal/handlers/schedule.go`:
  - `GET /api/schedules` — list all schedules for household
  - `POST /api/schedules` — create schedule `{chore_id, frequency_type, times_of_day, days_of_week, interval_days, target_count, assigned_user_id}`
  - `PATCH /api/schedules/{id}` — update schedule
  - `DELETE /api/schedules/{id}` — delete schedule
  - `GET /api/schedules/pending?date=2026-04-27` — which chores are pending now
- [ ] `internal/handlers/schedule_test.go`

#### 5.5 Integrate Schedule into Today View

- [ ] Modify `GET /api/logs/today` to also return `pending_schedules` — chores that should be done but aren't logged yet
- [ ] Modify `renderTodayView(state)`:
  - Pending chores show a subtle pulsing indicator or "Due" badge
  - Overdue chores (past their scheduled time with no log) show as highlighted/emphasized
  - Chores with no schedule are always shown as optional
  - Sort order: Overdue first → Pending → Done → Unscheduled

#### 5.6 Frontend Schedule Management

- [ ] `web/static/js/schedule.js`:
  - `renderScheduleView(state)` — list of chores with their schedules, tap to edit
  - `renderScheduleForm(state, schedule?)` — form to set:
    - Frequency: "Daily" | "Specific days" | "Every N days"
    - Times: time input for each slot, "+" to add more
    - Days of week: toggle buttons S M T W T F S
    - Assigned to: user picker (optional)
  - `renderTimeSlotInput(slot)` — time picker component
  - Event handlers: `create-schedule`, `update-schedule`, `delete-schedule`, `add-time-slot`, `remove-time-slot`

#### 5.7 Frontend Schedule Tests

- [ ] `web/static/js/tests/runner.js` — add schedule tests:
  - Schedule form renders with correct defaults
  - Adding time slots updates the form
  - Daily frequency disables day-of-week picker
  - Validation: at least one time slot required

**Stage 5 Verification:**
```bash
make test        # All tests pass, schedule engine has 100% coverage
make e2e         # E2E: create schedule → today view shows pending chores →
                 #       log chore → pending indicator disappears →
                 #       verify overdue highlighting works
```

---

### Stage 6: Reminders & Notifications (Day 8)

**Goal:** Push notifications (via service worker) and optional email reminders for scheduled chores.

#### 6.1 Notification Domain & Store

- [ ] `internal/models/notification.go` — `Notification` struct, `ReminderPreference` struct
- [ ] `internal/notification/store.go` — `Store` interface
- [ ] `internal/notification/postgres_store.go` — Postgres implementation
- [ ] `internal/notification/postgres_store_test.go`

#### 6.2 Notification Service

- [ ] `internal/notification/service.go` — `Service`:
  - `CreateNotification(ctx, userID, type, title, body, metadata)`
  - `ListNotifications(ctx, userID, limit, offset)`
  - `MarkRead(ctx, notificationID, userID)`
  - `MarkAllRead(ctx, userID)`
  - `GetUnreadCount(ctx, userID)`
  - `DeleteNotification(ctx, notificationID, userID)`
  - `GetReminderPreferences(ctx, userID)`
  - `UpdateReminderPreferences(ctx, userID, prefs)`
- [ ] `internal/notification/service_test.go`

#### 6.3 Reminder Background Worker

- [ ] `cmd/server/background_jobs.go` — background goroutine:
  - Runs every 1 minute
  - Queries all active schedules
  - For each schedule slot approaching within the reminder window (configurable, default 5 min before):
    - Check if chore already logged for that slot today
    - If not logged, check user reminder preferences
    - If push enabled, insert notification row for service worker to pick up
    - If email enabled, send email reminder
  - Respects quiet hours from `ReminderPreference`
- [ ] `internal/notification/reminder.go` — reminder logic
- [ ] `internal/notification/reminder_test.go`

#### 6.4 Notification Handlers

- [ ] `internal/handlers/notification.go`:
  - `GET /api/notifications` — list notifications
  - `GET /api/notifications/unread-count` — just the count
  - `POST /api/notifications/{id}/read` — mark one read
  - `POST /api/notifications/read-all` — mark all read
  - `DELETE /api/notifications/{id}` — delete notification
  - `GET /api/me/reminder-preferences` — get prefs
  - `PATCH /api/me/reminder-preferences` — update prefs
- [ ] `internal/handlers/notification_test.go`

#### 6.5 Frontend Notifications

- [ ] `web/static/js/notifications.js`:
  - `renderNotificationBadge(state)` — unread count badge on bell icon in top bar
  - `renderNotificationList(state)` — list of notifications (icon, title, body, time ago, read/unread state)
  - `renderReminderSettings(state)` — push toggle, email toggle, quiet hours start/end, timezone
  - Event handlers: `mark-read`, `mark-all-read`, `dismiss-notification`, `update-reminder-prefs`
- [ ] Notification CSS — badge (red circle with count), notification cards, settings form

#### 6.6 Push Notification via Service Worker

- [ ] `web/static/service-worker.js` — add push event listener:
  - `self.addEventListener('push', ...)` — show notification
  - `self.addEventListener('notificationclick', ...)` — open app on tap
- [ ] `web/static/js/notifications.js` — `subscribeToPush(state)`:
  - Request `Notification.permission`
  - `registration.pushManager.subscribe()` with VAPID public key
  - Send subscription to `POST /api/me/push-subscription`
- [ ] `internal/handlers/notification.go` — `POST /api/me/push-subscription`, `DELETE /api/me/push-subscription`
- [ ] `internal/notification/push.go` — Web Push protocol implementation (VAPID) using `encrypted-content-encoding` package
- [ ] Add `VAPID_PRIVATE_KEY`, `VAPID_PUBLIC_KEY`, `VAPID_SUBJECT` to config

**Stage 6 Verification:**
```bash
make test        # All tests pass
make e2e         # E2E: set reminder preference → schedule chore → wait for reminder window →
                 #       notification appears → mark as read → unread badge updates
```

---

### Stage 7: PWA Polish & Mobile UX (Day 9)

**Goal:** App passes Lighthouse PWA audit with 100% score. Feels like a native app on iPhone. Works offline. Grandma-ready UX.

#### 7.1 Service Worker Enhancements

- [ ] `web/static/service-worker.js` — review and enhance:
  - Precise cache strategy: install-time precache for CSS/JS/icons, runtime cache-first for `/static/`, network-first for API (no cache), network-first for nav (offline fallback)
  - Cache versioning via build hash in service worker
  - Skip waiting + claim clients on install
  - Background sync for offline log creation:
    - When offline, queue log requests in IndexedDB
    - When back online, replay queued logs
- [ ] `web/static/js/offline.js` — offline queue management:
  - `queueOfflineLog(choreID, completedAt)` — store in IndexedDB
  - `syncOfflineLogs()` — POST queued logs, clear on success
  - Listen for `online` event → sync

#### 7.2 PWA Manifest Polish

- [ ] `web/static/manifest.webmanifest` — finalize:
  - `name: "Choresy — Household Chore Tracker"`
  - `short_name: "Choresy"`
  - `display: "standalone"`
  - `orientation: "portrait"`
  - `scope: "/"`
  - Icons: 192x192 maskable, 512x512 maskable (PNG), SVG any-purpose
- [ ] Generate PNG icons from SVG (in build-assets.sh)

#### 7.3 Mobile UX Polish

- [ ] `web/static/css/app.css` — mobile-first enhancements:
  - `touch-action: manipulation` on all buttons (no 300ms delay)
  - `-webkit-tap-highlight-color: transparent` on interactive elements
  - `overscroll-behavior: contain` on body
  - `safe-area-inset-*` padding on bottom tabs, top bar
  - `-webkit-overflow-scrolling: touch` on scrollable areas
  - Large touch targets: minimum 44x44px per WCAG
  - Active states: `:active` pseudo-class with scale transform or background change
  - Transition on chore buttons: `transform: scale(0.97)` on tap
  - Pull-to-refresh on today view (using `touchstart`/`touchmove`/`touchend` or `overscroll-behavior` + refresh button)
- [ ] Splash screen: solid background color matching theme (iOS reads from manifest `background_color`)
- [ ] Status bar: `<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">`

#### 7.4 Accessibility

- [ ] Color contrast: all text meets WCAG AA (4.5:1 for normal, 3:1 for large)
- [ ] Focus indicators: visible outline on keyboard focus (`:focus-visible`)
- [ ] Semantic HTML: use `<button>`, `<nav>`, `<main>`, `<header>`
- [ ] ARIA labels on icon-only buttons
- [ ] `prefers-reduced-motion` media query to disable animations
- [ ] `prefers-color-scheme` support for dark mode (future enhancement — flag as stretch goal)

#### 7.5 App Shell & Loading States

- [ ] Skeleton loading states for initial data fetch
  - Chore button skeletons: gray rounded rectangles with pulse animation
  - Text skeletons: gray lines
- [ ] Toast notification system:
  - Position: bottom center, above tabs
  - Types: success (green), error (red), info (blue)
  - Auto-dismiss after 3 seconds
  - Action toasts: "[name] logged [chore]" with undo button
  - Stack multiple toasts vertically

#### 7.6 Install Prompt

- [ ] `web/static/js/app.js` — install prompt logic:
  - Listen for `beforeinstallprompt` event
  - Store event in state
  - Show "Add to Home Screen" banner at top of today view (dismissible)
  - On prompt, show install instructions modal for iOS (no `beforeinstallprompt` on iOS Safari):
    - "Tap the Share button → Add to Home Screen"
    - Show animated illustration of the process

#### 7.7 Lighthouse Audit

- [ ] Run Lighthouse on deployed app
- [ ] Fix any issues to achieve:
  - Performance: ≥90
  - Accessibility: ≥95
  - Best Practices: ≥100
  - SEO: ≥90
  - PWA: All checks pass (installable, service worker, offline, manifest, splash screen)

**Stage 7 Verification:**
```bash
make test        # All tests pass
make e2e         # E2E: PWA tests — manifest loads, service worker registers, offline page shows,
                 #       install prompt appears, app works offline (queue + sync)
# Manual: Lighthouse audit score ≥ all targets above
```

---

### Stage 8: Stats, Insights & Gamification (Day 10)

**Goal:** Household leaderboard, streak tracking, weekly recaps. Light gamification to encourage participation.

#### 8.1 Stats Engine

- [ ] `internal/stats/service.go` — `Service`:
  - `GetWeeklyLeaderboard(ctx, householdID)` — count per user for current week, sorted desc
  - `GetMonthlyLeaderboard(ctx, householdID, year, month)`
  - `GetUserStreaks(ctx, userID)` — current streak (consecutive days with at least 1 chore), longest streak
  - `GetChoreStreaks(ctx, householdID)` — per-chore streaks across all users
  - `GetHeatmap(ctx, householdID, start, end)` — date → count for GitHub-style heatmap
  - `GetCategoryBreakdown(ctx, householdID, start, end)` — pie chart data: category → count
  - `GetBusyHours(ctx, householdID, start, end)` — hour → count for time-of-day chart
- [ ] `internal/stats/service_test.go`

#### 8.2 Stats Handlers

- [ ] `internal/handlers/stats.go`:
  - `GET /api/stats/leaderboard?period=week|month` — top users
  - `GET /api/stats/streaks` — user streaks
  - `GET /api/stats/heatmap?start=2026-01-01&end=2026-12-31` — activity heatmap
  - `GET /api/stats/breakdown?start=2026-01-01&end=2026-12-31` — category breakdown

#### 8.3 Frontend Stats View

- [ ] `web/static/js/stats.js`:
  - `renderStatsView(state)`:
    1. **Leaderboard** (this week): ranked user cards with avatar, name, count, crown emoji for #1
    2. **Streak counter**: "🔥 7 day streak!" with motivational message
    3. **Activity heatmap**: 7 columns (days) × N rows (weeks), color intensity by count
    4. **Category breakdown**: horizontal bar chart (CSS-based, no chart library)
    5. **Weekly recap**: "This week you did 23 chores. Most active on Wednesday. 🐱 Feed Cats was the most popular chore."
  - `renderLeaderboard(state)` — ranked list
  - `renderHeatmap(state)` — CSS grid of colored cells
  - `renderCategoryBars(state)` — horizontal bar chart using CSS `width` percentage
  - Event handlers: `switch-stats-period` (week/month), `view-user-stats`
- [ ] Stats CSS — leaderboard cards, heatmap grid, bar chart styles, streak flame animation

#### 8.4 Weekly Recap (Email + In-App)

- [ ] `internal/notification/recap.go` — generate weekly recap:
  - Total chores done by household
  - Top performer
  - Most popular chore
  - Comparison to previous week
  - Fun fact/quote
- [ ] Send via email every Sunday at 6pm (or configurable)
- [ ] Show as in-app notification card

**Stage 8 Verification:**
```bash
make test        # All tests pass
make e2e         # E2E: log several chores across days → view stats → leaderboard shows correct ranking →
                 #       streaks calculate correctly → heatmap reflects activity
```

---

### Stage 9: Polish, Testing Coverage & Deployment (Day 11–12)

**Goal:** Hit 75%+ automated code coverage, fix all known issues, deploy to production.

#### 9.1 Test Coverage Drive

- [ ] Run `make coverage` to get baseline coverage
- [ ] Identify packages below 80%:
  - Add missing unit tests for edge cases
  - Add integration tests for database operations
  - Add tests for error paths (not just happy paths)
- [ ] Identify JS modules below 80%:
  - Add tests for all render functions
  - Add tests for all event handlers
  - Add tests for edge cases (empty states, error states, loading states)
- [ ] Target: ≥75% overall (Go + JS combined), with ≥80% on Go backend

#### 9.2 E2E Test Suite Completion

- [ ] `tests/e2e/run.mjs` — orchestrate all scenarios:
  1. `auth-flow.mjs` — register → verify email → login → logout → login with magic link → password reset → Google OIDC
  2. `household-flow.mjs` — create household → view members → invite partner → partner accepts → manage roles
  3. `chore-crud.mjs` — view defaults → create custom → edit → reorder → delete
  4. `logging-flow.mjs` — today view → log chore → verify checkmark → undo → log with custom time → add note → navigate days
  5. `schedule-flow.mjs` — create schedule → verify pending indicator → create multi-slot schedule → verify all slots
  6. `realtime-flow.mjs` — two browser contexts → user A logs chore → user B sees it appear in real-time
  7. `notification-flow.mjs` — set reminders → wait for reminder → notification appears → mark read
  8. `stats-flow.mjs` — log several chores → view stats → verify leaderboard → verify heatmap
  9. `pwa-flow.mjs` — manifest loads → service worker active → offline mode works → install prompt
  10. `mobile-flow.mjs` — iPhone viewport → touch interactions → bottom tabs → safe area insets

#### 9.3 Performance Optimization

- [ ] Go backend:
  - Add query optimization (proper indexes on all foreign keys and query columns)
  - Add response compression (gzip middleware)
  - Add `Cache-Control` headers for static assets with hashed filenames (immutable, 1 year)
  - Add ETag support for API responses
  - Profile with `pprof` and fix any bottlenecks
- [ ] Frontend:
  - CSS size audit, remove unused styles
  - JS bundle size audit
  - Lazy load stats/charts module (only load when navigating to stats)
  - Minimize DOM operations in render
  - Add `will-change` hints for animated elements

#### 9.4 Security Audit

- [ ] Review all handlers for authorization checks (can user X access household Y's data?)
- [ ] Verify CSRF on all state-changing endpoints
- [ ] Rate limit all auth endpoints
- [ ] CSP audit — ensure no inline scripts/styles
- [ ] SQL injection audit — verify parameterized queries everywhere
- [ ] Session security — secure/HttpOnly/SameSite cookie flags
- [ ] Environment variable audit — no secrets in code or logs

#### 9.5 Deployment

- [ ] Build and push container image to `quay.io/nabu/nabu`
- [ ] Set up Hetzner server using `cloud-init.yaml`
- [ ] Configure Cloudflare Tunnel
- [ ] Deploy via `compose.server.yaml`
- [ ] Set up R2 backup bucket
- [ ] Configure systemd backup timer
- [ ] Verify production health check
- [ ] Verify SSL via Cloudflare
- [ ] Smoke test all critical paths in production

#### 9.6 Documentation Update

- [ ] `README.md` — project overview, quick start, deployment guide
- [ ] `AGENTS.md` — coding conventions, architecture, testing, common tasks
- [ ] `docs/deployment.md` — detailed deployment guide
- [ ] `docs/backup_restore.md` — backup and restore procedures
- [ ] `docs/pwa_validation.md` — PWA validation notes

**Stage 9 Verification:**
```bash
make coverage     # 75%+ overall coverage
make lint         # Clean
make test         # All tests pass
make e2e          # All E2E scenarios pass
# Manual: Production deployment smoke test passes
# Lighthouse: All targets met
```

---

## Database Migrations (Summary)

| # | Name | Purpose |
|---|---|---|
| 001 | `initial.sql` | users, sessions, auth_tokens |
| 002 | `households.sql` | households, household_members, invites |
| 003 | `chores.sql` | chores table + seed pre-defined chores |
| 004 | `chore_schedules.sql` | chore_schedules |
| 005 | `chore_logs.sql` | chore_logs |
| 006 | `reminder_preferences.sql` | reminder_preferences, push_subscriptions |
| 007 | `notifications.sql` | notifications |
| 008 | `indexes.sql` | Add indexes for performance |

---

## API Route Summary

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/health` | No | Health check |
| `GET` | `/ready` | No | Readiness check |
| `GET` | `/static/*` | No | Embedded static assets |
| `POST` | `/api/auth/register` | No | Create account |
| `POST` | `/api/auth/login` | No | Sign in |
| `POST` | `/api/auth/logout` | Yes | Sign out |
| `GET` | `/api/me` | Yes | Current user |
| `GET` | `/api/auth/google/login` | No | Google OAuth redirect |
| `GET` | `/api/auth/google/callback` | No | Google OAuth callback |
| `POST` | `/api/auth/email/verification/resend` | Yes | Resend verification |
| `GET` | `/api/auth/email/verify` | No | Verify email token |
| `POST` | `/api/auth/magic-link/request` | No | Request magic link |
| `GET` | `/api/auth/magic-link/consume` | No | Consume magic link |
| `POST` | `/api/auth/password/forgot` | No | Request password reset |
| `POST` | `/api/auth/password/reset` | No | Reset password |
| `GET` | `/api/household` | Yes | Get household |
| `POST` | `/api/household` | Yes | Create household |
| `PATCH` | `/api/household` | Yes | Update household |
| `POST` | `/api/household/invites` | Yes | Create invite |
| `GET` | `/api/household/invites` | Yes | List invites |
| `DELETE` | `/api/household/invites/{id}` | Yes | Revoke invite |
| `POST` | `/api/household/join` | Yes | Join via invite |
| `PATCH` | `/api/household/members/{id}` | Yes | Update member role |
| `DELETE` | `/api/household/members/{id}` | Yes | Remove member |
| `POST` | `/api/household/leave` | Yes | Leave household |
| `POST` | `/api/household/transfer` | Yes | Transfer ownership |
| `GET` | `/api/chores` | Yes | List chores |
| `POST` | `/api/chores` | Yes | Create chore |
| `GET` | `/api/chores/{id}` | Yes | Get chore |
| `PATCH` | `/api/chores/{id}` | Yes | Update chore |
| `DELETE` | `/api/chores/{id}` | Yes | Delete chore |
| `POST` | `/api/chores/reorder` | Yes | Reorder chores |
| `GET` | `/api/chores/defaults` | Yes | List default chores |
| `POST` | `/api/chores/seed-defaults` | Yes | Seed defaults |
| `POST` | `/api/logs` | Yes | Log chore |
| `DELETE` | `/api/logs/{id}` | Yes | Delete log |
| `GET` | `/api/logs/today` | Yes | Today's logs |
| `GET` | `/api/logs/day` | Yes | Day logs |
| `GET` | `/api/logs/week` | Yes | Week logs |
| `GET` | `/api/logs/month` | Yes | Month logs |
| `GET` | `/api/logs/summary` | Yes | Daily summary |
| `GET` | `/api/schedules` | Yes | List schedules |
| `POST` | `/api/schedules` | Yes | Create schedule |
| `PATCH` | `/api/schedules/{id}` | Yes | Update schedule |
| `DELETE` | `/api/schedules/{id}` | Yes | Delete schedule |
| `GET` | `/api/schedules/pending` | Yes | Pending chores |
| `GET` | `/api/notifications` | Yes | List notifications |
| `GET` | `/api/notifications/unread-count` | Yes | Unread count |
| `POST` | `/api/notifications/{id}/read` | Yes | Mark read |
| `POST` | `/api/notifications/read-all` | Yes | Mark all read |
| `DELETE` | `/api/notifications/{id}` | Yes | Delete notification |
| `GET` | `/api/me/reminder-preferences` | Yes | Get reminder prefs |
| `PATCH` | `/api/me/reminder-preferences` | Yes | Update reminder prefs |
| `POST` | `/api/me/push-subscription` | Yes | Save push subscription |
| `DELETE` | `/api/me/push-subscription` | Yes | Remove push subscription |
| `GET` | `/api/stats/leaderboard` | Yes | User leaderboard |
| `GET` | `/api/stats/streaks` | Yes | User streaks |
| `GET` | `/api/stats/heatmap` | Yes | Activity heatmap |
| `GET` | `/api/stats/breakdown` | Yes | Category breakdown |
| `GET` | `/api/events` | Yes | SSE event stream |

---

## File Inventory (Intended Final State)

```
choresy/
├── .github/workflows/ci.yaml
├── .gitignore
├── .dockerignore
├── .env.example
├── AGENTS.md
├── README.md
├── LICENSE
├── Makefile
├── Containerfile
├── compose.yaml
├── compose.server.yaml
├── compose.prod.yaml
├── cloud-init.yaml
├── go.mod
├── go.sum
├── cmd/server/
│   ├── main.go
│   └── main_test.go
├── internal/
│   ├── app/
│   │   ├── server.go
│   │   └── server_test.go
│   ├── audit/
│   │   └── audit.go
│   ├── auth/
│   │   ├── service.go
│   │   ├── service_test.go
│   │   ├── store.go
│   │   ├── memory_store.go
│   │   ├── postgres_store.go
│   │   ├── postgres_store_test.go
│   │   ├── password.go
│   │   ├── password_test.go
│   │   ├── oidc.go
│   │   └── oidc_test.go
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── chore/
│   │   ├── service.go
│   │   ├── service_test.go
│   │   ├── store.go
│   │   ├── postgres_store.go
│   │   ├── postgres_store_test.go
│   │   └── memory_store.go
│   ├── database/
│   │   ├── open.go
│   │   ├── migrate.go
│   │   └── migrate_test.go
│   ├── handlers/
│   │   ├── auth.go
│   │   ├── auth_test.go
│   │   ├── chore.go
│   │   ├── chore_test.go
│   │   ├── health.go
│   │   ├── health_test.go
│   │   ├── household.go
│   │   ├── household_test.go
│   │   ├── log.go
│   │   ├── log_test.go
│   │   ├── notification.go
│   │   ├── notification_test.go
│   │   ├── schedule.go
│   │   ├── schedule_test.go
│   │   ├── stats.go
│   │   ├── stats_test.go
│   │   └── json.go
│   ├── household/
│   │   ├── service.go
│   │   ├── service_test.go
│   │   ├── store.go
│   │   ├── postgres_store.go
│   │   └── postgres_store_test.go
│   ├── log/
│   │   ├── service.go
│   │   ├── service_test.go
│   │   ├── store.go
│   │   ├── postgres_store.go
│   │   └── postgres_store_test.go
│   ├── mail/
│   │   ├── sender.go
│   │   ├── sender_test.go
│   │   ├── smtp_sender.go
│   │   ├── memory_sender.go
│   │   ├── log_sender.go
│   │   └── templates.go
│   ├── middleware/
│   │   ├── auth.go
│   │   ├── csrf.go
│   │   ├── security.go
│   │   ├── requestlog.go
│   │   ├── ratelimit.go
│   │   └── middleware_test.go
│   ├── models/
│   │   ├── user.go
│   │   ├── chore.go
│   │   ├── chore_log.go
│   │   ├── household.go
│   │   ├── schedule.go
│   │   └── notification.go
│   ├── notification/
│   │   ├── service.go
│   │   ├── service_test.go
│   │   ├── store.go
│   │   ├── postgres_store.go
│   │   ├── postgres_store_test.go
│   │   ├── reminder.go
│   │   ├── reminder_test.go
│   │   ├── push.go
│   │   ├── push_test.go
│   │   └── recap.go
│   ├── schedule/
│   │   ├── service.go
│   │   ├── service_test.go
│   │   ├── store.go
│   │   ├── postgres_store.go
│   │   ├── postgres_store_test.go
│   │   ├── engine.go
│   │   └── engine_test.go
│   ├── sse/
│   │   ├── broker.go
│   │   ├── broker_test.go
│   │   └── handler.go
│   └── stats/
│       ├── service.go
│       └── service_test.go
├── migrations/
│   ├── assets.go
│   ├── assets_test.go
│   ├── 001_initial.sql
│   ├── 002_households.sql
│   ├── 003_chores.sql
│   ├── 004_chore_schedules.sql
│   ├── 005_chore_logs.sql
│   ├── 006_reminders.sql
│   ├── 007_notifications.sql
│   └── 008_indexes.sql
├── web/
│   ├── assets.go
│   ├── templates/
│   │   └── index.html
│   └── static/
│       ├── css/
│       │   └── app.css
│       ├── js/
│       │   ├── app.js
│       │   ├── api.js
│       │   ├── state.js
│       │   ├── morph.js
│       │   ├── auth.js
│       │   ├── household.js
│       │   ├── setup.js
│       │   ├── chores.js
│       │   ├── today.js
│       │   ├── history.js
│       │   ├── schedule.js
│       │   ├── notifications.js
│       │   ├── stats.js
│       │   ├── events.js
│       │   ├── offline.js
│       │   └── tests/
│       │       └── runner.js
│       ├── icons/
│       │   └── icon.svg
│       ├── manifest.webmanifest
│       ├── offline.html
│       └── service-worker.js
├── tests/
│   └── e2e/
│       ├── run.mjs
│       ├── helpers.mjs
│       ├── auth-flow.mjs
│       ├── household-flow.mjs
│       ├── chore-crud.mjs
│       ├── logging-flow.mjs
│       ├── schedule-flow.mjs
│       ├── realtime-flow.mjs
│       ├── notification-flow.mjs
│       ├── stats-flow.mjs
│       ├── pwa-flow.mjs
│       └── mobile-flow.mjs
├── scripts/
│   ├── build-assets.sh
│   ├── e2e.sh
│   ├── e2e-podman.sh
│   ├── seed.sh
│   ├── backup.sh
│   ├── restore.sh
│   ├── verify-backup.sh
│   ├── wait-for-stack.sh
│   └── release.sh
└── docs/
    ├── plans/
    │   └── init.md
    ├── deployment.md
    ├── backup_restore.md
    └── pwa_validation.md
```

---

## Key Design Patterns (from CalTrack to Follow)

1. **No framework** — Go stdlib `net/http`, vanilla JS ES modules, plain CSS. Zero unnecessary dependencies.
2. **Interface-driven design** — Every domain has a `Store` interface with in-memory and Postgres implementations. Services depend on interfaces.
3. **Dependency injection for testability** — Go: pass interfaces to constructors. JS: pass `{state, apiFetch, render}` to handler functions.
4. **Single mutable state** — JS state is one flat object. Mutate it directly, then call `render()`.
5. **Event delegation** — All events handled via `data-action` attributes on `#app`.
6. **DOM morphing** — Render functions return HTML strings; `morph.js` reconciles existing DOM.
7. **Embedded assets** — Everything compiled into a single Go binary via `//go:embed`.
8. **Auto-migration on startup** — Migrations embedded and applied at boot.
9. **Raw SQL, no ORM** — All queries hand-written for clarity and performance.
10. **Double-submit cookie CSRF** — Same pattern as CalTrack.
11. **Rate limiting on auth endpoints** — In-memory IP+path rate limiter.
12. **Content-hashed static assets** — `build-assets.sh` generates hashed filenames, `manifest.json` maps them, 1-year cache.
13. **SSE for real-time** — Server-Sent Events with Redis Pub/Sub for cross-instance broadcast (future-proof for scaling).
