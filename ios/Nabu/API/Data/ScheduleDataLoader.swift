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
        let api = self.api.scoped()
        let owner = state.beginOperation("loadSchedules")
        do {
            let data: SchedulesResponse = try await api.get("/api/schedules")
            guard state.owns(owner) else { return }
            NSLog("[Nabu] ScheduleDataLoader.loadSchedules OK: \(data.schedules.count) schedules")
            state.schedules = data.schedules
        } catch {
            NSLog("[Nabu] Request failed")
        }
    }
}
