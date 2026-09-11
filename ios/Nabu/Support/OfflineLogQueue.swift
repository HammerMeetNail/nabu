import Foundation

/// An immutable journal. A request is durable before its first POST and is
/// removed only after confirmed success or explicit discard by its owner.
@MainActor
final class OfflineLogQueue: ObservableObject {
    static let shared = OfflineLogQueue()

    struct Item: Codable, Equatable {
        let body: CreateLogRequest
        let queuedAt: Date
        let origin: LogOrigin?
        var failure: String?
        var needsRetry: Bool?
    }
    @Published private(set) var items: [Item] = []
    @Published private(set) var inFlight: Set<String> = []
    @Published private(set) var storageUnavailable = false
    private let fileURL: URL
    private let defaultOrigin: LogOrigin?

    init(fileURL: URL? = nil, defaultOrigin: LogOrigin? = nil) {
        let directory = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        self.fileURL = fileURL ?? directory.appendingPathComponent("nabu-offline-log-queue.json")
        self.defaultOrigin = defaultOrigin
        reload()
    }

    var count: Int { items.count }
    func reload() {
        guard inFlight.isEmpty else { return }
        guard FileManager.default.fileExists(atPath: fileURL.path) else {
            storageUnavailable = false
            return
        }
        do {
            let decoded = try JSONDecoder().decode([Item].self, from: Data(contentsOf: fileURL))
            items = decoded
            storageUnavailable = false
        } catch {
            // Keep the original file and any last-good memory data. A locked
            // or corrupt journal must never be overwritten by a new save.
            storageUnavailable = true
        }
    }
    func scopedItems(_ origin: LogOrigin) -> [Item] { items.filter { $0.origin == origin } }
    func item(key: String, origin: LogOrigin) -> Item? {
        items.first { $0.origin == origin && $0.body.idempotencyKey == key }
    }

    @discardableResult
    func enqueue(_ body: CreateLogRequest, origin: LogOrigin? = nil) -> Bool {
        guard let origin = origin ?? defaultOrigin,
              let key = body.idempotencyKey, !key.isEmpty else { return false }
        if let existing = items.first(where: { $0.body.idempotencyKey == key }) {
            return existing.origin == origin && existing.body == body
        }
        var next = items
        next.append(Item(body: body, queuedAt: Date(), origin: origin))
        return persist(next)
    }

    /// Used only by explicit test/reset tooling. Normal identity changes hide
    /// other origins and leave their recovery data untouched.
    @discardableResult func removeAll() -> Bool {
        guard inFlight.isEmpty else { return false }
        return persist([])
    }

    @discardableResult
    func discard(key: String, origin: LogOrigin) -> Bool {
        guard !inFlight.contains(key) else { return false }
        return persist(items.filter { !($0.origin == origin && $0.body.idempotencyKey == key) })
    }

    func submit<T>(body: CreateLogRequest, origin: LogOrigin,
                   isCurrent: () -> Bool,
                   post: (CreateLogRequest) async throws -> T) async throws -> T {
        guard isCurrent() else { throw APIError.contextChanged }
        guard let key = body.idempotencyKey else { throw APIError.saveNotDurable }
        guard !inFlight.contains(key) else { throw APIError.submissionInProgress }
        guard enqueue(body, origin: origin) else { throw APIError.saveNotDurable }
        inFlight.insert(key)
        defer { inFlight.remove(key) }
        do {
            let response = try await post(body)
            // Reconciliation always addresses this exact origin and key. If
            // local removal fails, a later replay uses the same server key.
            _ = persist(items.filter { !($0.origin == origin && $0.body.idempotencyKey == key) })
            return response
        } catch {
            if let index = items.firstIndex(where: { $0.origin == origin && $0.body.idempotencyKey == key }) {
                var next = items
                next[index].failure = (error as? APIError)?.errorDescription ?? "Could not confirm this save."
                next[index].needsRetry = !LogStore.isNetworkFailure(error)
                _ = persist(next)
            }
            throw error
        }
    }

    func replay(origin: LogOrigin? = nil, retryFailed: Bool = true,
                isCurrent: () -> Bool = { true },
                post: (CreateLogRequest) async throws -> Void) async -> Int {
        guard let origin = origin ?? defaultOrigin else { return 0 }
        let candidates = scopedItems(origin)
        var synced = 0
        for item in candidates {
            guard isCurrent() else { break }
            guard let key = item.body.idempotencyKey,
                  !inFlight.contains(key), self.item(key: key, origin: origin) != nil,
                  retryFailed || item.needsRetry != true else { continue }
            do {
                try await submit(body: item.body, origin: origin, isCurrent: isCurrent, post: post)
                synced += 1
            } catch {
                if LogStore.isNetworkFailure(error) { break }
            }
        }
        return synced
    }

    private func persist(_ next: [Item]) -> Bool {
        guard !storageUnavailable else { return false }
        do {
            try FileManager.default.createDirectory(at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
            try JSONEncoder().encode(next).write(to: fileURL, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
            items = next
            return true
        } catch { return false }
    }
}

/// A queued-but-unsynced log synthesized for display in Activity with a
/// "pending" badge (PWA Phase 2.1), reconciled (cleared) on the next
/// successful replay.
struct PendingLog: Identifiable, Equatable {
    let id: String // idempotencyKey
    let choreId: Int
    var userId: Int?
    let note: String
    let indicators: [String]
    let indicatorVolumes: [String: Int]
    let volumeML: Int?
    let rating: Int?
    let subject: String?
    let title: String?
    let completedAt: Date

    init(body: CreateLogRequest, fallbackUserId: Int?) {
        self.id = body.idempotencyKey ?? UUID().uuidString
        self.choreId = body.choreId
        self.userId = body.userId ?? fallbackUserId
        self.note = body.note ?? ""
        self.indicators = body.indicators ?? []
        self.indicatorVolumes = body.indicatorVolumes ?? [:]
        self.volumeML = body.volumeML
        self.rating = body.rating
        self.subject = body.subject
        self.title = body.title
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let basic = ISO8601DateFormatter()
        self.completedAt = body.completedAt.flatMap { formatter.date(from: $0) ?? basic.date(from: $0) } ?? Date()
    }
}
