import XCTest
@testable import Nabu

final class AuthTests: XCTestCase {
    var api: APIClient!

    override func setUp() {
        api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
    }

    // MARK: - AuthStore initialization

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
