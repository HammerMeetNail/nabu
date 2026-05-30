# Multi-Household Support

Allow users to join multiple households and switch between them. Currently the data model enforces one user = one household (`users.household_id` is a single nullable FK).

## Current State

- `users` table has `household_id` (nullable FK to `households.id`) and `role`
- Middleware attaches a single household to the request context
- All household-scoped data (chores, logs, schedules, members) is loaded once at login
- The profile bottom sheet (v0.1.161) has a placeholder for household switching

## Database Changes

### New table: `user_households`

```sql
CREATE TABLE user_households (
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    role         TEXT NOT NULL DEFAULT 'member',  -- owner | admin | member
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, household_id)
);
```

### New column on `users`

```sql
ALTER TABLE users ADD COLUMN active_household_id BIGINT REFERENCES households(id);
```

The `active_household_id` tracks which household the user is currently viewing. Queried once at session start and cached in the session/state.

### Migration plan

1. Create `user_households` table
2. Backfill from existing `users.household_id` / `users.role`
3. Set `users.active_household_id` from `users.household_id`
4. Keep `users.household_id` / `users.role` for a transition period, then drop

## Backend Changes

### New API endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/households` | List all households the user belongs to |
| `POST` | `/api/households/:id/activate` | Switch active household |
| `GET` | `/api/household` | Get current household (existing, update to use active_household_id) |

### Service layer

- `household.Service.ListUserHouseholds(ctx, userID)` — returns all households with role
- `household.Service.SwitchHousehold(ctx, userID, householdID)` — sets active_household_id
- `household.Service.JoinHousehold(ctx, userID, inviteCode)` — inserts into user_households instead of updating users.household_id

### Middleware

- `RequireHousehold` checks `active_household_id` instead of `household_id`
- Session middleware loads `active_household_id` from the users table on each request (or cache it in the session cookie)

### Join flow changes

- A user who already has a household can still join another one (via invite)
- New members get role `member` in the joined household
- The `active_household_id` is set to the newly joined household

## Frontend Changes

### State

Add to `createAppState()`:

```js
userHouseholds: [],        // [{ id, name, role }]
activeHouseholdId: null,
```

### Profile sheet

The profile bottom sheet gains a household switcher section:

```
┌──────────────────────────┐
│  E  user@email.com       │
│     Current: Smith Home  │
├──────────────────────────┤
│  ○ Smith Home (owner)    │  ← tap to switch
│  ○ Jones Home (member)   │
│  + Join another household │
├──────────────────────────┤
│  ⚙ Settings              │
│  ⇱ Sign Out              │
└──────────────────────────┘
```

### Switching households

When the user switches:
1. `POST /api/households/:id/activate`
2. Reload all household-scoped data: chores, schedules, members, invites, today's logs
3. Re-render current view
4. Show toast: "Switched to [household name]"

### Chore creation / scheduling

All chore CRUD operates on the currently active household. The API handlers already scope by household ID, so no UI changes needed for basic operations — just ensure the correct `active_household_id` is sent.

### Settings

- "Leave Household" removes the user from `user_households` for the current household
- If it's the last household, behave as today (no household state)
- "Create Household" sets the new household as active

## Invariants to Maintain

1. A user must always have at least one household, or be able to function with none (join/create state)
2. Switching households must clear cached household-scoped data to avoid cross-contamination
3. Notification preferences are per-user (they already are), so they follow across households
4. Push subscriptions are per-user and per-household context should be handled carefully
5. The VAPID/WebPush subscription should remain valid regardless of active household

## Edge Cases

- **Last member leaves**: If the last member leaves a household, the household is orphaned. Consider auto-deleting or flagging.
- **Owner leaves**: Transfer ownership or prevent leaving without transfer.
- **Concurrent switches**: Two tabs switching households simultaneously — last write wins for `active_household_id`.
- **Invite to already-joined household**: Should return a friendly error, not a DB constraint violation.
- **Deleted household**: Clean up `user_households` rows and reset `active_household_id` if it pointed to the deleted household.

## Implementation Order

1. Database migration (new table + backfill + active_household_id)
2. Backend: new endpoints, updated service/store layer
3. Frontend: state + profile sheet switcher
4. E2E tests for join, switch, leave across multiple households
5. Remove old `users.household_id` column (after verifying no regressions)
