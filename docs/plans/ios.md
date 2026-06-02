# Plan: Native iOS Client Conversion

## Decisions

These decisions came from the product/architecture questions before drafting this plan.

| Area | Decision |
|------|----------|
| iOS client type | Native SwiftUI app, not a WebView wrapper |
| Backend | Use the existing Go backend and JSON API wherever possible |
| Offline behavior | Online-first, with lightweight read cache only |
| Test strategy | Maximum coverage: XCTest, XCUITest, snapshot tests, API contract tests, backend tests, and translated Playwright parity tests |
| Push notifications | Included in the first iOS release |
| Initial release parity | Full PWA feature parity before App Store release |

## Non-Goals

- Do not replace the PWA. The web app remains a first-class client.
- Do not ship a WKWebView shell as the App Store product.
- Do not fork business behavior between clients.
- Do not introduce offline writes or conflict resolution in the first release.
- Do not add bearer-token authentication unless a later security review explicitly chooses it. The native client must initially work with the existing session-cookie and CSRF model.

## Core Rule For Future Agents

Every feature, bug fix, validation change, security fix, API change, or UI behavior change must be evaluated for both clients.

When an agent changes the PWA, it must check whether the iOS app needs:

- A matching SwiftUI UI change.
- A matching API model change.
- A matching XCTest or XCUITest update.
- A matching snapshot update.
- A matching App Store capability, entitlement, or Info.plist change.

When an agent changes the iOS app, it must check whether the PWA needs:

- A matching JavaScript UI/state change.
- A matching Playwright E2E test.
- A matching JS render/unit test.
- A matching backend handler/service/store change.

Do not merge client changes unless the PR description explicitly states one of:

- "PWA and iOS both updated."
- "PWA-only change; iOS not affected because <reason>."
- "iOS-only change; PWA not affected because <reason>."

Phase 0 below adds repository files that make this rule visible to future AI agents.

## Target Directory Layout

Create all iPhone code under its own top-level directory:

```text
ios/
  AGENTS.md
  README.md
  Package.swift
  Nabu.xcodeproj/
  Nabu/
    App/
      NabuApp.swift
      AppState.swift
      AppEnvironment.swift
      NavigationModel.swift
    API/
      APIClient.swift
      APIError.swift
      CookieStore.swift
      CSRFTokenProvider.swift
      Endpoints.swift
      Models/
    Auth/
      AuthStore.swift
      LoginView.swift
      RegisterView.swift
      MagicLinkView.swift
      PasswordResetView.swift
      GoogleOAuthCoordinator.swift
    Household/
    Home/
    Activity/
    Schedule/
    Chores/
    Notifications/
    Stats/
    Settings/
    DesignSystem/
      Colors.swift
      Typography.swift
      Components.swift
      BottomSheet.swift
      ToastHost.swift
    Support/
      DateFormatting.swift
      Recurrence.swift
      TimeZoneSync.swift
      TestHooks.swift
    Resources/
      Assets.xcassets
      PreviewData.json
  NabuTests/
    APIClientTests.swift
    ModelDecodingTests.swift
    RecurrenceTests.swift
    LogPlacementTests.swift
    StateTests.swift
    SnapshotTests/
  NabuUITests/
    AuthUITests.swift
    HomeUITests.swift
    ActivityUITests.swift
    ScheduleUITests.swift
    ChoresUITests.swift
    HouseholdUITests.swift
    NotificationsUITests.swift
    StatsUITests.swift
  TestSupport/
    MockAPIServer.swift
    FixtureFactory.swift
    PlaywrightParityMap.swift
```

Use Swift Package Manager for non-Xcode build/test ergonomics where possible. Keep Xcode project files in `ios/`, not at the repository root.

## Required Repository Coordination Files

Add these before implementing the app:

1. `ios/AGENTS.md`

   Include iOS-specific instructions for future agents:

   - The iOS app must mimic PWA behavior unless this plan or a follow-up product decision says otherwise.
   - Before editing iOS, inspect the corresponding PWA module and E2E spec.
   - Before editing PWA behavior, inspect the corresponding iOS screen/model/test.
   - Every iOS feature must have XCTest and XCUITest coverage.
   - Run the documented iOS tests before finishing.
   - Do not place iOS files outside `ios/` except shared docs, backend changes, or CI changes required by the iOS app.

2. Root `AGENTS.md` update

   Add a short "Client parity" section that points to this file and to `ios/AGENTS.md`. The root instructions should say that any client behavior change must consider both PWA and iOS.

3. `docs/plans/client-parity.md`

   Create a living matrix with one row per feature. Columns:

   - Feature.
   - PWA module/specs.
   - iOS module/tests.
   - Shared API endpoint.
   - Parity status.
   - Known differences.

4. PR checklist update

   Add client parity checkboxes to the PR template if one exists. If none exists, add `.github/pull_request_template.md`.

