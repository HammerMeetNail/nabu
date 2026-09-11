# Review implementation

Source: [PWA and backend review](review-2026-09-10.md), reviewed commit `5197f27`.
This branch implements the review's code and operational-tooling changes across
the shared backend, PWA and native client. Production access and deployment were
excluded. The user explicitly deferred Xcode, simulator and device verification
to a separate Mac session after commit/push; see the [Mac handoff](review-2026-09-10-mac-handoff.md).

## Findings and resulting behavior

| Finding | Implementation and regression coverage |
| --- | --- |
| SEC-1 | Verified account claims atomically revoke unverified credentials, sessions and proofs. Credential epochs reject password checks already in flight. Google, Apple, verification, magic-link and reset paths share the claim transaction; verified established credentials remain trusted. Passwordless claimed users can set a password in either client. |
| SEC-2 | Member removal and role changes revoke previously disclosed household invitations; current owners can issue fresh invitations. |
| SEC-3 | Authentication and household actions use canonical active membership. Activation, removal and authorization share transaction/lock boundaries; stale or removed memberships cannot remain authoritative. |
| SEC-4 | Idempotency keys bind actor and original payload digest. Replays recheck current access, and a durable effects ledger makes schedule/follow-up effects resumable without duplication. PostgreSQL unique races, effect rollback/retry and concurrent callers are covered. |
| SEC-5 | Chore/membership lookup failures fail closed across log collections, exports, statistics, preferences and reminder preferences. Read visibility is applied before limits and rechecked at sensitive export boundaries. |
| SEC-6 | Web/APNs registrations belong to the authenticated session. Delivery and cleanup use current household, role and session ownership; browser/native identity changes fence old actions. Device delivery still needs the Mac/device checks. |
| SEC-7 | Removal, demotion and ownership transfer authorize both resources in the relevant household. Pending reminder/delivery guards observe membership changes. Self-removal is a classified 403; membership remains intact. |
| SEC-8 | Unfinished logout is durable and visible, revocation errors allow retry, and old-account failures cannot replace a newer identity. Canonical session recovery coordinates browser tabs and native requests. |
| SEC-9 | Request IDs and classified database/provider failures support diagnosis without logging private payloads, raw client IPs, secrets or capability URLs. Captured-log regressions cover wrapped errors. |
| SEC-10 | One bounded IP budget per limiter scope replaces raw-path buckets. Active allowances are not evicted to make room. Proxy attribution/configuration is validated and cancellation joins cleanup. Actual proxy attribution from two external networks remains an operational check. |
| DATA-1 | Account deletion validates every membership and removes dependent data in one transaction. Concurrent joins, log effects, lock order and injected failures have PostgreSQL regressions. Shared chore authorship is cleared safely. Rejected deletion keeps ownership guidance and respects later Cancel/navigation. |
| DATA-2 | Rejected/network/storage saves retain the draft, frozen payload and actionable status. Stopped timers survive relaunch with their original duration until acceptance or deliberate discard. |
| DATA-3 | Presence-aware PATCH distinguishes omitted fields from explicit null. Both clients preserve unrelated metrics and can clear nullable values. Concurrent PostgreSQL field updates merge without overwriting unrelated changes. |
| DATA-4 | Durable journal entries belong to account, household and logical submission. Replay/discard synchronize per key, bootstrap restores pending rows, and corruption blocks overwrite. Cross-tab/context, discard and storage-failure cases are covered. |
| DATA-5 | Double saves, lost responses, offline replay and simultaneous timer stops reuse one immutable request and idempotency key. Success is reported only after confirmed acceptance or explicit durable-pending status. |
| DATA-6 | Registration commits account, session, verification token and retryable mail outbox atomically. SMTP failure cannot turn a committed usable account into an apparent failed registration. Mail retries and shutdown have bounded lifetimes. |
| UX-1 | A dedicated authorized recent-amounts API returns three distinct positive amounts independent of visited tabs. Both clients retain last-good chips through errors and reject stale reads. Chips do not select unrelated feeding types. Browser cases cover reload, search, midnight, empty latest values and gram/count units. |
| UX-2 | Account/household resets clear dynamic caches, scope timers/journal entries and fence pending reads. Activity, Stats and timers are tested through switches and delayed old responses. |
| UX-3 | Activity entry, shortcut, retained search, refresh, edits and pagination use one query owner. Errors are distinct from empty history. Local chore filters retain page position and release canceled loading state. |
| UX-4 | Recurrence uses recipient-local calendar dates and DST-safe day arithmetic. Date-only range helpers preserve inclusive/exclusive semantics. New York, Tokyo, DST and end-date boundaries are covered. |
| UX-5 | Shared PWA sheet handling owns focus, inert background, Escape/Back and focus return. Datetime controls are labeled and fit narrow/enlarged layouts. Metric definitions drive generic units; duration and amount edits roundtrip. Native UI sources mirror behavior; screen-reader/device checks are deferred. |
| UX-6 | Authentication transitions own notification polling. Empty Schedule offers the real creation action, with cancel/save/reload coverage and matching native CTA. The optional proposal to redesign the default Stats layout was considered and left for a separate product-design change; configured layouts are preserved. |
| PERF-1 | SQL aggregates avoid hydrating full log text/JSON for scalar results. Authorized latest/search/history queries use measured indexes, stable ordering and visible older-page detection. PostgreSQL differential, revocation and index-migration tests preserve semantics. |
| PERF-2 | Identical in-flight reads are shared only within the current identity; explicit cancellation keeps ownership. Stats loads configured visible sections, one overview and query-owned widget aggregates, including all 20 supported widgets. Request-count and stale-result regressions cover both clients. |
| PERF-3 | The scheduler pages scalar candidates with bulk preference/dedup snapshots, evaluates exact local recurrence and resumes a stable cursor across bounded ticks. A reserved delivery pool keeps HTTP available; cancellation releases providers and leadership. |
| OPS-1 | Lightweight liveness and one-second dependency readiness are separate. Deployment uses bounded readiness checks and app rollback. Real database outage, pool exhaustion, migration failure, recovery and TERM-ignoring child processes are covered. |
| OPS-2 | Prepared continuous encrypted WAL plus full/differential off-site backups target five-minute RPO and one-hour RTO. Isolated logical/PITR restore tools, freshness/restore/heartbeat monitoring, cleanup and cutover procedures replace unsafe live restore. Adoption requires real infrastructure inventory and off-site verification; no production backup was changed. |
| OPS-3 | Exports enforce row/byte/concurrency/deadline limits before download; both clients expose date ranges, errors and cancellation. Notifications use stable recipient-scoped cursors, measured indexes and explicit retention. Pool/query metrics, reserved capacity, provider deadlines and production configuration are covered. |
| OPS-4 | Go 1.26.8 is shared by the module, CI and images; lint is pinned to 2.13.2. CI scans architecture-specific build digests and deploys the versioned manifest digest. Recovery-image packages and gosu were updated/rebuilt with the same compiler. VAPID parsing now rejects invalid/mismatched keys while retaining valid short legacy scalars. |

