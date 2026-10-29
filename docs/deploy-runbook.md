# Deploy & CI Babysitting Runbook

> Extracted from `AGENTS.md` for progressive disclosure — read this when cutting a release tag, watching the deploy pipeline, or verifying production.

## Deploy trigger

Push a `v*` tag on `main` (e.g. `git tag v0.1.7 && git push origin v0.1.7`). CI builds, tests, scans/signs the image, deploys via SSH + Cloudflare Tunnel, and creates a GitHub release. The deploy job verifies the tagged commit is reachable from `origin/main` before proceeding — **never tag on a branch.**

Production URL: `https://nabu-app.com`. Production has no test account: the signed-in check in §3 signs up a throwaway account and deletes it.

When the operator dashboard is enabled, set the GitHub Actions secret
`OPERATOR_OWNER_USER_ID` to the positive database user ID of the intended,
verified operator account before cutting a release tag. The deploy job writes
it to `/opt/nabu/.env`, and production Compose passes it to the app. An unset
value leaves `/operator` disabled. This is an account ID, not an API key; check
the account it identifies before setting the secret. The tag-triggered deploy
passes the value to the app.

Changing or clearing the GitHub secret does not change the running app. Cut a
tag on `main`, wait for a successful deploy and restart, then confirm `/ready`
returns 200 before treating the new owner setting as active. A failed rollout
can restore the previous environment and keep the former owner authorized.
After rotation, use an authorized test path to confirm the new owner can read
`/api/operator/v1/summary`, while the former owner's session is denied (403)
and each former-owner bearer key is denied (401). After clearing the setting,
confirm the operator route is unavailable. Do not print keys in deploy logs.

Disable and key revocation are separate operations. While the route is disabled,
keys cannot reach it, but re-enabling the same owner can make an unexpired,
unrevoked key usable again if its auth version still matches. Revoke keys from
the owner dashboard before rotation, or have a database administrator set
`operator_api_keys.revoked_at` for that owner's active keys. Verify denial after
the successful rollout; do not treat a failed deploy as completed rotation or
revocation.

## 1. Watch the CI run

After pushing a `v*` tag, monitor the pipeline to completion and verify production. Do not wait for the user to ask. (A cheaper subagent may be delegated to this.)

```bash
# Find the run ID for the tag
gh run list --limit 5

# Stream logs until the run completes (blocks until done)
gh run watch <run-id>

# If a job fails, check which step failed
gh run view <run-id> --json jobs \
  --jq '.jobs[] | {name: .name, conclusion: .conclusion, steps: [.steps[] | select(.conclusion == "failure") | .name]}'

# Re-run only failed jobs (for transient infra errors)
gh run rerun <run-id> --failed
```

## 2. Distinguish transient vs. real failures

- If **only** the checkout/setup step failed and all test jobs passed → transient GitHub Actions infra error → re-run with `gh run rerun <run-id> --failed`.
- If a test job (Go Tests, JS Tests, E2E, Lint, iOS Unit Tests) failed → real failure → read the full log, fix the code, commit, re-tag, and push a new `v*` tag.

## 3. Verify production after deploy

Once the `Deploy to Production` job goes green:

```bash
# Confirm liveness and dependency readiness
curl -sS -o /dev/null -w "%{http_code}\n" https://nabu-app.com/ready    # expect 200
curl -sS -o /dev/null -w "%{http_code}\n" https://nabu-app.com/health   # expect 200

# Confirm versioned imports carry the new tag
curl -s https://nabu-app.com/static/js/calendar.js | grep "^import"
# Expected: import { ... } from "./utils.js?v=0.1.X";

# Confirm cache headers — must be no-store / BYPASS, NOT max-age / HIT
curl -sI https://nabu-app.com/static/js/app.js | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: BYPASS

# Confirm correct version in the app shell — anonymous GET / now serves the
# marketing page, so use a SPA route like /login for the shell
curl -s https://nabu-app.com/login | grep 'app.js'
# Expected: src="/static/js/app.js?v=0.1.X"

# Confirm the stylesheet URL in the shell carries the same release version.
# An unversioned URL can reuse old service-worker CSS on the first reload,
# even when the server and CDN return current assets with no-store headers.
curl -s https://nabu-app.com/login | grep 'app.css'
# Expected: href="/static/css/app.css?v=0.1.X"

# Confirm the anonymous root serves the server-rendered marketing page with a
# canonical URL pointing at the root
curl -s https://nabu-app.com/ | grep 'rel="canonical"'
# Expected: <link rel="canonical" href="https://nabu-app.com/">

# HTML cache headers must also be no-store (Cloudflare reports DYNAMIC —
# its passthrough marker for uncacheable responses — rather than BYPASS)
curl -sI https://nabu-app.com/ | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: DYNAMIC
```

