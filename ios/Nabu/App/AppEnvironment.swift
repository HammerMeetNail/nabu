import Foundation

@MainActor
final class AppEnvironment: ObservableObject {
    @Published var baseURL: URL = URL(string: "https://nabu-app.com")!
    @Published var isOffline = false
    @Published var useMockAPI = false {

        didSet {
            configureAPIClient()
        }
    }

    private(set) var apiClient: APIClient = APIClient(baseURL: URL(string: "https://nabu-app.com")!)

    init() {
        baseURL = AppEnvironment.resolveBaseURL()
        apiClient = APIClient(baseURL: baseURL, identity: ClientIdentity.shared(for: baseURL))
    }

    /// The server base URL from launch arguments / env, falling back to
    /// production. Static so contexts that exist before SwiftUI state (the
    /// notification-action handler in AppDelegate) resolve identically.
    static func resolveBaseURL() -> URL {
        let fallback = URL(string: "https://nabu-app.com")!
        let args = ProcessInfo.processInfo.arguments
        if let idx = args.firstIndex(of: "-nabuBaseURL"), idx + 1 < args.count,
           let url = URL(string: args[idx + 1]) {
            return url
        }
        if let env = ProcessInfo.processInfo.environment["NABU_BASE_URL"],
           let url = URL(string: env) {
            return url
        }
        return fallback
    }

    func configure(with state: AppState) {
        let args = ProcessInfo.processInfo.arguments

        useMockAPI = args.contains("-useMockAPI") || TestHooks.seedHomeForUITest

        if args.contains("-resetState") {
#if DEBUG
            apiClient.cookieStore.clearAll()
            apiClient.identity.accept(nil)
#endif
            state.reset()
            state.activeTimer = nil
            DurationTimer.save(nil)
            OfflineLogQueue.shared.removeAll()
        }

        if TestHooks.seedHomeForUITest {
            seedHomeForUITestState(state)
        }
        if TestHooks.seedHomeForUITest { apiClient.identity.accept(state.user) }
        state.adopt(apiClient.identity.snapshot)
        // adopt aligns the phase/revision and clears the previous identity's
        // data. Restore the fixture only after that real ownership boundary.
        if TestHooks.seedHomeForUITest { seedHomeForUITestState(state) }
        apiClient.identity.observer = { [weak state] snapshot in state?.adopt(snapshot) }
        if useMockAPI { configureMockAPI(state: state) }
    }

