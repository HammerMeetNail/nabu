import XCTest
@testable import Nabu

/// Offline launch (PWA `snapshot.js`, `browser-context.js` and
/// `offline-launch.spec.js`): when the launch session check cannot reach the
/// server, Home opens from the copy saved for the last confirmed identity and
/// new logs queue in the journal. The saved copy never outlives its identity,
/// and only a server answer (no session, 401, 409) decides the session.
@MainActor
final class OfflineLaunchTests: XCTestCase {
    private var directory: URL!
    private var marker: URL { directory.appendingPathComponent("session.json") }
    private var homeFile: URL { directory.appendingPathComponent("home.json") }
    private var savedHomeExists: Bool { FileManager.default.fileExists(atPath: homeFile.path) }
    private let baseURL = URL(string: "http://localhost:9999")!

    override func setUpWithError() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: directory)
    }

    // MARK: - Fixtures

    nonisolated private func user(_ id: Int = 1, household: Int? = 1, role: String = "owner") -> User {
        User(id: id, householdId: household, email: "test@nabu.local", displayName: "Test",
             avatarColor: "#112233", emailVerified: true, role: role, createdAt: Date(timeIntervalSince1970: 0))
    }

    nonisolated private func chore(_ id: Int, _ name: String) -> Chore {
        Chore(id: id, householdId: 1, name: name, icon: "🌱", color: "#112233", sortOrder: id, category: "home",
              isPredefined: false, predefinedKey: nil, createdBy: 1, createdAt: Date(timeIntervalSince1970: 0),
              indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)
    }

    nonisolated private func log(_ id: Int, chore: Int, at seconds: TimeInterval = 1_000) -> ChoreLog {
        ChoreLog(id: id, householdId: 1, userId: 1, choreId: chore, completedAt: Date(timeIntervalSince1970: seconds),
                 note: "", indicators: [], slotHour: 9, createdAt: Date(timeIntervalSince1970: seconds),
                 volumeML: nil, indicatorVolumes: nil)
    }

    private func newIdentity() -> ClientIdentity { ClientIdentity(fileURL: marker, homeURL: homeFile) }

    /// Home data the server confirmed for user 1 in household 1.
    private func loadedState(for identity: ClientIdentity) -> AppState {
        let state = AppState()
        state.adopt(identity.snapshot)
        state.household = Household(id: 1, name: "Test Home", initials: "TH", inviteCode: nil,
                                    createdAt: Date(timeIntervalSince1970: 0))
        state.activeHouseholdId = 1
        state.members = [Member(userId: 1, email: "test@nabu.local", displayName: "Test",
                                avatarColor: "#112233", emailVerified: true, role: "owner")]
        state.chores = [chore(7, "Water Plants"), chore(8, "Cat Meds")]
        state.latestLogs = [7: log(70, chore: 7)]
        state.choreOrder = [8, 7]
        state.hiddenHomeChoreIDs = [9]
        state.volumeUnit = "oz"
        return state
    }

    /// Confirms user 1 online and saves its Home, then "relaunches".
    private func relaunchWithSavedHome() -> ClientIdentity {
        let online = newIdentity()
        online.accept(user())
        XCTAssertTrue(online.saveHome(loadedState(for: online).homeSnapshot()!, expected: online.snapshot))
        return newIdentity()
    }

    nonisolated private func reply(_ request: URLRequest, _ status: Int, _ body: String,
                       headers: [String: String]? = nil) -> (Data, URLResponse) {
        (Data(body.utf8), HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: headers)!)
    }

    nonisolated private func json<T: Encodable>(_ value: T) -> String {
        String(decoding: try! apiEncoder.encode(value), as: UTF8.self)
    }

    // MARK: - Identity

    func testUnreachableLaunchOpensSavedHomeOnce() throws {
        let relaunched = relaunchWithSavedHome()
        XCTAssertEqual(relaunched.snapshot.phase, .checking)
        let saved = try XCTUnwrap(relaunched.resumeSavedHome())
        XCTAssertEqual(saved.user.id, 1)
        XCTAssertEqual(relaunched.snapshot.phase, .ready)
        XCTAssertTrue(relaunched.snapshot.resumedOffline)
        // The journal and the server claim use the saved identity.
        XCTAssertEqual(relaunched.snapshot.origin, LogOrigin(actorID: 1, householdID: 1))
        XCTAssertNil(relaunched.resumeSavedHome(), "a launch resumes at most once")
    }

    func testServerAnswerAtLaunchDecidesInsteadOfSavedHome() {
        let signedOut = relaunchWithSavedHome()
        signedOut.accept(nil)
        XCTAssertNil(signedOut.resumeSavedHome())
        XCTAssertFalse(savedHomeExists, "no session: the saved Home is deleted")

        let confirmed = relaunchWithSavedHome()
        confirmed.accept(user())
        XCTAssertNil(confirmed.resumeSavedHome(), "a confirmed session never falls back to saved data")
        XCTAssertTrue(savedHomeExists, "the confirmed identity keeps its own saved Home")
    }

    func testPendingSignOutDeletesSavedHomeAtLaunch() throws {
        _ = relaunchWithSavedHome()
        try JSONEncoder().encode(["logoutPending": true]).write(to: marker)
        let relaunched = newIdentity()
        XCTAssertEqual(relaunched.snapshot.phase, .logoutPending)
        XCTAssertFalse(savedHomeExists)
        XCTAssertNil(relaunched.resumeSavedHome())
    }

    func testConfirmingTheSavedIdentityKeepsItsRevisionAndHome() throws {
        let relaunched = relaunchWithSavedHome()
        _ = try XCTUnwrap(relaunched.resumeSavedHome())
        let revision = relaunched.snapshot.revision
        XCTAssertTrue(relaunched.accept(user()))
        XCTAssertEqual(relaunched.snapshot.revision, revision, "same identity: no reset, the queued work stays current")
        XCTAssertFalse(relaunched.snapshot.resumedOffline)
        XCTAssertTrue(savedHomeExists)
    }

    func testAnyOtherAnswerDeletesSavedHome() throws {
        for next in [nil, user(2), user(1, household: 2), user(1, role: "member")] {
            let relaunched = relaunchWithSavedHome()
            _ = try XCTUnwrap(relaunched.resumeSavedHome())
            let revision = relaunched.snapshot.revision
            XCTAssertTrue(relaunched.accept(next))
            XCTAssertNotEqual(relaunched.snapshot.revision, revision, String(describing: next))
            XCTAssertFalse(savedHomeExists, String(describing: next))
        }
    }

    func testInvalidationAndIdentityChangesDeleteSavedHome() throws {
        let identity = newIdentity()
        identity.accept(user())
        let home = loadedState(for: identity).homeSnapshot()!

        XCTAssertTrue(identity.saveHome(home, expected: identity.snapshot))
        identity.invalidate(identity.snapshot)
        XCTAssertFalse(savedHomeExists, "a 409 context change")

        identity.accept(user())
        XCTAssertTrue(identity.saveHome(home, expected: identity.snapshot))
        _ = try identity.beginTransition()
        XCTAssertFalse(savedHomeExists, "sign-in, household create, join, leave or switch, account deletion")

        let other = newIdentity()
        other.accept(user())
        XCTAssertTrue(other.saveHome(home, expected: other.snapshot))
        _ = try other.beginTransition(logout: true)
        XCTAssertFalse(savedHomeExists, "sign-out")
    }

    func testOnlyTheConfirmedCurrentIdentitySaves() throws {
        let relaunched = relaunchWithSavedHome()
        let saved = try XCTUnwrap(relaunched.resumeSavedHome())
        XCTAssertFalse(relaunched.saveHome(saved, expected: relaunched.snapshot), "unconfirmed saved data is not re-saved")

        let identity = newIdentity()
        identity.accept(user())
        let home = loadedState(for: identity).homeSnapshot()!
        let old = identity.snapshot
        identity.accept(user(2))
        XCTAssertFalse(identity.saveHome(home, expected: old), "a save that began before a switch")
        XCTAssertFalse(identity.saveHome(home, expected: identity.snapshot), "another account's Home")
        identity.accept(nil)
        XCTAssertFalse(identity.saveHome(home, expected: identity.snapshot), "signed out")

        let unmanaged = ClientIdentity()
        unmanaged.accept(user())
        XCTAssertFalse(unmanaged.saveHome(home, expected: unmanaged.snapshot), "no file: nothing to save")
    }

    // MARK: - Saved data

    func testSavedHomeRestoresHomeData() throws {
        let relaunched = relaunchWithSavedHome()
        let saved = try XCTUnwrap(relaunched.resumeSavedHome())
        let state = AppState()
        state.adopt(relaunched.snapshot)
        state.applyHomeSnapshot(saved)
        XCTAssertTrue(state.showingSavedHome)
        XCTAssertEqual(state.user?.id, 1)
        XCTAssertEqual(state.household?.name, "Test Home")
        XCTAssertEqual(state.activeHouseholdId, 1)
        XCTAssertEqual(state.members.map(\.userId), [1])
        XCTAssertEqual(state.chores.map(\.name), ["Water Plants", "Cat Meds"])
        XCTAssertEqual(state.latestLogs[7], log(70, chore: 7))
        XCTAssertNil(state.latestLogs[8])
        XCTAssertEqual(state.choreOrder, [8, 7])
        XCTAssertEqual(state.hiddenHomeChoreIDs, [9])
        XCTAssertEqual(state.volumeUnit, "oz")
    }

    func testOnlyAHouseholdHomeIsSaved() {
        let identity = ClientIdentity()
        identity.accept(user())
        let state = loadedState(for: identity)
        XCTAssertNotNil(state.homeSnapshot())
        state.accessSuspendedMessage = "Your access to Test Home is temporarily disabled."
        XCTAssertNil(state.homeSnapshot(), "a disabled membership")
        state.accessSuspendedMessage = nil
        state.household = Household(id: 2, name: "Other", initials: "OH", inviteCode: nil,
                                    createdAt: Date(timeIntervalSince1970: 0))
        XCTAssertNil(state.homeSnapshot(), "a household that is not the account's active one")
        state.user = nil
        XCTAssertNil(state.homeSnapshot())
    }

    // MARK: - Session check

    func testSessionCheckTellsAnUnreachableServerFromAnAnswer() async {
        let cases: [(String, (URLRequest) throws -> (Data, URLResponse), AuthStore.SessionCheck)] = [
            ("no connection", { _ in throw URLError(.notConnectedToInternet) }, .unreachable),
            ("timeout", { _ in throw URLError(.timedOut) }, .unreachable),
            ("proxy 502", { self.reply($0, 502, "<html>Bad gateway</html>") }, .unreachable),
            ("server 500", { self.reply($0, 500, #"{"error":"internal error"}"#) }, .unreachable),
            ("no session", { self.reply($0, 401, #"{"error":"unauthorized"}"#) }, .answered(nil)),
            ("signed in", { self.reply($0, 200, self.json(UserResponse(user: self.user()))) }, .answered(user())),
            ("rate limited", { self.reply($0, 429, #"{"error":"slow down"}"#) }, .failed),
        ]
        for (name, respond, expected) in cases {
            var api = APIClient(baseURL: baseURL, identity: ClientIdentity())
            api.mockAsyncHandler = { request in try respond(request) }
            let result = await AuthStore(api: api).checkSession()
            XCTAssertEqual(result, expected, name)
        }
    }

    // MARK: - Launch to sync

    func testOfflineLaunchQueuesALogThatReplaysOnceConfirmed() async throws {
        final class Server { var online = false; var posted: [URLRequest] = [] }
        let server = Server()
        let identity = relaunchWithSavedHome()
        let state = AppState()
        state.adopt(identity.snapshot)
        identity.observer = { state.adopt($0) }
        var api = APIClient(baseURL: baseURL, identity: identity)
        api.mockAsyncHandler = { request in
            guard server.online else { throw URLError(.notConnectedToInternet) }
            switch request.url!.path {
            case "/api/me": return self.reply(request, 200, self.json(UserResponse(user: self.user())))
            case "/api/logs":
                server.posted.append(request)
                return self.reply(request, 201, self.json(LogResponse(log: self.log(71, chore: 7, at: 2_000))))
            default: return self.reply(request, 404, #"{"error":"not found"}"#)
            }
        }

        let offline = await AuthStore(api: api).checkSession()
        XCTAssertEqual(offline, .unreachable)
        let saved = try XCTUnwrap(identity.resumeSavedHome())
        identity.publish()
        state.applyHomeSnapshot(saved)
        XCTAssertTrue(state.showingSavedHome)
        XCTAssertEqual(state.chores.count, 2)

        let queue = OfflineLogQueue(fileURL: directory.appendingPathComponent("queue.json"))
        let outcome = try await LogStore(api: api, offlineQueue: queue).createLog(choreId: 7, slotHour: 9, idempotencyKey: "offline-1")
        guard case .queued(let pending) = outcome else { return XCTFail("expected the log to queue offline") }
        XCTAssertEqual(pending.choreId, 7)
        XCTAssertEqual(queue.scopedItems(LogOrigin(actorID: 1, householdID: 1)).count, 1)

        // The server answers for the same identity: no reset, and the journal
        // replays under the claim it was queued with.
        server.online = true
        let revision = state.revision
        let confirmed = await AuthStore(api: api).checkSession()
        XCTAssertEqual(confirmed, .answered(user()))
        XCTAssertFalse(state.showingSavedHome)
        XCTAssertEqual(state.revision, revision)
        XCTAssertEqual(state.chores.count, 2, "saved data stays until the reload replaces it")
        let synced = await LogStore(api: api, offlineQueue: queue).replayOfflineQueue()
        XCTAssertEqual(synced, 1)
        XCTAssertTrue(queue.items.isEmpty)
        XCTAssertEqual(server.posted.count, 1)
        XCTAssertEqual(server.posted.first?.value(forHTTPHeaderField: "X-Nabu-User-ID"), "1")
        XCTAssertEqual(server.posted.first?.value(forHTTPHeaderField: "X-Nabu-Household-ID"), "1")
    }

    func testRevokedSessionAfterOfflineLaunchSignsOutAndKeepsTheJournal() async throws {
        final class Server { var online = false }
        let server = Server()
        let identity = relaunchWithSavedHome()
        let state = AppState()
        state.adopt(identity.snapshot)
        identity.observer = { state.adopt($0) }
        var api = APIClient(baseURL: baseURL, identity: identity)
        api.mockAsyncHandler = { request in
            guard server.online else { throw URLError(.notConnectedToInternet) }
            return self.reply(request, 401, #"{"error":"unauthorized"}"#)
        }
        let saved = try XCTUnwrap(identity.resumeSavedHome())
        identity.publish()
        state.applyHomeSnapshot(saved)
        let queue = OfflineLogQueue(fileURL: directory.appendingPathComponent("queue.json"))
        _ = try await LogStore(api: api, offlineQueue: queue).createLog(choreId: 7, idempotencyKey: "offline-2")

        server.online = true
        let revoked = await AuthStore(api: api).checkSession()
        XCTAssertEqual(revoked, .answered(nil))
        XCTAssertNil(state.user)
        XCTAssertNil(state.household)
        XCTAssertFalse(state.showingSavedHome)
        XCTAssertFalse(savedHomeExists)
        // Hidden, not discarded: the journal keeps it for its own account.
        XCTAssertEqual(queue.items.count, 1)
    }

    func testForegroundRefreshConfirmsReloadsLatestLogsAndSaves() async throws {
        let identity = relaunchWithSavedHome()
        let state = AppState()
        state.adopt(identity.snapshot)
        identity.observer = { state.adopt($0) }
        var api = APIClient(baseURL: baseURL, identity: identity)
        api.mockAsyncHandler = { request in
            switch request.url!.path {
            case "/api/me": return self.reply(request, 200, self.json(UserResponse(user: self.user())))
            case "/api/logs/latest-per-chore":
                return self.reply(request, 200, self.json(LatestLogsResponse(latestLogs: ["7": self.log(72, chore: 7, at: 3_000)])))
            default: return self.reply(request, 404, #"{"error":"not found"}"#)
            }
        }
        let saved = try XCTUnwrap(identity.resumeSavedHome())
        identity.publish()
        state.applyHomeSnapshot(saved)
        let loader = DataLoader()
        loader.configure(api: api, state: state)

        await loader.foregroundRefresh()
        XCTAssertFalse(state.showingSavedHome)
        // The connectivity monitor that `configure` starts can run its own
        // refresh, which takes over this one; wait for whichever finishes.
        var resaved: [Int] = []
        for _ in 0..<50 {
            resaved = newIdentity().resumeSavedHome()?.latestLogs.map(\.id) ?? []
            if resaved == [72] { break }
            try await Task.sleep(for: .milliseconds(100))
        }
        XCTAssertEqual(state.latestLogs[7]?.id, 72, "foreground refresh reloads the latest logs")
        XCTAssertEqual(resaved, [72], "and saves the confirmed Home")
    }
}
