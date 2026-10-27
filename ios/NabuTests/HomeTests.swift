import XCTest
import SwiftUI
@testable import Nabu

final class HomeTests: XCTestCase {

    // MARK: - HomeViewMode

    func testHomeViewModeLog() {
        XCTAssertEqual(HomeViewMode.log.title, "Log")
    }

    func testHomeViewModeManage() {
        XCTAssertEqual(HomeViewMode.manage.title, "Manage")
    }

    func testHomeViewModeAllCases() {
        XCTAssertEqual(HomeViewMode.allCases.count, 2)
        XCTAssertTrue(HomeViewMode.allCases.contains(.log))
        XCTAssertTrue(HomeViewMode.allCases.contains(.manage))
    }

    // MARK: - Color hex

    func testColorHexValid() {
        let color = Color(hex: "#FF0000")
        XCTAssertNotNil(color)
    }

    func testColorHexValidWithoutHash() {
        let color = Color(hex: "00FF00")
        XCTAssertNotNil(color)
    }

    func testColorHexInvalid() {
        XCTAssertNil(Color(hex: "not-a-color"))
        XCTAssertNil(Color(hex: "12345"))  // too short
    }

    // MARK: - Chore sorting

    func testSortChoresByPreferenceOrder() {
        let a = Chore(id: 1, householdId: 1, name: "A", icon: "a", color: "#000000",
                      sortOrder: 0, category: "test", isPredefined: false,
                      predefinedKey: nil, createdBy: nil, createdAt: Date(),
                      indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)
        let b = Chore(id: 2, householdId: 1, name: "B", icon: "b", color: "#000000",
                      sortOrder: 0, category: "test", isPredefined: false,
                      predefinedKey: nil, createdBy: nil, createdAt: Date(),
                      indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)
        let c = Chore(id: 3, householdId: 1, name: "C", icon: "c", color: "#000000",
                      sortOrder: 0, category: "test", isPredefined: false,
                      predefinedKey: nil, createdBy: nil, createdAt: Date(),
                      indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)

        let chores = [a, b, c]
        let order = [2, 1]  // b first, then a, c not in order (goes last)

        let orderMap = Dictionary(uniqueKeysWithValues: order.enumerated().map { ($1, $0) })
        let sorted = chores.sorted {
            (orderMap[$0.id] ?? Int.max) < (orderMap[$1.id] ?? Int.max)
        }

        XCTAssertEqual(sorted.map(\.id), [2, 1, 3])
    }

    func testSortChoresNoPreferenceOrder() {
        let a = Chore(id: 10, householdId: 1, name: "A", icon: "a", color: "#000000",
                      sortOrder: 0, category: "test", isPredefined: false,
                      predefinedKey: nil, createdBy: nil, createdAt: Date(),
                      indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)
        let b = Chore(id: 5, householdId: 1, name: "B", icon: "b", color: "#000000",
                      sortOrder: 0, category: "test", isPredefined: false,
                      predefinedKey: nil, createdBy: nil, createdAt: Date(),
                      indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)

        let sorted = [a, b].sorted { $0.id < $1.id }
        XCTAssertEqual(sorted.map(\.id), [5, 10])
    }

    // MARK: - formatTimeAgo

    func testTimeAgoJustNow() {
        let now = Date()
        XCTAssertEqual(formatTimeAgo(now), "just now")
    }

    func testTimeAgoMinutes() {
        let date = Date().addingTimeInterval(-300)  // 5 min ago
        XCTAssertEqual(formatTimeAgo(date), "5m ago")
    }

    func testTimeAgoHours() {
        let date = Date().addingTimeInterval(-7200)  // 2 hours ago
        XCTAssertEqual(formatTimeAgo(date), "2h ago")
    }

    func testTimeAgoYesterday() {
        let date = Date().addingTimeInterval(-90000)  // 25 hours ago
        XCTAssertEqual(formatTimeAgo(date), "yesterday")
    }

    func testTimeAgoDays() {
        let date = Date().addingTimeInterval(-259200)  // 3 days ago
        XCTAssertEqual(formatTimeAgo(date), "3d ago")
    }

    func testTimeAgoWeeks() {
        let date = Date().addingTimeInterval(-1209600)  // 2 weeks ago
        XCTAssertEqual(formatTimeAgo(date), "2w ago")
    }
}

/// A log write answers with its chore's latest log, so Home updates one tile
/// instead of reloading every chore's latest log (PWA `home-log-latest.spec.js`).
@MainActor
final class HomeLatestLogTests: XCTestCase {
    private func log(_ id: Int, minutesAgo: Double) -> ChoreLog {
        ChoreLog(id: id, householdId: 1, userId: 1, choreId: 5, completedAt: Date(timeIntervalSinceNow: -minutesAgo * 60),
                 note: "", indicators: [], slotHour: nil, createdAt: Date(), volumeML: nil, indicatorVolumes: nil)
    }