5. CI update

   Add an iOS test job once the project exists. If GitHub Actions macOS minutes are a concern, start with a required manual workflow and then make it required after tests stabilize.

## Architecture Overview

### High-Level Shape

The iOS app should be a native SwiftUI app with a thin API client and a single observable application state, mirroring the PWA's `createAppState()` pattern.

Use this layering:

```text
SwiftUI Views
  -> Feature ViewModels or reducers
  -> AppState and domain services
  -> APIClient
  -> URLSession + cookie store + CSRF header
  -> Existing Go backend
```

Keep feature logic in Swift where the PWA currently uses JavaScript state/render helpers. Keep business authority on the server.

### AppState

Create a single `@MainActor` observable app state with fields equivalent to the PWA state:

```swift
@MainActor
final class AppState: ObservableObject {
    @Published var user: User?
    @Published var household: Household?
    @Published var userHouseholds: [HouseholdWithRole] = []
    @Published var activeHouseholdId: Int?
    @Published var members: [Member] = []
    @Published var chores: [Chore] = []
    @Published var todayLogs: [ChoreLog] = []
    @Published var weekLogs: [ChoreLog] = []
    @Published var schedules: [ChoreSchedule] = []
    @Published var latestLogs: [Int: ChoreLog] = [:]
    @Published var notifications: [AppNotification] = []
    @Published var unreadNotifications = 0
    @Published var notificationPrefs: ReminderPreference?
    @Published var availableNotificationTypes: [NotificationTypeInfo] = []
    @Published var choreOrder: [Int] = []
    @Published var hiddenHomeChoreIds: [Int] = []
    @Published var currentTab: MainTab = .home
    @Published var activityView: ActivityViewMode = .history
    @Published var calendarView: CalendarViewMode = .day
    @Published var calendarDate: LocalDate?
    @Published var homeView: HomeViewMode = .log
    @Published var activeSheet: ActiveSheet?
    @Published var toast: Toast?
}
```

The state must be reset on logout and household activation. Household-scoped data must be cleared before fetching the newly active household.

### API Client

Use the existing API shape. The iOS client must handle cookies and CSRF:

- Capture `Set-Cookie` from login/register/magic-link/OAuth responses.
- Store and send `nabu_session` automatically via `URLSessionConfiguration.httpCookieStorage`.
- Read the `nabu_csrf` cookie and send it as `X-CSRF-Token` on every `POST`, `PATCH`, `PUT`, and `DELETE`.
- Send `Content-Type: application/json` for JSON requests.
- Decode JSON errors shaped as `{"error":"message"}`.
- Preserve anti-enumeration behavior. Do not show UI copy that reveals whether an email exists.

Recommended API client types:

```swift
struct APIClient {
    var baseURL: URL
    var session: URLSession
    var cookieStore: CookieStore

    func get<T: Decodable>(_ path: String, query: [URLQueryItem] = []) async throws -> T
    func post<Request: Encodable, Response: Decodable>(_ path: String, body: Request) async throws -> Response
    func patch<Request: Encodable, Response: Decodable>(_ path: String, body: Request) async throws -> Response
    func delete<Response: Decodable>(_ path: String) async throws -> Response
}
```

### Dates And Timezones

The PWA's most important date invariant is `slotHour`.

- `slotHour == nil` means the log appears in the Anytime row.
- `slotHour == 0...23` means the log appears in that hour row.
- Home-tab direct logs must use the device's current local hour.
- Home log sheet saves must derive `completedAt`, `date`, and `slotHour` from the selected date/time control.
- Calendar hour taps must pass the tapped hour.
- Schedule taps must pass the schedule hour.
- Quick-log flows without a chosen hour may omit `hour`, causing Anytime placement.

Use `TimeZone.current.identifier` to sync `/api/preferences.timezone` after authentication and household load, matching the PWA's `syncTimezone()` behavior.

Represent date-only values with a dedicated `LocalDate` type that encodes as `YYYY-MM-DD`.

### Online-First Cache

Use an online-first cache for fast launch only:

- Cache the last successful decoded state needed to render Home, Activity, Schedule, Stats, Settings, and Notifications.
- Mark cached data as stale immediately.
- Always fetch fresh data on launch and foreground.
- Do not allow offline writes in phase one.
- If offline, show cached data with a clear offline banner and disable mutating actions or let them fail with a user-facing toast.

Use SQLite or file-backed JSON. Prefer file-backed JSON first unless data size becomes a problem.

## Backend Work Required For Native iOS

Most existing endpoints can be reused. Push notifications are the major exception because native iOS uses APNs, not Web Push/VAPID.

### APNs Support

Add a native push channel alongside the existing Web Push channel.

Backend additions:

- Add migration for `apns_device_tokens`:

