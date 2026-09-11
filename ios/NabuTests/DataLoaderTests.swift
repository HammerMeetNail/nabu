import XCTest
@testable import Nabu

@MainActor
final class DataLoaderTests: XCTestCase {
    var state: AppState!
    var api: APIClient!
    var dataLoader: DataLoader!

    override func setUp() async throws {
        state = AppState()
        api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
        dataLoader = DataLoader()
        dataLoader.configure(api: api, state: state)
    }

    func testDataLoaderConfiguration() {
        XCTAssertNotNil(dataLoader.household)
        XCTAssertNotNil(dataLoader.chores)
        XCTAssertNotNil(dataLoader.logs)
        XCTAssertNotNil(dataLoader.schedules)
        XCTAssertNotNil(dataLoader.notifs)
        XCTAssertNotNil(dataLoader.preferences)
    }

    func testReloadAfterAuthWithoutUser() async {
        // No user set — should gracefully skip
        await dataLoader.reloadAfterAuth()
        // Should not crash
        XCTAssertTrue(true)
    }

    func testForegroundRefreshWithoutUser() async {
        await dataLoader.foregroundRefresh()
        // Should not crash
        XCTAssertTrue(true)
    }

    func testReloadAfterAuthWithUserNoHousehold() async {
        state.user = User(id: 1, householdId: nil, email: "test@test.com",
                          displayName: "Test", avatarColor: "#FF0000",
                          emailVerified: true, role: "owner",
                          createdAt: Date())
        await dataLoader.reloadAfterAuth()
        // Phase 1 (household + preferences) runs, Phase 3 skipped
        // All API calls should fail silently since there's no server
        XCTAssertTrue(true)
    }

    func testActivityViewWhenEmpty() {
        state.todayLogs = []
        XCTAssertTrue(state.todayLogs.isEmpty)
    }

    func testActivityViewWithLogs() {
        let log = ChoreLog(id: 1, householdId: 1, userId: 1, choreId: 1,
                           completedAt: Date(), note: "", indicators: [],
                           slotHour: 9, createdAt: Date(), volumeML: nil,
                           indicatorVolumes: nil)
        state.todayLogs = [log]
        XCTAssertFalse(state.todayLogs.isEmpty)
    }
}

/// Every test awaits the request continuation and the enclosing task, so a stale
/// completion cannot escape a negative assertion after the test has finished.
actor NativeResponseGate {
    private var paused = false
    private var entryExpired = false
    private var releaseContinuation: CheckedContinuation<Void, Never>?
    private var observers: [CheckedContinuation<Void, Never>] = []

    func pause() async {
        // Single-use barrier: unexpected later requests finish so negative
        // request-count assertions fail instead of hanging the test suite.
        guard !paused && !entryExpired else { return }
        await withCheckedContinuation { continuation in
            releaseContinuation = continuation
            paused = true
            observers.forEach { $0.resume() }
            observers = []
        }
    }
    func waitUntilPaused(file: StaticString = #filePath, line: UInt = #line) async {
        if paused || entryExpired { return }
        let deadline = Task { [weak self] in
            do { try await Task.sleep(nanoseconds: 5_000_000_000) } catch { return }
            await self?.expireEntry(file: file, line: line)
        }
        await withCheckedContinuation { observers.append($0) }
        deadline.cancel()
        await deadline.value
    }
    private func expireEntry(file: StaticString, line: UInt) {
        guard !paused && !entryExpired else { return }
        entryExpired = true
        XCTFail("Expected request did not reach its response barrier", file: file, line: line)
        observers.forEach { $0.resume() }
        observers = []
    }
    func release() {
        releaseContinuation?.resume()
        releaseContinuation = nil
    }
}

@MainActor
final class NativeRecentAmountsTests: XCTestCase {
    private func user(_ id: Int) -> User {
        User(id: id, householdId: id, email: "test@nabu.local", displayName: "Test",
             avatarColor: "#112233", emailVerified: true, role: "owner", createdAt: Date())
    }

    func testRecentAmountsUseDedicatedAPIAndRetainLastGoodOnFailure() async {
        let identity = ClientIdentity()
        identity.accept(user(1))
        let state = AppState()
        state.adopt(identity.snapshot)
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
        var requests = 0
        api.mockAsyncHandler = { request in
            requests += 1
            XCTAssertEqual(request.url?.path, "/api/logs/recent-amounts")
            XCTAssertEqual(URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems,
                           [URLQueryItem(name: "choreId", value: "7")])
            let status = requests == 2 ? 503 : 200
            let body = requests == 1 ? "{\"amounts\":[120,90,60]}" : "{\"amounts\":[]}"
            return (Data(body.utf8), HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!)
        }
        let store = LogStore(api: api)
        let first = await store.loadRecentAmounts(choreId: 7, state: state)
        XCTAssertEqual(first, [120,90,60])
        XCTAssertTrue(state.todayLogs.isEmpty)
        let failed = await store.loadRecentAmounts(choreId: 7, state: state)
        XCTAssertNil(failed)
        XCTAssertEqual(state.recentAmounts[7], [120,90,60])
        let empty = await store.loadRecentAmounts(choreId: 7, state: state)
        XCTAssertEqual(empty, [])
        XCTAssertEqual(state.recentAmounts[7], [])
        XCTAssertEqual(requests, 3)
    }

