import Foundation

struct APIClient {
    var baseURL: URL
    var session: URLSession
    var cookieStore: CookieStore
    var csrfProvider: CSRFTokenProvider
    var mockHandler: ((URLRequest) -> (Data, URLResponse)?)?
    var mockAsyncHandler: ((URLRequest) async throws -> (Data, URLResponse))?
    let identity: ClientIdentity
    private var requestOwner: ClientIdentity.Snapshot?
    var requestContext: ClientIdentity.Snapshot { requestOwner ?? identity.snapshot }

    func scoped(to owner: ClientIdentity.Snapshot? = nil) -> APIClient {
        var copy = self
        copy.requestOwner = owner ?? requestContext
        return copy
    }

    init(baseURL: URL, identity: ClientIdentity? = nil) {
        self.baseURL = baseURL
        self.identity = identity ?? ClientIdentity(managed: false)
        let config = URLSessionConfiguration.default
        config.httpCookieStorage = HTTPCookieStorage.shared
        config.httpShouldSetCookies = true
        config.timeoutIntervalForRequest = 20
        config.timeoutIntervalForResource = 30
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

    func put<Request: Encodable, Response: Decodable>(_ path: String, body: Request) async throws -> Response {
        try await mutate("PUT", path, body)
    }

    func delete<Response: Decodable>(_ path: String) async throws -> Response {
        try await mutateWithoutBody("DELETE", path)
    }

    /// DELETE with a JSON body — used by `DELETE /api/me`, whose typed
    /// confirmation body guards against accidental account destruction.
    func delete<Request: Encodable, Response: Decodable>(_ path: String, body: Request) async throws -> Response {
        try await mutate("DELETE", path, body)
    }

    /// GET returning the raw response body (e.g. the CSV log export).
    func getData(_ path: String, query: [URLQueryItem] = [], expectedContentType: String? = nil) async throws -> Data {
        var components = URLComponents(url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false)
        if !query.isEmpty {
            components?.queryItems = query
        }
        guard let url = components?.url else {
            throw APIError.invalidURL
        }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        return try await performData(request, expectedContentType: expectedContentType)
    }

    func postEmpty<Response: Decodable>(_ path: String) async throws -> Response {
        try await mutateWithoutBody("POST", path)
    }

    private func mutate<Request: Encodable, Response: Decodable>(_ method: String, _ path: String, _ body: Request) async throws -> Response {
        let data = try apiEncoder.encode(body)
        return try await mutateRequest(method, path: path, body: data, contentType: "application/json")
    }

    private func mutateWithoutBody<Response: Decodable>(_ method: String, _ path: String) async throws -> Response {
        try await mutateRequest(method, path: path, body: nil, contentType: nil)
    }

    /// Sends a state-changing request, replaying it once if the server's
    /// double-submit middleware rejected it for a missing cookie. That
    /// rejection is a plain-text 403 raised before any handler runs — it
    /// surfaces as `httpError`, where an authorization failure carries a JSON
    /// body and surfaces as `serverError` — so the replay cannot duplicate a
    /// write. Signing out clears the cookie store, which is how the *next*
    /// sign-in used to answer users with "HTTP error 403".
    private func mutateRequest<Response: Decodable>(_ method: String, path: String, body: Data?, contentType: String?) async throws -> Response {
        let url = baseURL.appendingPathComponent(path)
        func build() -> URLRequest {
            var request = URLRequest(url: url)
            request.httpMethod = method
            if let contentType {
                request.setValue(contentType, forHTTPHeaderField: "Content-Type")
            }
            request.setValue("application/json", forHTTPHeaderField: "Accept")
            if let csrfToken = csrfProvider.token {
                request.setValue(csrfToken, forHTTPHeaderField: "X-CSRF-Token")
            }
            request.httpBody = body
            return request
        }
        do {
            return try await perform(build())
        } catch APIError.httpError(statusCode: 403) {
            await refreshCSRFToken()
            return try await perform(build())
        }
    }

    /// Any request makes the server re-issue the double-submit cookie when none
    /// is present; `/api/me` is the cheapest. It goes out unwrapped so the
    /// identity state is untouched by a cookie refresh.
    private func refreshCSRFToken() async {
        var request = URLRequest(url: baseURL.appendingPathComponent("/api/me"))
        request.httpMethod = "GET"
        _ = try? await send(request)
    }

    private func perform<T: Decodable>(_ request: URLRequest) async throws -> T {
        let data = try await performData(request)
        do {
            return try apiDecoder.decode(T.self, from: data)
        } catch {
            throw APIError.decodingError(error)
        }
    }

    /// Like `perform`, but returns the raw body instead of decoding JSON.
    private func performData(_ request: URLRequest, expectedContentType: String? = nil) async throws -> Data {
        let owner = requestOwner ?? identity.snapshot
        guard identity.isCurrent(owner) else { throw APIError.contextChanged }
        let path = request.url?.path ?? ""
        let logout = path == "/api/auth/logout"
        let transition = identity.managed && Self.changesIdentity(request)
        // Reading /api/me confirms the session. Updating the profile there is
        // an ordinary write by the current identity: it needs a ready session
        // and carries the identity claim like any other request.
        let sessionRead = path == "/api/me" && request.httpMethod != "PATCH"
        let publicRead = request.httpMethod == "GET" && Self.publicReads.contains(path)
        var request = request
        var ticket: UUID?
        if transition {
            do { ticket = try identity.beginTransition(logout: logout, expected: owner) }
            catch { await identity.publish(); throw error }
            await identity.publish()
        } else if identity.managed && !sessionRead && !publicRead && !path.hasPrefix("/api/auth/") {
            guard owner.phase == .ready else { throw APIError.contextChanged }
        } else if identity.managed && (owner.phase == .logoutPending || owner.phase == .changing) {
            throw APIError.contextChanged
        }
        if identity.managed, let user = owner.user, !sessionRead, !publicRead {
            request.setValue(String(user.id), forHTTPHeaderField: "X-Nabu-User-ID")
            // A deliberate claim about the identity that authored the request,
            // including "this account has no active household" (0). The server
            // rejects a mismatch with 409 `X-Nabu-Context-Changed` so work
            // captured for one account can never be attributed to another.
            request.setValue(String(user.householdId ?? 0), forHTTPHeaderField: "X-Nabu-Household-ID")
        }
        let result: (Data, HTTPURLResponse)
        do {
            result = try await send(request)
        } catch {
            if let ticket {
                if logout { identity.failTransition(ticket, logout: true); await identity.publish() }
                else { _ = try? await confirmTransition(ticket) }
            }
            throw error
        }
        let (data, httpResponse) = result
        if let ticket {
            if logout {
                if (200...299).contains(httpResponse.statusCode) || httpResponse.statusCode == 401 {
                    cookieStore.clearAll()
                    try identity.finishTransition(ticket, user: nil, logout: true)
                    await identity.publish()
                    return Data("{\"status\":\"ok\"}".utf8)
                }
                identity.failTransition(ticket, logout: true)
                await identity.publish()
            } else {
                // Even an error or a lost response may have changed the server
                // session. Resolve its canonical identity before exposing data.
                _ = try await confirmTransition(ticket)
            }
        } else if identity.managed {
            guard identity.isCurrent(owner) else { throw APIError.contextChanged }
            if sessionRead {
                if httpResponse.statusCode == 401 {
                    guard identity.accept(nil, expected: owner) else { throw APIError.contextChanged }
                }
                else if (200...299).contains(httpResponse.statusCode) {
                    let response = try apiDecoder.decode(UserResponse.self, from: data)
                    guard identity.accept(response.user, expected: owner) else { throw APIError.contextChanged }
                }
                await identity.publish()
            } else if httpResponse.value(forHTTPHeaderField: "X-Nabu-Context-Changed") == "true" {
                // The server is explicit here: the identity that authored this
                // request is no longer the session's. Drop the old claim at once
                // and let the next confirmed read (/api/me) re-establish it.
                identity.invalidate(owner)
                await identity.publish()
            } else if httpResponse.statusCode == 401 {
                // A 401 is not always a lost session: the server also answers
                // 401 "no household" for household-scoped reads when the account
                // has no active household — a fresh account, or a membership an
                // administrator has disabled. Re-read the canonical session and
                // only drop it if /api/me says so, mirroring the PWA's
                // nabu-session-invalid -> confirmBrowserSession behaviour.
                await confirmSessionAfterUnauthorized(owner)
            }
        }
        let accepted = try checked(data, httpResponse)
        if let expectedContentType, !(httpResponse.value(forHTTPHeaderField: "Content-Type") ?? "").lowercased().contains(expectedContentType.lowercased()) {
            throw APIError.invalidResponse
        }
        return accepted
    }

    private func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        let result: (Data, URLResponse)
        if let handler = mockAsyncHandler { result = try await handler(request) }
        else if let handler = mockHandler, let mock = handler(request) { result = mock }
        else { result = try await session.data(for: request) }
        guard let response = result.1 as? HTTPURLResponse else { throw APIError.invalidResponse }
        return (result.0, response)
    }