```sql
CREATE TABLE apns_device_tokens (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token TEXT NOT NULL,
  environment TEXT NOT NULL CHECK (environment IN ('sandbox', 'production')),
  bundle_id TEXT NOT NULL,
  device_name TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(user_id, token, environment)
);
```

- Add `internal/apns/` package:
  - Token signing for APNs provider authentication.
  - HTTP/2 client for `api.sandbox.push.apple.com` and `api.push.apple.com`.
  - Payload builder using the existing notification title/body.
  - Error handling for invalid tokens. Delete tokens on `BadDeviceToken`, `Unregistered`, and equivalent terminal errors.

- Add handlers:
  - `POST /api/mobile/apns/register` with `{ "token": string, "environment": "sandbox" | "production", "bundleId": string, "deviceName": string }`.
  - `POST /api/mobile/apns/unregister` with `{ "token": string, "environment": string }`.

- Extend `notification.Service` so `NotifyChoreLogged` and household join notifications can fan out to both Web Push and APNs senders.

- Reuse `/api/notification-preferences`. Do not create separate iOS preference types unless APNs needs a setting that Web Push does not have.

Backend tests required before iOS uses APNs:

- Handler auth and CSRF tests for register/unregister.
- Cross-user token isolation tests.
- Store tests for upsert, list by user, delete, and deletion on terminal APNs errors.
- Sender tests using a fake APNs HTTP server.
- Notification service tests proving disabled push prefs suppress APNs sends.

### Cookie And CSRF Compatibility Check

Before building iOS auth, write an integration test or a small Swift test harness that proves:

- Register/login receives both `nabu_session` and `nabu_csrf` cookies.
- A state-changing request without `X-CSRF-Token` fails.
- The same request with the cookie-derived token succeeds.
- Logout clears the session cookie.

If `nabu_csrf` cannot be read by native `URLSession` because of cookie attributes, add a safe endpoint such as `GET /api/csrf` that returns the current CSRF token only when the CSRF cookie is set. Prefer not to change this unless the native test proves it is needed.

## API Endpoint Parity Map

The native client must cover these endpoint groups.

| Feature | Existing endpoints |
|---------|--------------------|
| Session | `GET /api/me`, `POST /api/auth/login`, `POST /api/auth/register`, `POST /api/auth/logout` |
| Email auth | `/api/auth/email/verification/resend`, `/api/auth/email/verify`, `/api/auth/magic-link/request`, `/api/auth/magic-link/consume`, `/api/auth/password/forgot`, `/api/auth/password/reset`, `/api/auth/password` |
| Google OAuth | `/api/auth/google/login`, `/api/auth/google/callback` through `ASWebAuthenticationSession` |
| Household | `/api/household`, `/api/households`, `/api/households/{id}/activate`, `/api/household/join`, `/api/household/leave`, `/api/household/transfer` |
| Invites and members | `/api/household/invites`, `/api/household/invites/{id}`, `/api/household/members/{userId}` |
| Chores | `/api/chores`, `/api/chores/defaults`, `/api/chores/seed-defaults`, `/api/chores/reorder`, `/api/chores/{id}`, `/api/chores/{id}/restore-default` |
| Logs | `/api/logs`, `/api/logs/{id}`, `/api/logs/today`, `/api/logs/week`, `/api/logs/month`, `/api/logs/history`, `/api/logs/latest-per-chore` |
| Schedules | `/api/schedules`, `/api/schedules/{id}`, `/api/schedules/for-date` |
| Preferences | `/api/preferences` |
| Notifications | `/api/notifications`, `/api/notifications/read-all`, `/api/notifications/{id}/read`, `/api/notifications/{id}` |
| Notification prefs | `/api/notification-preferences` |
| Native push | New `/api/mobile/apns/register`, `/api/mobile/apns/unregister` |
| PWA push | Existing `/api/push/subscribe`, `/api/push/unsubscribe`, not used by native iOS |
| Stats | `/api/stats/overview`, `/api/stats/heatmap`, `/api/stats/busy-hours`, `/api/stats/top-chores`, `/api/stats/leaderboard`, `/api/stats/streaks`, `/api/stats/breakdown`, `/api/stats/recap`, `/api/stats/chores`, `/api/stats/chores/{id}`, `/api/stats/chores/{id}/time-series` |

## Feature Parity Inventory

### Auth And Onboarding

Native iOS must implement:

- Login with email/password.
- Register with password confirmation.
- Household creation after registration.
- Default chore seeding after household creation.
- Magic link request and consume.
- Forgot password and reset password.
- Email verification consume flow.
- Resend verification email from settings.
- Change password from settings.
- Logout.
- Join household by invite code or invite link.
- Google OAuth through `ASWebAuthenticationSession` if Google OAuth is enabled in the backend.

TDD translation:

