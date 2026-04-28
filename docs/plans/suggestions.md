# Optimization Suggestions

Findings from a full codebase review. Items are grouped by severity.

---

## Bugs

### 1. `hashToken` is not a cryptographic hash
**File:** `internal/auth/service.go:444`

`hashToken` base64-encodes the input rather than hashing it. Anyone with read access to the `sessions.token_hash` or `auth_tokens.token_hash` columns can trivially decode them back to live tokens. Replace with SHA-256:

```go
func hashToken(token string) string {
    h := sha256.Sum256([]byte(token))
    return base64.RawURLEncoding.EncodeToString(h[:])
}
```

### 2. `LeaveHousehold` always blocks owners
**File:** `internal/household/service.go:172-188`

The logic inside the owner branch loops over members looking for another owner, then unconditionally `break`s and falls through to `return ErrLastOwner`. The `break` only exits the loop — the function always returns `ErrLastOwner` for any owner, even when other owners exist. The check needs a boolean flag or an early return on finding a co-owner:

```go
if role == RoleOwner {
    members, err := s.store.GetMembers(ctx, hhID)
    if err != nil {
        return err
    }
    for _, m := range members {
        if m.Role == RoleOwner && m.UserID != userID {
            // another owner exists, safe to leave
            return s.store.RemoveMember(ctx, hhID, userID)
        }
    }
    return ErrLastOwner
}
```

### 3. Postgres stores are never wired up
**File:** `internal/app/server.go:173-189`

`BuildServer` opens a DB connection and runs migrations, but then calls `NewServer(cfg)` which always creates memory stores. All the Postgres store implementations (`auth`, `chore`, `log`, `household`) exist but are never instantiated in production. `BuildServer` needs to construct Postgres stores and pass them to the services when `cfg.DatabaseURL != ""`.

### 4. `ChoreHandler.Get` passes `householdID = 0`
**File:** `internal/handlers/chore.go:73`

`h.service.ListChores(r.Context(), 0)` queries for chores belonging to household 0, which always returns an empty list. The handler should use `r.PathValue("id")` to look up the chore directly via `service.GetChore`, or at minimum pass the authenticated user's household ID.

### 5. `SeedPredefinedChores` silently ignores errors (Postgres)
**File:** `internal/chore/postgres_store.go:71`

```go
s.db.ExecContext(ctx, `INSERT INTO chores ...`)  // return value discarded
```

The error from `ExecContext` is dropped. If the insert fails for any reason other than a duplicate (e.g., FK violation, connection error), it is silently swallowed. The loop should check and propagate errors.

---

## Security

### 6. No request body size limit
**File:** `internal/handlers/json.go:18`

`readJSON` reads `r.Body` without any size cap. A client can send a multi-gigabyte body to exhaust memory. Wrap the body before decoding:

```go
func readJSON(r *http.Request, target any) error {
    r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1 MB
    return json.NewDecoder(r.Body).Decode(target)
}
```

### 7. Session cookie missing `Secure` flag
**File:** `internal/handlers/auth.go:249-258`

`SetSessionCookie` never sets `Secure: true`. In production the session cookie will be sent over plain HTTP. The flag should mirror the same `requestIsHTTPS` check used for OIDC cookies, or be driven by `cfg.ServerSecure`.

### 8. OAuth state parameter not validated
**File:** `internal/handlers/auth.go:223-240`

`GoogleCallback` reads the `code` parameter and proceeds with the token exchange, but never reads or validates the `choresy_oidc_state` cookie against the `state` query parameter. This leaves the OAuth callback open to CSRF. The handler should compare `r.URL.Query().Get("state")` with `h.getOIDCCookie(r, "choresy_oidc_state")` and reject mismatches.

### 9. Rate limiter map grows unbounded
**File:** `internal/middleware/ratelimit.go`

Expired entries in `l.entries` are never evicted. Under sustained traffic the map grows indefinitely, leaking memory. Add a periodic cleanup goroutine, or evict stale entries during the `allow` call.

### 10. Rate limiter ignores `X-Forwarded-For` / trusted proxies
**File:** `internal/middleware/ratelimit.go:65`

`clientIP` uses `r.RemoteAddr` directly. When the app sits behind a reverse proxy, every request has the same remote address (the proxy), so the rate limiter is effectively disabled. `TrustedProxyCIDRs` exists in `config.go` but is never read. The rate limiter should extract the real client IP from `X-Forwarded-For` when the request originates from a trusted proxy CIDR.

---

## Performance

### 11. `GetUserStreaks` fetches all logs since epoch
**File:** `internal/stats/service.go:96`

```go
s.logStore.ListLogsRange(ctx, householdID, time.Time{}, ...)
```

Passing `time.Time{}` (the zero value) as the start fetches every log entry ever recorded. For streak calculation only the last ~365 days are needed. Bounding the query to `now.AddDate(-1, 0, 0)` will make a material difference as data grows.

### 12. `ReorderChores` issues N individual UPDATE queries
**File:** `internal/chore/postgres_store.go:60-66`

