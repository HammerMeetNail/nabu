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
            state.schedules = data.schedules
        } catch {
            // Silent failure
        }
    }
}