- Translate `tests/e2e/validation.spec.js` to `AuthUITests.swift` and `OnboardingUITests.swift`.
- Translate `tests/e2e/magic-link.spec.js` using Mailpit-backed integration tests where available, plus mock API unit tests for local deterministic runs.
- Translate `tests/e2e/settings-auth.spec.js` to settings account tests.

### Main Navigation

The iOS app should have five bottom tabs matching the PWA order and meaning:

1. Stats.
2. Activity.
3. Home.
4. Schedule.
5. Settings.

Home should be the initial tab after successful onboarding/login, matching the PWA's default route behavior.

Native layout does not need to mimic PWA CSS bugs or service-worker layout workarounds, but must preserve user-visible semantics:

- Five tabs are always reachable.
- Active tab state is visually clear.
- Sheets do not hide destructive confirmations.
- Dynamic Type and VoiceOver labels are supported.

TDD translation:

- Translate `tests/e2e/nav-tabs-position.spec.js`, `tests/e2e/three-fixes.spec.js`, and `tests/e2e/stats-tab.spec.js` into XCUITest checks for tab existence, order, and active state. Do not port CSS-specific assertions literally.

### Home

Native iOS must implement:

- Home grid with chore icon, name, and latest time label.
- Progress summary for viewed day.
- Date navigation for Home day.
- Tap chore to open log sheet.
- Log with indicators, note, volume, member attribution, and When picker.
- Undo toast after log creation.
- Long-press to enter jiggle mode.
- Reorder chores in jiggle mode.
- Hide chores from Home without deleting them.
- Hidden chores persist via `/api/preferences.hiddenHomeChoreIds`.
- Chore order persists via `/api/preferences.choreOrder`.
- Manage subview for full chore management.

Critical invariants:

- Home direct logs must send `hour = Calendar.current.component(.hour, from: now)` and `completedAt = now`.
- Home sheet logs must derive `completedAt`, `date`, and `hour` from the selected When value.
- The When picker must preserve minutes and must not round to `:00`.
- A log from Home must not land in the calendar Anytime row unless the user explicitly chooses an Anytime-only path that has no hour.

TDD translation:

- Translate `home-grid.spec.js`, `home-time-accuracy.spec.js`, `home-when-picker.spec.js`, `home-remove-chore.spec.js`, and `home-jiggle-grid.spec.js`.
- Add XCTest unit tests for time label formatting and log request construction.
- Add snapshot tests for Home normal mode, log sheet, jiggle mode, hide confirmation, and empty/loading states.

### Activity: History, Day, Week

Native iOS must implement:

- Activity sub-tabs: History, Day, Week.
- History list grouped by day with paginated 7-day chunks.
- History chore filter with all/none/per-chore toggles.
- Editing logs from history.
- Day calendar with Anytime row and 24 hour rows.
- Week calendar with 7 day columns, Anytime row, and 24 hour rows.
- Ad-hoc log placement by `slotHour` in both day and week views.
- Schedule cards and logged cards in the same hour row.
- Add chore/log from an hour row.
- Long-press or context action to edit existing scheduled/logged card.
- Undo/delete log.

Critical invariants:

- `slotHour == nil` appears in Anytime.
- `slotHour == hour` appears in that hour row.
- Week view must not force ad-hoc logs into Anytime.
- A scheduled chore tap should log into the schedule's hour row.
- Adding a schedule does not create a history entry until the chore is explicitly logged.

TDD translation:

- Translate `schedule.spec.js`, `log-from-slot.spec.js`, `calendar-add-history.spec.js`, `history-pagination.spec.js`, and `history-filter.spec.js`.
- Add unit tests for `LogPlacement` and `CalendarGridBuilder`.
- Add snapshot tests for History, Day, Week, pick-chore sheet, and edit-log sheet.

### Schedule

Native iOS must implement:

- Upcoming 14-day schedule list.
- Schedule rows with chore icon, name, recurrence summary, assigned member, and time.
- Today rows with direct log action.
- Tap row to open log sheet.
- Edit action to update recurrence, time, assignment, and recurrence end.
- FAB to create schedule by picking a chore.
- Inline custom chore creation from pick-chore flow.
- Schedule delete.
- Calendar drag/drop equivalent. On iPhone, implement a native re-schedule interaction that is reliable with touch, such as drag/drop if stable or an explicit "Move to time" sheet.

Supported recurrence types:

- `once`.
- `daily`.
- `weekly` with `daysOfWeek` where `0 = Sunday`.
- `every_n_days` with `intervalDays` and `startDate`.
- `monthly_by_date`.
- `monthly_by_weekday`.
- `yearly`.

Critical invariants:

- Client-side recurrence logic must match `web/static/js/calendar.js` and backend schedule logic.
- Rescheduling must not reset `isActive` to false.
- Already scheduled chores must still be available in pick-chore sheets. Chores are repeatable.
- PATCH requests are sparse. To clear a nullable field, send explicit `null`.

