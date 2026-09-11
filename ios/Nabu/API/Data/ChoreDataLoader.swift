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
        let api = self.api.scoped()
        let owner = state.beginOperation("loadChoreData")
        do {
            let data: ChoresResponse = try await api.get("/api/chores")
            guard state.owns(owner) else { return }
            NSLog("[Nabu] ChoreDataLoader OK: \(data.chores.count) chores")
            state.chores = data.chores
        } catch {
            NSLog("[Nabu] Request failed")
        }
    }
}
