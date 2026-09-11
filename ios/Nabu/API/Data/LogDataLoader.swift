import Foundation

@MainActor
final class LogDataLoader {
    let api: APIClient
    let state: AppState

    init(api: APIClient, state: AppState) {
        self.api = api
        self.state = state
    }

    func loadTodayData() async {
        let api = self.api.scoped()
        let owner = state.beginOperation("loadTodayData")
        let date = todayISO()
        do {
            let data: TodayResponse = try await api.get("/api/logs/today", query: [URLQueryItem(name: "date", value: date)])
            guard state.owns(owner) else { return }
            state.todayLogs = data.logs
        } catch {
            // Silent failure
        }
    }

    func loadLatestLogsData() async {
        let api = self.api.scoped()
        let owner = state.beginOperation("loadLatestLogsData")
        do {
            let data: LatestLogsResponse = try await api.get("/api/logs/latest-per-chore")
            guard state.owns(owner) else { return }
            var dict: [Int: ChoreLog] = [:]
            for (key, log) in data.latestLogs {
                if let choreId = Int(key) {
                    dict[choreId] = log
                }
            }
            state.latestLogs = dict
            // Refresh the Home-Screen widget's app-group snapshot — this is
            // exactly the data ("time since last log") the widget renders.
            WidgetDataCache.write(chores: WidgetDataCache.snapshots(from: state))
        } catch {
            // Silent failure
        }
    }

    private func todayISO() -> String {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.timeZone = .current
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.string(from: Date())
    }
}

extension WidgetDataCache {
    /// Snapshots from app state: home-visible chores in the user's order,
    /// with their latest log time. Lives here (app target) — the shared
    /// WidgetDataCache file also compiles into the widget extension, which
    /// has no AppState.
    @MainActor
    static func snapshots(from state: AppState) -> [WidgetChoreSnapshot] {
        let hidden = Set(state.hiddenHomeChoreIDs)
        let orderMap = Dictionary(uniqueKeysWithValues: state.choreOrder.enumerated().map { ($1, $0) })
        return state.chores
            .filter { !hidden.contains($0.id) }
            .sorted { (orderMap[$0.id] ?? Int.max, $0.id) < (orderMap[$1.id] ?? Int.max, $1.id) }
            .map { chore in
                WidgetChoreSnapshot(
                    id: chore.id,
                    name: chore.name,
                    icon: chore.icon,
                    lastCompletedAt: state.latestLogs[chore.id]?.completedAt
                )
            }
    }
}
