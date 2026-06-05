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

        if args.contains("-resetState") {
            state.reset()
        }

        if TestHooks.seedHomeForUITest {
            // Inject a minimal logged-in household with one chore so XCUITests
            // can exercise the home grid without a real server.
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
            state.chores = [Chore(
                id: 1, householdId: 1, name: "Feed Cats", icon: "🐱",
                color: "#F59E0B", sortOrder: 0, category: "feeding",
                isPredefined: true, predefinedKey: "Feed Cats",
                createdBy: nil, createdAt: now,
                indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false
            )]
            state.currentTab = .home
        }
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
