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
