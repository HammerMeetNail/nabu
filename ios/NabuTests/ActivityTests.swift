import XCTest
import Foundation
@testable import Nabu

final class ActivityTests: XCTestCase {

    // MARK: - Date helpers

    func testTodayISO() {
        let result = todayISO()
        XCTAssertTrue(result.matches("^\\d{4}-\\d{2}-\\d{2}$"))
    }

    func testShiftISODate() {
        let result = shiftISO("2026-06-01", by: 1)
        XCTAssertEqual(result, "2026-06-02")
    }

    func testShiftISODateBackward() {
        let result = shiftISO("2026-06-01", by: -1)
        XCTAssertEqual(result, "2026-05-31")
    }

    func testWeekStartYieldsMonday() {
        let result = weekStart(from: "2026-06-03") // Wednesday
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        guard let d = f.date(from: result) else { return XCTFail("Invalid date") }
        let weekday = Calendar.current.component(.weekday, from: d)
        XCTAssertEqual(weekday, 2, "Expected Monday (weekday=2), got \(weekday)")
    }

    // MARK: - fmtHour

    func testFmtHourMidnight() { XCTAssertEqual(fmtHour(0), "12 AM") }
    func testFmtHourOneAM() { XCTAssertEqual(fmtHour(1), "1 AM") }
    func testFmtHourElevenAM() { XCTAssertEqual(fmtHour(11), "11 AM") }
    func testFmtHourNoon() { XCTAssertEqual(fmtHour(12), "12 PM") }
    func testFmtHourOnePM() { XCTAssertEqual(fmtHour(13), "1 PM") }
    func testFmtHourElevenPM() { XCTAssertEqual(fmtHour(23), "11 PM") }

    // MARK: - Slot hour filtering

    func testAnytimeLogsHaveNullSlotHour() {
        let log = ChoreLog(id: 1, householdId: 1, userId: 1, choreId: 1,
                           completedAt: Date(), note: "", indicators: [],
                           slotHour: nil, createdAt: Date(), volumeML: nil,
                           indicatorVolumes: nil)
        XCTAssertNil(log.slotHour)
    }

    func testTimedLogsHaveSlotHour() {
        let log = ChoreLog(id: 1, householdId: 1, userId: 1, choreId: 1,
                           completedAt: Date(), note: "", indicators: [],
                           slotHour: 9, createdAt: Date(), volumeML: nil,
                           indicatorVolumes: nil)
        XCTAssertEqual(log.slotHour, 9)
    }

    func testFilterAnytimeLogs() {
        let anytime = ChoreLog(id: 1, householdId: 1, userId: 1, choreId: 1,
                                completedAt: Date(), note: "", indicators: [],
                                slotHour: nil, createdAt: Date(), volumeML: nil,
                                indicatorVolumes: nil)
        let timed = ChoreLog(id: 2, householdId: 1, userId: 1, choreId: 2,
                              completedAt: Date(), note: "", indicators: [],
                              slotHour: 9, createdAt: Date(), volumeML: nil,
                              indicatorVolumes: nil)
        let logs = [anytime, timed]
        let anytimeLogs = logs.filter { $0.slotHour == nil }
        XCTAssertEqual(anytimeLogs.count, 1)
    }

    // MARK: - Log grouping by date

    func testGroupLogsByDate() {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        let d1 = f.date(from: "2026-06-02")!
        let d2 = f.date(from: "2026-06-01")!

        let log1 = ChoreLog(id: 1, householdId: 1, userId: 1, choreId: 1,
                            completedAt: d1, note: "", indicators: [],
                            slotHour: 9, createdAt: Date(), volumeML: nil,
                            indicatorVolumes: nil)
        let log2 = ChoreLog(id: 2, householdId: 1, userId: 1, choreId: 2,
                            completedAt: d2, note: "", indicators: [],
                            slotHour: 10, createdAt: Date(), volumeML: nil,
                            indicatorVolumes: nil)

        let logs = [log1, log2]
        var groups: [String: [ChoreLog]] = [:]
        for log in logs {
            let dateStr = f.string(from: log.completedAt)
            groups[dateStr, default: []].append(log)
        }
        let sorted = groups.sorted { $0.key > $1.key }
        XCTAssertEqual(sorted.count, 2)
        XCTAssertEqual(sorted[0].key, "2026-06-02")
        XCTAssertEqual(sorted[1].key, "2026-06-01")
    }
    // MARK: - Chore filter (additive selection)

    private func makeLog(id: Int, choreId: Int) -> ChoreLog {
        ChoreLog(id: id, householdId: 1, userId: 1, choreId: choreId,
                 completedAt: Date(), note: "", indicators: [],
                 slotHour: nil, createdAt: Date(), volumeML: nil,
                 indicatorVolumes: nil)
    }

