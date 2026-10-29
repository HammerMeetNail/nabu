# Running two app instances

Production runs one app instance. The code is ready for more than one that share a database; this page lists what that needs, for when one instance stops being enough. Nothing here is deployed.

## What already works across instances

Everything that must hold across instances coordinates through PostgreSQL:

| What | How |
|---|---|
| Sessions, accounts, all household data | PostgreSQL |
| Sign-in (`/api/auth`) and join (`/api/household/join`) rate limits | `rate_limits`, counted per hashed client IP (`middleware.PostgresCounter`) |
| One export per household at a time | A session advisory lock per household (`handlers.PostgresHouseholdLock`) |
| Reminder ticks | One leader at a time (`reminder.PostgresAdvisoryLock`) |
| Notification fan-out, push delivery, sign-in mail | Rows claimed with `FOR UPDATE SKIP LOCKED` under a lease |
| Sweeps of ended sessions and spent tokens | A transaction-scoped advisory lock, `SKIP LOCKED` rows |
| Migrations | Applied at startup under an advisory lock, so only one instance migrates |

## What stays per instance

These bound one instance's load, so each instance keeps its own:

- **The global API backstop** (`RATE_LIMIT_GLOBAL_MAX`). Behind a load balancer that spreads one client over N instances, that client gets up to N times the limit.
- **The operator API limit.**
- **The two concurrent export slots**, which bound each instance's memory.
- **Caches:** OIDC and APNs keys, metrics, and the versioned JavaScript. Every instance builds the same versioned JavaScript from the same image.

## What a second instance needs

1. **One PostgreSQL that every instance reaches.** Managed Postgres, with backups and point-in-time recovery handled, is the plan before a paid launch. Give each instance enough pool for HTTP plus its 8 delivery connections (`DB_MAX_OPEN_CONNS`).
2. **The same image and configuration on every instance.**
   - Roll a release out to all of them, and keep each migration compatible with the previous release while both versions run.
   - Every instance needs the same `APP_BASE_URL`, `SMTP_*`, `VAPID_*` and `APNS_*`.
3. **A load balancer or proxy in front of the instances.** Set `TRUSTED_PROXY_CIDRS` on each instance to the proxy's addresses, so rate limits key on real client IPs.
4. **`/ready` as the health check.** An instance answers it once its database is reachable.

Keep the rule in AGENTS.md: new state that must hold across instances goes in PostgreSQL, never in a process's memory.