TDD translation:

- Translate `schedule-tab.spec.js`, `schedule.spec.js`, and schedule-related cases from `chores.spec.js`.
- Add `RecurrenceTests.swift` porting the JS `isActiveForDayJS` cases.
- Add API contract tests for schedule create, patch, delete, and `for-date`.

### Chores Management

Native iOS must implement:

- List all chores, including hidden chores.
- Add custom chore.
- Edit chore name, icon, color, category, indicator labels, and indicator defaults.
- Delete custom chore with confirmation.
- Restore predefined chore defaults.
- Hide/show chores from Home.
- Reorder chores.
- Quick emoji and color choices.
- Volume support for Feed Baby.
- Indicator label/default editing.

Validation must mirror server rules:

- `name` required in UI, max 60 server-side.
- `icon` max 8 server-side.
- `color` must match `^#[0-9A-Fa-f]{6}$`.
- `category` max 30 and no control characters.
- Max 8 indicator labels.
- Each indicator label is 1 to 30 characters and no control characters.
- Indicator defaults must be a subset of labels.

TDD translation:

- Translate `chores-management.spec.js`, `chores.spec.js`, and validation/security cases from `security-escape.spec.js`.
- Add model/request validation unit tests for local UI validation.
- Keep server validation tests as source of truth.

### Household And Members

Native iOS must implement:

- Create household.
- Edit household name and initials.
- List all households for current user.
- Activate/switch household.
- Create tracked invite link.
- Display invite URL and code.
- Revoke invite.
- Join by invite code.
- Member list with roles and email verification state.
- Owner role management.
- Transfer ownership.
- Remove member.
- Leave household.
- Correct UI permissions for owner, admin, and member.

Critical invariants:

- Switching household clears household-scoped state before fetching new data.
- Admins cannot create invites if server forbids it.
- Members cannot see owner-only destructive controls.
- Removed users should return to the no-household onboarding state.

TDD translation:

- Translate `household-multi.spec.js`, `invite-link.spec.js`, `household-roles.spec.js`, `settings-remove-member.spec.js`, and `household-join-notify.spec.js`.
- Add API contract tests for each role-restricted endpoint.

### Notifications

Native iOS must implement:

- In-app notifications list.
- Unread badge.
- Mark one read.
- Mark all read.
- Delete notification.
- Notification preferences screen.
- Push permission request.
- APNs device token registration and unregister on logout.
- Respect `pushEnabled` and enabled notification types.

Critical invariants:

- Notification preferences apply to APNs and Web Push consistently.
- `chore_logged` notifications are not sent to the actor/logger when current server behavior excludes them.
- `household_joined` notifications are created for the appropriate household members.

TDD translation:

- Translate `notifications.spec.js`, `settings-notification-prefs.spec.js`, and `household-join-notify.spec.js`.
- Add a fake APNs sender test on the backend.
- Add XCUITest for permission prompt handling using test hooks where needed.

### Stats

Native iOS must implement full stats parity:

- Overview cards.
- Heatmap.
- Busy hours chart with chore/member/date filters.
- Leaderboard.
- Category breakdown.
- Chore stats expandable list.
- Volume chart for Feed Baby.
- Indicator chart for Change Baby.
- Weekly recap.
- Top chores with per-user filter.
- Timezone-aware bucketing through server endpoints.

Critical invariants:

- Sync `TimeZone.current.identifier` to `/api/preferences` before relying on stats.
- Stats should not appear in Settings.
- Empty states must be explicit and non-broken.
- User-controlled metadata from the server must be rendered as text, not interpreted as markup. SwiftUI `Text` already helps here, but avoid `AttributedString(markdown:)` for user content.

TDD translation:

- Translate `stats-tab.spec.js`, `stats-top-chores.spec.js`, `stats-busy-hours-filter.spec.js`, and `stats-timezone.spec.js`.
- Add snapshot tests for every chart state.
- Add unit tests for chart data normalization and date range request construction.

### Baby Care Features

Native iOS must implement:

- Feed Baby volume picker in every log sheet path.
- Volume values from 0 to 200 mL in 5 mL increments, plus empty.
- Formula/breast indicator chips.
- Change Baby indicator chips.
- Previous volume prefill for new Feed Baby logs.
- Existing log edit must use that log's own volume, not cached latest volume.
- History rows show volume and emoji-only indicator display where PWA does.
- Stats show volume and indicator charts.

Critical invariants:

- If a chore has both `hasVolumeML` and indicator labels, require both volume and at least one indicator in the UI, matching current PWA behavior.
- Editing an older log must not overwrite with a newer cached volume.

TDD translation:

- Translate `feed-baby-volume.spec.js`.
- Add unit tests for volume prefill selection and edit-log volume preservation.

## Test Strategy

