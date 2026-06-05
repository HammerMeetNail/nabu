import Foundation

@MainActor
final class AppState: ObservableObject {
    @Published var user: User?
    @Published var household: Household?
    @Published var userHouseholds: [HouseholdWithRole] = []
    @Published var activeHouseholdId: Int?
    @Published var members: [Member] = []
    @Published var invites: [Invite] = []
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
    @Published var hiddenHomeChoreIDs: [Int] = []
    @Published var currentTab: MainTab = .home
    @Published var activityView: ActivityViewMode = .history
    @Published var calendarView: CalendarViewMode = .day
    @Published var calendarDate: LocalDate?
    @Published var homeView: HomeViewMode = .log
    @Published var activeSheet: ActiveSheet?
    @Published var toast: Toast?
    @Published var jiggleMode = false
    @Published var historyChoreFilter: [Int]?
    @Published var historyFilterOpen = false

    func reset() {
        user = nil
        household = nil
        userHouseholds = []
        activeHouseholdId = nil
        members = []
        invites = []
        chores = []
        todayLogs = []
        weekLogs = []
        schedules = []
        latestLogs = [:]
        notifications = []
        unreadNotifications = 0
        notificationPrefs = nil
        availableNotificationTypes = []
        choreOrder = []
        hiddenHomeChoreIDs = []
        currentTab = .home
        activityView = .history
        calendarView = .day
        calendarDate = nil
        homeView = .log
        activeSheet = nil
        toast = nil
        jiggleMode = false
        historyChoreFilter = nil
        historyFilterOpen = false
    }

    func resetHouseholdScoped() {
        household = nil
        activeHouseholdId = nil
        members = []
        invites = []
        chores = []
        todayLogs = []
        weekLogs = []
        schedules = []
        latestLogs = [:]
        choreOrder = []
        hiddenHomeChoreIDs = []
        historyChoreFilter = nil
        historyFilterOpen = false
    }
}