    func seedHomeForUITestState(_ state: AppState) {
        let now = Date()

        state.user = User(
            id: 1, householdId: 1, email: "ui-test@nabu.local",
            displayName: "UI Tester", avatarColor: "#2E86AB",
            emailVerified: true, role: "owner", createdAt: now
        )
        if ProcessInfo.processInfo.arguments.contains("-passwordlessAccount") {
            state.user?.hasPassword = false
        }
        state.household = Household(
            id: 1, name: "Test Home", initials: "TH",
            inviteCode: nil, createdAt: now
        )
        state.members = [Member(
            userId: 1, email: "ui-test@nabu.local",
            displayName: "UI Tester", avatarColor: "#2E86AB",
            emailVerified: true, role: "owner"
        )]

        let twoMinutesAgo = now.addingTimeInterval(-120)
        let oneHourAgo = now.addingTimeInterval(-3600)
        let yesterday = now.addingTimeInterval(-86400)

        state.chores = [
            Chore(
                id: 1, householdId: 1, name: "Feed Cats", icon: "🐱",
                color: "#F59E0B", sortOrder: 0, category: "feeding",
                isPredefined: true, predefinedKey: "Feed Cats",
                createdBy: nil, createdAt: now,
                indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false
            ),
            Chore(
                id: 2, householdId: 1, name: "Walk Dog", icon: "🐕",
                color: "#8B5CF6", sortOrder: 1, category: "exercise",
                isPredefined: true, predefinedKey: "Walk Dog",
                createdBy: nil, createdAt: now,
                indicatorLabels: ["Short", "Long", "Park"], indicatorDefaults: ["Short"], hasVolumeML: false
            ),
            Chore(
                id: 3, householdId: 1, name: "Water Plants", icon: "🌱",
                color: "#10B981", sortOrder: 2, category: "household",
                isPredefined: true, predefinedKey: "Water Plants",
                createdBy: nil, createdAt: now,
                indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false
            ),
            Chore(
                id: 4, householdId: 1, name: "Feed Baby", icon: "🍼",
                color: "#EC4899", sortOrder: 3, category: "feeding",
                isPredefined: true, predefinedKey: "Feed Baby",
                createdBy: nil, createdAt: now,
                indicatorLabels: ["Formula", "Breast", "Solids"],
                indicatorDefaults: ["Formula"],
                hasVolumeML: true
            ),
            Chore(
                id: 5, householdId: 1, name: "Take Vitamins", icon: "💊",
                color: "#EF4444", sortOrder: 4, category: "health",
                isPredefined: true, predefinedKey: "Take Vitamins",
                createdBy: nil, createdAt: now,
                indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false
            ),
        ]

        state.latestLogs = [
            1: ChoreLog(
                id: 101, householdId: 1, userId: 1, choreId: 1,
                completedAt: twoMinutesAgo, note: "", indicators: [],
                slotHour: Calendar.current.component(.hour, from: twoMinutesAgo),
                createdAt: twoMinutesAgo, volumeML: nil, indicatorVolumes: nil
            ),
            4: ChoreLog(
                id: 104, householdId: 1, userId: 1, choreId: 4,
                completedAt: oneHourAgo, note: "", indicators: ["Formula"],
                slotHour: Calendar.current.component(.hour, from: oneHourAgo),
                createdAt: oneHourAgo, volumeML: 120, indicatorVolumes: nil
            ),
            5: ChoreLog(
                id: 105, householdId: 1, userId: 1, choreId: 5,
                completedAt: yesterday, note: "", indicators: [],
                slotHour: 8, createdAt: yesterday, volumeML: nil, indicatorVolumes: nil
            ),
        ]

        state.todayLogs = [
            state.latestLogs[1]!,
            state.latestLogs[4]!,
        ]

        state.currentTab = .home
#if DEBUG
        if ProcessInfo.processInfo.arguments.contains("-reviewScenario") {
            state.chores.append(Chore(id: 6, householdId: 1, name: "Weigh flour", icon: "⚖️",
                color: "#6080AA", sortOrder: 5, category: "cooking", isPredefined: false,
                predefinedKey: nil, createdBy: 1, createdAt: now, indicatorLabels: [],
                indicatorDefaults: [], hasVolumeML: true, metricType: "amount", metricUnit: "g"))
            let scenario = launchArgumentValue(for: "-reviewScenario", in: ProcessInfo.processInfo.arguments) ?? ""
            if scenario.hasPrefix("timer") {
                state.chores.append(Chore(id: 7, householdId: 1, name: "Nap", icon: "😴", color: "#6080AA",
                    sortOrder: 6, category: "rest", isPredefined: false, predefinedKey: nil, createdBy: 1,
                    createdAt: now, indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false, metricType: "duration"))
            }
            if scenario == "count" {
                state.chores.append(Chore(id: 7, householdId: 1, name: "Count reps", icon: "🔢", color: "#6080AA",
                    sortOrder: 6, category: "exercise", isPredefined: false, predefinedKey: nil, createdBy: 1,
                    createdAt: now, indicatorLabels: [], indicatorDefaults: [], hasVolumeML: true, metricType: "amount", metricUnit: "reps"))
            }
            if scenario.hasPrefix("stats") {
                state.statsSectionHidden = StatsSections.all.filter { !["overview", "recap"].contains($0) }
                    + state.chores.map { StatsSections.choreSectionKey($0.id) }
                state.latestLogs = [:]
                state.todayLogs = []
            }
            if scenario == "recent-error" { state.recentAmounts[6] = [75]; state.recentAmountUnits[6] = "g" }
            if scenario == "midnight" {
                let prior = Calendar.current.startOfDay(for: TestHooks.reviewDate ?? now).addingTimeInterval(-60)
                state.latestLogs[4] = ChoreLog(id: 104, householdId: 1, userId: 1, choreId: 4,
                    completedAt: prior, note: "", indicators: ["Formula"], slotHour: 23,
                    createdAt: prior, volumeML: 120, indicatorVolumes: ["Formula": 120, "Breast": 90])
                state.todayLogs = []
            }
        }
#endif
    }

