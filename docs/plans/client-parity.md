# Client Parity Matrix

Living matrix tracking feature parity between the PWA and native iOS app.

## Phase progress

| Phase | Description | Status |
|-------|-------------|--------|
| 0 | Guardrails and parity infrastructure | Done |
| 1 | iOS project skeleton | Pending |
| 2 | API models and contract tests | Pending |
| 3 | Auth, session, and onboarding | Pending |
| 4 | Bootstrap data loading and preferences | Pending |
| 5 | Home and log sheet | Pending |
| 6 | Chores management | Pending |
| 7 | Activity history, day, and week | Pending |
| 8 | Schedule | Pending |
| 9 | Household, members, and multi-household | Pending |
| 10 | Notifications and APNs | Pending |
| 11 | Stats | Pending |
| 12 | Security, accessibility, and polish | Pending |
| 13 | Release readiness | Pending |

## How to use

When implementing an iOS feature or changing PWA behavior, update the corresponding row. Set the parity status to one of:

- **Done** — Both clients implement the feature with test coverage.
- **iOS pending** — Not yet implemented in the native app.
- **PWA pending** — PWA change needed to match iOS.
- **N/A** — Feature applies to only one client (with justification).

## Feature matrix

| Feature | PWA module/specs | iOS module/tests | Shared API | Parity | Known differences |
|---------|-----------------|------------------|------------|--------|-------------------|
| **Auth & Onboarding** |
| Login/register | `auth.js`, `validation.spec.js`, `settings-auth.spec.js` | `Auth/LoginView.swift`, `Auth/RegisterView.swift`, `AuthUITests.swift`, `AuthContractTests.swift` | `/api/auth/login`, `/api/auth/register` | iOS pending | |
| Magic link | `auth.js`, `magic-link.spec.js` | `Auth/MagicLinkView.swift`, `MagicLinkUITests.swift` | `/api/auth/magic-link/request`, `/api/auth/magic-link/consume` | iOS pending | |
| Password reset | `auth.js`, `settings-auth.spec.js` | `Auth/PasswordResetView.swift`, `AccountSettingsUITests.swift` | `/api/auth/password/forgot`, `/api/auth/password/reset`, `/api/auth/password` | iOS pending | |
| Email verification | `auth.js`, `magic-link.spec.js` | `Auth/MagicLinkView.swift`, `AuthContractTests.swift` | `/api/auth/email/verify`, `/api/auth/email/verification/resend` | iOS pending | |
| Google OAuth | `auth.js` | `Auth/GoogleOAuthCoordinator.swift` | `/api/auth/google/login`, `/api/auth/google/callback` | iOS pending | |
| Logout | `auth.js`, `validation.spec.js` | `Auth/AuthStore.swift` | `/api/auth/logout` | iOS pending | |
| Session bootstrap | `app.js` | `App/AppState.swift`, `API/APIClient.swift` | `/api/me` | iOS pending | |
| **Household & Members** |
| Household CRUD | `household.js`, `household-multi.spec.js` | `Household/`, `HouseholdSwitchingUITests.swift` | `/api/household`, `/api/households`, `/api/households/{id}/activate` | iOS pending | |
| Join by invite code | `household.js`, `invite-link.spec.js` | `Household/`, `InviteUITests.swift` | `/api/household/join` | iOS pending | |
| Invite management | `household.js`, `invite-link.spec.js` | `Household/`, `InviteUITests.swift` | `/api/household/invites`, `/api/household/invites/{id}` | iOS pending | |
| Member roles | `household.js`, `household-roles.spec.js` | `Household/`, `HouseholdRolesUITests.swift` | `/api/household/members/{userId}`, `/api/household/transfer` | iOS pending | |
| Remove member | `household.js`, `settings-remove-member.spec.js` | `Household/`, `HouseholdMembersUITests.swift` | `/api/household/members/{userId}` | iOS pending | |
| Leave household | `household.js`, `household-multi.spec.js` | `Household/`, `HouseholdSwitchingUITests.swift` | `/api/household/leave` | iOS pending | |
| Multi-household switching | `household.js`, `household-multi.spec.js` | `Household/`, `HouseholdSwitchingUITests.swift` | `/api/households`, `/api/households/{id}/activate` | iOS pending | |
| Join notifications | `notifications.js`, `household-join-notify.spec.js` | `Notifications/`, `HouseholdNotificationsUITests.swift` | `/api/notifications` | iOS pending | |
| **Home** |
| Home grid | `home.js`, `home-grid.spec.js` | `Home/`, `HomeUITests.swift`, `HomeSnapshotTests.swift` | `/api/logs/latest-per-chore`, `/api/logs/today` | iOS pending | |
| Direct tap log | `today.js`, `home-time-accuracy.spec.js` | `Home/`, `LogRequestTests.swift`, `HomeUITests.swift` | `/api/logs` | iOS pending | |
| Log sheet (when picker) | `schedule.js`, `home-when-picker.spec.js` | `Home/`, `LogRequestTests.swift`, `HomeUITests.swift` | `/api/logs`, `/api/logs/{id}` | iOS pending | |
| Jiggle mode reorder | `home.js`, `home-jiggle-grid.spec.js` | `Home/`, `HomeReorderUITests.swift` | `/api/preferences` | iOS pending | |
| Hide from home | `home.js`, `home-remove-chore.spec.js` | `Home/`, `HomeManageUITests.swift` | `/api/preferences` | iOS pending | |
| Undo toast | `today.js`, `home-grid.spec.js` | `Home/`, `HomeUITests.swift` | `/api/logs/{id}` | iOS pending | |
| **Activity** |
| History list (paginated) | `today.js`, `history-pagination.spec.js` | `Activity/`, `HistoryUITests.swift` | `/api/logs/history` | iOS pending | |
| History filter | `today.js`, `history-filter.spec.js` | `Activity/`, `HistoryFilterUITests.swift` | `/api/logs/history` | iOS pending | |
| Day calendar | `calendar.js`, `schedule.spec.js` | `Activity/`, `ActivityCalendarUITests.swift` | `/api/logs/today`, `/api/schedules/for-date` | iOS pending | |
| Week calendar | `calendar.js`, `schedule.spec.js` | `Activity/`, `ActivityCalendarUITests.swift` | `/api/logs/week`, `/api/schedules/for-date` | iOS pending | |
| Ad-hoc log placement | `calendar.js`, `log-from-slot.spec.js` | `Activity/`, `LogPlacementTests.swift` | `/api/logs` | iOS pending | |
| Log edit from calendar | `calendar.js`, `schedule.spec.js` | `Activity/`, `ActivityCalendarUITests.swift` | `/api/logs/{id}` | iOS pending | |
| Add chore from hour slot | `calendar.js`, `calendar-add-history.spec.js` | `Activity/`, `ActivityCalendarUITests.swift` | `/api/chores`, `/api/schedules`, `/api/logs` | iOS pending | |
| **Schedule** |
| Schedule CRUD | `schedule.js`, `schedule-tab.spec.js` | `Schedule/`, `ScheduleUITests.swift` | `/api/schedules`, `/api/schedules/{id}` | iOS pending | |
| Recurrence logic | `calendar.js`, `schedule.spec.js` | `Schedule/`, `RecurrenceTests.swift` | N/A (client-side) | iOS pending | |
| Pick chore sheet | `schedule.js`, `schedule-tab.spec.js` | `Schedule/`, `ScheduleUITests.swift` | `/api/chores`, `/api/schedules` | iOS pending | |
| Schedule edit (sparse PATCH) | `schedule.js`, `schedule-tab.spec.js` | `Schedule/`, `ScheduleContractTests.swift` | `/api/schedules/{id}` | iOS pending | |
| **Chores Management** |
| Chore CRUD | `chores.js`, `chores-management.spec.js` | `Chores/`, `ChoresManagementUITests.swift` | `/api/chores`, `/api/chores/{id}` | iOS pending | |
| Seed defaults | `chores.js`, `chores-management.spec.js` | `Chores/`, `ChoresManagementUITests.swift` | `/api/chores/defaults`, `/api/chores/seed-defaults` | iOS pending | |
| Restore default | `chores.js`, `chores-management.spec.js` | `Chores/`, `ChoresManagementUITests.swift` | `/api/chores/{id}/restore-default` | iOS pending | |
| Indicator editing | `chores.js`, `chores-management.spec.js` | `Chores/`, `ChoresManagementUITests.swift` | `/api/chores/{id}` | iOS pending | |
| Color/emoji pickers | `chores.js`, `chores-management.spec.js` | `Chores/`, `ChoresManagementUITests.swift` | `/api/chores/{id}` | iOS pending | |
| Validation | `chores.js`, `security-escape.spec.js` | `Chores/`, `ValidationContractTests.swift` | `/api/chores`, `/api/chores/{id}` | iOS pending | |
| **Notifications** |
| In-app notification list | `notifications.js`, `notifications.spec.js` | `Notifications/`, `NotificationsUITests.swift` | `/api/notifications` | iOS pending | |
| Mark read / read all | `notifications.js`, `notifications.spec.js` | `Notifications/`, `NotificationsUITests.swift` | `/api/notifications/{id}/read`, `/api/notifications/read-all` | iOS pending | |
| Delete notification | `notifications.js`, `notifications.spec.js` | `Notifications/`, `NotificationsUITests.swift` | `/api/notifications/{id}` | iOS pending | |
| Notification preferences | `notifications.js`, `settings-notification-prefs.spec.js` | `Settings/`, `NotificationPreferencesUITests.swift` | `/api/notification-preferences` | iOS pending | |
| **Push** |
| Web Push (VAPID) | `notifications.js` | N/A (PWA only) | `/api/push/subscribe`, `/api/push/unsubscribe` | N/A | PWA-only feature |
| APNs (native iOS) | N/A (iOS only) | `Notifications/`, `APNsContractTests.swift` | `/api/mobile/apns/register`, `/api/mobile/apns/unregister` | N/A | Requires backend APNs package (Phase 10) |
| **Stats** |
| Overview | `stats.js`, `stats-tab.spec.js` | `Stats/`, `StatsUITests.swift`, `StatsSnapshotTests.swift` | `/api/stats/overview` | iOS pending | |
| Heatmap | `stats.js`, `stats-tab.spec.js` | `Stats/`, `StatsSnapshotTests.swift` | `/api/stats/heatmap` | iOS pending | |
| Busy hours | `stats.js`, `stats-busy-hours-filter.spec.js` | `Stats/`, `BusyHoursUITests.swift` | `/api/stats/busy-hours` | iOS pending | |
| Leaderboard | `stats.js`, `stats-tab.spec.js` | `Stats/`, `StatsSnapshotTests.swift` | `/api/stats/leaderboard` | iOS pending | |
| Top chores | `stats.js`, `stats-top-chores.spec.js` | `Stats/`, `TopChoresUITests.swift` | `/api/stats/top-chores` | iOS pending | |
| Breakdown | `stats.js`, `stats-tab.spec.js` | `Stats/`, `StatsSnapshotTests.swift` | `/api/stats/breakdown` | iOS pending | |
| Streaks | `stats.js`, `stats-tab.spec.js` | `Stats/`, `StatsSnapshotTests.swift` | `/api/stats/streaks` | iOS pending | |
| Recap | `stats.js`, `stats-tab.spec.js` | `Stats/`, `StatsSnapshotTests.swift` | `/api/stats/recap` | iOS pending | |
| Chore stats | `stats.js`, `stats-top-chores.spec.js` | `Stats/`, `TopChoresUITests.swift` | `/api/stats/chores`, `/api/stats/chores/{id}`, `/api/stats/chores/{id}/time-series` | iOS pending | |
| Timezone sync | `preferences.js`, `stats-timezone.spec.js` | `Support/TimeZoneSync.swift`, `StatsTimezoneContractTests.swift` | `/api/preferences` | iOS pending | |
| **Baby Care** |
| Feed Baby volume | `schedule.js`, `feed-baby-volume.spec.js` | `Home/`, `Stats/`, `BabyCareUITests.swift`, `BabyCareUnitTests.swift` | `/api/logs` | iOS pending | |
| Change Baby indicators | `schedule.js`, `feed-baby-volume.spec.js` | `Home/`, `Stats/`, `BabyCareUITests.swift` | `/api/logs` | iOS pending | |
| Volume prefill | `schedule.js`, `feed-baby-volume.spec.js` | `BabyCareUnitTests.swift` | `/api/logs` | iOS pending | |
| **Preferences** |
| Chore order | `preferences.js`, `home-jiggle-grid.spec.js` | `Settings/`, `HomeReorderUITests.swift` | `/api/preferences` | iOS pending | |
| Hidden home chores | `preferences.js`, `home-remove-chore.spec.js` | `Settings/`, `HomeManageUITests.swift` | `/api/preferences` | iOS pending | |
| Timezone | `preferences.js`, `stats-timezone.spec.js` | `Support/TimeZoneSync.swift` | `/api/preferences` | iOS pending | |
| **Navigation** |
| Five tabs | `app.js`, `nav-tabs-position.spec.js` | `App/NavigationModel.swift`, `NavigationUITests.swift` | N/A (client routing) | iOS pending | |
| Tab order (Stats, Activity, Home, Schedule, Settings) | `app.js`, `nav-tabs-position.spec.js` | `App/NavigationModel.swift`, `NavigationUITests.swift` | N/A (client routing) | iOS pending | |
| **Log Member Attribution** |
| Log by member | `schedule.js`, `log-member-attribution.spec.js` | `Home/`, `LogAttributionUITests.swift` | `/api/logs` | iOS pending | |
| **Security** |
| Escaping user content | `utils.js`, `security-escape.spec.js` | `SecurityRenderingTests.swift` | N/A (client rendering) | iOS pending | |
| CSRF protection | `api.js` | `API/CSRFTokenProvider.swift`, `APIClientTests.swift` | All state-changing endpoints | iOS pending | |
| **Service Worker** |
| Update/reload | `sw-update-reload.spec.js` | N/A (native app) | N/A | N/A | PWA-only; native apps use App Store updates |

