import Foundation

@MainActor
final class ChoreStore {
    let api: APIClient

    init(api: APIClient) {
        self.api = api.scoped()
    }

    func createChore(name: String, icon: String, color: String, category: String = "custom",
                     indicatorLabels: [String] = [], indicatorDefaults: [String] = [],
                     followUpEnabled: Bool? = nil,
                     metricType: String? = nil, metricUnit: String? = nil,
                     subjects: [String]? = nil, visibility: String? = nil) async throws -> ChoreResponse {
        let body = CreateChoreRequest(
            name: name, icon: icon, color: color, category: category,
            indicatorLabels: indicatorLabels.isEmpty ? nil : indicatorLabels,
            indicatorDefaults: indicatorDefaults.isEmpty ? nil : indicatorDefaults,
            followUpEnabled: followUpEnabled,
            metricType: metricType, metricUnit: metricUnit, subjects: subjects, visibility: visibility
        )
        return try await api.post("/api/chores", body: body)
    }

    func updateChore(choreId: Int, name: String, icon: String, color: String,
                     indicatorLabels: [String], indicatorDefaults: [String],
                     followUpEnabled: Bool? = nil,
                     metricType: String? = nil, metricUnit: String? = nil,
                     subjects: [String]? = nil, visibility: String? = nil) async throws -> ChoreResponse {
        let body = PatchChoreRequest(name: name, icon: icon, color: color, category: nil,
                              indicatorLabels: indicatorLabels,
                              indicatorDefaults: indicatorDefaults,
                              followUpEnabled: followUpEnabled,
                              metricType: metricType,
                              metricUnit: metricUnit,
                              subjects: subjects, visibility: visibility)
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
