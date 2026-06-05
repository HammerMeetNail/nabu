import Foundation

@MainActor
final class ChoreDataLoader {
    let api: APIClient
    let state: AppState

    init(api: APIClient, state: AppState) {
        self.api = api
        self.state = state
    }

    func loadChoreData() async {
        do {
            let data: ChoresResponse = try await api.get("/api/chores")
            state.chores = data.chores
        } catch {
            // Silent failure
        }
    }
}