Security/persistence and asynchronous behavior/test synchronization received
independent reviews. Findings from those reviews were corrected before broad
validation. These reviews do not establish production configuration or device
behavior.

## Validation record

All locally required gates completed successfully. Native Xcode/device checks
remain deferred as agreed; production checks remain outside this session.

- Build and vet passed with Go 1.26.8. The full PostgreSQL-backed Go race run
  passed 1,069 test cases across 30 packages (`/tmp/nabu-final-go-v2.log`). Three
  explicit skips remain: the existing sqlmock array stub for chore reordering and
  two opt-in performance workloads. Both performance workloads were run separately.
- The first race run hit the 300-second package timeout in a bcrypt test. That
  case passed in isolation; the full run passed with CI's 600-second limit.
  Statement coverage was 72.4%, above the reviewed 64.2% baseline but below the
  repository's 80% target. This is a remaining coverage limitation.
- Later lint corrections, VAPID validation and household-error classification
  passed focused race regressions. Lint passed with uncapped issue reporting.
- The final JS/operations run passed 111 tests, zero failures or skips
  (`/tmp/nabu-final-js-v3.log`). The final PostgreSQL-backed Playwright suite
  passed all 382 tests, zero failures/skips/flakes/global errors
  (`/tmp/nabu-final-e2e-v3.json`, exit 0). Earlier full runs exposed implementation
  and fixture races; each correction received focused validation and the final
  complete suite. The timer synchronization correction also passed five repeats.
