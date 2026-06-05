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
        let date = todayISO()
        do {
            let data: TodayResponse = try await api.get("/api/logs/today", query: [URLQueryItem(name: "date", value: date)])
            state.todayLogs = data.logs
        } catch {
            // Silent failure
        }
    }

    func loadLatestLogsData() async {
        do {
            let data: LatestLogsResponse = try await api.get("/api/logs/latest-per-chore")
            var dict: [Int: ChoreLog] = [:]
            for (key, log) in data.latestLogs {
                if let choreId = Int(key) {
                    dict[choreId] = log
                }
            }
            state.latestLogs = dict
        } catch {
            // Silent failure
        }
    }

    private func todayISO() -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withFullDate]
        return formatter.string(from: Date())
    }
}
