# Plan: Increase Go Test Coverage to 80% and Reduce E2E Test Time

## Current State

| Metric | Value |
|--------|-------|
| Go statement coverage | **33.5%** |
| Target | **80%** |
| Functions at 0% coverage | **305** |
| E2E specs | 27 files, ~286 tests |
| E2E CI workers | 1 (serial) |
| Total hardcoded `waitForTimeout` delay | **~195 seconds** |

The biggest coverage gaps are in core service and store packages. The E2E slowness comes from two sources: serial execution (single worker) and hundreds of fixed `waitForTimeout` calls.

---

## Part 1: Go Unit Test Coverage

### Strategy

Each tier below lists the files to create or extend. Use the existing `internal/userprefs/userprefs_test.go` as the style reference — it tests the MemoryStore directly and the Service on top of it, using no external dependencies for non-Postgres tests. Do not use `go-sqlmock` unless testing a Postgres store specifically.

Run after each file: `go test ./internal/<pkg>/... -v` to confirm tests pass.

After all tiers: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | grep "total:"` — verify the number is above 80%.

---

### Tier 1 — New test files (currently 0%, biggest gains)

#### 1a. `internal/chore/service_test.go`

Package under test: `internal/chore` (package `chore_test`).  
Zero-coverage files: `service.go`, `store_memory.go`.

Functions to cover in `service.go`:
- `NewService` — instantiates cleanly
- `CreateChore` — happy path: verify returned Chore has name/icon/color/category/indicatorLabels set; defaults: empty name stays empty, empty icon/color get defaults (check what `service.go` assigns)
- `ListChores` — returns empty slice for new household; returns created chores
- `GetChore` — returns chore by ID; returns error for missing ID
- `UpdateChore` — mutates name/icon/color/category/indicatorLabels; verify via `GetChore`
- `DeleteChore` — chore is gone after delete; `DeleteChore` on a predefined chore should return an error (check the guard in `service.go:75`)
- `ReorderChores` — verify order is accepted without error
- `RestoreDefaultChore` — call on a predefined chore ID; call on an unknown ID
- `GetSystemDefaults` — returns non-empty slice; each entry has name and icon set
- `SeedDefaultChores` — after call, `ListChores` returns entries matching `GetSystemDefaults`

Functions to cover in `store_memory.go` (these are called indirectly by the service tests above, but add direct store tests for edge cases):
- `Create` — sequential IDs, stored by household
- `List` — empty household returns empty; multiple households isolated
- `Get` — returns correct chore; returns error for unknown ID
- `Update` — mutates fields; returns error for unknown ID
- `Delete` — removes from list; returns error for unknown ID
- `Reorder` — test with a subset of IDs; test with extra IDs not in household
- `GetByHouseholdAndPosition` — if this function exists; call it

#### 1b. `internal/log/service_test.go`

Package: `internal/log` (`package log_test`).  
Zero-coverage files: `service.go`, `memory_store.go`.

Functions to cover in `service.go`:
- `NewService`
- `LogChore` — basic log; with `completedAt` provided vs nil (nil should default to now); with `slotHour` non-nil vs nil; with `volumeML`; returns `ChoreLog` with correct fields
- `UpdateLog` — update note, indicators, volumeML, userID, completedAt, slotHour, logDate; verify via `GetDayLogs`
- `UndoLog` — log is removed; undo unknown log returns error
- `GetTodayLogs` — returns logs with today's date
- `GetDayLogs` — returns logs for a specific date; logs on other dates not included
- `GetWeekLogs` — returns logs within a 7-day window
- `GetMonthLogs` — returns logs within a calendar month
- `GetDailySummary` / `DailySummaryFromLogs` — non-empty summary has correct count and unique chore list
- `LatestPerChore` — map keyed by choreID; most-recent log wins when multiple entries exist
- `GetHistoryLogs` — returns logs in range; `hasMore` is true when more exist beyond the limit

Functions to cover in `memory_store.go` (edge cases):
- `Create` — sequential IDs; `LogDate` defaults to today if zero
- `List` / `ListByHousehold` / `ListByDate` — household isolation; date filtering
- `HistoryLogs` — pagination: `hasMore` correct; ordering newest-first
- `LatestPerChore` — multiple logs for same chore returns only the latest

#### 1c. `internal/household/service_test.go`

Package: `internal/household` (`package household_test`).  
Zero-coverage files: `service.go`, `memory_store.go`.

The service takes a `Store` and an `AuthStore`. For unit tests, implement a minimal `AuthStore` stub (struct with a `SetUserHousehold` method) — check the `AuthStore` interface in `service.go` to see the exact method signatures required.

Functions to cover in `service.go`:
- `NewService`
- `CreateHousehold` — returns Household; owner is added as member with owner role; empty name should probably succeed (confirm what the service does)
- `GetHousehold` — returns household and members for known user; returns error for user with no household
- `UpdateHousehold` — name changes; user not in household returns error
- `CreateInvite` — returns Invite with non-empty code; invite is retrievable via `GetInvites`
- `GetInvites` — empty list for new household; populated after `CreateInvite`
- `DeleteInvite` — invite gone after delete; deleting another user's invite returns error
- `JoinHousehold` — user joins via valid code; becomes member; user already in a household returns error; invalid code returns error
- `UpdateMemberRole` — owner can change member role; non-owner cannot; cannot demote the owner
- `RemoveMember` — owner removes member; member is gone; owner cannot remove themselves
- `LeaveHousehold` — member leaves; last member (owner) leaving returns error or succeeds based on actual behavior
- `TransferOwnership` — new owner has owner role; old owner has member role; non-owner cannot transfer

Functions to cover in `memory_store.go`:
- `Create`, `GetByID`, `GetByUserID`, `Update` — basic CRUD, user isolation
- `AddMember`, `GetMembers`, `RemoveMember`, `UpdateMemberRole` — verify correct membership state
- `CreateInvite`, `GetInvitesByHousehold`, `GetInviteByCode`, `DeleteInvite`

#### 1d. `internal/stats/service_test.go`

Package: `internal/stats` (`package stats_test`).  
This package has no test file at all and no coverage (not even in the coverage profile).

The service takes a `log.Store` and an internal `choreStore` interface. Use the MemoryStore implementations.

Functions to cover:
- `NewService`
- `GetWeeklyLeaderboard` — returns ranked entries; empty household returns empty list
- `GetMonthlyLeaderboard` — same but for a specific month
- `GetUserStreaks` — user with consecutive daily logs has correct `Current` and `Longest`; user with no logs has zero streaks; streak breaks correctly on a gap day
- `GetHeatmap` — cells have correct count per day; days with no logs are absent or zero
- `GetCategoryBreakdown` — categories sum to total log count; empty range returns empty
- `GetBusyHours` — hours with most logs rank highest
- `GetWeeklyRecap` — correct total count; correct unique chores; correct top logger
- `GetChoreStats` — each chore entry has correct total count; average interval set when multiple logs exist
- `GetWeeklyOverview` — today's logs appear in today's column; correct total for the week
- `logInRange` (unexported) — test via `GetHeatmap` with boundary dates

#### 1e. `internal/notification/service_test.go`

Package: `internal/notification` (`package notification_test`).  
The `MemoryStore` lives in `store.go` (same package). Both have 0% coverage.

Functions to cover in service:
- `NewService`, `WithPushSender`
- `List` — returns notifications for user; unread count matches
- `UnreadCount` — 0 for new user; increments after `NotifyChoreLogged`; decrements after `MarkRead`
- `MarkRead` — notification IsRead becomes true; other user's notifications unaffected
- `MarkAllRead` — all notifications for user become read; other user's unaffected
- `Delete` — notification gone; other user's unaffected
- `GetNotificationPreferences` / `UpdateNotificationPreferences` — round-trip
- `AvailableNotificationTypes` — returns non-empty slice with `chore_logged`
- `NotifyChoreLogged` — creates notification for each member except loggerID and actorID; calls PushSender for members with push enabled; skips push for members with `PushEnabled: false`; skips push for members whose `EnabledPushTypes` excludes `"chore_logged"`

For `PushSender` testing, implement a minimal stub that records calls:
```go
type stubPush struct{ calls []int64 }
func (s *stubPush) SendPushToUser(_ context.Context, userID int64, _, _ string) error {
    s.calls = append(s.calls, userID); return nil
}
```

Functions to cover in `MemoryStore` (via service tests + direct):
- `CreateNotification`, `ListNotifications` (pagination), `GetUnreadCount`
- `MarkRead`, `MarkAllRead`, `DeleteNotification`
- `GetReminderPreferences` (returns default when missing), `UpdateReminderPreferences`

---

### Tier 2 — Extend existing test files

#### 2a. `internal/schedule/service_test.go` — add DateOnly marshal/unmarshal

Currently `DateOnly.MarshalJSON` and `UnmarshalJSON` are at 0%.

Add to the existing test file:
- `TestDateOnly_MarshalJSON` — `DateOnly{2024, 3, 15}` marshals to `"2024-03-15"`
- `TestDateOnly_UnmarshalJSON_Valid` — `"2024-03-15"` unmarshals to `DateOnly{2024, 3, 15}`
- `TestDateOnly_UnmarshalJSON_Invalid` — non-date string returns error

#### 2b. `internal/push/vapid_test.go` — new file

Currently `vapid.go` is at 0%.

```
internal/push/vapid_test.go
```

Tests:
- `TestGenerateVAPIDKeys` — call `GenerateVAPIDKeys()`; both strings non-empty; both are valid base64url
- `TestNewVAPIDSigner_RoundTrip` — generate keys, create signer, call `PublicKeyBase64()` matches pubB64
- `TestNewVAPIDSigner_InvalidKey` — bad base64 returns error
- `TestSignJWT` — signer produces a non-empty JWT string; JWT has three `.`-separated parts; the middle part base64-decodes to JSON containing `"sub"` and `"exp"` claims
- `TestEndpointOrigin` (via `SignJWT` with different endpoints) — endpoint with path returns just origin

#### 2c. `internal/auth/service_test.go` — extend for `SetUserHousehold`

`SetUserHousehold` (line 379) is at 0%. The Postgres store methods are Postgres-only and hard to mock without sqlmock, so skip those for now and focus on the service-level method if it delegates to the store.

Check the function signature. If it just calls `s.store.SetUserHousehold(...)`, add a test that wires a MemoryStore auth store and calls it.

#### 2d. `internal/middleware/ratelimit_test.go` — new file

`ratelimit.go` lines 38 and 60 are at 0%.

Tests:
- `TestRateLimiter_AllowsUnderLimit` — N requests under the limit all return 200
- `TestRateLimiter_BlocksOverLimit` — N+1 requests returns 429 on the last one
- `TestRateLimiter_ResetsAfterWindow` — after window elapses (mock clock or short window), new requests are allowed

Check the `ratelimit.go` constructor signature to wire the middleware correctly.

---

### Tier 3 — Handler tests (medium effort, medium gain)

Handlers currently sit at 25.1% overall. The zero-coverage handlers are:

| File | 0% functions |
|------|-------------|
| `notification.go` | `NewNotificationHandler`, `List`, `MarkRead`, `MarkAllRead`, `Delete` |
| `notification_preferences.go` | `NewNotificationPreferencesHandler`, `Get`, `Update` |
| `preferences.go` | `NewPreferencesHandler`, `Get`, `Update` |
| `push.go` | `NewPushHandler`, `Subscribe`, `Unsubscribe` |
| `stats.go` | all 7 handler functions |
| `chore.go` | `GetChore`, `ListChores`, `UpdateChore`, `DeleteChore`, `RestoreDefault` |
| `household.go` | most functions |
| `log.go` | `CreateLog`, `UpdateLog`, `UndoLog`, `GetDay`, `GetWeek`, `GetHistory` |
| `auth.go` | magic-link, OIDC, password-reset handlers |

For each handler file, create `internal/handlers/<name>_test.go` using `net/http/httptest`. Pattern from the existing handler tests:

```go
func TestHandlerFoo(t *testing.T) {
    svc := /* wire memory stores */
    h := handlers.NewFooHandler(svc)
    
    req := httptest.NewRequest("GET", "/api/foo", nil)
    // add session cookie / CSRF header as needed
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, req)
    
    if rr.Code != http.StatusOK { t.Fatalf(...) }
    // decode body and assert
}
```

For auth-protected handlers, look at how existing handler tests inject a user into the request context (via `middleware.WithUser` or equivalent — check existing tests).

Priority order for handler tests:
1. `notification_preferences.go` + `preferences.go` — simple GET/PATCH, no complex dependencies
2. `notification.go` — straightforward CRUD with MemoryStore
3. `stats.go` — wire stats.Service with log/chore MemoryStores; seed a few logs
4. `chore.go` missing functions
5. `log.go` missing functions
6. `push.go` — stub the push store

---

## Part 2: E2E Test Time Reduction

### 2.1 Enable parallel execution in CI

**File:** `playwright.config.js`

Change:
```js
workers: process.env.CI ? 1 : undefined,
```
to:
```js
workers: process.env.CI ? 4 : undefined,
fullyParallel: true,
```

All 27 spec files already use `uniqueEmail()` per test so they are fully isolated — parallel execution is safe. Expected speedup: 3–4x for the spec-level wall time.

**Verify:** Run `make e2e` locally (or with `BASE_URL=... make e2e`) and confirm all tests pass with no flakes before merging.

### 2.2 Replace hardcoded waits with DOM-based waits

Total hardcoded wait time: **~195 seconds**. The top offenders:

| File | Total ms | Primary pattern |
|------|----------|-----------------|
| `schedule.spec.js` | 87,900 | 112 × `waitForTimeout(800)` after drag/drop and modal interactions |
| `validation.spec.js` | 29,800 | waits after form submissions |
| `chores.spec.js` | 19,500 | waits after chore creation/update |
| `log-from-slot.spec.js` | 9,000 | waits after log sheet open |
| `home-grid.spec.js` | 7,650 | waits for grid re-render |

#### Replacement strategy

For each `waitForTimeout(N)` ask: "what DOM change signals that the operation is complete?" then replace with one of:

```js
// Instead of:
await page.waitForTimeout(800);