    func testFilterEmptySelectionReturnsAll() {
        let logs = [makeLog(id: 1, choreId: 1), makeLog(id: 2, choreId: 2)]
        let result = filterLogsByChores(logs, selected: [])
        XCTAssertEqual(result.count, 2)
    }

    func testFilterSingleChore() {
        let logs = [makeLog(id: 1, choreId: 1), makeLog(id: 2, choreId: 2), makeLog(id: 3, choreId: 1)]
        let result = filterLogsByChores(logs, selected: [1])
        XCTAssertEqual(result.count, 2)
        XCTAssertTrue(result.allSatisfy { $0.choreId == 1 })
    }

    func testFilterMultipleChoresIsAdditive() {
        let logs = [makeLog(id: 1, choreId: 1), makeLog(id: 2, choreId: 2), makeLog(id: 3, choreId: 3)]
        let result = filterLogsByChores(logs, selected: [1, 3])
        XCTAssertEqual(result.count, 2)
        XCTAssertEqual(Set(result.map { $0.choreId }), [1, 3])
    }

    func testFilterNoMatchReturnsEmpty() {
        let logs = [makeLog(id: 1, choreId: 1), makeLog(id: 2, choreId: 2)]
        let result = filterLogsByChores(logs, selected: [99])
        XCTAssertTrue(result.isEmpty)
    }

    // MARK: - Household CSV export

    @MainActor
    func testExportDateFiltersAndErrorsRemainVisible() async throws {
        var requested: URL?
        var api = APIClient(baseURL: URL(string: "http://localhost:8080")!)
        api.mockHandler = { request in
            requested = request.url
            return (Data("{\"error\":\"Choose a smaller date range\"}".utf8), HTTPURLResponse(url: request.url!, statusCode: 413, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!)
        }
        do {
            _ = try await ActivityStore(api: api).exportLogsCSV(start: "2026-09-01", end: "2026-09-10")
            XCTFail("oversized response was saved as a CSV")
        } catch {
            XCTAssertEqual(error.localizedDescription, "Choose a smaller date range")
        }
        XCTAssertEqual(requested?.path, "/api/logs/export")
        let items = URLComponents(url: try XCTUnwrap(requested), resolvingAgainstBaseURL: false)?.queryItems ?? []
        XCTAssertEqual(items.first(where: { $0.name == "start" })?.value, "2026-09-01")
        XCTAssertEqual(items.first(where: { $0.name == "end" })?.value, "2026-09-10")
    }

    @MainActor
    func testExportRejectsIncompleteOrNonCSVSuccess() async throws {
        for contentType in ["text/html", "text/csv"] {
            var api = APIClient(baseURL: URL(string: "http://localhost:8080")!)
            api.mockHandler = { request in
                (Data(), HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": contentType])!)
            }
            do { _ = try await ActivityStore(api: api).exportHouseholdCSV(); XCTFail("incomplete export became a file") }
            catch { /* The caller presents this error and keeps the date selection. */ }
        }
    }

    @MainActor
    func testHouseholdExportDownloadsCSVFromAdminEndpoint() async throws {
        var requestedPath: String?
        var api = APIClient(baseURL: URL(string: "http://localhost:8080")!)
        api.mockHandler = { request in
            requestedPath = request.url?.path
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "text/csv; charset=utf-8"]
            )!
            return (Data("record_type,id\nhousehold,1\n".utf8), response)
        }

        let url = try await ActivityStore(api: api).exportHouseholdCSV()
        defer { try? FileManager.default.removeItem(at: url) }

        XCTAssertEqual(requestedPath, "/api/household/data")
        XCTAssertEqual(try String(contentsOf: url, encoding: .utf8), "record_type,id\nhousehold,1\n")
    }
}

private extension String {
    func matches(_ pattern: String) -> Bool {
        range(of: pattern, options: .regularExpression) != nil
    }
}

@MainActor
final class ActivityOwnershipTests: XCTestCase {
    private func user(_ id: Int) -> User {
        User(id: id, householdId: 1, email: "test@nabu.local", displayName: "Test",
             avatarColor: "#112233", emailVerified: true, role: "owner", createdAt: Date())
    }