    private func configureMockAPI(state: AppState) {
#if DEBUG
        if TestHooks.seedHomeForUITest {
            let scenario = launchArgumentValue(for: "-reviewScenario", in: ProcessInfo.processInfo.arguments) ?? "home"
            let responses = ReviewUITestResponses(scenario: scenario, state: state)
            apiClient.mockAsyncHandler = { request in try await responses.respond(to: request) }
            return
        }
#endif
        apiClient.mockHandler = { request in
            guard let url = request.url else { return nil }
            let path = url.path

            switch path {
            case "/api/logs":
                if request.httpMethod == "POST" {
                    return AppEnvironment.mockCreateLog(request)
                }
                return nil
            case "/api/logs/latest-per-chore":
                return AppEnvironment.mockLatestLogs(request)
            case "/api/logs/today":
                return AppEnvironment.mockToday(request)
            case "/api/preferences":
                if request.httpMethod == "PATCH" {
                    return AppEnvironment.mockPatchPreferences(request)
                }
                return nil
            default:
                let logPattern = try? NSRegularExpression(pattern: "^/api/logs/\\d+$")
                if logPattern?.firstMatch(in: path, range: NSRange(path.startIndex..., in: path)) != nil {
                    return AppEnvironment.mockDeleteLog(request, path: path)
                }
                return nil
            }
        }
    }