### Test Pyramid

Use these layers:

1. Swift unit tests with XCTest.
2. API contract tests against a mock server and against the local Go server.
3. Snapshot tests for SwiftUI screens.
4. XCUITest user-flow tests translated from Playwright.
5. Existing Go handler/service/store tests for backend behavior.
6. Existing Playwright tests for PWA parity.

Do not rely only on XCUITest. Most edge cases should be unit or contract tests so they run quickly.

### Test Commands

Document exact commands in `ios/README.md` after project creation. Target commands should look like:

```bash
xcodebuild test \
  -project ios/Nabu.xcodeproj \
  -scheme Nabu \
  -destination 'platform=iOS Simulator,name=iPhone 16'

xcodebuild test \
  -project ios/Nabu.xcodeproj \
  -scheme NabuUITests \
  -destination 'platform=iOS Simulator,name=iPhone 16'
```

For backend and PWA parity, continue to run:

```bash
make test-go
make test-js
make local-fresh
make e2e
```

### Playwright To XCUITest Translation Rules

- Start every translated iOS test from the equivalent Playwright spec.
- Keep the original spec filename in a comment at the top of the XCUITest file.
- Use one fresh user per test, equivalent to `uniqueEmail()`.
- Prefer API setup for expensive preconditions, equivalent to Playwright `page.request` setup.
- Assert visible UI state, persisted API state, and reload/relaunch survival where the Playwright test does.
- Do not translate CSS-only assertions literally. Translate them into native layout/user semantics.
- Preserve all security and validation assertions.

### Required Test Mapping

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

## Phased Implementation Plan

Each phase must be completed with tests before moving to the next. If a phase uncovers a backend gap, add backend tests first, then implement the backend, then continue iOS work.

### Phase 0: Guardrails And Parity Infrastructure

Goal: Make future agents aware that both clients must stay coordinated.

Tasks:

1. Add `ios/AGENTS.md` with client parity instructions.
2. Update root `AGENTS.md` with a client parity section.
3. Add `docs/plans/client-parity.md` with the feature matrix.
4. Add or update `.github/pull_request_template.md` with PWA/iOS parity checkboxes.
5. Add an initial `ios/README.md` explaining the planned directory, test commands, and setup requirements.

Tests/gates:

- Documentation review only.
- Confirm links in docs point to existing files or planned files.

### Phase 1: iOS Project Skeleton

Goal: Create a buildable native SwiftUI app with empty feature shells and test targets.

Tasks:

1. Create `ios/Nabu.xcodeproj` or SwiftPM-backed project.
2. Add `Nabu` app target.
3. Add `NabuTests` unit target.
4. Add `NabuUITests` UI test target.
5. Add `AppState`, `AppEnvironment`, `APIClient`, `APIError`, `CookieStore`, and base models.
6. Add a five-tab shell with placeholder screens.
7. Add launch arguments for UI tests:
   - `-nabuBaseURL <url>`.
   - `-resetState`.
   - `-disableAnimations`.
   - `-useMockAPI` when running against `MockAPIServer`.

Tests/gates:

- Unit test: `AppState` default values match PWA initial state semantics.
- Unit test: logout reset clears authenticated and household-scoped fields.
- UI test: app launches and shows auth or placeholder Home depending on mock session.
- CI can build the iOS target on macOS.

### Phase 2: API Models And Contract Tests

Goal: Decode every server response shape before building feature screens.

Tasks:

1. Add Swift `Codable` models for User, Household, Member, Invite, Chore, ChoreLog, DailySummary, ChoreSchedule, Notification, ReminderPreference, Preferences, and all stats DTOs.
2. Add request models for auth, household, chores, logs, schedules, preferences, notifications, and APNs.
3. Add fixture JSON copied from real local API responses.
4. Add `MockAPIServer` for deterministic XCTest and XCUITest.
5. Add local Go-server contract test mode using `BASE_URL`.

Tests/gates:

- Model decoding tests for every endpoint response.
- Request encoding tests for logs, schedules, preferences, and APNs.
- Contract tests for CSRF behavior.
- Contract tests for error decoding and `Retry-After` handling on `429`.

### Phase 3: Auth, Session, And Onboarding

Goal: Native app can register, log in, keep session, create/join household, seed defaults, and log out.

Tasks:

1. Implement login/register screens.
2. Implement session bootstrap through `/api/me`.
3. Implement cookie and CSRF handling.
4. Implement household creation screen.
5. Implement default chore seeding after household creation.
6. Implement invite-code join flow.
7. Implement magic link request/consume and password reset flows.
8. Implement email verification consume flow.
9. Implement Google OAuth with `ASWebAuthenticationSession`.
10. Implement logout and session clearing.

Tests/gates:

