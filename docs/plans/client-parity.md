# Client Parity Matrix

Living matrix tracking feature parity between the PWA and native iOS app.

Shared server rate limits default to 30 auth, 600 total API, and 30 household-join
requests per minute per IP per replica. These allowances apply equally to PWA
and iOS; no client UI or API model changes are required.

> **Re-baselined 2026-06-28** against the code on `main`. The previous matrix
> was inaccurate in both directions: it marked ~50 rows "iOS pending" even
> though the corresponding SwiftUI views and API calls already shipped, and its
> per-row iOS test column referenced ~30 test files that do not exist
> (e.g. `HomeUITests.swift`, `StatsSnapshotTests.swift`, `APNsContractTests.swift`).
> The real iOS test suite is a smaller set of unit/contract tests
> (`NabuTests/*.swift`) plus a single `NabuUITests.swift`. Statuses and test
> references below now reflect what is actually in the repository.

## Status legend

- **Built** — Implemented in iOS (view + API wiring present on `main`).
  Behavioral parity against the *current* PWA has not been re-confirmed by an
  automated UI run, so treat as "implemented, verify before release."
- **Done** — Implemented on both clients with passing iOS test coverage and
  parity confirmed.
- **iOS pending** — Not yet implemented in the native app.
- **Deferred** — Intentionally absent from both clients' navigation.
- **Not built** — The feature is non-functional end-to-end (a stub or
  contract may exist). Currently: none — the last three (APNs, account
  deletion, Sign in with Apple) were built across P1–P5; APNs still needs
  physical-device verification, see `docs/plans/ios-v1/p5-push-auth.md`.
- **N/A** — Feature applies to only one client (with justification).

## Phase progress

| Phase | Description | Status |
|-------|-------------|--------|
| 0 | Guardrails and parity infrastructure | Done |
| 1 | iOS project skeleton | Done |
| 2 | API models and contract tests | Done |
| 3 | Auth, session, and onboarding | Built |
| 4 | Bootstrap data loading and preferences | Built |
| 5 | Home and log sheet | Built |
| 6 | Chores management | Built |
| 7 | Activity history (day/week calendar removed to match PWA) | Built |
| 8 | Schedule | Built (for-date overlay Deferred — no live PWA surface) |
| 9 | Household, members, and multi-household | Built |
| 10 | Notifications (in-app) | Built |
| 10b | APNs native push | Built (physical-device verification pending) |
| 11 | Stats | Built (P4 parity complete: all sections, widgets, customize, Swift Charts + snapshots) |
| 12 | Security, accessibility, and polish | Built |
| 13 | Release readiness | In progress |

## How to use

When implementing an iOS feature or changing PWA behavior, update the
corresponding row and set its parity status from the legend above. A status of
**Built** should be promoted to **Done** only once the behavior is covered by a
test that actually runs in CI (see the iOS CI lane in `.github/workflows/ci.yaml`).

## Feature matrix

