import Foundation

@MainActor
final class LogStore {
    let api: APIClient

    init(api: APIClient) {
        self.api = api
    }

    func createLog(choreId: Int, note: String = "", date: String? = nil,
                   indicators: [String] = [], slotHour: Int? = nil,
                   completedAt: String? = nil, volumeML: Int? = nil,
                   userId: Int? = nil, indicatorVolumes: [String: Int]? = nil) async throws -> LogResponse {
        let body = CreateLogRequest(
            choreId: choreId, note: note, indicators: indicators,
            date: date, hour: slotHour, completedAt: completedAt,
            volumeML: volumeML, userId: userId,
            indicatorVolumes: indicatorVolumes
        )
        return try await api.post("/api/logs", body: body)
    }

    func updateLog(logId: Int, note: String? = nil, indicators: [String]? = nil,
                   volumeML: Int? = nil, userId: Int? = nil,
                   completedAt: String? = nil, hour: Int? = nil,
                   date: String? = nil, indicatorVolumes: [String: Int]? = nil) async throws -> LogResponse {
        let body = UpdateLogRequest(
            note: note, indicators: indicators, volumeML: volumeML,
            userId: userId, completedAt: completedAt, hour: hour, date: date,
            indicatorVolumes: indicatorVolumes
        )
        return try await api.patch("/api/logs/\(logId)", body: body)
    }

    func deleteLog(logId: Int) async throws -> StatusResponse {
        return try await api.delete("/api/logs/\(logId)")
    }
}