Each element in the `choreIDs` slice results in a separate round-trip. Use a single query with `UNNEST` or an inline `VALUES` table:

```sql
UPDATE chores SET sort_order = v.ord
FROM (SELECT UNNEST($1::bigint[]) AS id, GENERATE_SERIES(0, $2) AS ord) v
WHERE chores.id = v.id AND chores.household_id = $3
```

### 13. Index template parsed on every request
**File:** `internal/app/server.go:196`

`renderIndex` calls `template.Must(template.ParseFS(...))` on every HTTP request. Parse the template once at server startup and store it:

```go
var indexTmpl = template.Must(template.ParseFS(webassets.Assets, "templates/index.html"))
```

### 14. `GetDailySummary` re-queries data already fetched
**File:** `internal/handlers/log.go:83-90`

The `Today` handler calls `GetDayLogs` and then `GetDailySummary` separately. Both fetch the same rows from the store. `GetDailySummary` should accept an already-fetched `[]ChoreLog` rather than issuing a second query.

### 15. `sortChores` uses O(n²) bubble sort
**File:** `internal/chore/store_memory.go:144-151`

Replace the manual nested loop with `sort.Slice` from the standard library:

```go
func sortChores(chores []Chore) {
    sort.Slice(chores, func(i, j int) bool {
        if chores[i].SortOrder != chores[j].SortOrder {
            return chores[i].SortOrder < chores[j].SortOrder
        }
        return chores[i].Name < chores[j].Name
    })
}
```

### 16. DB connection pool missing lifetime settings
**File:** `internal/database/open.go`

`SetConnMaxLifetime` and `SetConnMaxIdleTime` are not configured. Without them, idle connections can linger for the lifetime of the process and become stale after network resets or server-side timeouts. Recommended values:

```go
db.SetConnMaxLifetime(5 * time.Minute)
db.SetConnMaxIdleTime(1 * time.Minute)
```

---

## Architecture

### 17. `RequireAuth` middleware is defined but never used
**File:** `internal/middleware/auth.go:42`

A `RequireAuth` helper exists but every handler repeats the same manual auth check instead of using it. Adopt `RequireAuth` consistently to reduce boilerplate and ensure no endpoint accidentally omits the check.

### 18. `Me` handler duplicates the Session middleware's work
**File:** `internal/handlers/auth.go:76-89`

`Me` calls `authService.Authenticate` with the raw cookie value even though the `Session` middleware already ran `Authenticate` and attached the user to the request context. The handler should just call `middleware.CurrentUser(r.Context())` instead of issuing a second DB/store lookup.

### 19. `choreStatsAdapter` is hardcoded to `*chore.MemoryStore`
**File:** `internal/app/server.go:239-260`

The adapter struct captures `*chore.MemoryStore` by concrete type. When the Postgres store is wired in, this adapter will not compile. It should accept the `chore.Store` interface instead.

### 20. Config fields that are wired up but never consumed
**File:** `internal/config/config.go`

The following fields are populated from environment variables but have no corresponding implementation:
- `RedisURL` — no Redis client anywhere; was presumably planned for caching or session storage.
- `SessionSecret` / `CSRFSecret` — the CSRF middleware generates stateless random tokens; these secrets are never applied.
- `VAPIDPrivateKey` / `VAPIDPublicKey` / `VAPIDSubject` — push notification infrastructure is absent.
- `TrustedProxyCIDRs` — parsed but never passed to the rate limiter or IP extraction logic.

Either implement the features or remove the dead config to avoid confusion.

### 21. `isUniqueViolation` uses fragile string matching
**File:** `internal/auth/postgres_store.go:239-258`

The function re-implements `strings.Contains` with a hand-rolled `indexOf`, and detects Postgres errors by substring-matching error messages. Use the `pgconn.PgError` type from `pgx` instead:

```go
import "github.com/jackc/pgx/v5/pgconn"

func isUniqueViolation(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
```

---

## Frontend

### 22. `escapeHTML` duplicated across modules
**Files:** `web/static/js/app.js:202`, `web/static/js/today.js:103`, `web/static/js/stats.js:9`

Three identical implementations exist. Move the function to a shared `utils.js` module and import it everywhere.

### 23. `apiFetch` duplicated in `today.js` and `stats.js`
**Files:** `web/static/js/today.js:3-10`, `web/static/js/stats.js:3-7`

Both modules define a local `apiFetch` that duplicates the one exported from `api.js`, with slight differences (e.g. `stats.js` always sends `Content-Type`, `today.js` does not). Both should import and use `apiFetch` from `api.js` directly.

### 24. `seedDefaultChores` bypasses `apiFetch`
**File:** `web/static/js/app.js:523-544`

This function uses raw `fetch` with a manual cookie regex to extract the CSRF token, duplicating logic that `apiFetch` already handles. It should call `apiFetch("/api/chores/seed-defaults", { method: "POST", body: JSON.stringify({...}) })`.

### 25. History view is a stub
**File:** `web/static/js/today.js:96-101`

