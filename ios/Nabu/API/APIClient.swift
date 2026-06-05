import Foundation

struct APIClient {
    var baseURL: URL
    var session: URLSession
    var cookieStore: CookieStore
    var csrfProvider: CSRFTokenProvider

    init(baseURL: URL) {
        self.baseURL = baseURL
        let config = URLSessionConfiguration.default
        config.httpCookieStorage = HTTPCookieStorage.shared
        config.httpShouldSetCookies = true
        self.session = URLSession(configuration: config)
        self.cookieStore = CookieStore()
        self.csrfProvider = CSRFTokenProvider(cookieStore: cookieStore)
    }

    func get<T: Decodable>(_ path: String, query: [URLQueryItem] = []) async throws -> T {
        var components = URLComponents(url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false)
        if !query.isEmpty {
            components?.queryItems = query
        }
        guard let url = components?.url else {
            throw APIError.invalidURL
        }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        return try await perform(request)
    }

    func post<Request: Encodable, Response: Decodable>(_ path: String, body: Request) async throws -> Response {
        try await mutate("POST", path, body)
    }

    func patch<Request: Encodable, Response: Decodable>(_ path: String, body: Request) async throws -> Response {
        try await mutate("PATCH", path, body)
    }

    func delete<Response: Decodable>(_ path: String) async throws -> Response {
        try await mutateWithoutBody("DELETE", path)
    }

    func postEmpty<Response: Decodable>(_ path: String) async throws -> Response {
        try await mutateWithoutBody("POST", path)
    }

    private func mutate<Request: Encodable, Response: Decodable>(_ method: String, _ path: String, _ body: Request) async throws -> Response {
        let url = baseURL.appendingPathComponent(path)
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let csrfToken = csrfProvider.token {
            request.setValue(csrfToken, forHTTPHeaderField: "X-CSRF-Token")
        }
        request.httpBody = try apiEncoder.encode(body)
        return try await perform(request)
    }

    private func mutateWithoutBody<Response: Decodable>(_ method: String, _ path: String) async throws -> Response {
        let url = baseURL.appendingPathComponent(path)
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let csrfToken = csrfProvider.token {
            request.setValue(csrfToken, forHTTPHeaderField: "X-CSRF-Token")
        }
        return try await perform(request)
    }

    private func perform<T: Decodable>(_ request: URLRequest) async throws -> T {
        let (data, response) = try await session.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse else {
            throw APIError.invalidResponse
        }

        if httpResponse.statusCode == 429 {
            let retryAfter = httpResponse.value(forHTTPHeaderField: "Retry-After")
            throw APIError.rateLimited(retryAfter: retryAfter)
        }

        guard (200...299).contains(httpResponse.statusCode) else {
            if let apiErr = try? apiDecoder.decode(APIErrorResponse.self, from: data) {
                throw APIError.serverError(statusCode: httpResponse.statusCode, message: apiErr.error)
            }
            throw APIError.httpError(statusCode: httpResponse.statusCode)
        }

        do {
            return try apiDecoder.decode(T.self, from: data)
        } catch {
            throw APIError.decodingError(error)
        }
    }
}
