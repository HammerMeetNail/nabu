# Nabu — Plan to Become a Product People Choose

**Date:** 2026-10-13 · **Baseline:** `main` @ `3547d67` · **Scope:** PWA, iOS app, Go backend, ops, product, go-to-market
**Status:** Proposal. Nothing in this document has been implemented. Every bug listed below was reproduced or confirmed in code; file:line references are against the baseline commit.

---

## 1. Executive summary

Nabu's engineering foundation is stronger than its product surface. The backend is careful (hashed sessions, CSRF, idempotent offline log queue, leader-gated scheduler, request-ownership fencing, about 24k lines of Go tests), and past security audits are almost fully remediated. **What holds Nabu back is that first-run and everyday use feel like an internal tool, not a consumer product.** The native iOS app is better than "unreviewed" suggests (it builds cleanly, uses native idiom, and has about 380 unit tests), but it has launch-blocking defects that tests did not catch.

### What I found, in priority order

| # | Finding | Where | Severity |
|---|---|---|---|
| 1 | **iOS Stats says "Couldn't load" for every new household** (and any household with no logs in the recap week). The server sends `byCategory: null`; iOS decodes it into a non-optional array and throws. | `ios/Nabu/API/Models.swift:547`; server recap | **P0** (launch blocker) |
| 2 | **At large Dynamic Type sizes the iOS log sheet's Log button is off-screen** in the default detent. My automated run's logs never saved. | `ios/Nabu/Views/LogSheet.swift` | **P0** (accessibility, App Review) |
| 3 | **Deleting an account silently deletes every log that person made in shared households** (`chore_logs.user_id ON DELETE CASCADE`). The warning copy on both clients says only sole-member households lose logs. A co-parent deleting their account erases half the baby's history. | `migrations/001_initial.sql:56`; `web/static/js/app.js:730`; `ios/.../HouseholdView.swift:719` | **P0** (data loss) |
| 4 | **Suspended members still receive schedule reminders on Postgres**: the candidate query and `GetMembershipForHousehold` ignore `suspended_at` (the in-memory path checks it). | `internal/reminder/candidates.go`; `internal/household/postgres_store.go:405` | **P1** (authorization) |
| 5 | **Stored HTML injection via chore icon**: icons are interpolated unescaped in 3 render sinks; the server allows any 8 runes. CSP blocks script, but any member can inject markup such as `<style>` that breaks shared views for the whole household. | `web/static/js/today.js:385`, `stats.js:733`, `stats.js:830`; `internal/handlers/chore.go:27` | **P1** (security) |
| 6 | **Every new household gets the same 16 tiles** (Feed Cats, Feed Baby, Clean Litter Box, Walk Dog, Watch Movie…) whatever the household contains. The first screen a new user sees is mostly irrelevant. | `web/static/js/app.js:4382-4403`; `internal/chore/service.go:375-392` | **P1** (activation) |
| 7 | **On iOS no confirmation or undo appears after a new log**: the undo path can only be reached from edit, and it removes `latestLogs` by log ID although the map is keyed by chore ID. | `ios/.../HomeView.swift:62-104`, `LogSheet.swift:238-244` | **P1** (core loop) |
| 8 | **Cannot log offline after a cold launch**: both clients block on a "confirm your session / reconnect" screen even though a durable offline log queue exists. | `web/static/js/browser-context.js`; `ios/.../ContentView.swift:30-38` | **P1** ("always within reach") |
| 9 | **People are shown by email local-part** ("rev1791941045711"). No display-name prompt at sign-up, and no way to change it on either client. | `internal/auth/postgres_store.go:53` (`emailToDisplay`) | **P1** (shared household UX) |
| 10 | **No GDPR groundwork for health data.** Feeds, diapers and medications are very likely "data concerning health" (Art. 9). There is no explicit-consent step, no DPIA, no documented processor agreements, and no stated retention or backup-erasure policy. | onboarding; privacy policy; ops | **P1** (blocks EU marketing) |
| 11 | PWA polish: tabs never change the URL (no back button or deep link per tab), the undo toast covers the next sheet's primary button, the dark-mode bell icon is near-invisible, a `vdev` version label shows in the header, desktop is the phone layout stretched to 1440 px, and first paint waits for all boot API calls (4.5 s to the Home grid on slow 4G cold). | §4.1, §4.3 | P2 |

### The plan in one paragraph

Fix the launch blockers and data-safety bugs (**Phase 0**, about 1–2 weeks). Then make launch **offline-first** on both clients and make the PWA fast, with a real router and a cached data layer (**Phase 1**). Replace "here are 16 random tiles" with **guided household setup**: who lives here (baby, pets, plants, people), names, and an invite (**Phase 2**). Make Nabu reachable without opening it: **interactive widgets, Siri/Shortcuts, Control Center, the Action button, a Live Activity for timers, an Apple Watch app, and a real desktop/Mac layout** (**Phase 3**). Put **GDPR compliance** in place: lawful basis and explicit consent for health data, erasure that protects other members, retention, processor agreements (**Phase 4**). Harden for **horizontal scale and observability** (**Phase 5**) and **launch** with a real App Store presence, an honest marketing site, privacy-respecting analytics, and a free-core / paid-plus model (**Phase 6**).

---

## 2. How this review was done

- **Running app, PWA.** I started an isolated Podman stack (project `nabu-review`, ports 18080/15432/18025) so I would not collide with another project's containers already running on this machine. I drove it with Playwright in WebKit (iPhone 15 profile) and Chromium (desktop 1440×900, dark mode, throttled networks): registered, created a household, logged chores, visited every tab, and measured cold, warm, and offline launches.
- **Running app, iOS.** I built the app (Xcode 27, iOS 27 runtime) on a dedicated simulator (`nabu-review-iphone`), separate from another session's booted simulator. I ran a throwaway XCUITest walkthrough from a scratch worktree (never committed) against the local server at default and `AccessibilityL` text sizes, then traced the failures it surfaced to root cause.
- **Code.** I read the core paths myself and used parallel read-only inventories of the backend, PWA, iOS, and every prior review document. I re-checked every inventory claim that this plan relies on against source (the inventories came from a cheaper model).
- **Market.** I did a brief scan of baby-tracker and chore-app competitors (§3).