    static func mockCreateLog(_ request: URLRequest) -> (Data, URLResponse)? {
        var choreId = 1
        var note = ""
        var indicators: [String] = []
        var volumeML: Int? = nil
        var metricUnit: String?
        var slotHour: Int? = nil
        var userId = 1
        var durationSeconds: Int?
        var indicatorVolumes: [String: Int]?

        if let body = request.httpBody,
           let json = try? JSONSerialization.jsonObject(with: body) as? [String: Any] {
            choreId = json["choreId"] as? Int ?? 1
            note = json["note"] as? String ?? ""
            indicators = json["indicators"] as? [String] ?? []
            volumeML = json["volumeML"] as? Int
            metricUnit = json["metricUnit"] as? String
            slotHour = json["hour"] as? Int
            userId = json["userId"] as? Int ?? 1
            durationSeconds = json["durationSeconds"] as? Int
            indicatorVolumes = json["indicatorVolumes"] as? [String: Int]
        }

        let log = ChoreLog(
            id: 9001, householdId: 1, userId: userId, choreId: choreId,
            completedAt: Date(), note: note, indicators: indicators,
            slotHour: slotHour, createdAt: Date(), volumeML: volumeML,
            indicatorVolumes: indicatorVolumes, durationSeconds: durationSeconds, metricUnit: metricUnit
        )
        let response = LogResponse(log: log)
        let data = try! apiEncoder.encode(response)
        let httpResponse = HTTPURLResponse(
            url: request.url!,
            statusCode: 201,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        return (data, httpResponse)
    }

    static func mockLatestLogs(_ request: URLRequest) -> (Data, URLResponse)? {
        let response = LatestLogsResponse(latestLogs: [:])
        let data = try! apiEncoder.encode(response)
        let httpResponse = HTTPURLResponse(
            url: request.url!,
            statusCode: 200,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        return (data, httpResponse)
    }

    static func mockToday(_ request: URLRequest) -> (Data, URLResponse)? {
        let df = DateFormatter()
        df.dateFormat = "yyyy-MM-dd"
        let summary = DailySummary(
            date: df.string(from: Date()),
            totalChores: 5, choresDone: 1,
            byUser: ["1": 1], byCategory: ["feeding": 1]
        )
        let response = TodayResponse(logs: [], summary: summary, date: df.string(from: Date()))
        let data = try! apiEncoder.encode(response)
        let httpResponse = HTTPURLResponse(
            url: request.url!,
            statusCode: 200,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        return (data, httpResponse)
    }

    static func mockDeleteLog(_ request: URLRequest, path: String) -> (Data, URLResponse)? {
        let response = StatusResponse(status: "ok")
        let data = try! apiEncoder.encode(response)
        let httpResponse = HTTPURLResponse(
            url: request.url!,
            statusCode: 200,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        return (data, httpResponse)
    }

    static func mockPatchPreferences(_ request: URLRequest) -> (Data, URLResponse)? {
        let prefs = UserPreferences(choreOrder: [], hiddenHomeChoreIds: [], timezone: "UTC")
        let response = UserPreferencesResponse(preferences: prefs)
        let data = try! apiEncoder.encode(response)
        let httpResponse = HTTPURLResponse(
            url: request.url!,
            statusCode: 200,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        return (data, httpResponse)
    }

    private func configureAPIClient() {
#if DEBUG
        if TestHooks.seedHomeForUITest {
            apiClient = APIClient(baseURL: baseURL, identity: ClientIdentity())
            return
        }
#endif
        apiClient = APIClient(baseURL: baseURL, identity: ClientIdentity.shared(for: baseURL))
    }

    private func launchArgumentValue(for key: String, in arguments: [String]) -> String? {
        guard let index = arguments.firstIndex(of: key),
              index + 1 < arguments.count else {
            return nil
        }
        return arguments[index + 1]
    }
}

#if DEBUG
/// Deterministic responses for screen tests. Every path is handled locally;
/// unknown fixture requests fail visibly instead of reaching a real server.
@MainActor
final class ReviewUITestResponses {
    let scenario: String
    private var user: User
    private var household: Household
    private let members: [Member]
    private let chores: [Chore]
    private var latestLogs: [Int: ChoreLog]
    private var todayLogs: [ChoreLog]
    private var preferences: UserPreferences
    private var attempts: [String: Int] = [:]
    private var lastLog: ChoreLog?
    private var deletedNotices: Set<Int> = []
    private var readNotices: Set<Int> = []
    private let notificationDate = Date().addingTimeInterval(-2 * 86400)
    private var schedules: [ChoreSchedule] = []

    init(scenario: String, state: AppState) {
        self.scenario = scenario
        user = state.user!
        household = state.household!
        members = state.members
        chores = state.chores
        latestLogs = state.latestLogs
        todayLogs = state.todayLogs
        preferences = UserPreferences(choreOrder: state.chores.map(\.id), hiddenHomeChoreIds: [], timezone: TimeZone.current.identifier,
            statsSectionHidden: state.statsSectionHidden)
    }

    private struct HouseholdFixture: Encodable {
        let household: Household
        let members: [Member]
    }

    func respond(to request: URLRequest) async throws -> (Data, URLResponse) {
        let url = request.url!
        let path = url.path
        let query = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems ?? []
        let key = (request.httpMethod ?? "GET") + " " + url.absoluteString
        attempts[key, default: 0] += 1
        let attempt = attempts[key]!
        func json<T: Encodable>(_ body: T, status: Int = 200) throws -> (Data, URLResponse) {
            (try apiEncoder.encode(body), HTTPURLResponse(url: url, statusCode: status,
                httpVersion: nil, headerFields: ["Content-Type": "application/json"])!)
        }
        func failed(_ message: String, status: Int = 500) throws -> (Data, URLResponse) {
            try json(["error": message], status: status)
        }
        if path == "/api/me" {
            if request.httpMethod == "DELETE" {
                return try failed("Transfer ownership before deleting your account.", status: 409)
            }
            return try json(UserResponse(user: user))
        }
        if path == "/api/auth/password" {
            let body = try JSONSerialization.jsonObject(with: request.httpBody ?? Data()) as? [String: String]
            guard body?["current_password"] != nil, let password = body?["new_password"], password.count >= 8 else {
                return try failed("Invalid password request", status: 400)
            }
            user.hasPassword = true
            return try json(UserResponse(user: user))
        }
        if path == "/api/household" { return try json(HouseholdFixture(household: household, members: members)) }
        if path == "/api/households" {
            if scenario == "stats-switch" {
                return try json(HouseholdsResponse(households: [
                    HouseholdWithRole(id: 1, name: "Test Home", initials: "TH", role: "owner"),
                    HouseholdWithRole(id: 2, name: "Second Home", initials: "SH", role: "owner")]))
            }
            return try json(HouseholdsResponse(households: [HouseholdWithRole(id: household.id, name: household.name, initials: household.initials, role: "owner")]))
        }
        if path == "/api/households/2/activate", scenario == "stats-switch" {
            household = Household(id: 2, name: "Second Home", initials: "SH", inviteCode: nil, createdAt: user.createdAt)
            user = User(id: user.id, householdId: 2, email: user.email, displayName: user.displayName,
                avatarColor: user.avatarColor, emailVerified: user.emailVerified, role: user.role,
                createdAt: user.createdAt, hasPassword: user.hasPassword)
            latestLogs = [:]; todayLogs = []
            return try json(StatusResponse(status: "ok"))
        }
        if path == "/api/chores" { return try json(ChoresResponse(chores: household.id == 1 ? chores : [])) }
        if path == "/api/preferences" {
            if request.httpMethod == "PATCH", let body = request.httpBody {
                var values = try JSONSerialization.jsonObject(with: apiEncoder.encode(preferences)) as! [String: Any]
                let patch = try JSONSerialization.jsonObject(with: body) as! [String: Any]
                values.merge(patch) { _, new in new }
                preferences = try apiDecoder.decode(UserPreferences.self, from: JSONSerialization.data(withJSONObject: values))
            }
            return try json(UserPreferencesResponse(preferences: preferences))
        }
        if path == "/api/notification-preferences" {
            return try json(NotificationPrefsResponse(preferences: ReminderPreference(userId: 1, pushEnabled: false,
                emailEnabled: false, quietHoursStart: "", quietHoursEnd: "", timezone: TimeZone.current.identifier,
                enabledPushTypes: [], defaultReminderLeadMinutes: 0), availableTypes: []))
        }
        if path == "/api/chore-reminder-prefs" { return try json(ChoreReminderPrefsResponse(prefs: [])) }
        if path == "/api/logs/recent-amounts" {
            if scenario == "recent-error" { return try failed("Recent amounts unavailable") }
            if scenario == "recent-empty" { return try json(RecentAmountsResponse(amounts: [])) }
            if scenario == "count" { return try json(RecentAmountsResponse(amounts: [12,8,4])) }
            return try json(RecentAmountsResponse(amounts: [120,90,60]))
        }
        if path == "/api/logs", request.httpMethod == "POST" {
            if ["amount", "journal", "timer"].contains(scenario), attempt == 1 {
                return try failed("Could not save. Please retry.")
            }
            guard let response = AppEnvironment.mockCreateLog(request),
                  let log = try? apiDecoder.decode(LogResponse.self, from: response.0).log else {
                return try failed("Invalid test log")
            }
            lastLog = log
            latestLogs[log.choreId] = log
            todayLogs.insert(log, at: 0)
            return response
        }
        if path == "/api/logs/latest-per-chore" {
            return try json(LatestLogsResponse(latestLogs: Dictionary(uniqueKeysWithValues: latestLogs.map { (String($0.key), $0.value) })))
        }
        if path == "/api/logs/today" {
            let date = todayISO()
            return try json(TodayResponse(logs: todayLogs, summary: DailySummary(date: date, totalChores: chores.count,
                choresDone: todayLogs.count, byUser: [:], byCategory: [:]), date: date))
        }
        if path.hasPrefix("/api/logs/"), request.httpMethod == "PATCH", let current = lastLog, let body = request.httpBody {
            var values = try JSONSerialization.jsonObject(with: apiEncoder.encode(current)) as! [String: Any]
            let patch = try JSONSerialization.jsonObject(with: body) as! [String: Any]
            values.merge(patch) { _, new in new }
            let updated = try apiDecoder.decode(ChoreLog.self, from: JSONSerialization.data(withJSONObject: values))
            lastLog = updated
            todayLogs = todayLogs.map { $0.id == updated.id ? updated : $0 }
            latestLogs[updated.choreId] = updated
            return try json(StatusResponse(status: "ok"))
        }
        if path.hasPrefix("/api/logs/"), request.httpMethod == "DELETE", let id = Int(path.split(separator: "/").last ?? "") {
            todayLogs.removeAll { $0.id == id }
            latestLogs = latestLogs.filter { $0.value.id != id }
            return try json(StatusResponse(status: "ok"))
        }
        if path == "/api/logs/history" {
            if scenario == "activity", attempt == 1, query.isEmpty { return try failed("Could not load Activity. Retry.") }
            let log = lastLog ?? ChoreLog(id: 51, householdId: 1, userId: 1, choreId: 1,
                completedAt: Date(), note: "Needle result", indicators: [], slotHour: 12, createdAt: Date(),
                volumeML: nil, indicatorVolumes: nil)
            let term = query.first(where: { $0.name == "q" })?.value ?? ""
            let rows = term.isEmpty || log.note.localizedCaseInsensitiveContains(term) ? [log] : []
            return try json(HistoryResponse(logs: rows, hasMore: false, start: nil, end: nil))
        }
        if path == "/api/notifications", request.httpMethod == "GET" {
            if scenario != "notifications" { return try json(NotificationsResponse(notifications: [], unreadCount: 0, nextCursor: nil)) }
            let older = query.contains(where: { $0.name == "cursor" })
            if older, attempt == 1 { return try failed("Could not load older notifications.") }
            let ids = older ? [3,4] : [1,2]
            let rows = ids.filter { !deletedNotices.contains($0) }.map { id in
                AppNotification(id: id, userId: 1, type: "chore_logged",
                    title: id < 3 ? "Read notice \(id)" : "Older notice \(id)", body: "Synthetic history",
                    isRead: id < 3 || readNotices.contains(id), createdAt: notificationDate)
            }
            let unread = [3,4].filter { !deletedNotices.contains($0) && !readNotices.contains($0) }.count
            return try json(NotificationsResponse(notifications: rows, unreadCount: unread,
                                                  nextCursor: older ? nil : "older-fixture"))
        }
        if path.hasPrefix("/api/notifications/") {
            if path == "/api/notifications/read-all" { readNotices.formUnion([3,4]) }
            if let id = Int(path.split(separator: "/").dropFirst(2).first ?? "") {
                if request.httpMethod == "DELETE" { deletedNotices.insert(id) }
                else { readNotices.insert(id) }
            }
            return try json(StatusResponse(status: "ok"))
        }
        if path == "/api/logs/export" || path == "/api/household/data" {
            if attempt == 1 { return try failed("Choose a smaller date range.", status: 413) }
            if attempt == 2 {
                // The screen must cancel this request. Core tests separately
                // cover transports which deliver a response after cancellation.
                try await Task.sleep(nanoseconds: 30_000_000_000)
            }
            return (Data("Date,Chore,Note\n2026-09-10,Task,Synthetic export\n".utf8),
                    HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil,
                        headerFields: ["Content-Type": "text/csv"])!)
        }
        if path == "/api/schedules" {
            if request.httpMethod == "POST", scenario == "schedule" {
                var body = try JSONSerialization.jsonObject(with: request.httpBody ?? Data()) as! [String: Any]
                let timestamp = ISO8601DateFormatter().string(from: Date())
                body.merge(["id": 1, "householdId": 1, "intervalDays": 0, "targetCount": 1,
                            "isFollowUp": false, "createdAt": timestamp, "updatedAt": timestamp]) { old, _ in old }
                if let date = body["recurrenceEnd"] as? String,
                   ISO8601DateFormatter().date(from: date) == nil { return try failed("Invalid end timestamp", status: 400) }
                let schedule = try apiDecoder.decode(ChoreSchedule.self, from: JSONSerialization.data(withJSONObject: body))
                schedules = [schedule]
                return try json(ScheduleResponse(schedule: schedule))
            }
            return try json(SchedulesResponse(schedules: schedules))
        }
        if path == "/api/schedules/1", request.httpMethod == "PATCH", scenario == "schedule", let existing = schedules.first {
            var body = try JSONSerialization.jsonObject(with: apiEncoder.encode(existing)) as! [String: Any]
            let patch = try JSONSerialization.jsonObject(with: request.httpBody ?? Data()) as! [String: Any]
            body.merge(patch) { _, new in new }
            let schedule = try apiDecoder.decode(ChoreSchedule.self, from: JSONSerialization.data(withJSONObject: body))
            schedules = [schedule]
            return try json(ScheduleResponse(schedule: schedule))
        }
        if path == "/api/stats/overview", scenario.hasPrefix("stats") {
            if scenario == "stats-switch" {
                let total = household.id == 1 ? 13 : 84
                let body = "{\"overview\":{\"leaderboard\":[],\"streaks\":{\"current\":2,\"longest\":5},\"breakdown\":[],\"recap\":{\"totalChores\":\(total),\"topPerformer\":null,\"mostActiveDay\":\"Monday\",\"byCategory\":[]}}}"
                return (Data(body.utf8), HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil,
                    headerFields: ["Content-Type": "application/json"])!)
            }
            if scenario == "stats-loading" {
                // A cancellable suspended request keeps the loading screen stable.
                while true { try await Task.sleep(nanoseconds: 1_000_000_000) }
            }
            if attempt == 1 {
                return try failed("Stats unavailable")
            }
            let body = #"{"overview":{"leaderboard":[],"streaks":{"current":0,"longest":0},"breakdown":[],"recap":{"totalChores":0,"topPerformer":null,"mostActiveDay":"","byCategory":[]}}}"#
            return (Data(body.utf8), HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!)
        }
        return try failed("Unhandled UI fixture request")
    }
}
#endif
