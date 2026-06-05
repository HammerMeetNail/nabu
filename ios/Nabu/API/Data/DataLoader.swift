import Foundation

@MainActor
final class DataLoader: ObservableObject {
    private(set) var api: APIClient
    private(set) var state: AppState

    private(set) var household: HouseholdDataLoader!
    private(set) var chores: ChoreDataLoader!
    private(set) var logs: LogDataLoader!
    private(set) var schedules: ScheduleDataLoader!
    private(set) var notifs: NotificationDataLoader!
    private(set) var preferences: PreferencesDataLoader!

    init() {
        self.api = APIClient(baseURL: URL(string: "http://localhost:8080")!)
        self.state = AppState()
    }

    func configure(api: APIClient, state: AppState) {
        self.api = api
        self.state = state
        self.household = HouseholdDataLoader(api: api, state: state)
        self.chores = ChoreDataLoader(api: api, state: state)
        self.logs = LogDataLoader(api: api, state: state)
        self.schedules = ScheduleDataLoader(api: api, state: state)
        self.notifs = NotificationDataLoader(api: api, state: state)
        self.preferences = PreferencesDataLoader(api: api, state: state)
    }

    // Called after initial auth (login/register/onboarding)
    func reloadAfterAuth() async {
        await withTaskGroup(of: Void.self) { group in
            group.addTask { await self.household.loadHouseholdData() }
            group.addTask { await self.preferences.loadPreferences() }
        }
        await preferences.syncTimezone()

        guard state.household != nil else { return }

        await withTaskGroup(of: Void.self) { group in
            group.addTask { await self.chores.loadChoreData() }
            group.addTask { await self.logs.loadTodayData() }
            group.addTask { await self.logs.loadLatestLogsData() }
            group.addTask { await self.schedules.loadSchedules() }
            group.addTask { await self.notifs.loadNotifData() }
        }
    }

    // Called on foreground / visibility change
    func foregroundRefresh() async {
        guard state.user != nil else { return }
        await notifs.loadNotifData()
        if state.household != nil {
            await household.loadHouseholdData()
        }
    }
}