// Use (pick the right one):
await expect(page.locator('.some-element')).toBeVisible();
await expect(page.locator('.some-element')).toBeHidden();
await expect(page.locator('.some-element')).toHaveCount(N);
await expect(page.locator('.some-element')).toHaveText('expected');
await page.waitForSelector('.some-element');
await page.waitForFunction(() => document.querySelector('.some-element') !== null);
```

#### Specific patterns in `schedule.spec.js`

Most `waitForTimeout(800)` calls follow a drag-and-drop (`dragTo`) or a button click that triggers a sheet to open/close. Replace each with:

- After drag: `await expect(page.locator('.schedule-slot[data-chore-id="..."]')).toBeVisible()`
- After opening a sheet: `await expect(page.locator('.sheet')).toBeVisible()`
- After closing a sheet: `await expect(page.locator('.sheet')).toBeHidden()`
- After saving: `await expect(page.locator('.toast, .success-indicator')).toBeVisible()` (check what the UI actually shows)

The `longPress` helper adds a fixed 650 ms `mousedown` hold — this is inherent to the long-press gesture threshold (500 ms). Do **not** reduce this; the app will not fire the long-press event.

#### Approach for the agent executing these changes

1. Open one spec file at a time.
2. For each `waitForTimeout` call, read the surrounding 5 lines to understand what just happened.
3. Identify the observable DOM change.
4. Replace the wait with the appropriate `expect(...).toBeVisible/Hidden/toHaveCount()` call.
5. Run `make e2e` with the file in isolation (`npx playwright test tests/e2e/schedule.spec.js`) to confirm no flakes.
6. Commit after each file.

**Do not** delete `waitForTimeout` calls that are genuinely testing timing behavior (e.g., verifying that a notification does NOT appear within 500 ms). Those are intentional.

### 2.3 CI worker count change

**File:** `.github/workflows/ci.yaml`

Find the E2E step. If it passes `--workers=1` explicitly, remove that flag. If the parallelism is only controlled via `playwright.config.js`, the config change in 2.1 is sufficient.

---

## Part 3: Estimated Coverage Impact

| Work item | Estimated gain |
|-----------|---------------|
| Tier 1a: chore service + memory store | +4–5% |
| Tier 1b: log service + memory store | +6–8% |
| Tier 1c: household service + memory store | +6–8% |
| Tier 1d: stats service | +5–7% |
| Tier 1e: notification service + memory store | +3–4% |
| Tier 2: schedule DateOnly, VAPID, ratelimit | +2–3% |
| Tier 3: handler tests | +10–15% |
| **Total estimated** | **+36–50%** → **~70–84%** |

The Tier 1 and 2 items alone should reach ~65–70%. Tier 3 handler tests push past 80%.

---

## Execution Order

1. Tier 1 in sequence (1a → 1b → 1c → 1d → 1e) — each can be done independently.
2. Tier 2 — small additions, do after Tier 1 to confirm coverage delta is tracking.
3. Tier 3 — handler tests; start with `notification_preferences.go` and `preferences.go` (least dependencies).
4. E2E parallelism (`playwright.config.js` change) — low risk, high reward; do early.
5. E2E `waitForTimeout` replacements — tackle spec files in descending order of total wait time.

After completing each tier, run `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | grep total` to track progress.
