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
    func adopt(_ session: ClientIdentity.Snapshot) {
        if revision != session.revision {
            reset()
            revision = session.revision
        }
        sessionPhase = session.phase
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
    @Published var chores: [Chore] = []
    @Published var todayLogs: [ChoreLog] = []
    @Published var schedules: [ChoreSchedule] = []
    @Published var recentAmounts: [Int: [Int]] = [:]
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
    /// Connectivity from DataLoader's NWPathMonitor; drives the global
    /// offline banner (C4). Not reset on logout — it's device state.
    @Published var isOffline = false
    /// Stats customization (P4): stored section order/hidden sets and the
    /// user-defined widgets, synced via `/api/preferences`.
    @Published var statsSectionOrder: [String] = []
    @Published var statsSectionHidden: [String] = []
    @Published var statsWidgets: [StatsWidget] = []

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
        user = nil
        household = nil
        userHouseholds = []
        activeHouseholdId = nil
        members = []
        historicalMembers = []
        invites = []
        chores = []
        todayLogs = []
        schedules = []
        latestLogs = [:]
        recentAmounts = [:]
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
        chores = []
        todayLogs = []
        schedules = []
        latestLogs = [:]
        recentAmounts = [:]
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
