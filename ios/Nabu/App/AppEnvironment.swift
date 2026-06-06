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
        let args = ProcessInfo.processInfo.arguments
        if let urlStr = launchArgumentValue(for: "-nabuBaseURL", in: args)
            ?? ProcessInfo.processInfo.environment["NABU_BASE_URL"] {
            baseURL = URL(string: urlStr) ?? baseURL
            apiClient = APIClient(baseURL: baseURL)
        }
    }

    func configure(with state: AppState) {
        let args = ProcessInfo.processInfo.arguments

        useMockAPI = args.contains("-useMockAPI")

        if useMockAPI {
            configureMockAPI()
        }

        if args.contains("-resetState") {
            state.reset()
        }

        if TestHooks.seedHomeForUITest {
            seedHomeForUITestState(state)
        }
    }

    private func seedHomeForUITestState(_ state: AppState) {
        let now = Date()

        state.user = User(
            id: 1, householdId: 1, email: "ui-test@nabu.local",
            displayName: "UI Tester", avatarColor: "#2E86AB",
            emailVerified: true, role: "owner", createdAt: now
        )
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
    }

    private func configureMockAPI() {
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
        var slotHour: Int? = nil
        var userId = 1

        if let body = request.httpBody,
           let json = try? JSONSerialization.jsonObject(with: body) as? [String: Any] {
            choreId = json["chore_id"] as? Int ?? 1
            note = json["note"] as? String ?? ""
            indicators = json["indicators"] as? [String] ?? []
            volumeML = json["volume_ml"] as? Int
            slotHour = json["hour"] as? Int
            userId = json["user_id"] as? Int ?? 1
        }

        let log = ChoreLog(
            id: 9001, householdId: 1, userId: userId, choreId: choreId,
            completedAt: Date(), note: note, indicators: indicators,
            slotHour: slotHour, createdAt: Date(), volumeML: volumeML,
            indicatorVolumes: nil
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
        apiClient = APIClient(baseURL: baseURL)
    }

    private func launchArgumentValue(for key: String, in arguments: [String]) -> String? {
        guard let index = arguments.firstIndex(of: key),
              index + 1 < arguments.count else {
            return nil
        }
        return arguments[index + 1]
    }
}
