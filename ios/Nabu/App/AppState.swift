import Foundation
import Observation

@MainActor
@Observable
final class AppState {
    private(set) var revision = UUID()
    var sessionPhase: ClientIdentity.Phase = .checking
    var logoutIsDurable = true
    @ObservationIgnored private var operations: [String: UUID] = [:]
    struct Owner { let revision: UUID; let key: String; let request: UUID }
    func beginOperation(_ key: String) -> Owner {
        let request = UUID()
        operations[key] = request
        return Owner(revision: revision, key: key, request: request)
    }
    func owns(_ owner: Owner) -> Bool {
        revision == owner.revision && operations[owner.key] == owner.request
    }
    /// Reports a failed action as the PWA's error toast does: the server's
    /// message when it sent one, otherwise `fallback`, above the tabs
    /// (`ErrorToast`). Always logged. With an `owner`, shown only while that
    /// identity is current, as its screens are gone otherwise. Without one,
    /// for a request that changes the identity (`APIClient.changesIdentity`):
    /// its failure belongs to whichever identity the app confirmed after it,
    /// on screens rebuilt for that identity.
    func reportFailure(_ fallback: String, _ error: any Error, owner: UUID? = nil) {
        let message = Log.actions.failureMessage(fallback, error)
        guard owner == nil || revision == owner else { return }
        toast = Toast(message: message)
    }
    func adopt(_ session: ClientIdentity.Snapshot) {
        if revision != session.revision {
            reset()
            revision = session.revision
        }
        sessionPhase = session.phase
        showingSavedHome = session.resumedOffline
        logoutIsDurable = session.logoutIsDurable
        user = session.user
        activeHouseholdId = session.user?.householdId
        householdDisplayOn = HouseholdDisplay.setting(for: session.origin) != nil
        if let origin = session.origin {
            activeTimer = DurationTimer.load(origin: origin)
            pendingLogs = OfflineLogQueue.shared.scopedItems(origin).map {
                PendingLog(body: $0.body, fallbackUserId: origin.actorID)
            }
        }
    }
    var user: User?
    var household: Household?
    var userHouseholds: [HouseholdWithRole] = []
    var activeHouseholdId: Int?
    var members: [Member] = []
    var historicalMembers: [HistoricalMember] = []
    var invites: [Invite] = []
    /// Non-nil when this account's household membership is disabled. The
    /// message comes from the server: "Your access to <household> is
    /// temporarily disabled. Contact an administrator to re-enable access."
    var accessSuspendedMessage: String?
    var chores: [Chore] = []
    var todayLogs: [ChoreLog] = []
    var schedules: [ChoreSchedule] = []
    var recentAmounts: [Int: [Int]] = [:]
    var recentAmountUnits: [Int: String] = [:]
    var latestLogs: [Int: ChoreLog] = [:]
    var notifications: [AppNotification] = []
    var notificationCursor: String?
    var notificationLoading = false
    var notificationLoadingMore = false
    var notificationMutating = false
    var notificationError: String?
    var notificationErrorIsAppend = false
    var notificationPanelOpen = false
    var unreadNotifications = 0
    var notificationPrefs: ReminderPreference?
    var availableNotificationTypes: [NotificationTypeInfo] = []
    var choreReminderPrefs: [ChoreReminderPref] = []
    var choreOrder: [Int] = []
    var hiddenHomeChoreIDs: [Int] = []
    var currentTab: MainTab = .home
    /// ⌘F asked Activity to focus its search field; Activity clears it.
    var activitySearchRequested = false
    var homeView: HomeViewMode = .log
    var activeSheet: ActiveSheet?
    /// A failed action's message (`reportFailure`).
    var toast: Toast?
    /// This device is the household display for the signed-in account
    /// (`HouseholdDisplay`): it shows `HouseholdDisplayView` instead of the tabs.
    var householdDisplayOn = false
    var jiggleMode = false
    var historyChoreFilter: [Int]?
    var historyFilterOpen = false
    /// Per-user volume display/input unit ("ml" | "oz"); volumes stay mL in the API.
    var volumeUnit: String = "ml"
    /// Per-user pref: hide the unread count badge on the Settings tab and
    /// bell rows. Notifications still accumulate (PWA `hideNotificationBadge`).
    var hideNotificationBadge = false
    /// The single running duration timer (persisted via `DurationTimer`).
    var activeTimer: ActiveTimer?
    /// Offline-queued logs shown inline in Activity with a "pending" badge
    /// until the queue replays.
    var pendingLogs: [PendingLog] = []
    /// Shared per-day diary notes keyed by "YYYY-MM-DD" (Phase 5.4).
    var dayNotes: [String: String] = [:]
    /// Deep-link target from a notification "Log now" action, widget tap, or
    /// quick action (parity with the PWA's `?quicklog=`); HomeView consumes
    /// it once chores exist.
    var pendingQuickLog: QuickLogTarget?
    /// Invite code from a `/join?code=…` universal link opened while logged
    /// out; OnboardingView prefills its Join tab from it.
    var pendingInviteCode: String?
    /// What that invite link is for, shown above sign-in and sign-up (the
    /// PWA's /join page). A dead link's `.invalid` outlives its dropped code.
    var invitePreview: InvitePreviewStatus?
    /// Connectivity from DataLoader's NWPathMonitor; drives the global
    /// offline banner (C4). Not reset on logout — it's device state.
    var isOffline = false
    /// Home shows the copy saved on this device: the server could not be
    /// reached to confirm the session at launch. Cleared by the server's
    /// answer, which reloads everything (`ClientIdentity.resumeSavedHome`).
    var showingSavedHome = false
    /// Stats customization (P4): stored section order/hidden sets and the
    /// user-defined widgets, synced via `/api/preferences`.
    var statsSectionOrder: [String] = []
    var statsSectionHidden: [String] = []
    var statsWidgets: [StatsWidget] = []