    private func checked(_ data: Data, _ httpResponse: HTTPURLResponse) throws -> Data {
        if httpResponse.statusCode == 429 {
            throw APIError.rateLimited(retryAfter: httpResponse.value(forHTTPHeaderField: "Retry-After"))
        }
        guard (200...299).contains(httpResponse.statusCode) else {
            if let apiErr = try? apiDecoder.decode(APIErrorResponse.self, from: data) {
                if apiErr.status == "access_suspended" {
                    throw APIError.accessSuspended(message: apiErr.error, household: apiErr.household)
                }
                throw APIError.serverError(statusCode: httpResponse.statusCode, message: apiErr.error)
            }
            throw APIError.httpError(statusCode: httpResponse.statusCode)
        }
        return data
    }

    /// Also used by the external Google browser session, whose cookie handoff
    /// must hold the same identity transition as password/Apple sign-in.
    func confirmTransition(_ ticket: UUID) async throws -> User? {
        do {
            var request = URLRequest(url: baseURL.appendingPathComponent("/api/me"))
            request.httpMethod = "GET"
            let (data, response) = try await send(request)
            let user: User?
            if response.statusCode == 401 { user = nil }
            else { user = try apiDecoder.decode(UserResponse.self, from: checked(data, response)).user }
            try identity.finishTransition(ticket, user: user)
            await identity.publish()
            return user
        } catch {
            identity.failTransition(ticket, logout: false)
            await identity.publish()
            throw error
        }
    }

