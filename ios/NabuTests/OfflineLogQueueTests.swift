import XCTest
@testable import Nabu

/// Durable, actor/household-owned recovery matching the PWA journal.
@MainActor
final class OfflineLogQueueTests: XCTestCase {
    private var fileURL: URL!
    private let origin = LogOrigin(actorID: 1, householdID: 1)

    override func setUp() async throws {
        try await super.setUp()
        fileURL = FileManager.default.temporaryDirectory
            .appendingPathComponent("offline-queue-tests-\(UUID().uuidString).json")
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: fileURL)
        try await super.tearDown()
    }

    private func makeQueue() -> OfflineLogQueue {
        OfflineLogQueue(fileURL: fileURL, defaultOrigin: origin)
    }

    private func body(key: String?, choreId: Int = 1, note: String = "") -> CreateLogRequest {
        CreateLogRequest(
            choreId: choreId, note: note, indicators: nil, date: nil, hour: nil,
            completedAt: "2026-07-03T09:00:00Z", volumeML: nil, userId: nil,
            indicatorVolumes: nil, followUpMinutes: nil, followUpTime: nil,
            idempotencyKey: key
        )
    }

    // MARK: - Enqueue

    func testEnqueueRequiresIdempotencyKey() {
        let queue = makeQueue()
        XCTAssertFalse(queue.enqueue(body(key: nil)))
        XCTAssertFalse(queue.enqueue(body(key: "")))
        XCTAssertEqual(queue.count, 0)
    }

    func testEnqueueStoresItem() {
        let queue = makeQueue()
        XCTAssertTrue(queue.enqueue(body(key: "k1")))
        XCTAssertEqual(queue.count, 1)
    }

    func testReEnqueueSameKeyCannotChangePayload() {
        let queue = makeQueue()
        queue.enqueue(body(key: "k1", note: "first"))
        XCTAssertFalse(queue.enqueue(body(key: "k1", note: "second")))
        XCTAssertEqual(queue.count, 1)
        XCTAssertEqual(queue.items.first?.body.note, "first")
    }

    func testPersistsAcrossInstances() {
        let queue = makeQueue()
        queue.enqueue(body(key: "k1"))
        queue.enqueue(body(key: "k2"))

        let reloaded = makeQueue()
        XCTAssertEqual(reloaded.count, 2)
        XCTAssertEqual(reloaded.items.map { $0.body.idempotencyKey }, ["k1", "k2"])
    }

    // MARK: - Replay

    func testReplaySuccessRemovesAndCounts() async {
        let queue = makeQueue()
        queue.enqueue(body(key: "k1"))
        queue.enqueue(body(key: "k2"))

        var posted: [String?] = []
        let synced = await queue.replay { body in
            posted.append(body.idempotencyKey)
        }
        XCTAssertEqual(synced, 2)
        XCTAssertEqual(queue.count, 0)
        XCTAssertEqual(posted, ["k1", "k2"])
    }

    func testReplayRetainsPermanentClientError() async {
        let queue = makeQueue()
        queue.enqueue(body(key: "gone", choreId: 99))

        let synced = await queue.replay { _ in
            throw APIError.serverError(statusCode: 404, message: "chore not found")
        }
        XCTAssertEqual(synced, 0)
        XCTAssertEqual(queue.count, 1, "a rejected save stays available for explicit recovery")
    }

    func testReplayKeeps429AndContinues() async {
        let queue = makeQueue()
        queue.enqueue(body(key: "k1"))
        queue.enqueue(body(key: "k2"))

        var attempts = 0
        let synced = await queue.replay { body in
            attempts += 1
            if body.idempotencyKey == "k1" {
                throw APIError.rateLimited(retryAfter: "5")
            }
        }
        XCTAssertEqual(synced, 1)
        XCTAssertEqual(attempts, 2, "429 keeps the item but continues the pass")
        XCTAssertEqual(queue.items.map { $0.body.idempotencyKey }, ["k1"])
    }

    func testReplayKeepsServerErrorAndContinues() async {
        let queue = makeQueue()
        queue.enqueue(body(key: "k1"))
        queue.enqueue(body(key: "k2"))

        let synced = await queue.replay { body in
            if body.idempotencyKey == "k1" {
                throw APIError.serverError(statusCode: 500, message: "boom")
            }
        }
        XCTAssertEqual(synced, 1)
        XCTAssertEqual(queue.items.map { $0.body.idempotencyKey }, ["k1"])
    }

    func testReplayStopsOnNetworkFailure() async {
        let queue = makeQueue()
        queue.enqueue(body(key: "k1"))
        queue.enqueue(body(key: "k2"))

        var attempts = 0
        let synced = await queue.replay { _ in
            attempts += 1
            throw URLError(.notConnectedToInternet)
        }
        XCTAssertEqual(synced, 0)
        XCTAssertEqual(attempts, 1, "still offline — stop the pass, don't hammer")
        XCTAssertEqual(queue.count, 2)
    }

    func testReplayRetainsUnconfirmedResponse() async {
        // A 2xx without a confirmed response must retain the immutable request.
        let queue = makeQueue()
        queue.enqueue(body(key: "k1"))

        struct Dummy: Error {}
        let synced = await queue.replay { _ in
            throw APIError.decodingError(Dummy())
        }
        XCTAssertEqual(synced, 0)
        XCTAssertEqual(queue.count, 1)
    }

    func testReplayMixedPass() async {
        let queue = makeQueue()
        queue.enqueue(body(key: "ok1"))
        queue.enqueue(body(key: "bad"))
        queue.enqueue(body(key: "ok2"))

        let synced = await queue.replay { body in
            if body.idempotencyKey == "bad" {
                throw APIError.httpError(statusCode: 400)
            }
        }
        XCTAssertEqual(synced, 2)
        XCTAssertEqual(queue.items.map { $0.body.idempotencyKey }, ["bad"])
    }

    func testPersistenceFailureDoesNotClaimDurableSave() throws {
        try Data("occupied".utf8).write(to: fileURL)
        let queue = OfflineLogQueue(fileURL: fileURL.appendingPathComponent("queue.json"), defaultOrigin: origin)
        XCTAssertFalse(queue.enqueue(body(key: "unsaved")))
        XCTAssertEqual(queue.count, 0)
    }

    func testReplayCannotCrossActorOrHousehold() async {
        let queue = makeQueue()
        XCTAssertTrue(queue.enqueue(body(key: "owned")))
        for foreign in [LogOrigin(actorID: 2, householdID: 1), LogOrigin(actorID: 1, householdID: 2)] {
            let count = await queue.replay(origin: foreign) { _ in XCTFail("Foreign replay") }
            XCTAssertEqual(count, 0)
        }
        XCTAssertEqual(makeQueue().count, 1)
    }

    func testConcurrentReplayHasOneOwnerAndDiscardWaits() async {
        let queue = makeQueue()
        queue.enqueue(body(key: "once"))
        let gate = NativeResponseGate()
        let first = Task { await queue.replay { _ in await gate.pause() } }
        await gate.waitUntilPaused()
        let second = await queue.replay { _ in XCTFail("Duplicate replay") }
        XCTAssertEqual(second, 0)
        XCTAssertFalse(queue.discard(key: "once", origin: origin))
        await gate.release()
        let synced = await first.value
        XCTAssertEqual(synced, 1)
        XCTAssertEqual(queue.count, 0)
    }

    func testLegacyItemsRemainQuarantined() throws {
        let old = [["body": try JSONSerialization.jsonObject(with: JSONEncoder().encode(body(key: "legacy"))),
                    "queuedAt": Date().timeIntervalSinceReferenceDate]]
        try JSONSerialization.data(withJSONObject: old).write(to: fileURL)
        let queue = makeQueue()
        XCTAssertEqual(queue.count, 1)
        XCTAssertTrue(queue.scopedItems(origin).isEmpty)
    }

    func testUnreadableJournalIsNotOverwritten() throws {
        let original = Data("corrupt but recoverable saved data".utf8)
        try original.write(to: fileURL)
        let queue = makeQueue()
        XCTAssertTrue(queue.storageUnavailable)
        XCTAssertFalse(queue.enqueue(body(key: "new")))
        XCTAssertEqual(try Data(contentsOf: fileURL), original)
    }

    // MARK: - PendingLog synthesis

    func testPendingLogFromBody() {
        let request = CreateLogRequest(
            choreId: 4, note: "big feed", indicators: ["🍼 formula"],
            date: "2026-07-03", hour: 9, completedAt: "2026-07-03T09:15:00Z",
            volumeML: 120, userId: nil, indicatorVolumes: ["🍼 formula": 120],
            followUpMinutes: nil, followUpTime: nil,
            rating: nil, title: nil, durationSeconds: nil, subject: "Ada",
            idempotencyKey: "key-1"
        )
        let pending = PendingLog(body: request, fallbackUserId: 42)
        XCTAssertEqual(pending.id, "key-1")
        XCTAssertEqual(pending.choreId, 4)
        XCTAssertEqual(pending.userId, 42)
        XCTAssertEqual(pending.note, "big feed")
        XCTAssertEqual(pending.volumeML, 120)
        XCTAssertEqual(pending.subject, "Ada")
        XCTAssertEqual(pending.indicatorVolumes, ["🍼 formula": 120])
        let expected = ISO8601DateFormatter().date(from: "2026-07-03T09:15:00Z")!
        XCTAssertEqual(pending.completedAt, expected)
    }
}