Then check the signed-in paths with a throwaway account:

```bash
bash scripts/prod-smoke.sh
# Expected: an "ok" line for each step, then OK
```

The script signs up a new account, sets up a household with the default activities, reads the main endpoints, logs an activity and deletes the log, signs out and back in, and deletes the account. The address is in the reserved `.invalid` domain, which the server never sends mail to (`mail.SkipInvalid`), so a run sends no email and uses none of the Resend quota. `SMOKE_EMAIL` chooses another address, but one outside `.invalid` gets a real verification email. If a step fails, the script still deletes the account, and if that fails too it prints the address to delete by hand. Each run uses a new address, so an account left behind never blocks the next run.

### Verify per-IP rate limiting (after the trusted-proxy deploy, once only)

With `TRUSTED_PROXY_CIDRS` set, the auth limiter must key on real client IPs — not one shared tunnel bucket:

```bash
# With the default auth budget, 33 requests yield 30 allowed, then 429s.
# Use GET to exercise the limiter without performing password hashing.
for i in $(seq 1 33); do
  curl -s -o /dev/null -w "%{http_code}\n" \
    https://nabu-app.com/api/auth/login
done
# Expected: 405 x30, then 429 x3 (with Retry-After on the 429s)

# From a second client on a different network (e.g. phone on LTE), a login
# GET must still return 405 while the first client is limited — that
# proves per-IP bucketing rather than a sitewide bucket.
```

Also spot-check request logs: the hashed `client` attribute should now vary between visitors instead of being one tunnel IP for everyone.

### Troubleshooting

- If `cf-cache-status` is `HIT` or `MISS` (not `BYPASS` for JS / `DYNAMIC` for HTML), the `no-store` header is not reaching Cloudflare — investigate `internal/app/server.go` and the CI build logs.
- If imports still show the old version number, the binary was not rebuilt with the new tag — check that `internal/version/version.go` is populated at build time via `-ldflags`.

## iOS releases

The iOS app is released separately, from **Actions → iOS Release → Run workflow** on `main`, after the web app. The workflow stops unless production already runs the server code on `main` (it reads the version from `/health`), so deploy the web app first when the server changed. The steps and the App Store Connect key it needs are under "Releasing" in `ios/README.md`.

### iOS tests and web releases