    func testCanceledOrOldIdentityExportCannotCreateAFile() async throws {
        for changeIdentity in [false, true] {
            let identity = ClientIdentity()
            identity.accept(user(1))
            var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
            let gate = NativeResponseGate()
            api.mockAsyncHandler = { request in
                await gate.pause()
                return (Data("date,note\n2026-09-10,old\n".utf8), HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "text/csv"])!)
            }
            let store = ActivityStore(api: api)
            let old = Task { try await store.exportLogsCSV() }
            await gate.waitUntilPaused()
            if changeIdentity { identity.accept(user(2)) } else { old.cancel() }
            // Exercise the final ownership fence even if transport ignores cancel.
            await gate.release()
            do {
                let unexpected = try await old.value
                try? FileManager.default.removeItem(at: unexpected)
                XCTFail("obsolete export returned a file")
            } catch {
                if case APIError.contextChanged = error { /* Expected identity fence. */ }
                else { XCTAssertTrue(error is CancellationError) }
            }
        }
    }

    func testConfirmedDeletionInvalidatesPendingRefresh() async throws {
        let state = AppState()
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
        let gate = NativeResponseGate()
        let row = ChoreLog(id: 42, householdId: 1, userId: 1, choreId: 1,
                           completedAt: Date(), note: "historical row", indicators: [],
                           slotHour: nil, createdAt: Date(), volumeML: nil, indicatorVolumes: nil)
        let snapshot = try apiEncoder.encode(HistoryResponse(logs: [row], hasMore: false, start: "start", end: "end"))
        api.mockAsyncHandler = { request in
            XCTAssertEqual(request.url!.path, "/api/logs/history")
            await gate.pause()
            return (snapshot, HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
        }
        let model = ActivityModel(store: ActivityStore(api: api))
        model.configure(state: state)
        model.logs = [row]
        let old = Task { await model.refresh() }
        await gate.waitUntilPaused()
        model.confirmDeletion(row.id)
        await gate.release()
        await old.value
        XCTAssertTrue(model.logs.isEmpty)
        XCTAssertFalse(model.isLoading)
        XCTAssertNil(model.errorMessage)
    }

    func testOldRefreshCannotPublishDuringNewQueryDebounce() async {
        for status in [200, 500] {
            let state = AppState()
            var api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
            let gate = NativeResponseGate()
            api.mockAsyncHandler = { request in
                if request.url!.path == "/api/day-notes" {
                    XCTFail("Obsolete refresh must not start dependent day notes")
                }
                await gate.pause()
                return (Data(#"{"logs":[],"hasMore":true,"start":"old-cursor","end":"end"}"#.utf8),
                        HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!)
            }
            let model = ActivityModel(store: ActivityStore(api: api))
            model.configure(state: state)
            let old = Task { await model.refresh() }
            await gate.waitUntilPaused()
            // The binding invalidates A synchronously. B has not started its
            // request: this is the exact vulnerable debounce interval.
            model.selectQuery("new")
            await gate.release()
            await old.value
            XCTAssertTrue(model.isLoading)
            XCTAssertNil(model.errorMessage)
            XCTAssertEqual(model.searchResults, [])
            XCTAssertNil(model.before)
            XCTAssertFalse(model.hasMore)
        }
    }

    func testRetainedActivityCannotSendForNewIdentity() async {
        let identity = ClientIdentity()
        identity.accept(user(1))
        let state = AppState()
        state.adopt(identity.snapshot)
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
        let gate = NativeResponseGate()
        api.mockAsyncHandler = { request in
            XCTAssertEqual(request.value(forHTTPHeaderField: "X-Nabu-User-ID"), "1")
            await gate.pause()
            return (Data(#"{"logs":[],"hasMore":true,"start":"old","end":"end"}"#.utf8),
                    HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
        }
        let model = ActivityModel(store: ActivityStore(api: api))
        model.configure(state: state)
        let old = Task { await model.refresh() }
        await gate.waitUntilPaused()
        identity.accept(user(2))
        state.adopt(identity.snapshot)
        await gate.release()
        await old.value
        // This returns without transport; the only installed gate was released.
        await model.refresh()
        XCTAssertFalse(model.hasMore)
        XCTAssertNil(model.before)
        XCTAssertNil(model.errorMessage)
    }

    func testPaginationCannotReplaceRefreshCursor() async {
        let state = AppState()
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
        let gate = NativeResponseGate()
        api.mockAsyncHandler = { request in
            if request.url!.path == "/api/day-notes" {
                return (Data(#"{"notes":[]}"#.utf8), HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
            }
            let pagination = request.url!.query != nil
            if pagination { await gate.pause() }
            let json = pagination ? #"{"logs":[],"hasMore":false,"start":"obsolete","end":"end"}"# : #"{"logs":[],"hasMore":true,"start":"current","end":"end"}"#
            return (Data(json.utf8), HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
        }
        let model = ActivityModel(store: ActivityStore(api: api))
        model.configure(state: state)
        await model.refresh()
        let old = Task { await model.loadMore() }
        await gate.waitUntilPaused()
        await model.refresh()
        await gate.release()
        await old.value
        XCTAssertEqual(model.before, "current")
        XCTAssertTrue(model.hasMore)
        XCTAssertFalse(model.isLoading)
        XCTAssertFalse(model.isLoadingMore)
    }
}
