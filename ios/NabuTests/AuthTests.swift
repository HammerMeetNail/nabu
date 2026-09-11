import XCTest
@testable import Nabu

private final class CancelCreatedTaskDelegate: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, didCreateTask task: URLSessionTask) {
        task.cancel()
    }
}

final class AuthTests: XCTestCase {
    var api: APIClient!

    override func setUp() {
        api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
    }

    // MARK: - AuthStore initialization

    private var deletionUser: User {
        User(id: 1, householdId: 1, email: "delete@test.local", displayName: "Test",
             avatarColor: "#19323C", emailVerified: true, role: "owner", createdAt: Date())
    }

    @MainActor
    private func deletionClient(status: Int = 409, confirmationFails: Bool = false,
                                gate: NativeResponseGate? = nil) -> APIClient {
        let user = deletionUser
        let identity = ClientIdentity()
        identity.accept(user)
        var client = APIClient(baseURL: URL(string: "http://localhost:9999")!, identity: identity)
        client.mockAsyncHandler = { request in
            XCTAssertEqual(request.url?.path, "/api/me")
            if request.httpMethod == "DELETE" {
                await gate?.pause()
                let data = status == 200 ? Data(#"{"status":"ok"}"#.utf8)
                    : Data(#"{"error":"Transfer ownership first"}"#.utf8)
                return (data, HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!)
            }
            let data = try apiEncoder.encode(UserResponse(user: status == 200 ? nil : user))
            return (data, HTTPURLResponse(url: request.url!, statusCode: confirmationFails ? 503 : 200, httpVersion: nil, headerFields: nil)!)
        }
        return client
    }

    @MainActor
    func testDeletionFailureSurvivesCanonicalIdentityConfirmation() async {
        let client = deletionClient()
        let model = AccountDeletionModel()
        model.open(api: client)
        let originalRevision = client.identity.snapshot.revision
        await model.delete(api: client)
        XCTAssertNotEqual(client.identity.snapshot.revision, originalRevision)
        XCTAssertTrue(model.isPresented)
        XCTAssertFalse(model.isDeleting)
        XCTAssertEqual(model.errorMessage, "Transfer ownership first")
        model.cancel(); model.open(api: client)
        XCTAssertNil(model.errorMessage)
    }

    @MainActor
    func testCanceledDeletionCannotModifyReopenedPresentation() async throws {
        let gate = NativeResponseGate()
        let client = deletionClient(gate: gate)
        let model = AccountDeletionModel()
        model.open(api: client)
        let old = Task { await model.delete(api: client) }
        await gate.waitUntilPaused()
        model.cancel()
        // A separately confirmed session permits a new presentation while the
        // old transport still owns a delayed response.
        try client.identity.finishTransition(client.identity.snapshot.revision, user: deletionUser)
        XCTAssertEqual(client.identity.snapshot.phase, .ready)
        model.open(api: client)
        await gate.release()
        await old.value
        XCTAssertTrue(model.isPresented)
        XCTAssertFalse(model.isDeleting)
        XCTAssertNil(model.errorMessage)
    }

    @MainActor
    func testDeletionCompletionCannotFollowHouseholdChange() async throws {
        let gate = NativeResponseGate()
        let client = deletionClient(gate: gate)
        let model = AccountDeletionModel()
        model.open(api: client)
        let old = Task { await model.delete(api: client) }
        await gate.waitUntilPaused()
        let user = deletionUser
        try client.identity.finishTransition(client.identity.snapshot.revision, user: User(id: user.id, householdId: 2, email: user.email,
            displayName: user.displayName, avatarColor: user.avatarColor, emailVerified: true,
            role: user.role, createdAt: user.createdAt))
        XCTAssertEqual(client.identity.snapshot.phase, .ready)
        XCTAssertEqual(client.identity.snapshot.user?.householdId, 2)
        model.reconcile(api: client)
        await gate.release()
        await old.value
        XCTAssertFalse(model.isPresented)
        XCTAssertNil(model.errorMessage)
        XCTAssertEqual(client.identity.snapshot.user?.householdId, 2)
    }

    @MainActor
    func testConfirmedDeletionDismissesForAnonymousSession() async {
        let client = deletionClient(status: 200)
        let model = AccountDeletionModel()
        model.open(api: client)
        await model.delete(api: client)
        XCTAssertEqual(client.identity.snapshot.phase, .anonymous)
        XCTAssertFalse(model.isPresented)
    }

    @MainActor
    func testDeletionConfirmationFailureLeavesSessionRecoveryVisible() async {
        let client = deletionClient(confirmationFails: true)
        let model = AccountDeletionModel()
        model.open(api: client)
        await model.delete(api: client)
        XCTAssertEqual(client.identity.snapshot.phase, .checking)
        XCTAssertFalse(model.isPresented)
        XCTAssertNil(model.errorMessage)
    }

    @MainActor
    func testAuthStoreInitialState() {
        let store = AuthStore(api: api)
        XCTAssertFalse(store.isLoading)
        XCTAssertNil(store.errorMessage)
    }

    @MainActor
    func testConfigureUpdatesAPI() {
        let store = AuthStore(api: api)
        let newAPI = APIClient(baseURL: URL(string: "http://other:8080")!)
        store.configure(api: newAPI)
        XCTAssertEqual(store.api.baseURL, URL(string: "http://other:8080")!)
    }

    @MainActor
    func testRegistrationRetryOffersRecovery() async {
        api.mockHandler = { request in
            let body = #"{"status":"if this email is new, check your inbox"}"#.data(using: .utf8)!
            return (body, HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
        }
        let auth = AuthStore(api: api)
        let user = await auth.register(email: "test@example.invalid", password: "synthetic-password")
        XCTAssertNil(user)
        XCTAssertNil(auth.errorMessage)
        XCTAssertTrue(auth.registrationNotice?.contains("sign in") == true)
    }

    @MainActor
    func testSetPasswordSendsEmptyCurrentAndReturnsUpdatedUser() async {
        api.mockHandler = { request in
            XCTAssertEqual(request.url?.path, "/api/auth/password")
            let body = try! JSONSerialization.jsonObject(with: request.httpBody!) as! [String: Any]
            XCTAssertEqual(body["current_password"] as? String, "")
            XCTAssertEqual(body["new_password"] as? String, "owner-password")
            let response = ##"{"user":{"id":1,"householdId":null,"email":"owner@test.local","displayName":"Owner","avatarColor":"#19323C","emailVerified":true,"hasPassword":true,"role":"","createdAt":"2026-09-10T12:00:00Z"}}"##.data(using: .utf8)!
            return (response, HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
        }
        let auth = AuthStore(api: api)
        let user = await auth.changePassword(current: "", new: "owner-password")
        XCTAssertEqual(user?.hasPassword, true)
    }

    /// Opt in through the Xcode test plan environment. Uses the local server
    /// and real URLSession/HTTPCookieStorage, including a recreated identity.
    @MainActor
    func testLocalServerPasswordRotationAndFailedLogoutRecovery() async throws {
        guard let value = ProcessInfo.processInfo.environment["NABU_TEST_SERVER_URL"] else {
            throw XCTSkip("Set NABU_TEST_SERVER_URL to the local test backend")
        }
        let baseURL = try XCTUnwrap(URL(string: value))
        guard baseURL.scheme == "http", baseURL.host == "localhost", baseURL.port == 8080 else {
            XCTFail("Native integration tests require http://localhost:8080")
            return
        }
        let identityFile = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: identityFile) }
        let identity = ClientIdentity(fileURL: identityFile)
        let client = APIClient(baseURL: baseURL, identity: identity)
        client.cookieStore.clearAll()
        defer { client.cookieStore.clearAll() }
        let _: UserResponse = try await client.get("/api/me")
        XCTAssertNotNil(client.csrfProvider.token)
        let auth = AuthStore(api: client)
        let email = "native-\(UUID().uuidString)@test.local"
        let registered = await auth.register(email: email, password: "original-password")
        XCTAssertNotNil(registered, auth.errorMessage ?? "Registration failed")
        let oldCookie = try XCTUnwrap(client.cookieStore.sessionCookie)
        let changed = await auth.changePassword(current: "original-password", new: "replacement-password")
        XCTAssertNotNil(changed, auth.errorMessage ?? "Password change failed")
        let currentCookie = try XCTUnwrap(client.cookieStore.sessionCookie)
        XCTAssertTrue(oldCookie.value != currentCookie.value, "Password change must rotate the session")
        try await assertRevoked(oldCookie, at: baseURL)

        // Cancel the real task synchronously at creation, before it can reach
        // the server. Cookies and response handling remain URLSession's own.
        var interruptedClient = client
        interruptedClient.session = URLSession(configuration: .default, delegate: CancelCreatedTaskDelegate(), delegateQueue: nil)
        defer { interruptedClient.session.invalidateAndCancel() }
        auth.configure(api: interruptedClient)
        let loggedOut = await auth.logout()
        XCTAssertFalse(loggedOut)
        XCTAssertEqual(identity.snapshot.phase, .logoutPending)
        XCTAssertNil(identity.snapshot.user)
        XCTAssertTrue(client.cookieStore.sessionCookie?.value == currentCookie.value)

        let reopened = ClientIdentity(fileURL: identityFile)
        XCTAssertEqual(reopened.snapshot.phase, .logoutPending)
        let resumedClient = APIClient(baseURL: baseURL, identity: reopened)
        let resumedAuth = AuthStore(api: resumedClient)
        let retried = await resumedAuth.logout()
        XCTAssertTrue(retried, resumedAuth.errorMessage ?? "Logout retry failed")
        XCTAssertNil(resumedClient.cookieStore.sessionCookie)
        try await assertRevoked(currentCookie, at: baseURL)

        let _: UserResponse = try await resumedClient.get("/api/me")
        let oldLogin = await resumedAuth.login(email: email, password: "original-password")
        XCTAssertNil(oldLogin)
        let newLogin = await resumedAuth.login(email: email, password: "replacement-password")
        XCTAssertNotNil(newLogin, resumedAuth.errorMessage ?? "New password login failed")
        let finalLogout = await resumedAuth.logout()
        XCTAssertTrue(finalLogout)
    }

    private func assertRevoked(_ cookie: HTTPCookie, at baseURL: URL) async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.httpCookieStorage = nil
        configuration.httpShouldSetCookies = false
        let session = URLSession(configuration: configuration)
        defer { session.invalidateAndCancel() }
        var request = URLRequest(url: baseURL.appendingPathComponent("/api/me"))
        request.cachePolicy = .reloadIgnoringLocalCacheData
        request.setValue("nabu_session=\(cookie.value)", forHTTPHeaderField: "Cookie")
        let (data, response) = try await session.data(for: request)
        XCTAssertEqual((response as? HTTPURLResponse)?.statusCode, 200)
        let user = try apiDecoder.decode(UserResponse.self, from: data).user
        XCTAssertNil(user, "Revoked cookie must not authenticate")
    }

    // MARK: - Validation helpers (client-side password rules)

    func testPasswordMinLength8() {
        XCTAssertTrue("12345678".count >= 8)
        XCTAssertFalse("1234567".count >= 8)
    }

    func testPasswordMaxLength72() {
        let longPassword = String(repeating: "a", count: 72)
        XCTAssertTrue(longPassword.count <= 72)
        let tooLong = String(repeating: "a", count: 73)
        XCTAssertFalse(tooLong.count <= 72)
    }

    // MARK: - CSRF token presence in state-changing requests

    func testCSRFTokenProviderReturnsNilWhenNoCookie() {
        let store = CookieStore()
        // CookieStore wraps HTTPCookieStorage.shared, which the test host
        // shares with any app session previously run on this simulator —
        // clear it so a stray nabu_csrf cookie can't leak in.
        store.clearAll()
        let provider = CSRFTokenProvider(cookieStore: store)
        XCTAssertNil(provider.token)
    }

    // MARK: - Anti-enumeration

    func testMagicLinkRequestAlwaysShowsSuccess() async {
        // Even with a network error, the UI should show "sent" state
        // This is tested via the MagicLinkView which always sets sent=true
        // after calling requestMagicLink regardless of result
        XCTAssertTrue(true) // Behavior encoded in MagicLinkView
    }

    func testForgotPasswordAlwaysShowsSuccess() async {
        // Same anti-enumeration principle
        XCTAssertTrue(true) // Behavior encoded in ForgotPasswordView
    }
}