- The portable native-core harness passed 172 XCTest cases, zero failures/skips
  (`/tmp/nabu-native-core-v12.log`). It compiles copied Foundation logic with
  Combine/UIKit shims and FoundationNetworking imports. It does **not** typecheck
  SwiftUI, exercise real cookies/APNs/file protection, or establish Xcode coverage.
  Swift 6.2 parsed all 90 Swift source files (`/tmp/nabu-native-parse-v7.log`).
- Final build, vet, lint, parity, compiler-alignment and whitespace checks passed
  (`/tmp/nabu-final-static-gates-v5.json`). Go 1.26.8/govulncheck 1.8.0 source
  analysis and a matching unstripped builder artifact both reported zero
  called/imported-package vulnerabilities. Three module-only advisories concern
  unused SSH/OpenPGP packages. Scanning the shipped stripped binary falls back to
  module precision and reports those advisories as synthetic symbols; extraction
  confirmed no symbol data, so that result is not a linked-package finding.
- The actual local runtime image `78fd77b4a2ae` and recovery image `e49b32ffab9f`
  both passed HIGH/CRITICAL Trivy scans with zero findings. Reports are
  `/tmp/nabu-final-app-scan-v3.json` and `/tmp/nabu-final-recovery-scan-v2.json`.
  The unstripped binary scan is `/tmp/nabu-final-unstripped-scan-v1.log`.
- All 301 changed/new files passed a secret scan of an isolated staging copy
  (`/tmp/nabu-final-secrets-v2.json`). Generated artifacts and local environment
  files are excluded from the commit.

Logs, traces, reports, scan archives and the temporary native harness are under
`/tmp`, outside the repository. They are session-local artifacts, not durable CI
records. The commands and committed tests provide the reproducible handoff.

## Performance and recovery evidence

See [performance-workload.md](performance-workload.md) for workload definitions,
query counts, p95/allocations, write/index costs and hardware limits. On 500,000
synthetic logs, indexed latest reads fell from 63.0 ms to 0.93 ms median, and scalar
aggregation avoided about 117 MB of full hydration per request. The notification
fixture's two-query cursor page fell from 12.89 ms to 1.66 ms p95. These are warmed
local measurements, not supported household counts or production capacity claims.

The reminder fixture covers 60 households/4,320 candidate pairs, exact query
budgets, bounded progress, deduplication and reserved-pool pressure. Slow provider
barriers verify shutdown and leader handover. Actual device receipt remains
outside that fixture.

See [recovery-runbook.md](recovery-runbook.md) for the prepared archive, alert,
retention, key escrow and isolated cutover design. Local encrypted WAL/PITR and
logical-dump drills exercise the real app, migrations/constraints, two-household
isolation, outage/readiness recovery, app rollback and cleanup. Wrong keys,
missing WAL, empty/truncated dumps and private SQL errors are negative cases.
The final integrated drill passed with complete cleanup (`/tmp/nabu-recovery-integrated-v3.json`): WAL/archive verification 0.265s, restore/application smoke 3.236s, encrypted logical restore 3.732s. These tiny local fixtures do not establish the one-hour production RTO or five-minute RPO. Loopback alert success/failure tests do not establish an external alert route.

## Remaining external verification

The [Mac handoff](review-2026-09-10-mac-handoff.md) owns Xcode build, unit/contract,
UI and snapshot runs, plus real-device cookie/logout, protected journal/timer,
APNs and VoiceOver checks. Existing/new UI sources are unrun; new visual baselines
must be recorded and reviewed on the designated iOS 26 runtime.

Production adoption must inventory the database major/data path, stage the
recovery image/configuration, verify off-site archive freshness and restore,
confirm alerts and proxy attribution, and follow the normal merged-main release
process. Nothing in the local results confirms current production backup health,
provider delivery, a live image scan, or achieved production RPO/RTO.