`renderHistoryView` returns a static placeholder. The backend has fully working `/api/logs/week` and `/api/logs/month` endpoints. The history view should load and display actual log data.

### 26. Leaderboard displays user IDs instead of names
**File:** `web/static/js/stats.js:46`

```js
`<span>User ${entry.userId}</span>`
```

The leaderboard API returns `userId` integers. The frontend should resolve these against `state.members` (which contains `displayName` and `avatarColor`) to show real names and avatars.

### 27. `resetAuthedState` does not clear `members` or `invites`
**File:** `web/static/js/state.js:15-21`

On logout, `state.members` and `state.invites` are not cleared. If a new user logs in during the same session, they briefly see the previous user's household data. Add both fields to `resetAuthedState`.

### 28. Stats race on first settings render
**File:** `web/static/js/app.js:145-160`, `app.js:470-477`

`loadStatsData()` is called without `await`, so on initial load the settings view always renders "Loading stats..." even when the data arrives within milliseconds. Either await the stats load before rendering or re-render when the data resolves.

---

## Testing

### 29. Handler tests missing for most domains
Handler tests only exist for `auth` and `health`. The `chore`, `log`, `household`, and `stats` handlers have no test files. Adding table-driven tests for happy-path and error cases would raise coverage meaningfully.

### 30. Postgres store tests missing for chore, log, household
`internal/auth` has `postgres_store` tests via `go-sqlmock`. The same pattern is absent for `internal/chore`, `internal/log`, and `internal/household`. At minimum, the query structure and error mapping for each store should be tested.

---

## Code Style

### 31. Compound statements on single lines in `household/postgres_store.go`
**File:** `internal/household/postgres_store.go:110-113`

Lines 110-113 pack multiple statements and error branches onto single lines. This makes the code hard to read and harder to set breakpoints on. Split into normal multi-line Go style consistent with the rest of the codebase.

---

## Additional Findings

### 32. Session middleware authenticates static asset requests too
**Files:** `internal/app/server.go:163-168`, `internal/middleware/auth.go:14-33`

`middleware.Session` wraps the entire mux. For authenticated browsers, requests for `/static/css/app.css`, `/static/js/app.js`, imported ES modules, `/service-worker.js`, and even `/health` all call `authService.Authenticate` and hit the backing store. On Postgres this turns a single page load into several unnecessary session lookups. Scope the session middleware to routes that actually need user context, or have it skip `/static/`, `/health`, and `/ready`.

### 33. Settings stats load fans out into overlapping log scans
**Files:** `web/static/js/app.js:145-159`, `internal/handlers/stats.go`, `internal/stats/service.go:78-92,156-185,204-261`

Opening the settings page triggers four requests in parallel: leaderboard, streaks, breakdown, and recap. Three of them scan overlapping weekly log ranges, and two of them also fetch chores separately. A single `/api/stats/overview` endpoint could fetch the week's logs and chores once, then derive all cards from the same data. If you want to keep the current endpoints, add a shared service method so the weekly calculations reuse one fetched dataset.

### 34. Composite indexes are missing for the dominant range queries
**Files:** `migrations/001_initial.sql:99-106`, `internal/log/postgres_store.go:60-62`, `internal/chore/postgres_store.go:34`

The hottest log query is:

```sql
SELECT ...
FROM chore_logs
WHERE household_id = $1 AND completed_at >= $2 AND completed_at < $3
ORDER BY completed_at
```

The schema only defines separate indexes on `household_id` and `completed_at`, so Postgres has to do more work than necessary as the table grows. Add a composite index on `(household_id, completed_at)`. The same pattern applies to chores: `ListChores` filters by `household_id` and orders by `sort_order`, so `(household_id, sort_order)` is a better fit than `idx_chores_household_id` alone.

### 35. Household create/join paths perform redundant membership writes
**Files:** `internal/handlers/household.go:65,168`, `internal/household/postgres_store.go:26,75-77`, `internal/household/memory_store.go:55,112-117`

The store layer already updates the user's household during `CreateHousehold` and `AddMember`, but the handler immediately calls `authService.SetUserHousehold(...)` again after both operations. That adds an extra write on every create/join path and duplicates the responsibility across layers. Remove the second write and let the store mutation remain the single source of truth.

### 36. Authenticated bootstrap work is serialized unnecessarily
**Files:** `web/static/js/app.js:247-255`, `web/static/js/app.js:469-475`

Once `state.household` is known, `loadChoreData`, `loadTodayData`, and `loadStatsData` are independent, but the app currently waits for chores before loading today, then starts stats afterward. Switching that stage to `Promise.all([...])` will shorten time-to-interactive after login and reduce the time the settings view spends in its loading state.

### 37. `render()` performs network side effects on auth routes
**Files:** `web/static/js/app.js:30-45`, `web/static/js/app.js:316-337`

`render()` directly calls `verifyEmail(token)` and `consumeMagicLink(token)`. Any rerender on those routes repeats the fetch, which wastes requests and can consume one-time tokens more than once. Move those calls behind one-shot route handlers or state flags so rendering stays pure and the network work only runs once per route entry.