    /// Applies a log write's answer for its chore's latest log ("time ago" on
    /// Home and the widget) instead of reloading every chore's latest log. A
    /// latest-per-chore read already in flight no longer owns the result.
    func applyLatestLog(_ latest: ChoreLog?, choreId: Int) {
        _ = beginOperation("loadLatestLogsData")
        if let latest { latestLogs[choreId] = latest } else { latestLogs.removeValue(forKey: choreId) }
        WidgetDataCache.write(chores: WidgetDataCache.snapshots(from: self))
    }

    /// The chore's latest log after a save: the server's answer, or (from an
    /// older server) the newer of the shown log and the saved one, so a
    /// backdated save never replaces a newer time on Home.
    func latestAfterSave(_ response: LogResponse, choreId: Int) -> ChoreLog {
        if let latest = response.latest { return latest }
        guard let shown = latestLogs[choreId],
              (shown.completedAt, shown.id) > (response.log.completedAt, response.log.id) else { return response.log }
        return shown
    }

    var authorMembers: [Member] {
        members + historicalMembers.map {
            Member(userId: $0.userId, email: "", displayName: $0.displayName,
                   avatarColor: $0.avatarColor, emailVerified: false, role: "")
        }
    }

    func reset() {
        revision = UUID()
        operations = [:]
        WidgetDataCache.write(chores: [])
        PushRegistrationController.shared.suspend()
        showingSavedHome = false
        user = nil
        household = nil
        userHouseholds = []
        activeHouseholdId = nil
        members = []
        historicalMembers = []
        invites = []
        accessSuspendedMessage = nil
        chores = []
        todayLogs = []
        schedules = []
        latestLogs = [:]
        recentAmounts = [:]
        recentAmountUnits = [:]
        notifications = []
        notificationCursor = nil
        notificationLoading = false
        notificationLoadingMore = false
        notificationMutating = false
        notificationError = nil
        notificationErrorIsAppend = false
        notificationPanelOpen = false
        unreadNotifications = 0
        notificationPrefs = nil
        availableNotificationTypes = []
        choreReminderPrefs = []
        choreOrder = []
        hiddenHomeChoreIDs = []
        currentTab = .home
        homeView = .log
        activeSheet = nil
        toast = nil
        householdDisplayOn = false
        jiggleMode = false
        historyChoreFilter = nil
        historyFilterOpen = false
        volumeUnit = "ml"
        hideNotificationBadge = false
        activeTimer = nil
        pendingLogs = []
        dayNotes = [:]
        pendingQuickLog = nil
        pendingInviteCode = nil
        invitePreview = nil
        statsSectionOrder = []
        statsSectionHidden = []
        statsWidgets = []
    }

    func resetHouseholdScoped() {
        revision = UUID()
        operations = [:]
        WidgetDataCache.write(chores: [])
        PushRegistrationController.shared.suspend()
        household = nil
        activeHouseholdId = nil
        members = []
        historicalMembers = []
        invites = []
        accessSuspendedMessage = nil
        chores = []
        todayLogs = []
        schedules = []
        latestLogs = [:]
        recentAmounts = [:]
        recentAmountUnits = [:]
        choreOrder = []
        hiddenHomeChoreIDs = []
        historyChoreFilter = nil
        historyFilterOpen = false
        dayNotes = [:]
        statsSectionOrder = []
        statsSectionHidden = []
        statsWidgets = []
        notifications = []
        notificationCursor = nil
        notificationLoading = false
        notificationLoadingMore = false
        notificationMutating = false
        notificationError = nil
        notificationErrorIsAppend = false
        notificationPanelOpen = false
        unreadNotifications = 0
        notificationPrefs = nil
        availableNotificationTypes = []
        choreReminderPrefs = []
        activeTimer = nil
        pendingLogs = []
        activeSheet = nil
        toast = nil
        householdDisplayOn = false
        pendingQuickLog = nil
        jiggleMode = false
        currentTab = .home
        homeView = .log
    }
}
