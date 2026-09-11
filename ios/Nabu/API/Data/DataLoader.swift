import Foundation
import Network

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

    private var pathMonitor: NWPathMonitor?
    private var flushing: Set<UUID> = []

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
        startConnectivityMonitor()
    }

    /// Replays the offline log queue when connectivity returns (the
    /// foreground path is `foregroundRefresh`). Mirrors the PWA's
    /// `online` listener.
    private func startConnectivityMonitor() {
        guard pathMonitor == nil else { return }
        let monitor = NWPathMonitor()
        monitor.pathUpdateHandler = { [weak self] path in
            let satisfied = path.status == .satisfied
            Task { @MainActor in
                guard let self else { return }
                self.state.isOffline = !satisfied
                if satisfied {
                    await self.foregroundRefresh()
                }
            }
        }
        monitor.start(queue: DispatchQueue(label: "nabu.offline-queue.path-monitor"))
        pathMonitor = monitor
    }

    /// Every replay pass first confirms /me. Failed confirmation retains the
    /// journal and never uses cached household state as write authority.
    func flushOfflineQueue(retryFailed: Bool = false) async {
        let api = self.api
        let state = self.state
        guard let response: UserResponse = try? await api.get("/api/me"), response.user != nil,
              let origin = api.identity.snapshot.origin else { return }
        let revision = state.revision
        guard flushing.insert(revision).inserted else { return }
        defer { flushing.remove(revision) }
        let owner = state.beginOperation("flushOfflineQueue")
        let logs = self.logs!
        let synced = await LogStore(api: api).replayOfflineQueue(retryFailed: retryFailed)
        guard state.owns(owner) else { return }
        state.pendingLogs = OfflineLogQueue.shared.scopedItems(origin).map {
            PendingLog(body: $0.body, fallbackUserId: origin.actorID)
        }
        if synced > 0 {
            await logs.loadTodayData()
            guard state.owns(owner) else { return }
            await logs.loadLatestLogsData()
        }
    }

    func reloadAfterAuth() async {
        let state = self.state
        guard state.user != nil else { return }
        let owner = state.beginOperation("reloadAfterAuth")
        let household = self.household!, preferences = self.preferences!, notifs = self.notifs!
        let chores = self.chores!, logs = self.logs!, schedules = self.schedules!
        await withTaskGroup(of: Void.self) { group in
            group.addTask { await household.loadHouseholdData() }
            group.addTask { await preferences.loadPreferences() }
            group.addTask { await notifs.loadNotificationPreferences() }
        }
        guard state.owns(owner) else { return }
        await preferences.syncTimezone()
        guard state.owns(owner), state.household != nil else { return }
        await withTaskGroup(of: Void.self) { group in
            group.addTask { await chores.loadChoreData() }
            group.addTask { await logs.loadTodayData() }
            group.addTask { await logs.loadLatestLogsData() }
            group.addTask { await schedules.loadSchedules() }
            group.addTask { await notifs.loadNotifData() }
        }
        guard state.owns(owner) else { return }
        await flushOfflineQueue()
    }

    func foregroundRefresh() async {
        let api = self.api
        let state = self.state
        guard api.identity.snapshot.phase != .logoutPending else { return }
        guard let response: UserResponse = try? await api.get("/api/me"), response.user != nil else { return }
        let owner = state.beginOperation("foregroundRefresh")
        let notifs = self.notifs!, household = self.household!, chores = self.chores!
        await flushOfflineQueue()
        guard state.owns(owner) else { return }
        // Foreground refresh must not collapse pages being read in the panel.
        if !state.notificationPanelOpen { await notifs.loadNotifData() }
        guard state.owns(owner) else { return }
        if state.user?.householdId != nil {
            await household.loadHouseholdData()
            guard state.owns(owner) else { return }
            await chores.loadChoreData()
        }
    }
}
