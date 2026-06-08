import Foundation

@MainActor
final class ScheduleDataLoader {
    let api: APIClient
    let state: AppState

    init(api: APIClient, state: AppState) {
        self.api = api
        self.state = state
    }

    func loadSchedules() async {
        do {
            let data: SchedulesResponse = try await api.get("/api/schedules")
            print("[ScheduleDataLoader] loadSchedules success: count=\(data.schedules.count)")
            state.schedules = data.schedules
        } catch {
            print("[ScheduleDataLoader] loadSchedules ERROR: \(error)")
        }
    }
}
