import XCTest
@testable import Nabu

/// App Intents (improvement plan, Phase 3): a log from Siri or Shortcuts is
/// the request a Home log sends, through the same journal; offline it waits
/// for the identity whose Home was saved on this device, without adopting it;
/// undo removes only the intent's own recent log; and "last done" says when
/// and by whom.
@MainActor
final class AppIntentsTests: XCTestCase {
    private var directory: URL!
    private var queue: OfflineLogQueue!
    private var originalClient: (() -> APIClient)!
    private var originalAppClient: (() -> APIClient?)?
    private var originalQueue: (() -> OfflineLogQueue)!
    private var originalDefaults: UserDefaults!
    private let baseURL = URL(string: "http://localhost:9999")!
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUpWithError() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        queue = OfflineLogQueue(fileURL: directory.appendingPathComponent("queue.json"))
        originalClient = IntentSession.makeClient
        originalAppClient = IntentSession.appClient
        // These tests are iOS starting the app only for an intent; the test
        // host's own app client is set aside.
        IntentSession.appClient = nil
        originalQueue = IntentSession.queue
        originalDefaults = LastIntentLog.defaults
        let queue = self.queue!
        IntentSession.queue = { queue }
        LastIntentLog.defaults = UserDefaults(suiteName: "AppIntentsTests-\(UUID().uuidString)")!
    }

    override func tearDownWithError() throws {
        IntentSession.makeClient = originalClient
        IntentSession.appClient = originalAppClient
        IntentSession.queue = originalQueue
        LastIntentLog.defaults = originalDefaults
        try? FileManager.default.removeItem(at: directory)
    }

    // MARK: - Fixtures

    final class Server {
        var online = true
        var meStatus = 200
        var posted: [CreateLogRequest] = []
        var postHeaders: [[String: String]] = []
        var deleted: [String] = []
        var latest: [String: ChoreLog] = [:]
        var choreReads = 0
    }

    nonisolated private func user(_ id: Int = 1) -> User {
        User(id: id, householdId: 1, email: "test@nabu.local", displayName: "Test",
             avatarColor: "#112233", emailVerified: true, role: "owner", createdAt: Date(timeIntervalSince1970: 0))
    }

    nonisolated private func feed() -> Chore {
        Chore(id: 7, householdId: 1, name: "Feed Baby", icon: "🍼", color: "#112233", sortOrder: 0, category: "feeding",
              isPredefined: false, predefinedKey: nil, createdBy: 1, createdAt: Date(timeIntervalSince1970: 0),
              indicatorLabels: ["Bottle", "Breast"], indicatorDefaults: ["Bottle"], hasVolumeML: true,
              metricType: "amount", metricUnit: "ml")
    }

    nonisolated private func catMeds() -> Chore {
        Chore(id: 8, householdId: 1, name: "Cat Meds", icon: "💊", color: "#112233", sortOrder: 1, category: "care",
              isPredefined: true, predefinedKey: "Cat Meds", createdBy: 1, createdAt: Date(timeIntervalSince1970: 0),
              indicatorLabels: [], indicatorDefaults: [], hasVolumeML: true, metricType: "amount", metricUnit: "mg",
              subjects: ["Mongo", "Roger"])
    }

    nonisolated private func plants() -> Chore {
        Chore(id: 9, householdId: 1, name: "Water Plants", icon: "🌱", color: "#112233", sortOrder: 2, category: "home",
              isPredefined: false, predefinedKey: nil, createdBy: 1, createdAt: Date(timeIntervalSince1970: 0),
              indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)
    }

    nonisolated private func log(_ id: Int, chore: Int, by userID: Int = 1, at date: Date) -> ChoreLog {
        ChoreLog(id: id, householdId: 1, userId: userID, choreId: chore, completedAt: date, note: "", indicators: [],
                 slotHour: nil, createdAt: date, volumeML: nil, indicatorVolumes: nil)
    }

    nonisolated private func reply(_ request: URLRequest, _ status: Int, _ body: String) -> (Data, URLResponse) {
        (Data(body.utf8), HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!)
    }

    nonisolated private func json<T: Encodable>(_ value: T) -> String {
        String(decoding: try! apiEncoder.encode(value), as: UTF8.self)
    }

    /// A household with Feed Baby, Cat Meds and Water Plants, and Dana (2).
    private func use(_ identity: ClientIdentity, _ server: Server) {
        let chores = [feed(), catMeds(), plants()]
        IntentSession.makeClient = { [baseURL] in
            var api = APIClient(baseURL: baseURL, identity: identity)
            api.mockAsyncHandler = { request in
                guard server.online else { throw URLError(.notConnectedToInternet) }
                let path = request.url!.path
                switch (request.httpMethod ?? "GET", path) {
                case ("GET", "/api/me"):
                    if server.meStatus == 401 { return self.reply(request, 401, #"{"error":"not authenticated"}"#) }
                    return self.reply(request, 200, self.json(UserResponse(user: self.user())))
                case ("GET", "/api/chores"):
                    server.choreReads += 1
                    return self.reply(request, 200, self.json(ChoresResponse(chores: chores)))
                case ("POST", "/api/logs"):
                    let body = try JSONDecoder().decode(CreateLogRequest.self, from: request.httpBody ?? Data())
                    server.posted.append(body)
                    server.postHeaders.append(request.allHTTPHeaderFields ?? [:])
                    return self.reply(request, 201, self.json(LogResponse(log: self.log(501, chore: body.choreId, at: self.now))))
                case ("DELETE", _):
                    server.deleted.append(path)
                    return self.reply(request, 200, #"{"status":"deleted","latest":null}"#)
                case ("GET", "/api/logs/latest-per-chore"):
                    return self.reply(request, 200, self.json(LatestLogsResponse(latestLogs: server.latest)))
                default:
                    return self.reply(request, 404, #"{"error":"not found"}"#)
                }
            }
            return api
        }
    }

    private var marker: URL { directory.appendingPathComponent("session.json") }
    private var homeFile: URL { directory.appendingPathComponent("home.json") }

    /// Home saved for user 1 by an earlier online run, then a new launch.
    private func identityWithSavedHome() -> ClientIdentity {
        let online = ClientIdentity(fileURL: marker, homeURL: homeFile)
        online.accept(user())
        let state = AppState()
        state.adopt(online.snapshot)
        state.household = Household(id: 1, name: "Home", initials: "H", inviteCode: nil, createdAt: Date(timeIntervalSince1970: 0))
        state.activeHouseholdId = 1
        state.members = [Member(userId: 1, email: "test@nabu.local", displayName: "Test", avatarColor: "#112233", emailVerified: true, role: "owner"),
                         Member(userId: 2, email: "dana@nabu.local", displayName: "Dana", avatarColor: "#112233", emailVerified: true, role: "member")]
        state.chores = [feed(), catMeds(), plants()]
        state.latestLogs = [7: log(70, chore: 7, by: 2, at: now.addingTimeInterval(-7200))]
        XCTAssertTrue(online.saveHome(state.homeSnapshot()!, expected: online.snapshot))
        return ClientIdentity(fileURL: marker, homeURL: homeFile)
    }

    private func localDate(_ date: Date) -> String {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.string(from: date)
    }

    // MARK: - Log

    func testALogIsTheRequestAHomeLogSends() async throws {
        let server = Server()
        use(ClientIdentity(fileURL: marker, homeURL: homeFile), server)

        let message = try await IntentActions.log(activityID: 7, amount: 120, subject: nil, now: now)

        XCTAssertEqual(message, "Logged Feed Baby.")
        let body = try XCTUnwrap(server.posted.first)
        XCTAssertEqual(body.choreId, 7)
        XCTAssertEqual(body.hour, Calendar.current.component(.hour, from: now), "the hour row of now")
        XCTAssertEqual(body.date, localDate(now))
        XCTAssertEqual(body.completedAt, ISO8601DateFormatter().string(from: now))
        XCTAssertEqual(body.userId, 1)
        XCTAssertEqual(body.indicators, ["Bottle"], "the activity's default indicators")
        XCTAssertEqual(body.volumeML, 120)
        XCTAssertEqual(body.metricUnit, "ml")
        XCTAssertFalse(body.idempotencyKey?.isEmpty ?? true)
        XCTAssertEqual(server.postHeaders.first?["X-Nabu-User-ID"], "1")
        XCTAssertTrue(queue.items.isEmpty, "a confirmed log leaves the journal")
        XCTAssertEqual(LastIntentLog.load()?.logID, 501)
    }

    func testAWhoIsSentAndCheckedAgainstTheActivitysNames() async throws {
        let server = Server()
        use(ClientIdentity(fileURL: marker, homeURL: homeFile), server)

        _ = try await IntentActions.log(activityID: 8, amount: nil, subject: "Roger", now: now)
        XCTAssertEqual(server.posted.first?.subject, "Roger")

        do {
            _ = try await IntentActions.log(activityID: 8, amount: nil, subject: "Rex", now: now)
            XCTFail("an unknown name must not be logged")
        } catch { XCTAssertEqual(error as? NabuIntentError, .unknownSubject("Rex", "Cat Meds")) }
        do {
            _ = try await IntentActions.log(activityID: 9, amount: 5, subject: nil, now: now)
            XCTFail("an amount on an activity that records none must not be logged")
        } catch { XCTAssertEqual(error as? NabuIntentError, .noAmount("Water Plants")) }
        do {
            _ = try await IntentActions.log(activityID: 99, amount: nil, subject: nil, now: now)
            XCTFail("a missing activity must not be logged")
        } catch { XCTAssertEqual(error as? NabuIntentError, .activityGone) }
        XCTAssertEqual(server.posted.count, 1)
    }

    func testSignedOutAsksToSignIn() async throws {
        let server = Server()
        server.meStatus = 401
        use(ClientIdentity(fileURL: marker, homeURL: homeFile), server)
        do {
            _ = try await IntentActions.log(activityID: 7, amount: nil, subject: nil, now: now)
            XCTFail("expected sign in first")
        } catch { XCTAssertEqual(error as? NabuIntentError, .signedOut) }
        XCTAssertTrue(server.posted.isEmpty)
    }

    func testOfflineALogWaitsForTheSavedIdentityWithoutAdoptingIt() async throws {
        let server = Server()
        server.online = false
        let identity = identityWithSavedHome()
        use(identity, server)

        let message = try await IntentActions.log(activityID: 7, amount: nil, subject: nil, now: now)

        XCTAssertEqual(message, "Saved Feed Baby on this device. It syncs when Nabu is back online.")
        let item = try XCTUnwrap(queue.scopedItems(LogOrigin(actorID: 1, householdID: 1)).first)
        XCTAssertEqual(item.body.choreId, 7)
        XCTAssertEqual(item.body.hour, Calendar.current.component(.hour, from: now))
        XCTAssertEqual(item.body.completedAt, ISO8601DateFormatter().string(from: now))
        XCTAssertEqual(item.body.userId, 1)
        // A later offline launch still opens the saved Home.
        XCTAssertEqual(identity.snapshot.phase, .checking)
        XCTAssertNotNil(identity.resumeSavedHome())
    }

    func testOfflineWithNothingSavedSaysSo() async throws {
        let server = Server()
        server.online = false
        use(ClientIdentity(fileURL: marker, homeURL: homeFile), server)
        do {
            _ = try await IntentActions.log(activityID: 7, amount: nil, subject: nil, now: now)
            XCTFail("expected unreachable")
        } catch { XCTAssertEqual(error as? NabuIntentError, .unreachable) }
        XCTAssertTrue(queue.items.isEmpty)
    }

    // MARK: - Undo

    func testUndoRemovesOnlyTheIntentsRecentLog() async throws {
        let server = Server()
        use(ClientIdentity(fileURL: marker, homeURL: homeFile), server)
        _ = try await IntentActions.log(activityID: 7, amount: nil, subject: nil, now: now)

        let undone = try await IntentActions.undoLast(now: now.addingTimeInterval(60))
        XCTAssertEqual(undone, "Removed the Feed Baby log.")
        XCTAssertEqual(server.deleted, ["/api/logs/501"])

        let again = try await IntentActions.undoLast(now: now.addingTimeInterval(90))
        XCTAssertEqual(again, "There's no log from the past hour to undo.")
        XCTAssertEqual(server.deleted.count, 1)
    }

    func testUndoAfterAnHourDoesNothing() async throws {
        let server = Server()
        use(ClientIdentity(fileURL: marker, homeURL: homeFile), server)
        _ = try await IntentActions.log(activityID: 7, amount: nil, subject: nil, now: now)
        let late = try await IntentActions.undoLast(now: now.addingTimeInterval(IntentActions.undoWindow + 1))
        XCTAssertEqual(late, "There's no log from the past hour to undo.")
        XCTAssertTrue(server.deleted.isEmpty)
    }

    func testUndoDropsALogStillWaitingOnThisDevice() async throws {
        let server = Server()
        server.online = false
        use(identityWithSavedHome(), server)
        _ = try await IntentActions.log(activityID: 7, amount: nil, subject: nil, now: now)
        XCTAssertEqual(queue.items.count, 1)

        let undone = try await IntentActions.undoLast(now: now.addingTimeInterval(30))
        XCTAssertEqual(undone, "Removed the Feed Baby log.")
        XCTAssertTrue(queue.items.isEmpty)
        XCTAssertTrue(server.deleted.isEmpty, "a log never sent needs no request")
    }

    // MARK: - Last done

    func testLastDoneSaysWhenAndWho() async throws {
        let server = Server()
        let identity = identityWithSavedHome()
        use(identity, server)
        let twoHoursAgo = now.addingTimeInterval(-7200)
        let when = RelativeDateTimeFormatter().localizedString(for: twoHoursAgo, relativeTo: now)

        server.latest = ["7": log(70, chore: 7, by: 2, at: twoHoursAgo)]
        let byDana = try await IntentActions.lastDone(activityID: 7, now: now)
        XCTAssertEqual(byDana, "Dana last logged Feed Baby \(when).")

        server.latest = ["7": log(71, chore: 7, by: 1, at: twoHoursAgo)]
        let byMe = try await IntentActions.lastDone(activityID: 7, now: now)
        XCTAssertEqual(byMe, "You last logged Feed Baby \(when).")

        let never = try await IntentActions.lastDone(activityID: 9, now: now)
        XCTAssertEqual(never, "Water Plants hasn't been logged yet.")
    }

    func testLastDoneOfflineUsesTheSavedHome() async throws {
        let server = Server()
        server.online = false
        use(identityWithSavedHome(), server)
        let when = RelativeDateTimeFormatter().localizedString(for: now.addingTimeInterval(-7200), relativeTo: now)
        let message = try await IntentActions.lastDone(activityID: 7, now: now)
        XCTAssertEqual(message, "Dana last logged Feed Baby \(when).")
    }

    // MARK: - With the app running

    func testWithTheAppRunningAnIntentUsesItsSessionWithoutCheckingIt() async throws {
        let server = Server()
        let identity = ClientIdentity(fileURL: marker, homeURL: homeFile)
        use(identity, server)
        final class Reads { var me = 0 }
        let reads = Reads()
        var counted = IntentSession.makeClient()
        let respond = try XCTUnwrap(counted.mockAsyncHandler)
        counted.mockAsyncHandler = { request in
            if request.url?.path == "/api/me" { reads.me += 1 }
            return try await respond(request)
        }
        IntentSession.appClient = { counted }

        // Mid-launch the intent waits for the app's own check, and never
        // answers the session itself: that would reset the app's screens.
        do {
            _ = try await IntentActions.log(activityID: 7, amount: nil, subject: nil, now: now)
            XCTFail("expected the intent to wait, then give up")
        } catch { XCTAssertEqual(error as? NabuIntentError, .unavailable) }
        XCTAssertEqual(reads.me, 0)
        XCTAssertEqual(identity.snapshot.phase, .checking)

        identity.accept(user())
        let message = try await IntentActions.log(activityID: 7, amount: nil, subject: nil, now: now)
        XCTAssertEqual(message, "Logged Feed Baby.")
        XCTAssertEqual(reads.me, 0)
    }

    // MARK: - Activities

    func testSiriIsOfferedTheHouseholdsActivities() async throws {
        let server = Server()
        use(ClientIdentity(fileURL: marker, homeURL: homeFile), server)
        let all = try await ActivityQuery().suggestedEntities()
        XCTAssertEqual(all.map(\.name), ["Feed Baby", "Cat Meds", "Water Plants"])
        XCTAssertEqual(all.first { $0.id == 8 }?.subjects, ["Mongo", "Roger"])
        let matched = try await ActivityQuery().entities(matching: "feed")
        XCTAssertEqual(matched.map(\.id), [7])
        let byID = try await ActivityQuery().entities(for: [9])
        XCTAssertEqual(byID.map(\.name), ["Water Plants"])
    }

    func testChoresTheAppJustLoadedAnswerSiriWithoutARequest() async throws {
        let server = Server()
        let identity = ClientIdentity(fileURL: marker, homeURL: homeFile)
        identity.accept(user())
        use(identity, server)
        IntentSession.noteLoaded([feed()], for: identity.snapshot)
        let offered = try await ActivityQuery().suggestedEntities()
        XCTAssertEqual(offered.map(\.name), ["Feed Baby"])
        XCTAssertEqual(server.choreReads, 0)

        // Another identity's list is never used.
        identity.accept(User(id: 2, householdId: 3, email: "b@nabu.local", displayName: "B", avatarColor: "#112233",
                             emailVerified: true, role: "owner", createdAt: Date(timeIntervalSince1970: 0)))
        _ = try await ActivityQuery().suggestedEntities()
        XCTAssertEqual(server.choreReads, 1)
    }
}