    private func logJSON(_ id: Int, at: String) -> String {
        #"{"id":\#(id),"householdId":1,"userId":1,"choreId":5,"completedAt":"\#(at)","note":"","indicators":[],"createdAt":"\#(at)"}"#
    }

    func testChangeAnswersTellNoLogsLeftFromNotReported() throws {
        let gone = try apiDecoder.decode(LogChangeResponse.self, from: Data(#"{"status":"deleted","latest":null}"#.utf8))
        XCTAssertTrue(gone.reportsLatest)
        XCTAssertNil(gone.latest)
        let older = try apiDecoder.decode(LogChangeResponse.self, from: Data(#"{"status":"deleted"}"#.utf8))
        XCTAssertFalse(older.reportsLatest, "an older server: reload instead")
        let edited = try apiDecoder.decode(LogChangeResponse.self,
            from: Data(#"{"status":"updated","latest":\#(logJSON(9, at: "2026-10-27T10:00:00Z"))}"#.utf8))
        XCTAssertEqual(edited.latest?.id, 9)
        let saved = try apiDecoder.decode(LogResponse.self,
            from: Data(#"{"log":\#(logJSON(10, at: "2026-10-27T07:00:00Z")),"latest":\#(logJSON(9, at: "2026-10-27T10:00:00Z"))}"#.utf8))
        XCTAssertEqual(saved.latest?.id, 9)
        XCTAssertNil(try apiDecoder.decode(LogResponse.self, from: Data(#"{"log":\#(logJSON(10, at: "2026-10-27T07:00:00Z"))}"#.utf8)).latest)
    }

    func testABackdatedSaveKeepsTheNewerTime() {
        let state = AppState()
        let shown = log(1, minutesAgo: 5)
        state.latestLogs[5] = shown
        // The server's answer wins.
        XCTAssertEqual(state.latestAfterSave(LogResponse(log: log(2, minutesAgo: 180), latest: shown), choreId: 5).id, 1)
        // An older server: the newer of the shown and the saved log.
        XCTAssertEqual(state.latestAfterSave(LogResponse(log: log(2, minutesAgo: 180)), choreId: 5).id, 1)
        XCTAssertEqual(state.latestAfterSave(LogResponse(log: log(3, minutesAgo: 0)), choreId: 5).id, 3)
        state.latestLogs = [:]
        XCTAssertEqual(state.latestAfterSave(LogResponse(log: log(2, minutesAgo: 180)), choreId: 5).id, 2)
    }

    func testApplyingTheAnswerSupersedesAnInFlightReload() {
        let state = AppState()
        state.latestLogs[5] = log(1, minutesAgo: 60)
        let reload = state.beginOperation("loadLatestLogsData")
        state.applyLatestLog(log(2, minutesAgo: 0), choreId: 5)
        XCTAssertEqual(state.latestLogs[5]?.id, 2)
        XCTAssertFalse(state.owns(reload), "an older latest-per-chore read must not overwrite the answer")
        state.applyLatestLog(nil, choreId: 5)
        XCTAssertNil(state.latestLogs[5])
    }

    func testLogStoreReturnsTheAnswersLatestLog() async throws {
        let identity = ClientIdentity()
        identity.accept(User(id: 1, householdId: 1, email: "t@nabu.local", displayName: "T", avatarColor: "#112233",
                             emailVerified: true, role: "owner", createdAt: Date(timeIntervalSince1970: 0)))
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
        let newest = logJSON(9, at: "2026-10-27T10:00:00Z")
        let bodies = [
            "POST": #"{"log":\#(logJSON(10, at: "2026-10-27T07:00:00Z")),"latest":\#(newest)}"#,
            "PATCH": #"{"status":"updated","latest":\#(newest)}"#,
            "DELETE": #"{"status":"deleted","latest":null}"#,
        ]
        api.mockHandler = { request in
            let body = bodies[request.httpMethod ?? ""] ?? "{}"
            return (Data(body.utf8), HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
        }
        let queue = OfflineLogQueue(fileURL: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
        let store = LogStore(api: api, offlineQueue: queue)
        guard case .created(let created) = try await store.createLog(choreId: 5, idempotencyKey: UUID().uuidString) else {
            return XCTFail("expected a created log")
        }
        XCTAssertEqual(created.latest?.id, 9)
        let edited = try await store.updateLog(logId: 10, note: "x")
        XCTAssertTrue(edited.reportsLatest)
        XCTAssertEqual(edited.latest?.id, 9)
        let deleted = try await store.deleteLog(logId: 9)
        XCTAssertTrue(deleted.reportsLatest)
        XCTAssertNil(deleted.latest)
    }
}