    func testOldOrCanceledRecentAmountsCannotReplaceCurrentCache() async {
        for action in ["refresh", "identity", "cancel"] {
            let identity = ClientIdentity()
            identity.accept(user(1))
            let state = AppState()
            state.adopt(identity.snapshot)
            state.recentAmounts[7] = [60]
            let gate = NativeResponseGate()
            var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
            var requests = 0
            api.mockAsyncHandler = { request in
                requests += 1
                let first = requests == 1
                if first { await gate.pause() }
                return (Data((first ? "{\"amounts\":[120]}" : "{\"amounts\":[90]}").utf8),
                        HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
            }
            let store = LogStore(api: api)
            let old = Task { await store.loadRecentAmounts(choreId: 7, state: state) }
            await gate.waitUntilPaused()
            if action == "refresh" {
                let current = await store.loadRecentAmounts(choreId: 7, state: state)
                XCTAssertEqual(current, [90])
            } else if action == "identity" {
                identity.accept(user(2))
                state.adopt(identity.snapshot)
            } else { old.cancel() }
            await gate.release()
            let obsolete = await old.value
            XCTAssertNil(obsolete, action)
            XCTAssertEqual(state.recentAmounts[7], action == "refresh" ? [90] : action == "cancel" ? [60] : nil, action)
        }
    }
}

@MainActor
final class NativeRequestOwnershipTests: XCTestCase {
    private func user(_ id: Int, household: Int = 1) -> User {
        User(id: id, householdId: household, email: "test@nabu.local",
             displayName: "Test", avatarColor: "#112233", emailVerified: true,
             role: "owner", createdAt: Date(timeIntervalSince1970: 0))
    }

    func testCopiesShareOwnershipAndRejectOldCompletion() async throws {
        let identity = ClientIdentity()
        identity.accept(user(1))
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
        let gate = NativeResponseGate()
        api.mockAsyncHandler = { request in
            XCTAssertEqual(request.value(forHTTPHeaderField: "X-Nabu-User-ID"), "1")
            XCTAssertEqual(request.value(forHTTPHeaderField: "X-Nabu-Household-ID"), "1")
            await gate.pause()
            return (Data("{\"status\":\"ok\"}".utf8), HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
        }
        let copy = api
        let old = Task { () -> Bool in
            do { let _: StatusResponse = try await copy.get("/api/chores"); return false }
            catch APIError.contextChanged { return true }
            catch { return false }
        }
        await gate.waitUntilPaused()
        identity.accept(user(2))
        await gate.release()
        let rejected = await old.value
        XCTAssertTrue(rejected)
    }

    func testPendingLogoutSurvivesRelaunchAndBlocksOrdinaryRequests() async throws {
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: file) }
        let identity = ClientIdentity(fileURL: file)
        identity.accept(user(1))
        _ = try identity.beginTransition(logout: true)
        let reloaded = ClientIdentity(fileURL: file)
        XCTAssertEqual(reloaded.snapshot.phase, .logoutPending)
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: reloaded)
        api.mockHandler = { _ in XCTFail("Must not send under unfinished logout"); return nil }
        do { let _: StatusResponse = try await api.postEmpty("/api/logs"); XCTFail("Expected blocked request") }
        catch APIError.contextChanged { }
        catch { XCTFail("Unexpected error: \(error)") }
    }

    func testStaleConfirmationAndTransitionCannotReplaceNewIdentity() throws {
        let identity = ClientIdentity()
        identity.accept(user(1))
        let old = identity.snapshot
        identity.accept(user(2))
        XCTAssertFalse(identity.accept(user(1), expected: old))
        XCTAssertFalse(identity.accept(nil, expected: old))
        XCTAssertThrowsError(try identity.beginTransition(expected: old))
        XCTAssertEqual(identity.snapshot.user?.id, 2)
    }

    func testBoundMutationRejectsBeforeTransport() async {
        let identity = ClientIdentity()
        identity.accept(user(1))
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity).scoped()
        api.mockHandler = { _ in XCTFail("Wrong-origin mutation reached transport"); return nil }
        identity.accept(user(2))
        do { let _: StatusResponse = try await api.postEmpty("/api/preferences"); XCTFail("Expected rejection") }
        catch APIError.contextChanged { }
        catch { XCTFail("Unexpected error: \(error)") }
    }

    func testServerContextMismatchClearsOldState() async {
        let identity = ClientIdentity()
        identity.accept(user(1))
        let state = AppState()
        state.adopt(identity.snapshot)
        identity.observer = { state.adopt($0) }
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
        api.mockHandler = { request in
            (Data("changed".utf8), HTTPURLResponse(url: request.url!, statusCode: 409, httpVersion: nil, headerFields: ["X-Nabu-Context-Changed": "true"])!)
        }
        let _: StatusResponse? = try? await api.get("/api/household")
        XCTAssertEqual(identity.snapshot.phase, .checking)
        XCTAssertNil(state.user)
    }

    func testLogoutStorageFailureHidesIdentityWithoutClaimingDurability() throws {
        let occupied = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try Data("occupied".utf8).write(to: occupied)
        defer { try? FileManager.default.removeItem(at: occupied) }
        let identity = ClientIdentity(fileURL: occupied.appendingPathComponent("marker"))
        identity.accept(user(1))
        XCTAssertThrowsError(try identity.beginTransition(logout: true))
        XCTAssertNil(identity.snapshot.user)
        XCTAssertEqual(identity.snapshot.phase, .logoutPending)
        XCTAssertFalse(identity.snapshot.logoutIsDurable)
    }

    func testOldPreferenceFailureCannotRestoreNewAccountValues() async {
        let identity = ClientIdentity()
        identity.accept(user(1))
        let state = AppState()
        state.adopt(identity.snapshot)
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
        let gate = NativeResponseGate()
        api.mockAsyncHandler = { request in
            XCTAssertEqual(request.value(forHTTPHeaderField: "X-Nabu-User-ID"), "1")
            await gate.pause()
            return (Data("{}".utf8), HTTPURLResponse(url: request.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!)
        }
        let loader = PreferencesDataLoader(api: api, state: state)
        let old = Task { await loader.setVolumeUnit("oz") }
        await gate.waitUntilPaused()
        identity.accept(user(2))
        state.adopt(identity.snapshot)
        state.volumeUnit = "ml"
        await gate.release()
        await old.value
        XCTAssertEqual(state.user?.id, 2)
        XCTAssertEqual(state.volumeUnit, "ml")
    }

    func testResetClearsTimerNotificationsAndPendingViews() {
        let state = AppState()
        state.activeTimer = ActiveTimer(choreId: 1, choreName: "Task", choreIcon: "⏱", startedAt: Date())
        state.unreadNotifications = 4
        state.activeSheet = .logSheet
        state.resetHouseholdScoped()
        XCTAssertNil(state.activeTimer)
        XCTAssertEqual(state.unreadNotifications, 0)
        XCTAssertNil(state.activeSheet)
    }
}