| Feature | PWA module/specs | iOS module/tests | Shared API | Parity | Known differences |
|---------|-----------------|------------------|------------|--------|-------------------|
| **Auth & Onboarding** |
| Login/register | `auth.js`, `validation.spec.js` | `Auth/LoginView.swift`, `Auth/RegisterView.swift`, `AuthTests.swift` | `/api/auth/login`, `/api/auth/register` | Built | Registration commits user/session/proof/mail together; mail retries asynchronously. Both clients offer sign-in recovery for an accepted registration without a returned user. |
| Magic link | `auth.js`, `magic-link.spec.js` | `Auth/MagicLinkView.swift`, `AuthTests.swift` | `/api/auth/magic-link/request`, `/api/auth/magic-link/consume` | Built | |
| Password reset | `auth.js`, `settings-auth.spec.js` | `Auth/`, `APIContractTests.swift` | `/api/auth/password/forgot`, `/api/auth/password/reset`, `/api/auth/password` | Built | Shared proof-based account claims revoke unverified credentials and sessions atomically; stale password operations are rejected by credential version. Both clients expose password setup when `hasPassword=false`. Native settings use a password sheet and explicit server snake_case password keys; successful setup and real-cookie rotation/revocation have Mac regression coverage. See [Mac validation](../review-2026-09-10-mac-validation.md). iOS has no dedicated PasswordResetView; reset is wired via the auth store |
| Email verification | `auth.js`, `magic-link.spec.js` | `Support/DeepLink.swift`, `ContentView.swift`, `Views/HouseholdView.swift`, `DeepLinkTests.swift` | `/api/auth/email/verify`, `/api/auth/email/verification/resend` | Built | P5: `/verify-email` universal link calls the same verify endpoint then refreshes `/api/me`; the cross-device fallback (verify in browser, app picks up verified state via `/api/me` foreground refresh) ships too. Magic-login and `/join` invite links ride the same AASA plumbing (`/.well-known/apple-app-site-association`, served when `APNS_TEAM_ID`/`APNS_BUNDLE_ID` are set). Owner-gated: Associated Domains capability on the App ID before links open natively |
| Google OAuth | `auth.js` | `Auth/GoogleOAuthCoordinator.swift` | `/api/auth/google/login`, `/api/auth/google/callback` | Built | Post-login `redirect` values are allowlisted server-side (same-origin paths or the iOS `nabu://callback` scheme only; full URLs rejected) — no client behavior change |
| Logout | `auth.js`, `session-recovery.spec.js` | `Auth/AuthStore.swift`, `DataLoaderTests.swift` | `/api/auth/logout` | Built | Both clients hide account data and retain unfinished logout until server revocation is confirmed. Failed device storage blocks automatic sign-in; retry remains available. Native real-URLSession tests verify failed logout, retained cookies, reconstructed pending state and server revocation; physical-device validation remains pending. |
| Session bootstrap | `app.js` | `App/AppState.swift`, `API/APIClient.swift`, `StateTests.swift` | `/api/me` | Built | iOS adds a CSRF pre-flight `GET /api/me` |
| Account deletion | `app.js` (Settings), `settings-delete-account.spec.js` | `Views/HouseholdView.swift` (`DeleteAccountSheet`) | `DELETE /api/me` | Built | One transaction validates every membership, removes personal logs/auth/preferences/delivery registrations, clears authorship on retained shared/admins-only chores, and deletes only sole-member households; safe FKs, concurrent join and injected rollback tested in PostgreSQL. Memory cleanup rejects stale writes. Both clients retain ownership-conflict guidance and reject stale completion after Cancel or identity changes. The native root-owned deletion sheet survives canonical session confirmation, with gated model and XCUITest regressions; the PWA keeps its existing Cancel/navigation recovery. |
| Sign in with Apple | `auth.js` (`renderOAuthButtons`), `app.js` | `Auth/SignInWithAppleCoordinator.swift`, `Auth/LoginView.swift`, `Auth/RegisterView.swift`, `APNsContractTests.swift` | `POST /api/auth/apple/native` (iOS), `GET /api/auth/apple/web/login` + `POST /api/auth/apple/web/callback` (PWA) | Built | iOS Built in P5: native `SignInWithAppleButton` on login + register, placed above the Google button (guideline 4.8 prominence); per-request random nonce echoed through the identity token and verified server-side (`internal/auth/apple.go`). PWA Built in P7 groundwork: "Continue with Apple" button (above Google, same 4.8 ordering) redirects into Apple's web flow — `response_type=code id_token` + `response_mode=form_post`, so the posted identity token is verified by the same `AppleVerifier`/`LoginWithApple` path as native (no client secret); state/nonce ride `SameSite=None` cookies because the callback is a cross-site POST (CSRF-exempt, state cookie is the double-submit). Enabled by `APPLE_WEB_CLIENT_ID` (the Services ID, auto-accepted as a token audience). Owner-gated: SIWA capability on the App ID, Services ID + return-URL registration in the portal, `APPLE_CLIENT_IDS`/`APPLE_WEB_CLIENT_ID` in prod |
| **Household & Members** |
| Household CRUD | `household.js`, `household-multi.spec.js` | `Views/HouseholdView.swift` | `/api/household`, `/api/households`, `/api/households/{id}/activate` | Built | |
| Join by invite code | `household.js`, `invite-link.spec.js` | `Views/HouseholdView.swift` | `/api/household/join` | Built | Atomic one-use consumption rolls back if membership insertion fails; previously disclosed links are invalidated by removal. |
| Invite management | `household.js`, `invite-link.spec.js` | `Views/HouseholdView.swift` | `/api/household/invites`, `/api/household/invites/{id}` | Built | Owner-only generated links expire after 7 days and permit one join; removal and role changes revoke generated links and rotate the household link. Both clients describe these rules. |
| Member roles | `household.js`, `household-roles.spec.js` | `Views/HouseholdView.swift` | `/api/household/members/{userId}`, `/api/household/transfer` | Built | |
| Remove member | `household.js`, `settings-remove-member.spec.js` | `Views/HouseholdView.swift` | `/api/household`, `/api/household/members/{userId}` | Built | Removal/demotion works when the target is active elsewhere. Removal clears assignments and household reminder preferences; bounded deliveries serialize with revocation. Historical author profiles for retained logs remain available. |
| Leave household | `household.js`, `household-multi.spec.js` | `Views/HouseholdView.swift` | `/api/household/leave` | Built | P6 fixed an iOS parity gap: the PWA's are-you-sure confirm before leaving now exists on iOS too (confirmation dialog + warning haptic) |
| Multi-household switching | `household.js`, `household-multi.spec.js` | `Views/HouseholdView.swift` | `/api/households`, `/api/households/{id}/activate` | Built | Backend activation and membership removal serialize and authorize from canonical membership; concurrent transfer is atomic. PWA view cache isolation is tracked in the review implementation. |
| Join notifications | `notifications.js`, `household-join-notify.spec.js` | `Views/NotificationsView.swift`, `NotificationTests.swift` | `/api/notifications` | Built | |
| **Home** |
| Home grid | `home.js`, `home-grid.spec.js` | `Views/HomeView.swift`, `Views/HomeGrid.swift`, `HomeTests.swift` | `/api/logs/latest-per-chore`, `/api/logs/today` | Built | |
| Direct tap log | `today.js`, `home-time-accuracy.spec.js` | `Views/HomeView.swift`, `RequestEncodingTests.swift` | `/api/logs` | Built | |
| Log sheet (when picker) | `schedule.js`, `home-when-picker.spec.js` | `Views/LogSheet.swift`, `Views/QuickLogSheet.swift` | `/api/logs`, `/api/logs/{id}` | Built | Server enforces per-field caps (audit #10: note ≤2000 runes, title ≤120, subject ≤30, ≤8 indicators, volumes 0–100000, hour 0–23, rating 0–50, duration ≤86400s); PWA `maxlength` attributes aligned (title 120, note 2000); iOS TextFields are unbounded and rely on the server's rejection — pathological to exceed, no client change |
| Jiggle mode reorder | `home.js`, `home-jiggle-grid.spec.js` | `Views/HomeView.swift` | `/api/preferences` | Built | |
| Hide from home | `home.js`, `home-remove-chore.spec.js` | `Views/HomeView.swift` | `/api/preferences` | Built | |
| Undo toast | `today.js`, `home-grid.spec.js` | `Views/UndoToast.swift`, `HomeTests.swift` | `/api/logs/{id}` | Built | |
| **Activity** |
| History list (paginated) | `activity-data.js`, `activity-context.spec.js` | `API/ActivityStore.swift`, `Views/ActivityView.swift`, `ActivityTests.swift` | `/api/logs/history` | Built | One query owner covers entry, search, refresh, edit, deletion and pagination. Old responses cannot restore deleted rows or clear newer loading/errors. Local chore filters retain the loaded page boundary; canceled pagination can retry immediately. Both clients preserve each entry’s unit, provide separate free numeric amount fields and mutable unit selectors in create/update sheets, including medication choices (mcg, mg, g, mL, L, drops, tablets, capsules, puffs, units); changing the unit preserves the typed number. Manage uses the same choices for activity defaults. Both clients separate incompatible units in totals and recent suggestions; iOS provides a Done control for the numeric keyboard. Cat Meds and Baby Meds default to mg amount tracking; existing custom settings are preserved. Native search uses the system search field; empty/error retry controls remain visible. |
| History filter | `today.js`, `history-filter.spec.js` | `Views/ActivityView.swift`, `ActivityTests.swift` | `/api/logs/history` | Built | Additive chore selection (empty = all); alphabetically-sorted multi-select sheet; empty-page keeps Load more so paginated matches aren't hidden |
| Day calendar | `calendar.js` (unrouted) | — | `/api/logs/today`, `/api/schedules/for-date` | Deferred | Removed from the Activity tab on both clients (PWA `e9a9527`); iOS DayView removed to match. PWA retains unrouted `renderCalendarView` code |
| Week calendar | `calendar.js` (unrouted) | — | `/api/logs/week`, `/api/schedules/for-date` | Deferred | As above; iOS WeekView removed |
| Ad-hoc log placement | `calendar.js`, `log-from-slot.spec.js` | — | `/api/logs` | Deferred | Was calendar-only; removed with the calendar |
| **Schedule** |
| Schedule CRUD | `schedule.js`, `schedule-tab.spec.js` | `Views/ScheduleView.swift`, `ScheduleTests.swift`, `NabuUITests.swift` | `/api/schedules`, `/api/schedules/{id}` | Built | Both empty states offer a direct Schedule a chore action; cancel returns to the empty state. Native screen tests also cover recurring create/reload and the retained inclusive end date. |
| Recurrence logic | `calendar.js`, `schedule.spec.js` | `API/ScheduleStore.swift`, `ScheduleTests.swift` | N/A (client-side) | Built | Both clients encode inclusive end dates as UTC-midnight timestamps. Native editing and summaries retain the selected calendar day across time zones. |
| Pick chore sheet | `schedule.js`, `schedule-tab.spec.js` | `Views/ScheduleView.swift` | `/api/chores`, `/api/schedules` | Built | |
| Schedule edit (sparse PATCH) | `schedule.js`, `schedule-tab.spec.js` | `API/ScheduleStore.swift`, `RequestEncodingTests.swift` | `/api/schedules/{id}` | Built | PWA schedule mutations validate positive integer IDs before URL construction (including external drag data); native ScheduleStore already accepts typed Int IDs. Unchecking the end date sends explicit null; native and browser persistence regressions cover setting and clearing it. |
| Schedule for-date overlay | `schedule.js` (`loadSchedulesForDate`, unrouted) | — | `/api/schedules/for-date` | Deferred | P3 verified the PWA's only consumer is `loadTodayWithSchedules` in the unrouted calendar-era code (removed from the Activity tab, PWA `e9a9527`) — there is no live surface to mirror. Revisit if the PWA re-routes it |
| **Chores Management** |
| Chore CRUD | `chores.js`, `chores-management.spec.js` | `Views/ManageChoresView.swift`, `Views/ChoreEditView.swift`, `ChoreTests.swift` | `/api/chores`, `/api/chores/{id}` | Built | |
| Seed defaults | `chores.js`, `chores-management.spec.js` | `Views/ManageChoresView.swift` | `/api/chores/defaults`, `/api/chores/seed-defaults` | Built | |
| Restore default | `chores.js`, `chores-management.spec.js` | `Views/ManageChoresView.swift` | `/api/chores/{id}/restore-default` | Built | |
| Indicator editing | `chores.js`, `chores-management.spec.js` | `Views/ChoreEditView.swift`, `ChoreTests.swift` | `/api/chores/{id}` | Built | |
| Color/emoji pickers | `chores.js`, `chores-management.spec.js` | `Views/ChoreEditView.swift` | `/api/chores/{id}` | Built | |
| Validation | `chores.js`, `security-escape.spec.js` | `Views/ChoreEditView.swift`, `ChoreTests.swift` | `/api/chores`, `/api/chores/{id}` | Built | |
| Per-chore metric config | `chores.js`, `app.js` | `Views/ChoreEditView.swift`, `RequestEncodingTests.swift` | `/api/chores`, `/api/chores/{id}` (`metricType`,`metricUnit`) | Built | Explicit "Track a value" picker (`metricType` ∈ {none,amount,rating,duration}, `metricUnit` for amount) replaces the implicit `hasVolumeML`/`hasRating` flags, which stay in the request for server compat |
| Duration metric value | `today.js`, `app.js` | `Views/LogSheet.swift`, `API/LogStore.swift`, `RequestEncodingTests.swift` | `/api/logs` (`durationSeconds`) | Built | Logs carry optional `durationSeconds`; set by the log-sheet timer on Stop & log |
| Duration timer mode | `timer.js`, `app.js`, `schedule.js` | `Support/DurationTimer.swift`, `Views/TimerChipView.swift`, `DurationTimerTests.swift` | `/api/logs` (`durationSeconds`) | Built | "Start timer" on the log sheet; persistent elapsed-time chip overlays all tabs; "Stop & log" completes with `durationSeconds`. Native timer state persists in an owner-scoped protected file (PWA: localStorage), including stopped-save recovery. Screen regressions cover failed stop, relaunch and successful retry; its accessibility label identifies Retry save |
| **Notifications** |
| In-app notification list | `notifications.js`, `notification-data.js`, `notification-history.spec.js`, `runner.js` | `Views/NotificationsView.swift`, `API/Data/NotificationDataLoader.swift`, `NotificationTests.swift` | `/api/notifications?cursor=` | Built | Both show read and unread receipts newest first, 50 per page with stable timestamp/ID cursor, visible failure/retry and stale request/mutation guards. Open panels retain loaded pages until Refresh. Receipts stay until user/account deletion; marking read keeps them. |
| Mark read / read all | `notifications.js`, `notifications.spec.js` | `Views/NotificationsView.swift` | `/api/notifications/{id}/read`, `/api/notifications/read-all` | Built | Shared loader invalidates pending pages before mutations; errors retain the row and actions for retry. The unread count only decrements for a previously unread receipt. |
| Delete notification | `notifications.js`, `notifications.spec.js` | `Views/NotificationsView.swift` | `/api/notifications/{id}` | Built | Shared loader invalidates pending pages before mutations; errors retain the row and actions for retry. The unread count only decrements for a previously unread receipt. |
| Notification preferences | `notifications.js`, `settings-notification-prefs.spec.js` | `Views/NotificationPreferencesView.swift` | `/api/notification-preferences` | Built | |
| **Push** |
| Web Push (VAPID) | `notifications.js` | N/A (PWA only) | `/api/push/subscribe`, `/api/push/unsubscribe` | N/A | PWA-only feature. Server now rejects subscriptions whose endpoint is not a known push host (https + allowlisted suffix: fcm.googleapis.com, .push.apple.com, .notify.windows.com, push.services.mozilla.com) or that lacks keys — SSRF hardening; browsers only ever produce such endpoints, so no client change |
| APNs (native iOS) | N/A (iOS only) | `App/PushRegistrationController.swift`, `App/AppDelegate.swift`, `Support/PushRegistration.swift`, `Views/PushPrePromptView.swift`, `APNsContractTests.swift`, `PushRegistrationTests.swift` | `/api/mobile/apns/register`, `/api/mobile/apns/unregister` | Built | P5 client half: permission pre-prompt (system dialog never fired cold), authorization → `registerForRemoteNotifications` → hex token → register (`sandbox` for DEBUG builds, `production` otherwise), silent re-register on launch when already authorized, server session revocation removes delivery bindings; failed logout retains its retry cookie and suspends native registration callbacks. Backend shipped in P1 (`internal/apns`). **End-to-end unverified**: needs Push capability on the App ID, `.p8` provisioned, and a physical device (simulator cannot receive remote push) |
| **Stats** |
| Overview | `stats-data.js`, `stats-context.spec.js`, `stats-performance.spec.js` | `Views/Stats/StatsModel.swift`, `StatsModelTests.swift` | `/api/stats/overview` | Built | One overview per refresh, visible sections only, scoped request/query ownership, complete widget aggregates and last-good retry on both clients. PWA navigation renders immediately and paints ready charts in batches per animation frame while slower resources load; stale page/household callbacks cannot repaint it. Native segmented controls remain visible on empty results. |
| Last done section | `stats.js`, `stats-last-done.spec.js` | `Views/Stats/StatsSectionViews.swift`, `StatsSectionsTests.swift` | `/api/logs/latest-per-chore` (reuse) | Built | Registry key `last-done`; time-since-last-log per chore, most recent first, never-logged last, reading `latestLogs` already in app state (no new fetch) — same as PWA |
| Heatmap | `stats.js`, `stats-tab.spec.js` | `Views/Stats/StatsCharts.swift` (`HeatmapChart`), `StatsSnapshotTests.swift` | `/api/stats/heatmap` | Built | Swift Charts `RectangleMark` grid, Monday-start weeks matching the PWA/server; intensity ramp from asset-catalog `Heatmap*` colors mirroring the PWA's `--heatmap-*` light/dark tokens. Per-cell tap tooltip not ported (cells too small for touch targets; counts visible via intensity + legend) |
| Chart color tokens & label palette | `stats.js` (`colorForIndicator`), `runner.js` | `Support/IndicatorColor.swift`, `IndicatorColorTests.swift` | N/A (client render) | Built | `colorForIndicator` ported: same stable hash→palette (UTF-16/imul-compatible hash, identical 12-color palette), four baby labels pinned to historical colors. Hash outputs and resolved colors unit-tested against values computed from the JS implementation |
| Busy hours | `stats.js`, `stats-busy-hours-filter.spec.js` | `Views/StatsView.swift` | `/api/stats/busy-hours` | Built | PostgreSQL groups viewer-local hours with chore/member filters before returning 24 slots; both clients retain the same response contract. |
| Leaderboard | `stats.js`, `stats-leaderboard.spec.js` | `Views/StatsView.swift` | `/api/stats/leaderboard` | Built | |
| Top chores | `stats.js`, `stats-top-chores.spec.js` | `Views/StatsView.swift` | `/api/stats/top-chores` | Built | |
| Breakdown | `stats.js`, `stats-tab.spec.js` | `Views/StatsView.swift` (categories section), `StatsModelTests.swift` | `/api/stats/breakdown?period=` | Built | Categories section fetches `/api/stats/breakdown?period=day\|week\|month` with a segmented period control; empty results stay empty for the selected period, with an explicit error/retry state on failure |
| Streaks | `stats.js`, `stats-tab.spec.js` | `Views/StatsView.swift` (overview cards) | `/api/stats/overview` (reuse) | Built | Like the PWA, streaks render in the overview cards from `/api/stats/overview`; neither client calls the standalone `/api/stats/streaks` endpoint |
| Recap | `stats.js`, `stats-tab.spec.js` | `Views/StatsView.swift` (recap section) | `/api/stats/overview` (reuse) | Built | Registry key `recap`; weekly recap card renders from `overview.recap` when `totalChores > 0` — same as PWA, which also doesn't call standalone `/api/stats/recap` |
| Chore stats | `stats.js`, `stats-top-chores.spec.js` | `Views/StatsView.swift`, `Views/Stats/StatsModel.swift`, `StatsModelTests.swift` | `/api/stats/chores?period=`, `/api/stats/chores/{id}`, `/api/stats/chores/{id}/time-series` | Built | **#84/#85 converged (P4):** iOS Chores section now sends `?period=day\|week\|month` (default month) via a segmented control and renders `totalInRange` counts, matching the PWA's period-scoped semantics. The old iOS start/end date-range pickers were replaced by the period toggle |
| Feeding gaps | `stats.js` | `Views/Stats/StatsCharts.swift` (`FeedingGapsScatter`), `StatsSectionViews.swift`, `StatsSnapshotTests.swift` | `/api/stats/feeding-gaps` | Built | Cluster-feeding scatter in the baby section: same dot classification (small top-off / close feed / full feed, identical thresholds and colors), 2h rule line, Day/Week/2-Weeks quick ranges + date pickers with inclusive UI end → exclusive API end (`apiExclusiveEnd` parity), tap-to-reveal per-dot date/volumes, info explainer |
| Generalized per-chore analytics | `stats.js`, `app.js`, `runner.js` | `Support/StatsSections.swift`, `Views/Stats/StatsSectionViews.swift` (`ChoreAnalyticsSection`), `StatsSectionsTests.swift`, `StatsModelTests.swift` | `/api/stats/chores/{id}/time-series` (reuse) | Built | Same eligibility (`choreHasAnalytics`: metric or indicators, baby chores excluded), auto-available `chore:<id>` sections with member split + metric-appropriate chart, day/week/month toggle mapping to daily/weekly/monthly grain, hidden sections skipped, fetch fan-out capped at 15 (PWA `MAX_ANALYTICS_FETCHES`) |
| Interval analysis (choreId) | `stats.js` | `Views/Stats/StatsModel.swift`, `API/Models.swift` (`FeedingGap`) | `/api/stats/feeding-gaps?choreId=` | Built | The `choreId` generalization is server-side; neither client sends `choreId` from its UI today (default Feed Baby) — iOS matches the PWA. Time-series `totalDuration`/`metricType`/`metricUnit` are decoded and drive duration/amount charts |
| Custom stats widgets | `stats.js`, `app.js`, `preferences.js`, `runner.js` | `Views/Stats/StatsWidgetViews.swift`, `Views/Stats/StatsModel.swift`, `StatsModelTests.swift`, `StatsSnapshotTests.swift` | `/api/preferences` (`statsWidgets`), `/api/stats/chores/{id}/summary` | Built | Phase 4: user-defined widgets stored as a typed, server-validated JSON list (migration 037, `user_preferences.stats_widgets`). Closed enum schema, choreIds ownership-checked, title length-capped and rendered escaped, max 20 widgets / 4KB. Both clients load every visible saved widget, including widgets 16–20; the PWA shares identical in-flight summary reads across cards. Total/member-split widgets read a period-scoped per-chore summary (`GET /api/stats/chores/{id}/summary?period=day\|week\|month\|all`, ownership-checked, closed period enum — still no user SQL); timeseries reads the time-series endpoint. Wizard in the customize panel (multi-select chore checkboxes, presentation, value); period is **not** chosen at create — each widget card carries a day/week/month toggle that re-scopes and persists it. Sections keyed `widget:<uuid>`. **iOS (P4):** decodes `statsWidgets` (`API/Models.swift`), renders all schema types (`Views/Stats/StatsWidgetViews.swift`: total big number, timeseries chart at the widget grain, member-split from period-scoped summaries, last-done from state; interval/top-list fall back to total like the PWA), per-card day/week/month segmented toggle persisted via `statsWidgets` PATCH + refetch, native Form wizard (name ≤60, multi-select chores, presentation, value; period not chosen at create). Titles render as plain `Text` only — hostile-title snapshot test ports the PWA XSS spec intent (`StatsSnapshotTests.swift`, `StatsModelTests.swift`) |
| Stats customize panel (section order/hide) | `stats.js` (`resolveStatsLayout`, `renderCustomizePanel`), `app.js`, `preferences.js` | `Support/StatsSections.swift`, `Views/Stats/StatsWidgetViews.swift` (`CustomizeStatsView`), `StatsSectionsTests.swift`, `StatsModelTests.swift` | `/api/preferences` (`statsSectionOrder`, `statsSectionHidden`) | Built | Same layout resolution (user order → canonical registry → eligible dynamic keys; hidden excluded; stale dynamic keys dropped) over the identical 10-key registry (`internal/userprefs/sections.go`). Native `List` drag-reorder + visibility toggles instead of HTML5 drag-and-drop; reorder saves the full ordered key list like the PWA's drop handler |
| Timezone sync | `preferences.js`, `stats-timezone.spec.js` | `Support/TimeZoneSync.swift` | `/api/preferences` | Built | |
| **Baby Care** |
| Feed Baby volume | `schedule.js`, `feed-baby-volume.spec.js`, `log-amount-layout.spec.js` | `Views/LogSheet.swift`, `Views/StatsView.swift` | `/api/logs` | Built | PWA plain amount logs (including medications) group the numeric input and unit selector side by side with aligned labels and 44px controls; recent amounts sit directly below. Indicator volumes retain their shared unit selector. Checked at narrow widths and 200% zoom. iOS retains native Form presentation with the same independent amount/unit behavior; no API or model changes. |
| Offline log queue + idempotency | `today.js`, `offline-queue.js`, `offline-log-queue.spec.js` | `Support/OfflineLogQueue.swift`, `API/LogStore.swift`, `OfflineLogQueueTests.swift` | `/api/logs` (`idempotencyKey`) | Built | Every log POST carries a client `idempotencyKey`; network failures enqueue to a file-backed queue replayed on foreground and connectivity restore (`NWPathMonitor`). Same replay contract as the PWA: 2xx/permanent-4xx removes, 5xx/429 keeps, network failure stops the pass  The shared server now binds a key to the original actor and payload, checks current access on both replay paths, and resumes failed follow-ups once. Client recovery from retained 4xx entries is tracked in the September review implementation. |
| CSV log export | `exports.js`, `app.js`, `export-logs.spec.js`, `export-recovery.spec.js` | `API/ActivityStore.swift`, `Views/HouseholdView.swift`, `ActivityTests.swift` | `/api/logs/export?start&end&choreId` | Built | Both clients offer inclusive date filters, visible server/network/size errors, cancel and retry. Complete CSV only: 10,000 rows, 16 MiB, 20 seconds; 2 server exports globally and 1 per household. Late identity/canceled responses cannot create downloads; native uses a protected temporary share file. |
| Admin household data CSV export | `app.js` (Settings), `admin-data-export.spec.js` | `API/ActivityStore.swift`, `Views/HouseholdView.swift`, `ActivityTests.swift` | `/api/household/data` | Built | Same date/cancel/error contract as log export; dates filter logs and day notes while current household metadata remains included. Aggregate 10,000-record cap, no partial success. |
| Change Baby indicators | `schedule.js`, `feed-baby-volume.spec.js` | `Views/LogSheet.swift` | `/api/logs` | Built | |
| Subject tagging (multi-baby) | `chores.js`, `schedule.js`, `today.js`, `app.js` | `Views/LogSheet.swift`, `Views/ActivityView.swift`, `Views/ChoreEditView.swift`, `RequestEncodingTests.swift` | `/api/chores` (`subjects`), `/api/logs` (`subject`) | Built | Chore editor edits `subjects`; log sheet shows a single-select subject chip row; history rows show the tag. Deselecting on edit sends explicit JSON `null` to clear (wire-format tested) |
| Volume prefill | `schedule.js`, `app.js`, `feed-baby-volume.spec.js`, `log-sheet-prefill-scope.spec.js` | `Views/LogSheet.swift` | `/api/logs` | Built | A new log sheet for a volume-metric indicator chore (Feed Baby pattern) echoes the latest log's own indicator selection (type) and only prefills volumes for those types — a cached volume for a type the user never selected is never shown (PWA gates per-label at render via `cachedIndicators`; iOS mirrors by seeding `selectedIndicators` from the latest log and filtering the volume map). Falls back to chore `indicatorDefaults` with no volumes when there is no prior log. Plain chip chores without volume (e.g. a custom Laundry) always start from `indicatorDefaults` and never echo the previous log's selection |
| Recent-value chips | `schedule.js`, `app.js`, `utils.js`, `recent-volume-chips.spec.js`, `feed-baby-volume.spec.js` | `Support/RecentVolumes.swift`, `Views/LogSheet.swift`, `RecentVolumesTests.swift` | `/api/logs` (client render) | Built | Last 3 distinct volumes as tappable chips above the volume picker, newest first. For indicator-volume chores, tapping a chip fills only the currently selected type(s) and never changes type selection |
| **Preferences** |
| Chore order | `preferences.js`, `home-jiggle-grid.spec.js` | `API/`, `RequestEncodingTests.swift` | `/api/preferences` | Built | |
| Hidden home chores | `preferences.js`, `home-remove-chore.spec.js` | `API/`, `RequestEncodingTests.swift` | `/api/preferences` | Built | |
| Timezone | `preferences.js`, `stats-timezone.spec.js` | `Support/TimeZoneSync.swift` | `/api/preferences` | Built | |
| Volume unit (mL/oz) | `preferences.js`, `utils.js`, `settings-volume-unit.spec.js` | `Support/VolumeUnits.swift`, `Views/HouseholdView.swift`, `VolumeUnitsTests.swift` | `/api/preferences` (`volumeUnit`) | Built | Settings toggle synced via `/api/preferences`; volumes stored canonically in mL, converted at display/input in LogSheet, history rows, and stats charts. Round-trip unit-tested against the JS cases |
| Hide notification badge | `preferences.js`, `app.js`, `notifications-hide-badge.spec.js` | `Views/HouseholdView.swift`, `ContentView.swift`, `ModelDecodingTests.swift`, `RequestEncodingTests.swift` | `/api/preferences` (`hideNotificationBadge`) | Done | Notifications keep accumulating server-side; only the unread count display is suppressed (PWA bell badge / iOS tab `.badge` + Settings-row capsule). PWA toggle lives in Settings → Preferences card; iOS toggle lives in Household → Notifications section (native placement) |
| History text search | `today.js`, `activity-search.spec.js` | `Views/ActivityView.swift`, `API/ActivityStore.swift` | `/api/logs/history?q=` | Built | Native `.searchable` field, debounced; flat capped newest-first results. Search mode hides the chore filter, pending rows, and pagination (PWA semantics) |
| Per-day diary notes | `today.js`, `app.js`, `day-notes.spec.js` | `Views/ActivityView.swift` (`DayNoteSheet`) | `/api/day-notes`, `/api/day-notes/{date}` | Built | Note affordance on Activity day headers; edit sheet with 500-char cap; empty clears |
| History infinite scroll + day counts | `today.js`, `app.js`, `history-pagination.spec.js` | `Views/ActivityView.swift` | `/api/logs/history` (reuse) | Built | iOS auto-loads the next page when the tail row appears (`onAppear` sentinel; Load-more button kept as fallback) and shows per-day count chips in headers |
| Pull-to-refresh | `app.js` (`setupPullToRefresh`) | all tab views (`.refreshable`) | N/A (client UX) | Built | Native `.refreshable` on every tab's scroll view (Stats, Activity, Home, Schedule, Settings, Notifications), refetching that tab's data |
| PWA manifest shortcuts / iOS quick actions | `manifest.webmanifest`, `app.js` (`?quicklog=`) | `App/AppDelegate.swift` (shortcuts + SceneDelegate), `Support/DeepLink.swift`, `DeepLinkTests.swift` | N/A (install surface) | Built | P6: iOS ships the native counterpart — Home-Screen quick actions Log Feed / Log Chore / Activity with the same three targets as the PWA's manifest shortcuts; `DeepLink` parses all four `?quicklog=` forms identically on both clients |
| **Navigation** |
| Five tabs | `app.js`, `nav-tabs-position.spec.js` | `App/NavigationModel.swift`, `NabuUITests.swift` | N/A (client routing) | Built | |
| Tab order (Stats, Activity, Home, Schedule, Settings) | `app.js`, `nav-tabs-position.spec.js` | `App/NavigationModel.swift`, `NabuUITests.swift` | N/A (client routing) | Built | Same tab set/order as PWA |
| **Log Member Attribution** |
| Log by member | `schedule.js`, `log-member-attribution.spec.js` | `Views/HomeView.swift`, `Views/LogSheet.swift` | `/api/logs` | Built | |
| **Security** |
| Escaping user content | `utils.js`, `security-escape.spec.js` | SwiftUI `Text` (auto-escapes) | N/A (client rendering) | Built | SwiftUI does not interpret markup, so HTML-escaping is not applicable |
| CSRF protection | `api.js` | `API/CSRFTokenProvider.swift`, `APIContractTests.swift` | All state-changing endpoints | Built | |
| **Schedule Reminders** |
| Schedule reminder notification type | `notifications.js`, `settings-notification-prefs.spec.js` | `Views/NotificationPreferencesView.swift`, `NotificationTests.swift` | `/api/notification-preferences` | Built | |
| Reminder "Log now" action | `service-worker.js`, `app.js`, `quicklog-deeplink.spec.js` | `App/AppDelegate.swift` (`NABU_REMINDER` category), `Views/HomeView.swift`, `DeepLinkTests.swift` | push payload (`choreId`,`type`,`category`) | Built | P5: reminder push now carries `category: NABU_REMINDER` (mapped to `aps.category`; ignored by the SW); the iOS action opens the log sheet pre-filled for the chore — same outcome as the PWA's `/?quicklog=chore:<id>`, routed in-process instead of via URL. Missing chore falls back to the home grid like the PWA |
| Reminder "Snooze 30m" action | `service-worker.js`, `reminder_snooze.go` | `App/AppDelegate.swift`, `APNsContractTests.swift` | `/api/reminders/snooze` | Built | P5: background notification action POSTs `{choreId, minutes: 30}` without opening the app — same CSRF-exempt, session-authenticated call the PWA service worker makes |
| Offline pending log badge | `today.js`, `app.js`, `offline-queue.js` | `Views/ActivityView.swift`, `Support/OfflineLogQueue.swift`, `OfflineLogQueueTests.swift` | N/A (client UX) | Built | Queued logs render inline in Activity with a non-tappable "pending" badge (synthesized from the queued body), reconciled on the next successful replay — same UX as the PWA |
| iOS Home-Screen widget / Live Activity | — | `NabuWidgets/NabuWidgets.swift`, `Support/WidgetDataCache.swift` | `/api/logs/latest-per-chore` (reuse, via app-group cache) | Built | P6: "Last Logged" WidgetKit widget (small + medium) showing time since each chore's last log, fed by an app-group snapshot refreshed whenever latest-per-chore loads; taps deep-link to the pre-filled log sheet. No PWA counterpart (native-only value). **Live Activity for the duration timer: deferred to v1.1** per the P6 plan — owner sign-off requested. Owner-gated: app group on the App ID before device builds |
| Per-chore reminder pref | `chores.js`, `app.js` | `Views/ChoreEditView.swift`, `ModelDecodingTests.swift` | `/api/chore-reminder-prefs`, `/api/chore-reminder-prefs/{id}` | Done | Update 403s for a `choreId` outside the caller's household (audit #10 hygiene) — no client-visible change for legitimate flows |
| Default lead time in settings | `notifications.js`, `settings-notification-prefs.spec.js` | `Views/NotificationPreferencesView.swift` | `/api/notification-preferences` | Done | |
| Schedule done visual (amber bg) | `schedule-tab.js`, `app.css` | `Views/ScheduleView.swift` | N/A (client rendering) | Done | |
| Once schedules not crossed out | `schedule-tab.js` | `Views/ScheduleView.swift` | N/A (client rendering) | Done | |
| followUpTime in log request | `today.js`, `app.js` | `Views/LogSheet.swift`, `API/RequestModels.swift`, `RequestEncodingTests.swift` | `/api/logs` | Done | |
| followUpEnabled in chore request | `chores.js`, `app.js` | `API/RequestModels.swift`, `RequestEncodingTests.swift` | `/api/chores` | Done | |
| **Private Household Tasks** |
| Private (Admins-only) tasks | `chores.js`, `app.js`, `today.js`, `schedule.js`, `schedule-tab.js`, `calendar.js`, `stats.js`, `preferences.js`, `notifications.js`, `service-worker.js`, `private-tasks.spec.js` | `Views/ChoreEditView.swift`, `Views/ManageChoresView.swift`, `API/Models.swift`, `API/RequestModels.swift`, `API/ChoreStore.swift`, `NabuTests/ChoreVisibilityTests.swift` | `/api/chores` (`visibility`), `/api/logs`, `/api/logs/history`, `/api/logs/export`, `/api/logs/latest-per-chore`, `/api/schedules`, `/api/schedules/for-date`, `/api/chore-reminder-prefs`, `/api/reminders/snooze`, `/api/preferences`, `/api/stats/*`, `/api/household/data` | Built | Visibility lookup failures return sanitized errors for collections, aggregates and preferences; idempotency replays reauthorize the stored log. Role-based visibility `household` (default) vs `admins` (Admins only); server-authoritative filtering for every derived surface (logs, history/search, export, latest-per-chore, schedules, reminders/snooze, notifications, stats, prefs, household export); admin-only create/transition with confirmations; member schedule assignments cleared on transition; iOS picker + confirmation parities PWA |
| **Service Worker** |
| Update/reload | `index.html`, `sw-update-reload.spec.js` | N/A (native app) | N/A | N/A | PWA stylesheets and JavaScript use release-versioned URLs so the first reload after deployment cannot combine current code with CSS from the previous service-worker cache. Browser regression covers a cached previous stylesheet. Native apps use App Store updates. |
| **Web & Marketing** |
| Marketing homepage at "/" | `web/templates/home.html`, `marketing-home.spec.js` | N/A (native app has no web marketing surface) | N/A (client surface) | N/A | PWA-only: "/" serves server-rendered marketing HTML to anonymous visitors and the SPA app shell to authenticated users; the legacy `/home` URL canonicalizes to "/" via `<link rel="canonical">` + `og:url`. All server-rendered HTML (`index.html`, `home.html`, `/privacy`, `/support`) ships `Cache-Control: no-store`. iOS is unaffected — it never loads the web root |

## Real iOS test inventory

The repository currently contains these iOS test targets (all under `ios/`):

| Target | Files |
|--------|-------|
| `NabuTests` (unit/contract) | `ActivityTests`, `APIContractTests`, `AuthTests`, `ChoreTests`, `DataLoaderTests`, `HomeTests`, `ModelDecodingTests`, `NotificationTests`, `RequestEncodingTests`, `ScheduleTests`, `StateTests` |
| `NabuUITests` (UI) | `NabuUITests.swift` |

Earlier revisions of this matrix referenced a large set of per-feature
`*UITests.swift` / `*SnapshotTests.swift` / `*ContractTests.swift` files that
were planned but never created. Add real test files before promoting a row from
**Built** to **Done**.

Shared history/statistics read update (review PERF-1): both clients receive
permission-filtered search limits and older-history existence, stable latest-log
ties, and equivalent SQL aggregates. Native/PWA API models are unchanged; the
shared API and PostgreSQL differential regressions cover the response contract.
The new `history-visibility.spec.js` covers browser pagination/search through a
private-only first page. PWA empty-page rendering retains Load more, and both
clients describe the loaded range instead of claiming all history is empty.
The PostgreSQL-backed browser run passed 12 focused History/Stats tests; native
SwiftUI validation still requires the macOS lane.

Stats large-history performance: both clients benefit from selective per-chore
metric reads and PostgreSQL heatmap/hour aggregates, with unchanged JSON models
and date/member/visibility semantics. The PWA additionally paints ready sections
while slower requests finish; the native loading presentation is unchanged.
`aggregate_postgres_test.go`, `performance_test.go` and `stats-performance.spec.js`
cover the shared results, workload and browser navigation behavior.