- Translate auth Playwright specs listed above.
- Unit tests for auth state transitions.
- Contract tests for duplicate registration anti-enumeration copy.
- UI tests for failed login, successful login, register, create household, and logout.
- Relaunch test proving session survives app restart.

### Phase 4: Bootstrap Data Loading And Preferences

Goal: After auth, native app loads the same data graph as the PWA.

Tasks:

1. Implement launch data sequence:
   - `loadSession()`.
   - `loadHouseholdData()`.
   - `loadPreferences()`.
   - `syncTimezone()`.
   - `loadChoreData()`.
   - `loadTodayData()`.
   - `loadLatestLogsData()`.
   - `loadNotifData()`.
   - Lazy stats loads when Stats tab appears.
2. Implement online-first read cache.
3. Implement foreground refresh.
4. Implement offline banner.
5. Implement preferences patching for chore order, hidden home chores, and timezone.

Tests/gates:

- Unit tests for bootstrap order using a mock API.
- Unit tests for timezone sync no-op and patch cases.
- UI test for cached data display while offline.
- Contract tests for `/api/preferences`.

### Phase 5: Home And Log Sheet

Goal: Home is fully usable and respects all log placement/time invariants.

Tasks:

1. Implement Home grid.
2. Implement latest time labels.
3. Implement progress summary.
4. Implement Home date navigation.
5. Implement shared log sheet.
6. Implement indicators, notes, volume, member picker, and When picker.
7. Implement log create/update/delete API calls.
8. Implement undo toast.
9. Implement jiggle mode, reorder, and hide-from-home.

Tests/gates:

- Unit tests for `CreateLogRequest` construction for every Home path.
- Unit tests for `slotHour` and `completedAt` behavior.
- Unit tests for volume prefill.
- XCUITest translations for all Home specs.
- Snapshot tests for Home states and log sheet.

### Phase 6: Chores Management

Goal: Chore CRUD and management are fully native.

Tasks:

1. Implement Manage Chores screen.
2. Implement add/edit chore sheet.
3. Implement delete custom chore confirmation.
4. Implement restore predefined chore.
5. Implement hide/show from Home.
6. Implement reorder from manage list.
7. Implement indicator label/default editing.
8. Implement color and emoji pickers.

Tests/gates:

- XCUITest translations for chores management specs.
- Unit tests for request validation helpers.
- Contract tests for server validation errors.
- Snapshot tests for add/edit/delete/restore states.

### Phase 7: Activity History, Day, And Week

Goal: Activity tab reaches parity with PWA history and calendar behavior.

Tasks:

1. Implement Activity segmented view.
2. Implement History pagination and grouping.
3. Implement History filter UI.
4. Implement Day calendar grid.
5. Implement Week calendar grid.
6. Implement Anytime rows.
7. Implement ad-hoc log placement by `slotHour`.
8. Implement pick-chore sheet from hour rows.
9. Implement edit log flow from calendar/history.

Tests/gates:

- Unit tests for day/week grid builders.
- Unit tests for log placement.
- XCUITest translations for history and calendar specs.
- Snapshot tests for Activity views.

### Phase 8: Schedule

Goal: Upcoming schedule list and schedule CRUD reach parity.

Tasks:

1. Port recurrence logic from `calendar.js` into Swift.
2. Implement Upcoming 14-day list.
3. Implement schedule create/edit/delete sheets.
4. Implement direct log action on today's schedule rows.
5. Implement schedule assignment to household members.
6. Implement recurrence end.
7. Implement rescheduling interaction for calendar rows.
8. Ensure sparse PATCH behavior and null clearing.

Tests/gates:

- `RecurrenceTests.swift` covering all recurrence types and edge cases.
- XCUITest translations for schedule tab and schedule specs.
- Contract tests for schedule PATCH preserving `isActive`.
- Snapshot tests for schedule list and edit sheet.

### Phase 9: Household, Members, And Multi-Household

Goal: Settings household features reach parity.

Tasks:

1. Implement household settings screen.
2. Implement household edit.
3. Implement invite creation, display, copy, and revoke.
4. Implement member role controls.
5. Implement transfer ownership.
6. Implement remove member.
7. Implement leave household.
8. Implement household switcher.
9. Implement state wipe and reload after activation.

Tests/gates:

- XCUITest translations for household specs.
- Contract tests for role-restricted endpoints.
- Unit tests for state reset after household switch.
- Snapshot tests for owner/admin/member settings states.

### Phase 10: Notifications And APNs

Goal: In-app notifications and native push work in the first release.

Tasks:

1. Implement APNs backend package, migration, handlers, and service integration.
2. Add iOS entitlements for push notifications.
3. Add App ID, bundle ID, APNs key/certificate setup docs.
4. Implement permission request flow.
5. Register APNs device token after login and preference enable.
6. Unregister token on logout.
7. Implement in-app notification list and badge.
8. Implement mark read, mark all read, and delete.
9. Implement notification preferences.
10. Add production/sandbox environment handling.