@MainActor
final class NativeSubmissionContractTests: XCTestCase {
    private func identity() -> ClientIdentity {
        let identity = ClientIdentity()
        identity.accept(User(id: 1, householdId: 1, email: "test@nabu.local", displayName: "Test",
                             avatarColor: "#112233", emailVerified: true, role: "owner", createdAt: Date()))
        return identity
    }
    private func response() throws -> Data {
        try apiEncoder.encode(LogResponse(log: ChoreLog(id: 42, householdId: 1, userId: 1, choreId: 1,
            completedAt: Date(), note: "first", indicators: [], slotHour: 12, createdAt: Date(), volumeML: nil, indicatorVolumes: nil)))
    }

    func testRejectedSubmissionKeepsFrozenBodyAcrossRelaunchAndRetry() async throws {
        for status in [400, 401, 403, 429, 500] {
            let file = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
            defer { try? FileManager.default.removeItem(at: file) }
            let identity = identity()
            let user = identity.snapshot.user
            var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
            let queue = OfflineLogQueue(fileURL: file)
            api.mockHandler = { request in
                let disk = OfflineLogQueue(fileURL: file)
                XCTAssertEqual(disk.count, 1, "Persist before sending the first POST")
                return (Data("{}".utf8), HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!)
            }
            do {
                _ = try await LogStore(api: api, offlineQueue: queue).createLog(choreId: 1, note: "first", slotHour: 12, durationSeconds: 90, idempotencyKey: "key")
                XCTFail("HTTP failure must retain the draft, not report completion")
            } catch { }
            let frozen = try XCTUnwrap(queue.items.first?.body)
            XCTAssertNotNil(frozen.completedAt)
            let reloaded = OfflineLogQueue(fileURL: file)
            identity.accept(user)
            let accepted = try response()
            api.mockHandler = { request in
                let retried = try! apiDecoder.decode(CreateLogRequest.self, from: request.httpBody!)
                XCTAssertEqual(retried, frozen, "Retry ignores later edits to this logical submission")
                return (accepted, HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
            }
            let result = try await LogStore(api: api, offlineQueue: reloaded).createLog(choreId: 99, note: "changed", slotHour: 1, durationSeconds: 999, idempotencyKey: "key")
            guard case .created(let response) = result else { return XCTFail("Expected confirmation") }
            XCTAssertEqual(response.log.id, 42)
            XCTAssertEqual(OfflineLogQueue(fileURL: file).count, 0)
        }
    }

    func testLostResponseReplaysTheExactDurablePayload() async throws {
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: file) }
        let identity = identity()
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
        api.mockAsyncHandler = { _ in throw URLError(.networkConnectionLost) }
        let queue = OfflineLogQueue(fileURL: file)
        let outcome = try await LogStore(api: api, offlineQueue: queue).createLog(choreId: 1, note: "first", durationSeconds: 90, idempotencyKey: "lost")
        guard case .queued = outcome else { return XCTFail("Expected a durable queued result") }
        let frozen = try XCTUnwrap(queue.items.first?.body)
        let accepted = try response()
        api.mockAsyncHandler = nil
        api.mockHandler = { request in
            XCTAssertEqual(try! apiDecoder.decode(CreateLogRequest.self, from: request.httpBody!), frozen)
            return (accepted, HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
        }
        let store = LogStore(api: api, offlineQueue: OfflineLogQueue(fileURL: file))
        let synced = await store.replayOfflineQueue()
        XCTAssertEqual(synced, 1)
        XCTAssertEqual(OfflineLogQueue(fileURL: file).count, 0)
    }

    func testDiskFailurePreventsRequestAndQueuedSuccess() async throws {
        let occupied = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try Data("occupied".utf8).write(to: occupied)
        defer { try? FileManager.default.removeItem(at: occupied) }
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity())
        api.mockHandler = { _ in XCTFail("Undurable request reached transport"); return nil }
        let queue = OfflineLogQueue(fileURL: occupied.appendingPathComponent("journal"))
        do {
            _ = try await LogStore(api: api, offlineQueue: queue).createLog(choreId: 1)
            XCTFail("Must not claim success without a durable save")
        } catch APIError.saveNotDurable { }
        catch { XCTFail("Unexpected error: \(error)") }
    }
}
