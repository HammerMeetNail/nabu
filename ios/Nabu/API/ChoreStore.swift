import Foundation

@MainActor
final class ChoreStore {
    let api: APIClient

    init(api: APIClient) {
        self.api = api
    }

    func createChore(name: String, icon: String, color: String, category: String = "custom",
                     indicatorLabels: [String] = [], indicatorDefaults: [String] = [],
                     followUpEnabled: Bool? = nil) async throws -> ChoreResponse {
        let body = CreateChoreRequest(
            name: name, icon: icon, color: color, category: category,
            indicatorLabels: indicatorLabels.isEmpty ? nil : indicatorLabels,
            indicatorDefaults: indicatorDefaults.isEmpty ? nil : indicatorDefaults,
            followUpEnabled: followUpEnabled
        )
        return try await api.post("/api/chores", body: body)
    }

    func updateChore(choreId: Int, name: String, icon: String, color: String,
                     indicatorLabels: [String], indicatorDefaults: [String],
                     followUpEnabled: Bool? = nil) async throws -> ChoreResponse {
        struct UpdateBody: Codable {
            var name: String
            var icon: String
            var color: String
            var indicatorLabels: [String]
            var indicatorDefaults: [String]
            var followUpEnabled: Bool?
        }
        let body = UpdateBody(name: name, icon: icon, color: color,
                              indicatorLabels: indicatorLabels,
                              indicatorDefaults: indicatorDefaults,
                              followUpEnabled: followUpEnabled)
        return try await api.patch("/api/chores/\(choreId)", body: body)
    }

    func deleteChore(choreId: Int) async throws -> StatusResponse {
        return try await api.delete("/api/chores/\(choreId)")
    }

    func restoreDefault(choreId: Int) async throws -> ChoreResponse {
        return try await api.postEmpty("/api/chores/\(choreId)/restore-default")
    }

    func loadChores() async throws -> [Chore] {
        let data: ChoresResponse = try await api.get("/api/chores")
        return data.chores
    }
}