    /// Re-reads the canonical session after a household-scoped endpoint answers
    /// 401. The server uses 401 "no household" both for an absent session and
    /// for an account whose active household is gone — including a membership an
    /// administrator has disabled — so the session is only dropped when /api/me
    /// itself says the session is gone.
    private func confirmSessionAfterUnauthorized(_ owner: ClientIdentity.Snapshot) async {
        var request = URLRequest(url: baseURL.appendingPathComponent("/api/me"))
        request.httpMethod = "GET"
        let user: User?
        do {
            let (data, response) = try await send(request)
            if response.statusCode == 401 {
                user = nil
            } else if (200...299).contains(response.statusCode) {
                user = try apiDecoder.decode(UserResponse.self, from: data).user
            } else {
                return // Ambiguous: leave the identity for the next confirmed read.
            }
        } catch {
            return // Transport failure: keep the last confirmed identity.
        }
        guard identity.accept(user, expected: owner) else { return }
        await identity.publish()
    }

    /// Reads that belong to no identity, so they go out signed out and carry
    /// no claim: an invite link's preview, shown before sign-in.
    private static let publicReads: Set<String> = ["/api/household/join/preview"]

    private static func changesIdentity(_ request: URLRequest) -> Bool {
        let path = request.url?.path ?? ""
        if ["/api/auth/email/verify", "/api/auth/magic-link/consume"].contains(path) { return true }
        guard request.httpMethod != "GET" else { return false }
        return ["/api/auth/login", "/api/auth/register", "/api/auth/logout", "/api/auth/password",
                "/api/auth/password/reset", "/api/auth/apple/native", "/api/household",
                "/api/household/join", "/api/household/leave", "/api/household/transfer"].contains(path)
            || (path == "/api/me" && request.httpMethod == "DELETE")
            || (path.hasPrefix("/api/households/") && path.hasSuffix("/activate"))
    }
}

