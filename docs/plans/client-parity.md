# Client Parity Matrix

Living matrix tracking feature parity between the PWA and native iOS app.

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
- **Not built** — A contract/stub exists but the feature is non-functional
  end-to-end (currently APNs only).
- **N/A** — Feature applies to only one client (with justification).

## Phase progress

| Phase | Description | Status |
|-------|-------------|--------|
| 0 | Guardrails and parity infrastructure | Done |
| 1 | iOS project skeleton | Done |
| 2 | API models and contract tests | Done |
| 3 | Auth, session, and onboarding | Built (email verification pending) |
| 4 | Bootstrap data loading and preferences | Built |
| 5 | Home and log sheet | Built |
| 6 | Chores management | Built |
| 7 | Activity history (day/week calendar removed to match PWA) | Built |
| 8 | Schedule | Built (schedules/for-date overlay pending) |
| 9 | Household, members, and multi-household | Built |
| 10 | Notifications (in-app) | Built |
| 10b | APNs native push | **Not built** — see notes |
| 11 | Stats | Built (breakdown / streaks / recap / feeding-gaps pending) |
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
| Login/register | `auth.js`, `validation.spec.js` | `Auth/LoginView.swift`, `Auth/RegisterView.swift`, `AuthTests.swift` | `/api/auth/login`, `/api/auth/register` | Built | |
| Magic link | `auth.js`, `magic-link.spec.js` | `Auth/MagicLinkView.swift`, `AuthTests.swift` | `/api/auth/magic-link/request`, `/api/auth/magic-link/consume` | Built | |
| Password reset | `auth.js`, `settings-auth.spec.js` | `Auth/`, `APIContractTests.swift` | `/api/auth/password/forgot`, `/api/auth/password/reset`, `/api/auth/password` | Built | iOS has no dedicated PasswordResetView; reset is wired via the auth store |
| Email verification | `auth.js`, `magic-link.spec.js` | — | `/api/auth/email/verify`, `/api/auth/email/verification/resend` | **iOS pending** | Endpoints not referenced anywhere in iOS source |
| Google OAuth | `auth.js` | `Auth/GoogleOAuthCoordinator.swift` | `/api/auth/google/login`, `/api/auth/google/callback` | Built | |
| Logout | `auth.js` | `Auth/AuthStore.swift`, `AuthTests.swift` | `/api/auth/logout` | Built | |
| Session bootstrap | `app.js` | `App/AppState.swift`, `API/APIClient.swift`, `StateTests.swift` | `/api/me` | Built | iOS adds a CSRF pre-flight `GET /api/me` |
| **Household & Members** |
| Household CRUD | `household.js`, `household-multi.spec.js` | `Views/HouseholdView.swift` | `/api/household`, `/api/households`, `/api/households/{id}/activate` | Built | |
| Join by invite code | `household.js`, `invite-link.spec.js` | `Views/HouseholdView.swift` | `/api/household/join` | Built | |
| Invite management | `household.js`, `invite-link.spec.js` | `Views/HouseholdView.swift` | `/api/household/invites`, `/api/household/invites/{id}` | Built | |
| Member roles | `household.js`, `household-roles.spec.js` | `Views/HouseholdView.swift` | `/api/household/members/{userId}`, `/api/household/transfer` | Built | |
| Remove member | `household.js`, `settings-remove-member.spec.js` | `Views/HouseholdView.swift` | `/api/household/members/{userId}` | Built | |
| Leave household | `household.js`, `household-multi.spec.js` | `Views/HouseholdView.swift` | `/api/household/leave` | Built | |
| Multi-household switching | `household.js`, `household-multi.spec.js` | `Views/HouseholdView.swift` | `/api/households`, `/api/households/{id}/activate` | Built | |
| Join notifications | `notifications.js`, `household-join-notify.spec.js` | `Views/NotificationsView.swift`, `NotificationTests.swift` | `/api/notifications` | Built | |
| **Home** |
| Home grid | `home.js`, `home-grid.spec.js` | `Views/HomeView.swift`, `Views/HomeGrid.swift`, `HomeTests.swift` | `/api/logs/latest-per-chore`, `/api/logs/today` | Built | |
| Direct tap log | `today.js`, `home-time-accuracy.spec.js` | `Views/HomeView.swift`, `RequestEncodingTests.swift` | `/api/logs` | Built | |
| Log sheet (when picker) | `schedule.js`, `home-when-picker.spec.js` | `Views/LogSheet.swift`, `Views/QuickLogSheet.swift` | `/api/logs`, `/api/logs/{id}` | Built | |
| Jiggle mode reorder | `home.js`, `home-jiggle-grid.spec.js` | `Views/HomeView.swift` | `/api/preferences` | Built | |
| Hide from home | `home.js`, `home-remove-chore.spec.js` | `Views/HomeView.swift` | `/api/preferences` | Built | |
| Undo toast | `today.js`, `home-grid.spec.js` | `Views/UndoToast.swift`, `HomeTests.swift` | `/api/logs/{id}` | Built | |
| **Activity** |
| History list (paginated) | `today.js`, `history-pagination.spec.js` | `Views/ActivityView.swift`, `ActivityTests.swift` | `/api/logs/history` | Built | |
| History filter | `today.js`, `history-filter.spec.js` | `Views/ActivityView.swift` | `/api/logs/history` | Built | iOS has `historyChoreFilter` state; UI surface to re-verify |
| Day calendar | `calendar.js` (unrouted) | — | `/api/logs/today`, `/api/schedules/for-date` | Deferred | Removed from the Activity tab on both clients (PWA `e9a9527`); iOS DayView removed to match. PWA retains unrouted `renderCalendarView` code |
| Week calendar | `calendar.js` (unrouted) | — | `/api/logs/week`, `/api/schedules/for-date` | Deferred | As above; iOS WeekView removed |
| Ad-hoc log placement | `calendar.js`, `log-from-slot.spec.js` | — | `/api/logs` | Deferred | Was calendar-only; removed with the calendar |
| **Schedule** |
| Schedule CRUD | `schedule.js`, `schedule-tab.spec.js` | `Views/ScheduleView.swift`, `ScheduleTests.swift` | `/api/schedules`, `/api/schedules/{id}` | Built | |
| Recurrence logic | `calendar.js`, `schedule.spec.js` | `API/ScheduleStore.swift`, `ScheduleTests.swift` | N/A (client-side) | Built | |
| Pick chore sheet | `schedule.js`, `schedule-tab.spec.js` | `Views/ScheduleView.swift` | `/api/chores`, `/api/schedules` | Built | |
| Schedule edit (sparse PATCH) | `schedule.js`, `schedule-tab.spec.js` | `API/ScheduleStore.swift`, `RequestEncodingTests.swift` | `/api/schedules/{id}` | Built | |
| Schedule for-date overlay | `schedule.js` | — | `/api/schedules/for-date` | **iOS pending** | Endpoint not referenced in iOS |
| **Chores Management** |
| Chore CRUD | `chores.js`, `chores-management.spec.js` | `Views/ManageChoresView.swift`, `Views/ChoreEditView.swift`, `ChoreTests.swift` | `/api/chores`, `/api/chores/{id}` | Built | |
| Seed defaults | `chores.js`, `chores-management.spec.js` | `Views/ManageChoresView.swift` | `/api/chores/defaults`, `/api/chores/seed-defaults` | Built | |
| Restore default | `chores.js`, `chores-management.spec.js` | `Views/ManageChoresView.swift` | `/api/chores/{id}/restore-default` | Built | |
| Indicator editing | `chores.js`, `chores-management.spec.js` | `Views/ChoreEditView.swift`, `ChoreTests.swift` | `/api/chores/{id}` | Built | |
| Color/emoji pickers | `chores.js`, `chores-management.spec.js` | `Views/ChoreEditView.swift` | `/api/chores/{id}` | Built | |
| Validation | `chores.js`, `security-escape.spec.js` | `Views/ChoreEditView.swift`, `ChoreTests.swift` | `/api/chores`, `/api/chores/{id}` | Built | |
| **Notifications** |
| In-app notification list | `notifications.js`, `notifications.spec.js` | `Views/NotificationsView.swift`, `NotificationTests.swift` | `/api/notifications` | Built | |
| Mark read / read all | `notifications.js`, `notifications.spec.js` | `Views/NotificationsView.swift` | `/api/notifications/{id}/read`, `/api/notifications/read-all` | Built | |
| Delete notification | `notifications.js`, `notifications.spec.js` | `Views/NotificationsView.swift` | `/api/notifications/{id}` | Built | |
| Notification preferences | `notifications.js`, `settings-notification-prefs.spec.js` | `Views/NotificationPreferencesView.swift` | `/api/notification-preferences` | Built | |
| **Push** |
| Web Push (VAPID) | `notifications.js` | N/A (PWA only) | `/api/push/subscribe`, `/api/push/unsubscribe` | N/A | PWA-only feature |
| APNs (native iOS) | N/A (iOS only) | `API/RequestModels.swift` (structs only) | `/api/mobile/apns/register`, `/api/mobile/apns/unregister` (not routed) | **Not built** | `APNsRegisterRequest`/`Unregister` structs are defined but referenced nowhere; no `registerForRemoteNotifications`/`UNUserNotificationCenter` client code; backend does not route `/api/mobile/apns/*`. Non-functional end-to-end. See `docs/apns-implementation-plan.md` |
| **Stats** |
| Overview | `stats.js`, `stats-tab.spec.js` | `Views/StatsView.swift` | `/api/stats/overview` | Built | |
| Heatmap | `stats.js`, `stats-tab.spec.js` | `Views/StatsView.swift` | `/api/stats/heatmap` | Built | |
| Busy hours | `stats.js`, `stats-busy-hours-filter.spec.js` | `Views/StatsView.swift` | `/api/stats/busy-hours` | Built | |
| Leaderboard | `stats.js`, `stats-leaderboard.spec.js` | `Views/StatsView.swift` | `/api/stats/leaderboard` | Built | |
| Top chores | `stats.js`, `stats-top-chores.spec.js` | `Views/StatsView.swift` | `/api/stats/top-chores` | Built | |
| Breakdown | `stats.js`, `stats-tab.spec.js` | — | `/api/stats/breakdown` | **iOS pending** | Endpoint not referenced in iOS |
| Streaks | `stats.js`, `stats-tab.spec.js` | — | `/api/stats/streaks` | **iOS pending** | Endpoint not referenced in iOS |
| Recap | `stats.js`, `stats-tab.spec.js` | — | `/api/stats/recap` | **iOS pending** | Endpoint not referenced in iOS |
| Chore stats | `stats.js`, `stats-top-chores.spec.js` | `Views/StatsView.swift` | `/api/stats/chores`, `/api/stats/chores/{id}`, `/api/stats/chores/{id}/time-series` | Built | |
| Feeding gaps | `stats.js` | — | `/api/stats/feeding-gaps` | **iOS pending** | Endpoint not referenced in iOS |
| Timezone sync | `preferences.js`, `stats-timezone.spec.js` | `Support/TimeZoneSync.swift` | `/api/preferences` | Built | |
| **Baby Care** |
| Feed Baby volume | `schedule.js`, `feed-baby-volume.spec.js` | `Views/LogSheet.swift`, `Views/StatsView.swift` | `/api/logs` | Built | |
| Change Baby indicators | `schedule.js`, `feed-baby-volume.spec.js` | `Views/LogSheet.swift` | `/api/logs` | Built | |
| Volume prefill | `schedule.js`, `feed-baby-volume.spec.js` | `Views/LogSheet.swift` | `/api/logs` | Built | |
| **Preferences** |
| Chore order | `preferences.js`, `home-jiggle-grid.spec.js` | `API/`, `RequestEncodingTests.swift` | `/api/preferences` | Built | |
| Hidden home chores | `preferences.js`, `home-remove-chore.spec.js` | `API/`, `RequestEncodingTests.swift` | `/api/preferences` | Built | |
| Timezone | `preferences.js`, `stats-timezone.spec.js` | `Support/TimeZoneSync.swift` | `/api/preferences` | Built | |
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
| Per-chore reminder pref | `chores.js`, `app.js` | `Views/ChoreEditView.swift`, `ModelDecodingTests.swift` | `/api/chore-reminder-prefs`, `/api/chore-reminder-prefs/{id}` | Done | |
| Default lead time in settings | `notifications.js`, `settings-notification-prefs.spec.js` | `Views/NotificationPreferencesView.swift` | `/api/notification-preferences` | Done | |
| Schedule done visual (amber bg) | `schedule-tab.js`, `app.css` | `Views/ScheduleView.swift` | N/A (client rendering) | Done | |
| Once schedules not crossed out | `schedule-tab.js` | `Views/ScheduleView.swift` | N/A (client rendering) | Done | |
| followUpTime in log request | `today.js`, `app.js` | `Views/LogSheet.swift`, `API/RequestModels.swift`, `RequestEncodingTests.swift` | `/api/logs` | Done | |
| followUpEnabled in chore request | `chores.js`, `app.js` | `API/RequestModels.swift`, `RequestEncodingTests.swift` | `/api/chores` | Done | |
| **Service Worker** |
| Update/reload | `sw-update-reload.spec.js` | N/A (native app) | N/A | N/A | PWA-only; native apps use App Store updates |

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
