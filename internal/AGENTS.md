# Backend instructions

Read the [root guide](../AGENTS.md) first. This guide covers `internal/`; also read it for changes in `cmd/` or `migrations/`.

## Execution paths

- [app/server.go](app/server.go) wires dependencies and registers routes on `http.ServeMux`. Follow the existing `method()` and `RequireAuth` patterns. Keep HTTP handling in `handlers/`, business rules in domain services, and persistence in stores.
- Services/stores use dependency injection. Most domains have memory and Postgres implementations; update both when their contract changes. In-memory storage is a development mode, not a production fallback.
- [database/migrate.go](database/migrate.go) applies embedded SQL from `migrations/`, explicitly sorts names lexicographically, and records each migration in a transaction. A Postgres session advisory lock serializes concurrent migration runners. Use the existing filename convention and inspect the newest migrations before choosing a name.
- [config/config.go](config/config.go) defines environment defaults and validation. Production requires `DATABASE_URL`; new required production settings must fail fast instead of silently falling back.
- For middleware changes, inspect the wrapping order in `app/server.go`: request entry traverses the last wrapper first. Do not confuse the order of assignment with execution order.

Use `rg` to find every caller, SQL query/Scan list, SQL mock expectation, JSON model, and test fixture affected by a signature or schema change. Shared API changes also require inspecting the PWA and iOS request/response models, even if only Go files change.

## Persistence and background work

- **Memory stores are concurrent.** The reminder scheduler and HTTP handlers share them even without a database. Guard every map/slice access with `sync.Mutex`/`RWMutex`, following [chore/store_memory.go](chore/store_memory.go). Do not return mutable backing data that callers can race with store operations.
- **Schedulers need ownership and shutdown.** The reminder scheduler ticks only while holding `reminder.PostgresAdvisoryLock` in Postgres mode; single-instance memory mode has no lock. Preserve this guard and the distinct migration lock. Do not add an always-on duplicate scheduler.
- **Goroutines must stop.** Wire long-lived work into the context/cancellation and `Server.Close()` lifecycle in `app/server.go`. `BuildServer` returns a closer; `cmd/server` handles graceful SIGINT/SIGTERM shutdown. Do not start background work with no cancellation/cleanup path.
- **Predefined chores need both paths.** Change `PredefinedChores` in [chore/service.go](chore/service.go) for new households and add an idempotent migration for existing ones. Key updates on `predefined_key`; use `WHERE NOT EXISTS` for additions or `UPDATE` for field changes. Search tests in both clients for affected default counts and fields.
- Add new migrations rather than editing already applied migrations. Check memory/Postgres behavior, existing data, concurrent startup, and transaction boundaries when altering persistence.

## Authorization and validation

- Handlers obtain the actor from `middleware.CurrentUser(ctx)`. Services must also verify the requested resource belongs to the actor's household; a handler check is not a substitute.
- Resource mutation/deletion must carry the owning household ID through the service contract. Reads by ID, stats, lookups, and exports need equivalent isolation.
- Verify client-supplied `choreId`, `inviteId`, `userId`, `scheduleId`, and other references before using them. For actions involving another member, verify both household memberships and the actor's role. Preserve private-task visibility rules.
- Validate required fields, lengths, colors (`^#[0-9A-Fa-f]{6}$`), free-text control characters, array bounds/item lengths, and cross-field relationships on create and update. UI controls do not replace server validation.
- Return user-facing errors through existing helpers; do not expose SQL, stack traces, or library internals. Log only safe identifiers/fingerprints: follow `hashClientIP`, `endpointHost`, and auth hashed-email logging.

## Authentication, HTTP, and crypto invariants

- Compare security tokens in constant time with `crypto/subtle.ConstantTimeCompare`, including CSRF and OIDC state.
- Session cookie `nabu_session` must be `HttpOnly`, `SameSite=Lax`, and `Secure` when configured for TLS; preserve flags when clearing it on logout. Retain hard expiry and sliding idle expiry.
- CSRF cookie `nabu_csrf` and session `Secure` flags use trusted server configuration. HSTS is enabled by `SERVER_SECURE=true` or direct TLS. Never derive security from the spoofable `X-Forwarded-Proto` header.
- Validate password bounds before hashing: current Go validation uses 8–72 bytes (bcrypt's maximum). New hashes use cost 13; use precomputed `bcrypt.MinCost` hashes for tests that do not exercise hashing policy.
- Preserve the dummy bcrypt comparison on unknown-email login. Password-reset and magic-link responses must not reveal account existence.
- **Accepted exception: registration auto-login.** New registration returns 201, user JSON, and a session; duplicates return 200 with generic inbox guidance. This is a deliberate account-existence oracle. Do not change it under general enumeration hardening; a requested redesign must coordinate register → verify → login in Go, PWA auth, and iOS `AuthStore`.
- OIDC JWT verification must check the RS256 signature against the provider JWKS, reconstruct the RSA key from `n`/`e`, and validate `iss`, `aud`, `exp`, and a required, nonempty matching `nonce`.
- VAPID ES256 signatures use raw 64-byte r∥s with each component padded to 32 bytes; DER encoding is invalid for this path.
- Preserve per-IP auth limiting (`RATE_LIMIT_AUTH_MAX`, default 5/min) and household-join limiting (`RATE_LIMIT_JOIN_MAX`, default 10/min). The global `/api/` backstop (`RATE_LIMIT_GLOBAL_MAX`, default 120/min) is enabled only with `TRUSTED_PROXY_CIDRS`, so a proxy's shared IP cannot rate-limit the entire user base. `429` responses carry `Retry-After`.

For Web Push troubleshooting, read [PUSH_DEBUG.md](../docs/plans/PUSH_DEBUG.md). Apple may return 201 for payloads the device cannot decrypt; gateway acceptance alone does not prove delivery. Use the documented service-worker diagnostics when delivery verification is part of the task.

## Embedded assets and cache busting

`buildVersionedJSCache` in `app/server.go` rewrites relative ES-module imports to `?v=<version>` at startup and serves JS from memory with `Cache-Control: no-store`. Imports already containing `?` are skipped. Never add hard-coded query versions to JS source. New modules are handled automatically.

Before changing this mechanism, read the [README caching rationale](../README.md#js-static-file-serving-and-cache-busting) and [deploy verification](../docs/deploy-runbook.md). Rebuild the owned server after changing embedded assets; editing files underneath an old binary is insufficient.

## Validation

- Start with affected Go packages/tests, then complete the root runtime-code gate. Format Go changes and keep the 80% Go statement-coverage target in mind.
- Store/scheduler/concurrency changes require `go test -race -timeout 600s ./...` and independent review as described in the root guide. Sensitive persistence and security require the security-capable reviewer.
- Authorization fixes need cross-household negative tests at the service/handler layer. Encoding and SQL changes need contract/Scan coverage. Add Playwright coverage when a change affects a browser-visible flow, plus native coverage when shared behavior changes.
- Do not run overlapping Go build, vet, lint, or test invocations. Check final command exits; keep logs and transient coverage artifacts outside the repository where the command permits.