struct LogOrigin: Codable, Equatable {
    let actorID: Int
    let householdID: Int
}

/// Shared by every copy of an APIClient. The app opts in at its composition
/// root; standalone transport contract tests can still use an isolated client.
final class ClientIdentity: @unchecked Sendable {
    enum Phase: Equatable { case checking, changing, ready, anonymous, logoutPending }
    struct Snapshot {
        let revision: UUID
        let phase: Phase
        let user: User?
        let logoutIsDurable: Bool
        /// Ready from Home saved on this device: the launch session check could
        /// not reach the server, and no server answer has confirmed it since.
        var resumedOffline = false
        var origin: LogOrigin? {
            guard phase == .ready, let user, let household = user.householdId else { return nil }
            return LogOrigin(actorID: user.id, householdID: household)
        }
    }
    private struct Marker: Codable { let logoutPending: Bool }
    let managed: Bool
    private let lock = NSLock()
    private var revision = UUID()
    private var phase: Phase
    private var user: User?
    private var logoutIsDurable = true
    private var resumedOffline = false
    /// Cleared by the first server answer or identity change after launch.
    private var mayResumeSavedHome = true
    private let fileURL: URL?
    private let homeURL: URL?
    @MainActor var observer: ((Snapshot) -> Void)?

    private static let registryLock = NSLock()
    private static var registry: [String: ClientIdentity] = [:]
    static func shared(for baseURL: URL) -> ClientIdentity {
        registryLock.lock(); defer { registryLock.unlock() }
        let key = baseURL.absoluteString
        if let identity = registry[key] { return identity }
        let directory = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        let name = Data(key.utf8).base64EncodedString().replacingOccurrences(of: "/", with: "_")
        let identity = ClientIdentity(fileURL: directory.appendingPathComponent("nabu-session-\(name).json"),
                                      homeURL: directory.appendingPathComponent("nabu-home-\(name).json"))
        registry[key] = identity
        return identity
    }

    init(fileURL: URL? = nil, homeURL: URL? = nil, managed: Bool = true) {
        self.fileURL = fileURL
        self.homeURL = homeURL
        self.managed = managed
        // A marker is removed only after confirmed revocation. An unreadable
        // marker must not turn an unfinished logout into an automatic login.
        phase = fileURL.map { FileManager.default.fileExists(atPath: $0.path) } == true ? .logoutPending : .checking
        // Saved Home data never outlives an unfinished sign-out.
        if phase == .logoutPending { removeSavedHome() }
    }