@MainActor
final class NativeLogoutRecoveryTests: XCTestCase {
    func testRejectedLogoutSurvivesRelaunchAndRetryWithoutDiscardingCookie() async throws {
        for status in [403, 429, 500, 0] {
            let file = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
            defer { try? FileManager.default.removeItem(at: file) }
            let identity = ClientIdentity(fileURL: file)
            identity.accept(User(id: 1, householdId: 1, email: "test@nabu.local", displayName: "Test",
                                 avatarColor: "#112233", emailVerified: true, role: "owner", createdAt: Date()))
            let state = AppState()
            state.adopt(identity.snapshot)
            identity.observer = { state.adopt($0) }
            let cookie = HTTPCookie(properties: [.domain:"localhost", .path:"/", .name:"nabu_session", .value:"retained-test-session"])!
            HTTPCookieStorage.shared.setCookie(cookie)
            defer { HTTPCookieStorage.shared.deleteCookie(cookie) }
            var api = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
            api.mockAsyncHandler = { request in
                if status == 0 { throw URLError(.networkConnectionLost) }
                return (Data("{}".utf8), HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!)
            }
            do { let _: StatusResponse = try await api.postEmpty("/api/auth/logout"); XCTFail("Expected unfinished logout") }
            catch { }
            XCTAssertNil(state.user)
            XCTAssertEqual(state.sessionPhase, .logoutPending)
            XCTAssertEqual(api.cookieStore.sessionCookie?.value, "retained-test-session")
            let restored = ClientIdentity(fileURL: file)
            XCTAssertEqual(restored.snapshot.phase, .logoutPending)
            var retry = APIClient(baseURL: api.baseURL, identity: restored)
            retry.mockHandler = { request in
                XCTAssertEqual(request.url!.path, "/api/auth/logout")
                return (Data(#"{"status":"ok"}"#.utf8), HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
            }
            let _: StatusResponse = try await retry.postEmpty("/api/auth/logout")
            XCTAssertNil(retry.cookieStore.sessionCookie)
            XCTAssertEqual(restored.snapshot.phase, .anonymous)
            XCTAssertFalse(FileManager.default.fileExists(atPath: file.path))
        }
    }
}
