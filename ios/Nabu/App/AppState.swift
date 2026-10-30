import Foundation

@MainActor
final class AppState: ObservableObject {
    @Published private(set) var revision = UUID()
    @Published var sessionPhase: ClientIdentity.Phase = .checking
    @Published var logoutIsDurable = true
    private var operations: [String: UUID] = [:]
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
        if let origin = session.origin {
            activeTimer = DurationTimer.load(origin: origin)
            pendingLogs = OfflineLogQueue.shared.scopedItems(origin).map {
                PendingLog(body: $0.body, fallbackUserId: origin.actorID)
            }
        }
    }
    @Published var user: User?
    @Published var household: Household?
    @Published var userHouseholds: [HouseholdWithRole] = []
    @Published var activeHouseholdId: Int?
    @Published var members: [Member] = []
    @Published var historicalMembers: [HistoricalMember] = []
    @Published var invites: [Invite] = []
    /// Non-nil when this account's household membership is disabled. The
    /// message comes from the server: "Your access to <household> is
    /// temporarily disabled. Contact an administrator to re-enable access."
    @Published var accessSuspendedMessage: String?
    @Published var chores: [Chore] = []
    @Published var todayLogs: [ChoreLog] = []
    @Published var schedules: [ChoreSchedule] = []
    @Published var recentAmounts: [Int: [Int]] = [:]
    var recentAmountUnits: [Int: String] = [:]
    @Published var latestLogs: [Int: ChoreLog] = [:]
    @Published var notifications: [AppNotification] = []
    @Published var notificationCursor: String?
    @Published var notificationLoading = false
    @Published var notificationLoadingMore = false
    @Published var notificationMutating = false
    @Published var notificationError: String?
    @Published var notificationErrorIsAppend = false
    @Published var notificationPanelOpen = false
    @Published var unreadNotifications = 0
    @Published var notificationPrefs: ReminderPreference?
    @Published var availableNotificationTypes: [NotificationTypeInfo] = []
    @Published var choreReminderPrefs: [ChoreReminderPref] = []
    @Published var choreOrder: [Int] = []
    @Published var hiddenHomeChoreIDs: [Int] = []
    @Published var currentTab: MainTab = .home
    @Published var homeView: HomeViewMode = .log
    @Published var activeSheet: ActiveSheet?
    /// A failed action's message (`reportFailure`).
    @Published var toast: Toast?
    @Published var jiggleMode = false
    @Published var historyChoreFilter: [Int]?
    @Published var historyFilterOpen = false
    /// Per-user volume display/input unit ("ml" | "oz"); volumes stay mL in the API.
    @Published var volumeUnit: String = "ml"
    /// Per-user pref: hide the unread count badge on the Settings tab and
    /// bell rows. Notifications still accumulate (PWA `hideNotificationBadge`).
    @Published var hideNotificationBadge = false
    /// The single running duration timer (persisted via `DurationTimer`).
    @Published var activeTimer: ActiveTimer?
    /// Offline-queued logs shown inline in Activity with a "pending" badge
    /// until the queue replays.
    @Published var pendingLogs: [PendingLog] = []
    /// Shared per-day diary notes keyed by "YYYY-MM-DD" (Phase 5.4).
    @Published var dayNotes: [String: String] = [:]
    /// Deep-link target from a notification "Log now" action, widget tap, or
    /// quick action (parity with the PWA's `?quicklog=`); HomeView consumes
    /// it once chores exist.
    @Published var pendingQuickLog: QuickLogTarget?
    /// Invite code from a `/join?code=…` universal link opened while logged
    /// out; OnboardingView prefills its Join tab from it.
    @Published var pendingInviteCode: String?
    /// What that invite link is for, shown above sign-in and sign-up (the
    /// PWA's /join page). A dead link's `.invalid` outlives its dropped code.
    @Published var invitePreview: InvitePreviewStatus?
    /// Connectivity from DataLoader's NWPathMonitor; drives the global
    /// offline banner (C4). Not reset on logout — it's device state.
    @Published var isOffline = false
    /// Home shows the copy saved on this device: the server could not be
    /// reached to confirm the session at launch. Cleared by the server's
    /// answer, which reloads everything (`ClientIdentity.resumeSavedHome`).
    @Published var showingSavedHome = false
    /// Stats customization (P4): stored section order/hidden sets and the
    /// user-defined widgets, synced via `/api/preferences`.
    @Published var statsSectionOrder: [String] = []
    @Published var statsSectionHidden: [String] = []
    @Published var statsWidgets: [StatsWidget] = []

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
        pendingQuickLog = nil
        jiggleMode = false
        currentTab = .home
        homeView = .log
    }
}