    var snapshot: Snapshot {
        lock.lock(); defer { lock.unlock() }
        return Snapshot(revision: revision, phase: phase, user: user, logoutIsDurable: logoutIsDurable,
                        resumedOffline: resumedOffline)
    }
    func isCurrent(_ owner: Snapshot) -> Bool {
        !managed || snapshot.revision == owner.revision
    }
    func ownsTransition(_ ticket: UUID) -> Bool {
        let value = snapshot
        return value.revision == ticket && value.phase == .changing
    }
    @discardableResult
    func accept(_ next: User?, expected owner: Snapshot? = nil) -> Bool {
        lock.lock(); defer { lock.unlock() }
        guard owner == nil || owner?.revision == revision,
              phase != .logoutPending && phase != .changing else { return false }
        if phase != (next == nil ? .anonymous : .ready) || user?.id != next?.id
            || user?.householdId != next?.householdId || user?.role != next?.role {
            revision = UUID()
            if !savedHomeBelongs(to: next) { removeSavedHome() }
        }
        user = next
        phase = next == nil ? .anonymous : .ready
        resumedOffline = false
        mayResumeSavedHome = false
        return true
    }
    func invalidate(_ owner: Snapshot) {
        lock.lock(); defer { lock.unlock() }
        guard revision == owner.revision else { return }
        revision = UUID(); user = nil; phase = .checking
        resumedOffline = false
        mayResumeSavedHome = false
        removeSavedHome()
    }
    func beginTransition(logout: Bool = false, expected owner: Snapshot? = nil) throws -> UUID {
        lock.lock(); defer { lock.unlock() }
        guard owner == nil || owner?.revision == revision,
              phase != .changing && (phase != .logoutPending || logout) else { throw APIError.contextChanged }
        resumedOffline = false
        mayResumeSavedHome = false
        removeSavedHome()
        if logout, let fileURL {
            do {
                try FileManager.default.createDirectory(at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
                try JSONEncoder().encode(Marker(logoutPending: true)).write(to: fileURL, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
                logoutIsDurable = true
            } catch {
                revision = UUID(); user = nil; phase = .logoutPending; logoutIsDurable = false
                throw APIError.saveNotDurable
            }
        }
        revision = UUID(); user = nil; phase = .changing
        return revision
    }
    func finishTransition(_ ticket: UUID, user next: User?, logout: Bool = false) throws {
        lock.lock(); defer { lock.unlock() }
        guard revision == ticket && phase == .changing else { throw APIError.contextChanged }
        if logout, let fileURL, FileManager.default.fileExists(atPath: fileURL.path) {
            do { try FileManager.default.removeItem(at: fileURL) }
            catch { phase = .logoutPending; throw error }
        }
        revision = UUID(); user = next; phase = next == nil ? .anonymous : .ready
        logoutIsDurable = true
    }
    func failTransition(_ ticket: UUID, logout: Bool) {
        lock.lock(); defer { lock.unlock() }
        guard revision == ticket else { return }
        revision = UUID(); user = nil; phase = logout ? .logoutPending : .checking
    }

#if DEBUG
    /// `-resetState` only: forgets an unfinished sign-out, so a UI test that
    /// left one (its server never confirmed it) can't strand every signed-out
    /// test that launches after it. The app itself never discards one.
    func discardPendingLogoutForTests() {
        lock.lock(); defer { lock.unlock() }
        guard phase == .logoutPending else { return }
        if let fileURL { try? FileManager.default.removeItem(at: fileURL) }
        revision = UUID(); user = nil; phase = .checking; logoutIsDurable = true
    }
#endif

    /// Offline launch: becomes the identity whose Home was last saved on this
    /// device when the launch session check could not reach the server. Only
    /// before any server answer or identity change, because an answer (no
    /// session, a 401, a 409 context change) decides the session, never saved
    /// data. Requests still carry this identity's claim, which the server
    /// rejects if the session now belongs to someone else.
    func resumeSavedHome() -> HomeSnapshot? {
        lock.lock(); defer { lock.unlock() }
        guard mayResumeSavedHome, phase == .checking, let saved = readSavedHome(),
              saved.belongs(to: saved.user) else { return nil }
        mayResumeSavedHome = false
        revision = UUID(); user = saved.user; phase = .ready; resumedOffline = true
        return saved
    }

    /// Saves Home for offline launch, only for the identity that is ready and
    /// server-confirmed right now. Holding the lock across the write means a
    /// sign-out or switch can never be followed by a save that began before it.
    /// Protected like the journal: a launch while the device is locked must be
    /// able to read it, or an identity change could not tell whose it is.
    @discardableResult
    func saveHome(_ home: HomeSnapshot, expected owner: Snapshot) -> Bool {
        lock.lock(); defer { lock.unlock() }
        guard let homeURL, owner.revision == revision, phase == .ready, !resumedOffline,
              let user, home.belongs(to: user) else { return false }
        do {
            try FileManager.default.createDirectory(at: homeURL.deletingLastPathComponent(), withIntermediateDirectories: true)
            try HomeSnapshot.encoder.encode(home).write(to: homeURL, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
            return true
        } catch { return false }
    }

    // The callers below hold `lock`.
    private func readSavedHome() -> HomeSnapshot? {
        guard let homeURL, let data = try? Data(contentsOf: homeURL),
              let saved = try? HomeSnapshot.decoder.decode(HomeSnapshot.self, from: data),
              saved.version == HomeSnapshot.currentVersion else { return nil }
        return saved
    }
    private func savedHomeBelongs(to next: User?) -> Bool {
        guard let next, let saved = readSavedHome() else { return false }
        return saved.belongs(to: next)
    }
    private func removeSavedHome() {
        guard let homeURL, FileManager.default.fileExists(atPath: homeURL.path) else { return }
        try? FileManager.default.removeItem(at: homeURL)
    }

    @MainActor func publish() { observer?(snapshot) }
}
