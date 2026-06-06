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

    func handle(_ request: URLRequest) -> (Data, URLResponse)? {
        guard let url = request.url else { return nil }
        let path = url.path
        guard let (statusCode, data) = handlers[path]?(request) else {
            return nil
        }
        let response = HTTPURLResponse(
            url: url,
            statusCode: statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        return (data, response)
    }
}