A web release (`v*` tag) runs the iOS unit tests only when it changes the API
contract (`api/contract`, the server's answers the iOS app decodes; see "The
API contract" in `AGENTS.md`). The Detect Changes job's step "Compare the API
contract with production's" (`scripts/api-contract-changed.sh`) reads the
version from `/health`, diffs `api/contract` against that release's tag and
lists the answers that changed. When they differ, or production's release
can't be found, the `ios` job runs and `scan-and-push` waits for it: the
image is pushed, released and deployed only after `NabuTests` passes. A
release that leaves the contract alone skips the job and doesn't wait.

If the iOS job fails on a release, nothing was pushed or deployed. A
transient failure (a simulator that didn't boot) can be re-run with "Re-run
failed jobs", which runs the image jobs that were waiting on it as well; for a
real one, fix the app or the answer on `main` and tag again. `NabuTests` also
runs on every pull request that changes `ios/**` or `api/contract/**`, and the
iOS Release workflow runs it before an iOS release.

## Readiness failure and application rollback

`/health` reports process liveness. `/ready` probes the application database pool
with a one-second deadline and returns 503 on an unavailable or exhausted pool,
with `Retry-After: 1`, `Cache-Control: no-store` and a correlation ID. Recovery
returns it to 200. Production cannot use the in-memory development fallback.
Migration failure prevents the HTTP listener from starting.

The container health check uses `/ready`. CI copies `compose.next.yaml` and saves
the previous restricted environment before running `scripts/deploy-local.sh`.
The script bounds image pull (300s), container startup (120s), and readiness (90s). Compose commands receive SIGKILL after a further five-second grace if they ignore SIGTERM.
It keeps the previous compose digest, avoids taking the database down, and tries
the previous application/environment if readiness fails. A successful rollback
still exits 1 so the deployment fails visibly; failed rollback exits 2.

For a failed rollout, inspect container state and sanitized application diagnostics
using the response request ID. Check database reachability and pool waits before
retrying. If the previous app cannot start, stop further deployment attempts and
escalate to the incident owner. Do not restore a backup over the live database.
Database/schema rollback is not automatic: migrations must remain compatible with
the preceding app for this application rollback to work. Incompatible changes need
a reviewed maintenance/cutover plan before release. Keep `.env.previous` mode 600;
remove obsolete secret snapshots after the rollout/rollback window closes.

Rate limits are per server replica. Verify trusted-client attribution and IPv4/IPv6
behavior using authorized test traffic before changing replica count or proxy
topology. A local proxy-chain test does not establish production network behavior.

### Statement and lock timeouts

Every app connection starts with `statement_timeout` 30s and `lock_timeout` 15s
(`internal/database/open.go`). A runaway query or a stuck lock wait fails its
own request with a server error instead of holding a pooled connection while
others queue behind it. The lock timeout stays above the 10s one push send
may hold its recipient's membership and its device's registration and session,
so a removal or sign-out waiting behind a send still succeeds. Each send is
guarded on its own (plan 5.2), so no wait is longer than one send. Migrations
run without these, as before: each migration's
transaction lifts both, and a migration may still set its own (048, 049 and 051
do). The migration lock wait is not bounded either, so an instance waits for
another instance's migration however long it takes.

To change them, add `statement_timeout=<ms>` and `lock_timeout=<ms>` to the
`DATABASE_URL` query string; values there win over the defaults.

### History index migration

Migration 048 adds a household/chore/time index and a `pg_trgm` GIN index for
literal note/title search. It uses the existing extension namespace, or installs
the trusted extension in `public` (database `CREATE` privilege required). Startup
index construction has a five-second lock timeout and a sixty-second statement
timeout. A timeout fails startup and uses the normal deployment rollback path.

Before a deployment to a database where construction would exceed that window,
prebuild the exact index names with `CREATE INDEX CONCURRENTLY` using an
administrative maintenance connection while the old app remains available, then
confirm `pg_index.indisvalid` for both. The migration skips existing names. An
invalid index left by a failed concurrent build must be dropped/rebuilt before
deployment; it is not evidence of a completed migration. Never run concurrent
index creation inside a transaction. This preparation is a separate authorized
maintenance operation; no production index work was performed during this review.

Migration 049 follows the same optional prebuild procedure for large notification
histories. Run each statement separately, outside a transaction, with the existing
maintenance role and the intended database/schema selected:

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_user_created
    ON notifications (user_id, created_at DESC, id DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_unread
    ON notifications (user_id) WHERE is_read = false;
```

Verify both definitions with `pg_get_indexdef` and `pg_index.indisvalid` before
starting the new app. An interrupted concurrent build can leave an invalid index
that `IF NOT EXISTS` cannot repair: drop that invalid index concurrently and rebuild
it before retrying. Startup migration rejects invalid prebuilds and times out a
contended index build after 5 seconds waiting for locks / 60 seconds total.

Migration 058 indexes the foreign keys that had none and drops three indexes
that duplicated UNIQUE constraints. Its tables are small; on an installation
where they are not (most likely `sessions`), prebuild the same way, one
statement at a time:

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_households_household ON user_households (household_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_user ON sessions (user_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_auth_tokens_user ON auth_tokens (user_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_auth_mail_outbox_user ON auth_mail_outbox (user_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_schedule_reminders_user ON schedule_reminders (user_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_active_household ON users (active_household_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chores_created_by ON chores (created_by);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chore_schedules_assignee ON chore_schedules (assigned_to_user_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_invites_created_by ON invites (created_by);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chore_reminder_prefs_chore ON chore_reminder_prefs (chore_id);
```

`internal/database/schema_indexes_test.go` fails when a foreign key has no
index leading with its columns, or when two indexes repeat each other, so a
new foreign key needs its index in the same migration.

Migration 059 adds the notification work tables (`notification_events`,
`push_jobs`), which start empty, and a `dedupe_key` column on `notifications`
with a unique index over the rows that have one. Building that index scans
`notifications` while holding off writes to it. On an installation where the
table is large, add the column and prebuild the index first, one statement at
a time:

```sql
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS dedupe_key TEXT;
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_dedupe ON notifications (dedupe_key) WHERE dedupe_key IS NOT NULL;
```

The previous app runs on the new schema, as rollback needs: it ignores the new
tables and column. It doesn't send work queued before a rollback; a later
deploy of this app drops what expired in the meantime (15 minutes after it was
queued) and sends the rest.

Database recovery and major-version changes use the separate [recovery runbook](recovery-runbook.md). Application deployments copy recovery tools but never apply the prepared PostgreSQL definition, replace `/etc/nabu` recovery secrets, or enable WAL timers automatically. Verify a recent isolated restore and working alert/deadman receiver before adopting that configuration.