Tests/gates:

- Backend APNs unit and handler tests.
- iOS unit tests for token registration state machine.
- XCUITest translations for notification specs.
- Manual device test on a real iPhone for APNs sandbox.
- Production smoke test after TestFlight install.

### Phase 11: Stats

Goal: Full stats parity.

Tasks:

1. Implement stats API client methods.
2. Implement Overview.
3. Implement Heatmap.
4. Implement Busy Hours chart and filters.
5. Implement Leaderboard.
6. Implement Category breakdown.
7. Implement Chore stats expandable cards.
8. Implement Baby Care time-series charts.
9. Implement Top Chores per-user filter.
10. Implement empty/loading/error states.

Tests/gates:

- Unit tests for stats request construction.
- Unit tests for chart normalization.
- XCUITest translations for stats specs.
- Snapshot tests for charts.
- Contract tests for timezone-sensitive stats.

### Phase 12: Security, Accessibility, And Polish

Goal: Native app is safe, accessible, and App Store-ready.

Tasks:

1. Review every endpoint call for ownership assumptions and error handling.
2. Ensure user-controlled metadata is rendered as plain text.
3. Add VoiceOver labels to custom controls.
4. Test Dynamic Type sizes.
5. Test dark mode if enabled.
6. Add network error, session expired, and rate-limit UI states.
7. Add privacy strings in Info.plist for notifications if needed.
8. Add App Store screenshots, app icon, launch screen, and metadata.
9. Add privacy policy references for App Store Connect.
10. Add TestFlight internal testing process.

Tests/gates:

- Accessibility audit in Xcode.
- XCUITest with larger content size category where feasible.
- Security rendering tests.
- Manual exploratory test pass on at least one real iPhone.

### Phase 13: Release Readiness

Goal: Ship only after full parity is proved.

Tasks:

1. Complete `docs/plans/client-parity.md` with all features marked done.
2. Run all Go, JS, Playwright, Swift, and XCUITest suites.
3. Run backend with production-like config in local stack.
4. Build archive in Xcode.
5. Upload to TestFlight.
6. Smoke test TestFlight build against staging or production.
7. Verify APNs production token registration and delivery.
8. Submit to App Review.

Tests/gates:

- No skipped parity tests without written justification.
- No known full-parity gaps unless explicitly accepted as launch blockers or product exceptions.
- App Review metadata complete.

## Implementation Notes For Less-Capable Agents

Follow these rules exactly:

1. Before implementing a feature, read the corresponding PWA files under `web/static/js/` and the corresponding Playwright specs under `tests/e2e/`.
2. Write or port tests first.
3. Implement the smallest native code that makes the tests pass.
4. Do not invent new behavior because it feels more iOS-like unless the plan explicitly allows it.
5. If PWA behavior appears buggy, stop and ask before intentionally diverging.
6. If a backend endpoint lacks native support, add backend tests before changing the backend.
7. Keep new iOS code under `ios/` unless changing shared backend/docs/CI.
8. Keep API DTO names close to server JSON names.
9. Use `Int` for Go `int64` IDs unless a test proves overflow risk on supported devices.
10. Use `Date` for RFC3339 timestamps and `LocalDate` for `YYYY-MM-DD` values.
11. Never drop `slotHour` or replace it with `completedAt.hour`; they are related but distinct behavior.
12. Never update only one client when behavior is shared.

## Open Risks

| Risk | Mitigation |
|------|------------|
| Cookie/CSRF auth may be awkward in native iOS | Prove with Phase 2 contract tests before building many screens |
| APNs requires new backend infrastructure | Build in Phase 10 with backend tests and fake sender first |
| Full parity is large | Keep phase gates strict and track parity in `docs/plans/client-parity.md` |
| Recurrence drift between JS and Swift | Port JS unit cases into `RecurrenceTests.swift` and keep both test suites updated |
| `slotHour` regressions | Centralize log request construction and test every log entry path |
| PWA and iOS drift over time | Add `ios/AGENTS.md`, root AGENTS parity note, PR checklist, and CI gates |
| XCUITest runtime grows too large | Keep most edge cases in unit/contract tests; reserve XCUITest for critical flows |

## Definition Of Done

The iOS conversion is complete when:

- The native SwiftUI app builds from `ios/`.
- The app can be distributed through TestFlight and submitted to the App Store.
- All existing PWA user-facing features have native equivalents.
- Push notifications work through APNs in sandbox and production.
- The PWA still passes `make test-js` and `make e2e`.
- The backend still passes `make test-go`.
- The iOS app passes XCTest, snapshot tests, API contract tests, and XCUITest.
- `docs/plans/client-parity.md` shows no unaddressed parity gaps.
- Future-agent instructions exist so later features and fixes update both clients.
