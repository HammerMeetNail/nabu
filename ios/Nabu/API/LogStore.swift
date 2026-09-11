import Foundation

/// Result of a log creation attempt: either the server accepted it, or the
/// network was down and the body was queued for replay (PWA `logChore`
/// semantics — the log and its timestamp are never lost).
enum CreateLogOutcome {
    case created(LogResponse)
    case queued(PendingLog)
}

@MainActor
final class LogStore {
    let api: APIClient
    let offlineQueue: OfflineLogQueue
    private let owner: ClientIdentity.Snapshot

    init(api: APIClient, offlineQueue: OfflineLogQueue? = nil) {
        self.api = api.scoped()
        self.owner = api.requestContext
        self.offlineQueue = offlineQueue ?? OfflineLogQueue.shared
    }

    func createLog(choreId: Int, note: String = "", date: String? = nil,
                   indicators: [String] = [], slotHour: Int? = nil,
                   completedAt: String? = nil, volumeML: Int? = nil,
                   userId: Int? = nil, indicatorVolumes: [String: Int]? = nil,
                   followUpMinutes: Int? = nil,
                   followUpTime: String? = nil,
                   rating: Int? = nil, title: String? = nil,
                   durationSeconds: Int? = nil,
                   subject: String? = nil,
                   idempotencyKey: String = UUID().uuidString) async throws -> CreateLogOutcome {
        let owner = self.owner
        guard api.identity.isCurrent(owner), let origin = owner.origin else { throw APIError.contextChanged }
        let body = offlineQueue.item(key: idempotencyKey, origin: origin)?.body ?? CreateLogRequest(
            choreId: choreId, note: note, indicators: indicators,
            date: date, hour: slotHour, completedAt: completedAt ?? ISO8601DateFormatter().string(from: Date()),
            volumeML: volumeML, userId: userId,
            indicatorVolumes: indicatorVolumes,
            followUpMinutes: followUpMinutes,
            followUpTime: followUpTime,
            rating: rating, title: title,
            durationSeconds: durationSeconds, subject: subject,
            // Idempotency key so an offline replay can't create a duplicate.
            idempotencyKey: idempotencyKey
        )
        do {
            let response: LogResponse = try await offlineQueue.submit(body: body, origin: origin,
                isCurrent: { self.api.identity.isCurrent(owner) }) { request in
                    try await self.api.post("/api/logs", body: request)
                }
            guard api.identity.isCurrent(owner) else { throw APIError.contextChanged }
            return .created(response)
        } catch let error where Self.isNetworkFailure(error) {
            // submit has already persisted the exact timestamp/body. Never
            // claim queued success when the disk write failed.
            guard api.identity.isCurrent(owner), offlineQueue.item(key: idempotencyKey, origin: origin) != nil else { throw error }
            return .queued(PendingLog(body: body, fallbackUserId: origin.actorID))
        }
    }

    /// Whether the error means the request never reached the server (queue
    /// and replay later) as opposed to the server rejecting it (surface to
    /// the user).
    static func isNetworkFailure(_ error: Error) -> Bool {
        if error is URLError { return true }
        if case APIError.networkError = error { return true }
        return false
    }

    /// Replays the offline queue. Returns the number of logs synced.
    func replayOfflineQueue(retryFailed: Bool = false) async -> Int {
        let owner = self.owner
        guard api.identity.isCurrent(owner), let origin = owner.origin else { return 0 }
        return await offlineQueue.replay(origin: origin, retryFailed: retryFailed,
            isCurrent: { self.api.identity.isCurrent(owner) }) { body in
                let _: LogResponse = try await self.api.post("/api/logs", body: body)
            }
    }

    func updateLog(logId: Int, note: String? = nil, indicators: [String]? = nil,
                   volumeML: Int?? = nil, userId: Int? = nil,
                   completedAt: String? = nil, hour: Int? = nil,
                   date: String? = nil, indicatorVolumes: [String: Int]? = nil,
                   rating: Int?? = nil, title: String?? = nil,
                   durationSeconds: Int?? = nil,
                   subject: String?? = nil) async throws -> StatusResponse {
        let body = UpdateLogRequest(
            note: note, indicators: indicators, volumeML: volumeML,
            userId: userId, completedAt: completedAt, hour: hour, date: date,
            indicatorVolumes: indicatorVolumes,
            rating: rating, title: title, durationSeconds: durationSeconds, subject: subject
        )
        return try await api.patch("/api/logs/\(logId)", body: body)
    }

    func deleteLog(logId: Int) async throws -> StatusResponse {
        return try await api.delete("/api/logs/\(logId)")
    }

    /// A failed refresh leaves the last good suggestions available. Requests
    /// from an earlier sheet or identity cannot replace the current cache.
    func loadRecentAmounts(choreId: Int, state: AppState) async -> [Int]? {
        guard api.identity.isCurrent(owner) else { return nil }
        let operation = state.beginOperation("recent-amounts:\(choreId)")
        do {
            let response: RecentAmountsResponse = try await api.get("/api/logs/recent-amounts",
                query: [URLQueryItem(name: "choreId", value: String(choreId))])
            guard !Task.isCancelled, api.identity.isCurrent(owner), state.owns(operation) else { return nil }
            state.recentAmounts[choreId] = response.amounts
            return response.amounts
        } catch { return nil }
    }
}