Things I did **not** do: test on a physical device, test APNs/Web Push end-to-end, touch production beyond reading two public response headers, or run the full E2E/Go suites (the plan changes no code).

---

## 3. Product thesis

### Who Nabu is for

Households where **several people share responsibility for things that must not be forgotten or double-done**: a baby's feeds, diapers and meds; the cat's food and meds; the plants; the bins. The founding pain is on the marketing page: *"no more 'did anyone feed the cat?' texts."*

### Jobs to be done

1. **"Record that I did it, in under two seconds, often one-handed, often at night."** This is the get-in/get-out moment. Everything else is secondary to it.
2. **"Know at a glance whether it's been done, and when it's next due."** This is glanceable state: on a widget, the lock screen, a watch, a fridge tablet.
3. **"Don't let it slip."** Reminders that respect quiet hours and go to the right person.
4. **"Understand the pattern."** Feeding intervals, who carried the load this week — already covered by the Stats tab; this plan improves its speed and reliability rather than adding new analytics.

### Competitive position

- **Baby trackers** (Huckleberry, Nara, Glow, Baby Tracker) are deep on infant sleep and feeding, offer free multi-caregiver sync, and charge $10–15/month for coaching and predictions ([Huckleberry listing](https://apps.apple.com/us/app/huckleberry-baby-child/id1169136078), [pricing tracker](https://apppricinglab.com/app/apple/1169136078), [Nara mention](https://www.recime.app/blog/?p=2054)). They end when the child grows out of them and ignore the rest of the house.
- **Chore apps** (OurHome, Sweepy, Tody, and a wave of 2026 couple-chore apps such as Chore Wars, Recure and Ryddy) focus on planning and fairness and are weak on fast logging and baby care ([couples roundup](https://www.yougot.ai/blog/reminders/general-reminders/what-is-the-best-app-for-couples-to-share-tasks), [Chore Wars](https://apps.apple.com/us/app/id6759489229), [Ryddy](https://mwm.ai/apps/ryddy/6765777772)). A recurring complaint is that one partner stops logging after two weeks, so **friction kills these apps**.
- **Nabu's wedge:** *one shared, glanceable log for everything a household keeps alive* (baby, pets, plants, home) that is faster to use than texting. It stays useful after the baby phase because the same habit carries over to pets, meds and chores. Differentiators to protect: **speed, cross-domain coverage, true multi-caregiver attribution, free core, no ads or trackers, web + native**.

### Positioning statement (draft)

> **Nabu — the shared memory for your home.** Log the feed, the meds, the walk, the watering, and everyone in the house sees it instantly. No more "did anyone…?" texts.

---

## 4. Detailed findings

### 4.1 PWA — first run and everyday use (observed)

| Area | What happens | Evidence |
|---|---|---|
| Sign-up | Blank card titled "Create Account": no logo, no value statement, no password rules shown, and a confirm-password field adds friction. Social buttons only appear when configured. | screenshot `m_register` |
| Post-sign-up | Greets the user by **full email address**. Shows the Log/Manage toggle and all five tabs before a household exists. The URL stays `/register`. | `o1` |
| Household setup | "Set Up Household" opens **Settings**, so the new user sees export, change password, delete account, mL/oz and notification toggles before creating their first household. | `o2` |
| First Home | 16 seeded tiles regardless of household (§1 #6). "never" captions in very low contrast. | `o3` |
| Logging | A tap opens a sheet with When, three Follow-up selects (0d/0h/0m) and Note. Each save fires 1 POST + 3 refetches (`/logs/today`, `/schedules`, `/logs/latest-per-chore`). | `l1`; network trace |
| Undo toast | Persists into the next sheet and **covers its primary Log button**; also floats over every tab for several seconds. | `l3` |
| Feed Baby sheet | Amount starts empty, and breast-feeding has no side or duration fields. These are standard in baby trackers. | `l3` |
| Activity | Rows show the email-derived handle. Header "Oct 13 – Oct 19" while the list shows only today. | `t_Activity` |
| Stats | 15 API requests on every visit to the tab (10 fixed + per-chore series), no client cache. | network trace |
| Navigation | Tabs never `pushState`: the address bar stays `/`, Back doesn't move between tabs, reload loses position, and there is no `aria-current`. | `app.js:1714-1781` |
| Dark mode | Bell icon renders black on the dark header, so it is near-invisible. | `dark_home` |
| Desktop | The phone layout's 480 px cap is removed by an `@media (orientation: landscape)` rule (`app.css:4474`), and every desktop window is landscape, so Home stretches tiles across 1440 px with a phone bottom-tab bar and tiny type. There is no desktop layout. | `d_home` |
| Marketing page | Screenshots are raw captures of a real household (pet names, "Clean Puke", 33 unread notifications, version string), and the "Know what's coming up next" frame shows a nearly empty Schedule. | `mkt_crop` |
| Header | `vdev` / `v0.1.348` version label permanently visible next to the logo. | all screenshots |

### 4.2 iOS app (observed and traced)

**Good:** it builds cleanly with zero dependencies in the app target. It uses native structure (tab bar, sheets, Swift Charts, SF Symbols, Liquid Glass chrome on iOS 27). It has about 380 unit/contract tests, snapshot tests, a widget, Sign in with Apple, APNs plumbing, and an idempotent on-disk offline queue. It already feels more native than the PWA.

**Defects:**

1. **Stats fails for empty recap windows** (§1 #1). It reproduced 2/2 at the large text size. The same model also shows the "Log a chore to start your stats" empty state at the same time, because `overview == nil` is treated as an error (`StatsView.swift:29-37`). Fix both sides: the server should never emit `null` for arrays, and iOS should use `decodeIfPresent ?? []`, as `ios/AGENTS.md` already requires.
2. **Log button below the fold** at the default detent for every sheet with more than When/Follow-up; at AX sizes it is unreachable without dragging the sheet up and scrolling (§1 #2). The amount field for Feed Baby is also hidden at the default detent.
3. **No feedback after a new log**: there is no toast, no undo and no haptic. The `.sensoryFeedback(.success, trigger: undoLogId)` never fires, because `undoLogId` is only set from the edit-mode "Remove log" button (`HomeView.swift:68-77`, `LogSheet.swift:238-244`). The undo handler removes `latestLogs[logId]` from a map keyed by chore ID (`HomeView.swift:91`) and swallows errors.
4. **Large-text layout**: Stats text is clipped at the left edge, overview cards are cut off, and 19 hard-coded `.font(.system(size:))` calls don't scale (as small as 8–9 pt in charts).
5. **Settings** shows the raw join URL (`http://…/join?code=…`) as the household subtitle, and member rows show email handles.
6. **Sign-up screen**: the beige background stops about two-thirds down the screen and white shows below.
7. **Cold launch offline** shows "Confirming your session / Reconnect" with no cached data, no Home and no way to log (§1 #8).
8. **Foreground refresh** doesn't reload today's logs or latest-per-chore, so "time ago" and the widget cache go stale (`DataLoader.swift:105-126`).
9. **Engineering:**
   - Swift 5 mode with minimal concurrency checking, one ~50-property `ObservableObject` god-object (`AppState`), and 43 silently swallowed errors.
   - No String Catalog, no `#Preview` blocks, no os.Logger, no crash reporting.
   - `Info.plist` hard-codes version 1.0 (1).
   - The README describes a `Package.swift` and folder layout that don't exist.
   - Test hooks (`-seedHomeForUITest`, `-resetState`) are compiled into release builds.
   - The CI lane runs on `macos-15`, where snapshot tests self-skip (they require iOS 26.x).
10. **Absent platform surfaces:** App Intents/Siri/Shortcuts, Control Center controls, interactive widgets, lock-screen widgets, Live Activity, Apple Watch, Spotlight, Mac.

### 4.3 Performance (measured locally, Chromium iPhone profile)

| Scenario | Home grid visible | Requests | Notes |
|---|---|---|---|
| Cold, fast network | 225 ms | 41 (31 JS, 8 API) | 605 KB uncompressed at the origin |
| Cold, slow 4G (150 ms RTT, 1.6 Mbps) | **4.5 s** | 41 | ES-module waterfall, then serial boot API phases, then first paint |
| Cold, 3G | **15.2 s** | 41 | |
| Warm (service worker), slow 4G | 854 ms | 33 from SW, 8 network | Good once installed |
| Offline relaunch | blocked | — | "Check your session / Reconnect" |

Production is served through Cloudflare with Brotli (`content-encoding: br` verified on `/static/js/app.js`), so origin compression is not urgent. The structural costs are:

1. **Import waterfall.** About 31 modules discovered level by level, with no `modulepreload`.
2. **`#app` stays empty until the last boot call returns.** The sequence is `/api/me` → a `Promise.all` of household/prefs/chores/latest → timezone PATCH → route data → first render (`app.js:1597-4213`). There is no skeleton and no cached shell.
3. **No client data cache.** Every tab visit and every resume refetches everything, and Stats fans out to 15 requests.
4. **Unkeyed full re-renders.** 126 `render(app)` call sites rebuild the whole view string and morph it positionally. Inputs only survive while focused, and there are two documented hacks for this.
5. **The SW precache lists bare URLs** (`service-worker.js:7-16`) while the page requests `?v=` URLs, so precached JS is never hit. Only runtime caching works.

Backend hot paths are in good shape per `docs/performance-workload.md` (latest-per-chore 0.93 ms and search 0.75 ms at 500k logs). Remaining items:

- `GET /api/household` is about 12–14 queries plus an **N+1** loop over former members (`internal/household/service.go:216-229`).
- Every authenticated request does 2–3 session queries, including HTML page loads.
- The PWA polls notifications every 30 s and household every 5 min.

### 4.4 Security and data integrity

Prior audits are largely fixed (see the status audit summarized in Appendix B). New or still-open items:

| Item | Detail | Severity |
|---|---|---|
| Account deletion cascades shared-household logs | §1 #3. Recommend keeping the household's records under an unlinkable "Former member" tombstone, with opt-in deletion of the user's own entries and notes, plus accurate copy. Design in Phase 0.3; GDPR reasoning in Phase 4. | P0 |
| Suspended members get reminders (Postgres) | §1 #4. Add the `suspended_at IS NULL` filter in the candidate query and in `GetMembershipForHousehold`/`WithMember`, plus a cross-mode regression test. | P1 |
| Chore icon HTML injection | §1 #5. Escape all three sinks, restrict icons server-side to emoji/grapheme clusters, and add XSS render tests for each sink. | P1 |
| Middleware order | `RequestLogger` and `SecurityHeaders` are innermost (`internal/app/server.go:717-732`), so 403 (CSRF), 409 (context changed) and 429 responses carry **no security headers, no `X-Request-ID` and no log line**. Move both outermost. | P2 |
| Request log hashes the proxy IP | `requestlog.go:63` uses `r.RemoteAddr`, not the trusted-proxy client IP, so behind Cloudflare every request hashes the same. | P2 |
| Client cancellations logged as 500 | Observed: `error_class=canceled` → status 500. This pollutes error rates and will cause false alerts. Map to 499 and exclude from SLOs. | P2 |
| Notification work dropped silently | The in-process queue (2 workers, 64 slots) drops with a log line when full and loses everything on restart (`internal/background/queue.go:44`). Move to a durable Postgres outbox like mail. | P2 |
| Push send inside a DB transaction | `WithMember` holds a transaction and an advisory lock across the push HTTP call (`internal/household/delivery.go:35-53`). The reminder is recorded after sending, so a crash duplicates it. | P2 |
| PR #141 (quiet hours) | Unmerged, 32 commits behind, and its migration `052` now collides with `052_operator_api_keys.sql`. Decide: rebase and fix review F1–F6, or close. | P2 |
| Deploy heredoc injection | Unquoted SSH heredoc in `ci.yaml` (~647) can be broken out of by secrets containing newlines. | P2 |
| Remaining low items | 4xx responses echo raw errors (UndoLog, chore create/update); APNs token upsert reassigns tokens across users; no re-auth for account deletion; store-level `GetChore` is not household-scoped; CSP `style-src 'unsafe-inline'`; no invite-code rotation; release iOS builds honor test launch args. | P3 |

### 4.5 Scalability and operations

Ready today: sessions in Postgres (any replica can serve), leader-gated reminder scheduler, mail outbox with `SKIP LOCKED`, pool budgets, request IDs, recovery runbook.

Gaps before multi-replica or serious traffic:

- **Per-process state:** rate limiters (N replicas give N× the limit), the export gate, the notification queue.
- **No metrics endpoint, tracing or error reporting.** Pool and query stats are only logged. The standard-library `log` prefixes a timestamp before the JSON payload, which breaks JSON log ingestion.
- **No sweeps** for expired sessions, auth tokens, or old `chore_log_effects`.
- **Missing indexes:**
  - `sessions(user_id)`
  - `user_households(household_id)`: `GetMembers` and the reminder join filter by household, but the primary key leads with `user_id`.
  - `chore_reminder_prefs(chore_id)`
  - FK columns that `DeleteUser` touches
- **Duplicate indexes:** three indexes duplicate UNIQUE constraints.
- **Production recovery adoption (WAL/PITR) is still pending** per the deploy log. Go coverage is 72.4% against an 80% target, with no CI gate.

### 4.6 Code health

- `web/static/js/app.js` is 4,480 lines. `init()` alone is about 2,600 lines, with a single `click` switch of 132 cases. The sheet wrapper template is duplicated 19 times.
- There is dead code: `renderCalendarView`, the week/day calendar branches, about 20 unused imports, and `renderStatsView`.
- `newServerWithDB` is about 660 lines of wiring. `HouseholdView.swift` (the iOS Settings tab) is 897 lines.
- **Docs have drifted from the code:**
  - AGENTS.md invariant #1 (tap = log) is false.
  - The README says 12 predefined chores and 26 migrations; there are 16 chores and 54 migrations.
  - `ios/README.md` describes files that don't exist.
- Several untracked review files sit in the repo root; `unblock-qwen-quality.md` isn't about Nabu.
- **Recurring bug classes across reviews:**
  - timezone/local-date (12+ fixes)
  - stale async responses
  - morph clobbering inputs
  - identity lifecycle after logout or removal
  - parity drift
  - read-modify-write lost updates

  The architecture work in Phase 1 targets the first three directly.

---

## 5. The plan

Sizing assumes one senior engineer working with coding agents; S ≈ ≤2 days, M ≈ 3–7 days, L ≈ 2–3 weeks. Every item follows the repo rules: worktree, parity matrix, a Playwright E2E for every PWA change, XCTest/XCUITest for iOS, and pre-push checks.

### Phase 0 — Stop the bleeding (1–2 weeks)

| # | Work | Size | Acceptance |
|---|---|---|---|
| 0.1 | Server: never serialize nil slices as `null` in API responses (recap `byCategory` first; audit all `[]T` response fields). iOS: `decodeIfPresent ?? []` for every array in `Models.swift`. Add a contract test that decodes empty-household fixtures for every endpoint. | S | Fresh household shows a designed empty Stats state on both clients; new fixture test fails on old code. |
| 0.2 | iOS log sheet: pin the primary **Log** button in a bottom safe-area bar (always visible at every detent and Dynamic Type size). | S | XCUITest at `AccessibilityXXXL` logs successfully; snapshot at AX sizes. |
| 0.3 | **GDPR-compliant account deletion** (see Phase 4 for the legal reasoning). (a) Before deleting, offer "Download my data" (extend the CSV export to every log the user made, across households). (b) The deletion sheet offers two opt-in checkboxes: "Also delete everything I logged" and "Also delete notes I wrote". (c) By default, shared-household logs are reassigned to a **per-household tombstone** that has no email, no name, no avatar and no stored link to the deleted user ID (not a stub user row). The "historical members" list shows only "Former member". (d) Delete everything else: account, email, name, avatar, sessions, push/APNs tokens, preferences, reminder prefs, notifications, auth tokens and mail-outbox rows, invites, and sole-member households. (e) Copy on both clients and in the privacy policy states exactly what is kept and what is deleted. | M | Go tests: A and B share a household; B deletes with defaults → B's logs remain under "Former member" and no column anywhere references B's ID or email; B deletes with "everything I logged" → B's logs are gone and A's remain; B's notes are stripped when that box is ticked. E2E and XCUITest cover the sheet on both clients. |
| 0.4 | Suspended members: filter `suspended_at IS NULL` in reminder candidates and delivery membership checks; also check notification fan-out. | S | Postgres integration test proves a suspended member gets no reminder or push. |
| 0.5 | Escape the chore-icon sinks (`today.js:385`, `stats.js:733`, `stats.js:830`) and the unescaped `initial`/`avatarColor` interpolations; validate icons server-side as at most 2 grapheme clusters with no `<>&"'` or control characters. | S | JS render tests per sink with a hostile icon; Go validation test. |
| 0.6 | Move `RequestLogger` and `SecurityHeaders` outermost; hash the trusted client IP; log client cancellations as 499. | S | Handler tests assert headers on 403/409/429. |
| 0.7 | iOS undo after every new log (toast + `.success` haptic); fix the `latestLogs` key; refetch latest-per-chore after undo. | S | XCUITest: log → toast → undo → tile back to the previous "time ago". |
| 0.8 | PWA: move the undo toast so it never overlaps a sheet's action area (or dismiss it when a sheet opens); fix the dark-mode bell color; hide the version label (move it to Settings → About). | S | E2E for toast/sheet overlap; dark-mode visual check. |
| 0.9 | Housekeeping: update AGENTS.md invariant #1, the README counts, and `ios/README.md`; remove or relocate root-level review files; decide PR #141; quote the deploy heredoc. | S | Docs match code. |

### Phase 1 — Offline-first and fast (3–5 weeks)

Nabu must open straight to a usable Home, online or not, and feel fast on both clients.

1. **Smart defaults** (S–M).
   - Feed Baby pre-fills the last amount and type. The owner declined a breast-feed side option (left/right/both); an optional feed timer is still open.
   - Meds pre-fill the last dose.
   - The owner declined "usual time" hints.
2. **Offline-first launch** (M each client).
   - **PWA:** cache the app shell in the SW (navigation → cached shell, then revalidate). Persist the last-known identity and the chores/latest/household snapshot in IndexedDB. **Render from the snapshot immediately**, then reconcile with `/api/me`. If the session turns out to be revoked, wipe the snapshot and drop to sign-in, but never block logging on a reachable-network check when the identity binding is valid. Pending logs already queue durably.
   - **iOS:** the same pattern. Persist a snapshot (JSON file in Application Support with file protection; SwiftData is unnecessary). Launch into Home from the snapshot. Treat `/api/me` failure from a network error as "offline", not "checking".
3. **PWA rendering and data layer** (L, incremental).
   - **Router:** real `pushState` per tab, `aria-current`, a working Back button, and deep links (`/activity`, `/stats/chore/12`).
   - **Data store:** a small stale-while-revalidate cache keyed by endpoint and household, deduping in-flight calls and invalidating on mutation. The log POST response should return the updated latest/today rows, so the 3 refetches disappear.
   - **Rendering:** keyed morphing (`data-key`) and per-region rendering, so the stats page doesn't rebuild on unrelated state. Split `app.js` by feature (actions registry per module).
   - **Build step:** stay build-step-free. Have the server emit `<link rel="modulepreload">` for the full import graph it already walks in `buildVersionedJSCache`, which removes the waterfall with zero tooling. Fix the SW precache to use the versioned URLs.
   - **First paint:** show a skeleton or the cached snapshot before network.
   - **Target:** Home interactive in ≤1.5 s cold on slow 4G and ≤300 ms warm.
4. **iOS architecture** (L, incremental).
   - Migrate to `@Observable` feature models (Home, Activity, Stats, Settings) owned by a thin `AppModel`, and shrink `AppState`.
   - Swift 6 language mode with complete concurrency checking.
   - Replace the 43 silent `catch {}` blocks with typed error surfaces and logging.
   - Add a String Catalog (even English-only, so pluralization is correct and localization is ready), os.Logger, `#Preview` blocks, an xcconfig per environment (Dev/Staging/Prod base URL), and `MARKETING_VERSION` wiring.
   - Gate test hooks behind `#if DEBUG`.
5. **Stats API consolidation** (M). Add a `GET /api/stats/dashboard?period=` that returns the visible sections in one response (the server already knows the layout preference), with ETag/`If-None-Match`. Both clients drop from 15 requests to 1–2.

**Exit metrics:** Home interactive in ≤1.5 s cold on slow 4G and ≤300 ms warm (instrumented, §5 Phase 6); offline cold-launch log succeeds on both clients (E2E + XCUITest).

### Phase 2 — Guided setup and real people (2–3 weeks)

1. **Onboarding flow** (M, both clients) replaces "Settings as onboarding":
   1. **Name** ("What should your household call you?") and an optional avatar color. Add `PATCH /api/me` with display name, plus edit in Settings.
   2. **Household name** (prefilled "The O'Connors' home" from the last name when given).
   3. **"Who and what do you look after?"** A multi-select: Baby (with name), Pets (with names and species), Plants, Home chores, Medications, Personal (reading, movies, workouts).
   4. Seed **only the matching activity packs**, personalized ("Feed Mongo", "Feed Roger", "Feed Ada"). Migration 014 already hard-codes pet names this way; this generalizes it.
   5. **Invite**: a share sheet with a link plus a QR code. Copy: "Nabu works best when everyone logs."
   6. **Optional reminders** with the push pre-prompt, only after the first log (not at sign-up).
2. **Activity packs** as data (S–M): `PredefinedChores` becomes versioned packs (`baby`, `cat`, `dog`, `plants`, `home`, `meds`, `personal`), each with per-pack defaults (metric, indicators, reminder suggestion). Packs can be added later from Manage ("Add a pack"). Keep the migration rule from AGENTS.md for existing households.
3. **Subjects as first-class** (M): the existing "subject tagging" becomes household **members-being-cared-for** (baby, each pet), each with an avatar. One "Feed" tile can ask "who?" with one tap when a household has two cats. This scales to twins and multi-pet homes without tile explosion.
4. **Sign-up polish** (S): logo and value line on auth screens, password rules inline, drop confirm-password (use show/hide), Apple/Google first on iOS, and fix the iOS background bug.
5. **Shareable invite landing page** (S): `/join?code=` renders a branded page with the household name, the inviter's name and avatar, and an "Open in app / Continue on web" choice.

**Exit metrics:** activation (first log within 10 minutes of sign-up) ≥ 70%; ≥ 40% of new households invite a second member in week 1.

### Phase 3 — Always within reach (4–6 weeks; parallelizable)

The app should rarely need to be opened in order to log.

| Surface | What | Size |
|---|---|---|
| **App Intents** | `LogActivityIntent(chore, amount?, subject?)`, `UndoLastLogIntent`, `LastDoneQuery` ("When did we last feed Ada?"), with `AppShortcutsProvider` phrases. This unlocks Siri, Shortcuts, Spotlight, the **Action button**, and automations (e.g. log "Walk Dog" when leaving home). Intents run in-process via the offline queue. | M |
| **Interactive widgets** | Small/medium/large widgets with `Button(intent:)` to log without opening the app; configurable chore set (`AppIntentConfiguration`); lock-screen accessory widgets ("Fed 2h 10m ago"); StandBy layout for the nightstand. Widget refresh on log via App Group snapshot + `reloadTimelines`. | M |
| **Control Center / lock-screen controls** | `ControlWidgetButton` for one-tap logging of a chosen chore. | S |
| **Live Activity + Dynamic Island** | Duration timers (feeding, nap) and "next feed due in…". The timer model already exists (`DurationTimer.swift`). Push-updated Live Activities for co-caregivers are a v2 option. | M |
| **Apple Watch app** | Complications showing time since last feed/diaper; a grid of the user's top chores; one tap logs; WidgetKit complications share code with the widgets. **One-handed 3 a.m. logging is the killer use case for a baby household.** | L |
| **Notifications** | Rich actions that log in the background without opening the app (currently "Log now" opens the sheet). Use time-sensitive interruption level for meds only, with user opt-in. Optional overdue nudges for chores past their usual interval ("Plants: 9 days, usually every 4"). | S |
| **Mac** | Short term: enable the iOS app on Apple-silicon Macs ("Designed for iPad") and fix keyboard/pointer basics. Medium term: a multiplatform SwiftUI target (menu-bar extra for quick logging, keyboard shortcuts). | S → L |
| **Desktop web** | A responsive PWA layout at ≥900 px: sidebar navigation, Home grid plus a live Activity column, wider Stats. Works as a Safari "Add to Dock" app on macOS and an installed PWA on Windows/ChromeOS. Keyboard shortcuts (`1`–`9` log tiles, `u` undo, `/` search). | M |
| **Kitchen display / shared tablet** | A "Household display" mode: large tiles, no account switching, PIN-guarded settings. Pick who logged it with one tap on the avatar row. This is a strong differentiator for families. | M |
| **Android** | The PWA already installs; add maskable icons and manifest shortcuts per pack, and verify Web Push. A native app is not needed for launch. | S |

**Parity note:** the matrix should record these as **iOS-only by design** (N/A for the PWA) where no web equivalent exists. Shared semantics (intent → same `POST /api/logs` with `hour`/`completedAt`) must stay identical.

### Phase 4 — Privacy and GDPR compliance (2–4 weeks, start alongside Phase 0; legal review required)

Nabu stores data about children's feeding, diapers and medications for households that may be anywhere in the EU. The items below are what a privacy lawyer will expect to see; they should be confirmed by counsel before EU marketing, because health data about children raises the bar.

1. **Data map and roles** (S). Document every personal-data field (account, household content, logs, notes, push tokens, IP hashes, backups), its purpose, its legal basis, and its retention. Nabu is the **controller** for all of it; the hosting provider, Cloudflare, the email provider, Apple (APNs) and browser push services are **processors or recipients**. Sign or record a **data processing agreement** with each, and check that transfers outside the EU rely on adequacy decisions or standard contractual clauses.
2. **Lawful basis and explicit consent for health data** (M, both clients). Account and household features run on *contract* (Art. 6(1)(b)). Baby and pet care logs (feeds, diapers, medications, growth) are very likely *data concerning health* (Art. 9), which needs **explicit consent** (Art. 9(2)(a)):
   - At onboarding (Phase 2), when someone enables the baby or medication packs, show a plain-language consent screen: what is recorded, who in the household sees it, and that it can be withdrawn.
   - Store consent records server-side (who, when, which version of the text).
   - Withdrawing consent stops collection for those activities and offers deletion of that data.
   - The person giving consent is a parent or guardian recording data about their own child, so no separate child-consent flow is needed (the users are adults), but say so in the privacy policy.
3. **Erasure that protects other members** (covered by Phase 0.3). The reasoning:
   - The right to erasure (Art. 17) covers personal data *about the person asking*. A record like "Ada fed 120 mL at 3:10" is mainly the household's record about the child; the part that is about the deleting member is the attribution (user ID, name, email) and anything they wrote.
   - Removing that link irreversibly, and offering opt-in deletion of their entries and notes, satisfies erasure while the remaining members keep their child's history under their own legitimate interest.
   - Deleting everything (today's behavior) is also compliant, but only if the copy says so. Today it doesn't, which breaks the transparency duty (Arts. 12–13).
4. **Access and portability** (S). One "Download my data" action (Arts. 15 and 20) that returns the account record, preferences, and every log, note and day note the user authored, as machine-readable JSON plus CSV. Admin household export already exists and stays.
5. **Retention and backups** (S–M).
   - Publish retention periods: active data while the account exists; deleted accounts purged immediately from the live database; encrypted backups and WAL rotated within a stated window (e.g. 35 days).
   - Keep a minimal **deletion ledger** (opaque IDs only) that is re-applied after any restore, so restoring a backup never brings back an erased account.
   - Add sweeps for expired sessions and auth tokens (Phase 5.3), and set a retention limit on request logs (hashed IPs).
6. **DPIA and records of processing** (S). Large-scale processing of health data about children is a standard trigger for a data protection impact assessment (Art. 35). Write it once, keep it with the records of processing (Art. 30), and update it when a new data type is added (e.g. growth measurements or third-party lookups).
7. **Security of processing** (ongoing). The existing controls (hashed sessions, CSRF, TLS, encrypted backups) count toward Art. 32. Add: a documented breach-response procedure with the 72-hour notification step (Art. 33), access logging for operator/admin data access, and least-privilege production access.
8. **Policy and in-app transparency** (S). Update the privacy policy to cover all of the above, add an in-app privacy page with the consent toggles, export and deletion, and mirror it in the App Store privacy labels.

**Exit criteria:** counsel sign-off on the data map, the DPIA, the consent text and the deletion design; DPAs on file for every processor; export and deletion tested end to end on both clients.

### Phase 5 — Scale, observability, and resilience (3–4 weeks, start in parallel with Phase 1)

1. **Observability** (M).
   - `log/slog` JSON (no timestamp prefix).
   - OpenTelemetry traces and metrics (HTTP, pgx, scheduler, push) exported to a managed backend or self-hosted Grafana/Tempo/Loki.
   - A `/metrics` endpoint behind auth.
   - Server error reporting (Sentry or self-hosted GlitchTip) with PII scrubbing; iOS MetricKit plus crash reporting.
   - SLOs: API p95 < 200 ms, log POST success ≥ 99.9%, push delivery latency p95 < 30 s.
2. **Durable background work** (M). Move notification fan-out and push sends to a Postgres job table (same `SKIP LOCKED` pattern as the mail outbox), run push I/O outside transactions, and make reminder recording idempotent (insert-then-send with a unique key).
3. **Multi-replica readiness** (M). Postgres- or Redis-backed rate limiting, a distributed export gate, and session/auth-token/effect sweeps run by the leader.
4. **Database** (S). Add the missing indexes (§4.5), drop the duplicates, use `CREATE INDEX CONCURRENTLY` in migrations, and add per-statement timeouts on hot paths. Fix the `GET /api/household` N+1 with a single join.
5. **Recovery** (S–M). Finish WAL/PITR production adoption and quarterly restore drills (the runbook exists).
6. **Load test** (S). A k6 scenario for 10k households, morning and evening peaks, and the reminder tick, run against staging before launch.
7. **Staging environment** (M). A prod-like stack with seeded demo data, used for TestFlight builds, App Review demo accounts, and marketing screenshots. Today the demo/test account lives in production.
8. **Hosting** (decision). The current single server plus Cloudflare Tunnel is fine to about 10k DAU. Plan the move to managed Postgres (backups/PITR handled) and 2+ app replicas behind the tunnel before paid launch.

### Phase 6 — Launch and growth (ongoing; begins after Phases 0–2)

1. **Brand and design system** (M).
   - A real logo and app icon.
   - A type scale and spacing tokens shared by CSS custom properties and the Swift asset catalog. CSS has 66 distinct hard-coded hex values today; iOS has about 80 numeric paddings.
   - A unified illustration style for empty states.
   - Motion guidelines that respect Reduce Motion.
2. **Marketing site** (S–M). Screenshots from a **seeded demo household on staging** (no real pet names or notification counts), a short screen-recorded video of logging a feed, a widget or watch hero shot, a Privacy section ("no ads, no third-party analytics, your data exportable and deletable"), and App Store / Install PWA buttons.
3. **App Store** (M; owner-gated items in `docs/plans/ios-v1/p7-release.md`).
   - Physical-device APNs checklist, TestFlight external beta, screenshots after Phases 0–2, privacy labels, and review notes with a staging demo account.
   - Category: Lifestyle (primary), Health & Fitness (secondary).
   - **Don't submit before 0.1/0.2 are fixed:** App Review would see a broken Stats tab on a fresh account.
4. **Business model** (decision; §9). Keep the promise on the marketing page: **tracking is free forever** (unlimited members, chores, logs, reminders). **Nabu Plus** (about $4.99/month or $39/year per household) would include:
   - the Watch app's advanced complications
   - the household display mode
   - multiple households beyond 2
   - priority support

   Use StoreKit 2 subscriptions on iOS and Stripe on the web, with a server-side entitlement table so one purchase covers the household on every client. Family Sharing-compatible.
5. **Privacy-respecting product analytics** (M). First-party, aggregate-only event counts stored in our own Postgres (activation, logs per household per day, retention cohorts, surface used: widget/watch/intent/app). No third-party SDKs, honoring the "no tracking" claim, documented in the privacy policy.
6. **Growth loops** (S–M).
   - Invites are the growth engine: every household needs ≥2 people.
   - Make invite links beautiful, show "Sam joined" moments, and add a "Grandparent mode" (read-only glance access, large text) as a low-friction invite for extended family.
7. **Support and trust** (S). In-app "Send feedback" with diagnostics opt-in, a status page, and a public changelog.
8. **Localization** (M, after launch). With the String Catalog and a PWA string table in place, Spanish, German and French cover the largest App Store markets for this category.

**North-star metric:** *weekly active households where ≥2 members logged*. Supporting metrics: activation rate, D7/D30 household retention, logs per active household per day, share of logs from widgets/watch/intents (the "always within reach" score), crash-free sessions ≥ 99.8%.

---

## 6. External data sources — evaluated

| Source | Use in Nabu | Verdict |
|---|---|---|
| **TMDB** | Movie/TV title autocomplete and posters for **Watch Movie** (exists today). Free for non-commercial use with attribution; commercial use needs a license, so check before Plus launches. | **Low priority**; only if the Personal pack stays in scope (§9). |
| **Open Library** | Book search and covers for **Read Book** (exists today). Free, no key. | **Low priority**, same condition as TMDB; same UI component. |
| **WHO Child Growth Standards** (public LMS tables) | Weight/length/head-circumference percentiles for a "Growth" activity in the baby pack. Bundle the tables (no API call needed). Show percentiles with care and a "not medical advice" note. | **High value** for the baby pack (Phase 2 packs). Adds a new health data type, so update the DPIA (Phase 4). |
| **openFDA / RxNorm (NLM)** | Medication name autocomplete and normalized names for the meds pack. **Do not** provide dosing guidance; only name lookup and the user's own interval reminders. | **Medium value**, Phase 2; needs a careful disclaimer and legal review. |
| **Plant care APIs** (Perenual, Trefle) | Suggested watering intervals when adding a plant by species, which pre-fills a schedule. | **Medium**, Phase 2 packs. |
| **Open-Meteo** (free, no key) | "Skip watering, it rained 12 mm" hints; walk-the-dog weather. | **Delight, low priority.** |
| **Calendars (ICS feed / EventKit)** | Subscribe to the household schedule in Apple/Google Calendar; optionally show upcoming calendar events on the Schedule tab. | **Medium**, Phase 3. |
| **Apple HealthKit** | No infant data types; optional export of a caregiver's own workouts or meds is out of scope. | **Skip** for now. |
| **Pet microchip / vet APIs** | Fragmented, no reliable public API. | **Skip.** |

All third-party calls go through a server-side proxy with caching, per-household rate limits, no user identifiers sent upstream, and attribution shown where licenses require it.

---

## 7. Architecture direction (summary)

- **Backend:**
  - Keep the standard-library Go design; it's a strength.
  - Split route registration per feature and move to `slog`.
  - Add a small job runner on Postgres (outbox pattern) and a per-household-cached dashboard read model for stats.
  - Expose `PATCH /api/me` for profile.
  - Adopt an "API never returns null arrays" rule enforced by a test helper.
- **PWA:**
  - Keep plain ES modules and no build step (server-side `modulepreload` instead of a bundler).
  - Add a router, an SWR data store, keyed rendering, and app-shell offline launch.
  - Split app.js per feature, and add a desktop layout.
- **iOS:**
  - `@Observable` feature models, Swift 6, and a snapshot cache for offline launch.
  - App Intents as the shared action layer for widgets, controls, Siri, Watch and notifications: one `LogActivityIntent` implementation feeding the existing `OfflineLogQueue`.
  - Watch and Mac targets follow.
- **Shared contract:**
  - Generate an OpenAPI spec from the Go handlers (or hand-maintain one with a CI drift check).
  - Generate the iOS DTOs and a JS type map from it.
  - This removes the class of null/shape mismatches behind defect 0.1 and makes parity checks mechanical.

---

## 8. Testing and quality gates to add

1. **Empty-state contract suite:** every endpoint is fetched for a brand-new household and decoded by the iOS models in `NabuTests`.
2. **Dynamic Type matrix:** XCUITest smoke (log, undo, Stats) at default, AX-L and AX-XXXL; PWA E2E at 200% text zoom.
3. **Offline launch:**
   - Playwright: context offline → reload → log → back online → synced.
   - XCUITest: the network link conditioner equivalent via a mock transport.
4. **Visual regression** for the PWA (Playwright screenshots per tab, light and dark, 390 px and 1280 px) and iOS snapshots on the CI runtime actually installed (today they self-skip off iOS 26).
5. **Security regressions** for each fix in Phase 0 at the vulnerable layer, as AGENTS.md requires.
6. **Coverage gate** at 80% for Go once Phase 0 lands; iOS lane runs UI smoke nightly.
7. **Privacy regressions:** after account deletion, a test scans every table for the deleted user's ID and email; export returns everything the user authored; a restore drill re-applies the deletion ledger.
8. **Performance budgets in CI:** cold slow-4G Home ≤1.5 s (Playwright with CDP throttling), Stats requests ≤3, and a bundle-graph depth check.

---

## 9. Decisions needed from the owner

1. **Account deletion semantics:** keep shared-household logs under an unlinkable "Former member" tombstone with opt-in deletion of the user's entries and notes (recommended, Phase 0.3), or delete them all (current behavior, but the copy must then say so).
2. **Default packs:** decided. The owner confirmed the pack list, and "Personal" (books, movies, workouts) stays.
3. **Monetization:** the free-core / Plus split and price points (§5, Phase 6).
4. **Apple Watch** in v1.0 or v1.1 (recommended: v1.1, right after App Store launch).
5. **Hosting:** managed Postgres plus 2 replicas before paid launch, or stay single-node longer.
6. **PR #141 quiet hours:** rebase and fix, or close.
7. **Analytics:** approve first-party aggregate analytics consistent with the "no tracking" promise.
8. **Brand:** keep the name "Nabu" (check trademark and App Store name availability) and commission an icon/logo.
9. **Privacy counsel:** engage a privacy lawyer for the data map, DPIA, consent text and deletion design (Phase 4), and decide whether an EU representative is needed (Art. 27) if Nabu has no EU establishment.

---

## 10. Suggested sequencing (about 16–18 weeks to a marketed v1)

```
Weeks  1–2   Phase 0 (all)                         ── Phase 5.1 observability · Phase 4 data map + counsel engaged
Weeks  3–7   Phase 1 (offline-first, PWA router/data layer, iOS architecture)
Weeks  6–9   Phase 2 (onboarding, packs, names, invites, health-data consent)    ── Phase 5.2–5.4 in parallel
Weeks  8–13  Phase 3 (App Intents → widgets/controls → Live Activity → desktop web; Watch starts)
Weeks 10–12  Phase 4 exit: DPIA, DPAs, privacy policy, export/deletion verified
Weeks 11–14  TestFlight external beta · marketing site · staging demo data · App Store submission
Weeks 13–18  Watch app · Plus (StoreKit/Stripe) · Phase 6 growth loops
```

---

## Appendix A — Evidence index

**Screenshots** were captured in this session's scratchpad (`/private/tmp/claude-501/-Users-dave-git-nabu/e76cd488-…/scratchpad/shots` and `…/ios/shots`). They are ephemeral; re-create them with the steps below.

- **PWA stack:** a compose file with project name `nabu-review` and ports 18080/15432/18025/11025, built from the repo `Containerfile`. Register via `/register`, create a household in Settings, then log from Home.
- **iOS:**
  - Create a dedicated simulator: `xcrun simctl create nabu-review-iphone com.apple.CoreSimulator.SimDeviceType.iPhone-17-Pro com.apple.CoreSimulator.SimRuntime.iOS-27-0`.
  - Build → build-for-testing → test-without-building per `ios/AGENTS.md`.
  - Launch arguments: `-resetState -nabuBaseURL http://localhost:18080 -nabuAutoRegister <email> <pw>`.
  - For large text, add `-UIPreferredContentSizeCategoryName UICTContentSizeCategoryAccessibilityL`.
- **Stats null repro:** register a user, create a household, seed defaults, then `GET /api/stats/overview` returns `"byCategory":null`, and the iOS `Recap` decoder throws.
- **Throttled performance:** Playwright Chromium with CDP `Network.emulateNetworkConditions` (150 ms RTT, 1.6 Mbps down) and service workers blocked for the cold run.

## Appendix B — Prior review status (summary)

- **Fixed:** all 29 findings of `docs/review-2026-09-10.md` (PR #131), the 2026-06-28 review items, security-audit #1–#8 and #10, and PR-146 F1–F7.
- **Open:**
  - recovery adoption in production (OPS-2)
  - physical-device iOS checks
  - audit #9 leftovers (raw 4xx errors)
  - PR-147 security #1 (deploy heredoc), #2 (owner-ID fingerprint), #5 (`workflow_dispatch` deploys unpinned `:latest`)
  - CSP `unsafe-inline`
  - invite-code rotation
  - APNs token reassignment
  - account-deletion re-auth
  - PR #141 (unmerged, migration collision)
- **Recurring themes:** timezone/local-date, stale async responses, morph input clobbering, identity lifecycle, parity drift, read-modify-write.
