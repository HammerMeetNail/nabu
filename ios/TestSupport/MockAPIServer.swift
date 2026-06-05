import Foundation

final class MockAPIServer {
    let baseURL: URL
    private var handlers: [String: (URLRequest) -> (Int, Data)] = [:]

    init(baseURL: URL = URL(string: "http://localhost:9999")!) {
        self.baseURL = baseURL
    }

    func register(path: String, handler: @escaping (URLRequest) -> (Int, Data)) {
        handlers[path] = handler
    }

    func handle(_ request: URLRequest) -> (Int, Data)? {
        guard let url = request.url else { return nil }
        let path = url.path
        return handlers[path]?(request)
    }
}
