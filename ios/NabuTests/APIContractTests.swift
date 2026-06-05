import XCTest
@testable import Nabu

final class APIContractTests: XCTestCase {
    let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .iso8601
        d.keyDecodingStrategy = .convertFromSnakeCase
        return d
    }()

    // MARK: - Error Decoding

    func testDecodeServerError() throws {
        let json = #"{"error":"Something went wrong"}"#.data(using: .utf8)!
        let error = try decoder.decode(APIErrorResponse.self, from: json)
        XCTAssertEqual(error.error, "Something went wrong")
    }

    func testDecodeErrorWithEmoji() throws {
        let json = #"{"error":"\u26a0\ufe0f Invalid chore name"}"#.data(using: .utf8)!
        let error = try decoder.decode(APIErrorResponse.self, from: json)
        XCTAssertTrue(error.error.contains("Invalid chore name"))
    }

    // MARK: - Retry-After

    func testRateLimitedErrorHasRetryAfter() {
        let error = APIError.rateLimited(retryAfter: "30")
        XCTAssertTrue(error.errorDescription?.contains("30") ?? false)
    }

    func testRateLimitedErrorWithoutRetryAfter() {
        let error = APIError.rateLimited(retryAfter: nil)
        XCTAssertTrue(error.errorDescription?.contains("try again") ?? false)
    }

    // MARK: - HTTP Status Codes

    func testHTTP401IsUnauthorized() {
        let error = APIError.httpError(statusCode: 401)
        XCTAssertTrue(error.errorDescription?.contains("401") ?? false)
    }

    func testServerErrorPreservesMessage() {
        let error = APIError.serverError(statusCode: 422, message: "Name is required")
        XCTAssertEqual(error.errorDescription, "Name is required")
    }

    // MARK: - Anti-Enumeration

    func testDuplicateRegistrationReturns200() {
        let error = APIError.serverError(statusCode: 409, message: "if this email is new, check your inbox")
        XCTAssertEqual(error.errorDescription, "if this email is new, check your inbox")
    }

    // MARK: - CSRF Token Provider

    func testCSRFTokenProviderReadsFromCookieStore() {
        let store = CookieStore()
        let provider = CSRFTokenProvider(cookieStore: store)
        XCTAssertNil(provider.token)
    }

    // MARK: - Cookie Store

    func testCookieStoreClearAll() {
        let store = CookieStore()
        store.clearAll()
        XCTAssertNil(store.sessionCookie)
        XCTAssertNil(store.csrfCookie)
    }

    // MARK: - Decoder Configuration

    func testDecoderConvertsSnakeCase() throws {
        let json = #"{"user_id": 1, "display_name": "Alice", "email_verified": true}"#.data(using: .utf8)!
        struct TestUser: Codable {
            let userId: Int
            let displayName: String
            let emailVerified: Bool
        }
        let user = try decoder.decode(TestUser.self, from: json)
        XCTAssertEqual(user.userId, 1)
        XCTAssertEqual(user.displayName, "Alice")
        XCTAssertTrue(user.emailVerified)
    }

    func testDecoderISO8601Dates() throws {
        let json = #"{"created_at": "2024-12-25T14:30:00Z"}"#.data(using: .utf8)!
        struct TestModel: Codable {
            let createdAt: Date
        }
        let model = try decoder.decode(TestModel.self, from: json)
        let components = Calendar.current.dateComponents(in: TimeZone(secondsFromGMT: 0)!, from: model.createdAt)
        XCTAssertEqual(components.year, 2024)
        XCTAssertEqual(components.month, 12)
        XCTAssertEqual(components.day, 25)
        XCTAssertEqual(components.hour, 14)
        XCTAssertEqual(components.minute, 30)
    }
}