## Test mapping

| PWA spec | iOS target |
|----------|------------|
| `validation.spec.js` | `AuthUITests.swift`, `ValidationContractTests.swift` |
| `magic-link.spec.js` | `MagicLinkUITests.swift`, `AuthContractTests.swift` |
| `settings-auth.spec.js` | `AccountSettingsUITests.swift` |
| `home-grid.spec.js` | `HomeUITests.swift`, `HomeSnapshotTests.swift` |
| `home-time-accuracy.spec.js` | `LogRequestTests.swift`, `HomeUITests.swift` |
| `home-when-picker.spec.js` | `LogRequestTests.swift`, `HomeUITests.swift` |
| `home-remove-chore.spec.js` | `HomeManageUITests.swift` |
| `home-jiggle-grid.spec.js` | `HomeReorderUITests.swift` |
| `schedule.spec.js` | `ActivityCalendarUITests.swift`, `ScheduleContractTests.swift` |
| `log-from-slot.spec.js` | `LogPlacementTests.swift`, `ActivityCalendarUITests.swift` |
| `calendar-add-history.spec.js` | `ActivityCalendarUITests.swift` |
| `schedule-tab.spec.js` | `ScheduleUITests.swift` |
| `chores-management.spec.js` | `ChoresManagementUITests.swift` |
| `chores.spec.js` | `ChoresContractTests.swift`, `ScheduleUITests.swift` |
| `household-multi.spec.js` | `HouseholdSwitchingUITests.swift` |
| `invite-link.spec.js` | `InviteUITests.swift` |
| `household-roles.spec.js` | `HouseholdRolesUITests.swift`, `HouseholdContractTests.swift` |
| `settings-remove-member.spec.js` | `HouseholdMembersUITests.swift` |
| `household-join-notify.spec.js` | `HouseholdNotificationsUITests.swift` |
| `notifications.spec.js` | `NotificationsUITests.swift`, `APNsContractTests.swift` |
| `settings-notification-prefs.spec.js` | `NotificationPreferencesUITests.swift` |
| `history-pagination.spec.js` | `HistoryUITests.swift` |
| `history-filter.spec.js` | `HistoryFilterUITests.swift` |
| `stats-tab.spec.js` | `StatsUITests.swift`, `StatsSnapshotTests.swift` |
| `stats-top-chores.spec.js` | `TopChoresUITests.swift` |
| `stats-busy-hours-filter.spec.js` | `BusyHoursUITests.swift` |
| `stats-timezone.spec.js` | `StatsTimezoneContractTests.swift` |
| `feed-baby-volume.spec.js` | `BabyCareUITests.swift`, `BabyCareUnitTests.swift` |
| `log-member-attribution.spec.js` | `LogAttributionUITests.swift` |
| `security-escape.spec.js` | `SecurityRenderingTests.swift`, `ValidationContractTests.swift` |
| `sw-update-reload.spec.js` | No direct native equivalent. Cover app relaunch/session survival instead. |
| `nav-tabs-position.spec.js` | `NavigationUITests.swift` semantic tab checks |
| `three-fixes.spec.js` | Split into `NavigationUITests.swift`, `LogDedupTests.swift`, and native layout checks |
| `home-log-to-calendar.spec.js` | `LogPlacementTests.swift`, `HomeUITests.swift` |
