import XCTest
@testable import Nabu

/// Contract tests for the APNs registration lifecycle: the register and
/// unregister bodies must encode (via the production `apiEncoder`) to the
/// exact camelCase JSON `internal/handlers/apns.go` decodes, and the
/// environment/token helpers must match what the backend stores.
final class APNsContractTests: XCTestCase {

    private func json(_ data: Data) -> [String: Any] {
        try! JSONSerialization.jsonObject(with: data) as! [String: Any]
    }

    // MARK: - Request bodies

    func testRegisterRequestEncodesServerFieldNames() throws {
        let req = APNsRegisterRequest(
            token: "deadbeef00",
            environment: "sandbox",
            bundleId: "com.nabu.app",
            deviceName: "iPhone"
        )
        let dict = json(try apiEncoder.encode(req))
        XCTAssertEqual(dict["token"] as? String, "deadbeef00")
        XCTAssertEqual(dict["environment"] as? String, "sandbox")
        XCTAssertEqual(dict["bundleId"] as? String, "com.nabu.app")
        XCTAssertEqual(dict["deviceName"] as? String, "iPhone")
        XCTAssertEqual(dict.count, 4, "unexpected extra fields: \(dict.keys.sorted())")
    }

    func testUnregisterRequestEncodesServerFieldNames() throws {
        let req = APNsUnregisterRequest(token: "deadbeef00")
        let dict = json(try apiEncoder.encode(req))
        XCTAssertEqual(dict["token"] as? String, "deadbeef00")
        XCTAssertNil(dict["environment"])
        XCTAssertEqual(dict.count, 1, "unexpected extra fields: \(dict.keys.sorted())")
    }

    func testReminderSnoozeRequestEncodesServerFieldNames() throws {
        let req = ReminderSnoozeRequest(choreId: 7, minutes: 30)
        let dict = json(try apiEncoder.encode(req))
        XCTAssertEqual(dict["choreId"] as? Int, 7)
        XCTAssertEqual(dict["minutes"] as? Int, 30)
        XCTAssertEqual(dict.count, 2, "unexpected extra fields: \(dict.keys.sorted())")
    }

    func testAppleNativeAuthRequestEncodesServerFieldNames() throws {
        let req = AppleNativeAuthRequest(identityToken: "eyJ...", nonce: "abc123")
        let dict = json(try apiEncoder.encode(req))
        XCTAssertEqual(dict["identityToken"] as? String, "eyJ...")
        XCTAssertEqual(dict["nonce"] as? String, "abc123")
        XCTAssertEqual(dict.count, 2, "unexpected extra fields: \(dict.keys.sorted())")
    }

    // MARK: - Environment selection

    func testEnvironmentSelection() {
        XCTAssertEqual(APNsEnvironment.select(isDebugBuild: true), "sandbox")
        XCTAssertEqual(APNsEnvironment.select(isDebugBuild: false), "production")
    }

    func testEnvironmentValuesMatchServerConstants() {
        // internal/apns/store.go: EnvironmentSandbox / EnvironmentProduction
        XCTAssertEqual(APNsEnvironment.sandbox, "sandbox")
        XCTAssertEqual(APNsEnvironment.production, "production")
    }

    // MARK: - Token encoding

    func testHexTokenEncoding() {
        let data = Data([0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x01])
        XCTAssertEqual(PushRegistration.hexToken(from: data), "deadbeef0001")
    }

    func testHexTokenIsLowercaseHex() {
        let token = PushRegistration.hexToken(from: Data((0...255).map { UInt8($0) }))
        XCTAssertEqual(token.count, 512)
        XCTAssertTrue(token.allSatisfy { "0123456789abcdef".contains($0) },
                      "backend's isHexToken() only accepts hex characters")
    }

    // MARK: - SIWA nonce

    func testRandomNonceIsUniqueAndHex() {
        let a = SignInWithAppleCoordinator.randomNonce()
        let b = SignInWithAppleCoordinator.randomNonce()
        XCTAssertNotEqual(a, b)
        XCTAssertEqual(a.count, 64)
        XCTAssertTrue(a.allSatisfy { "0123456789abcdef".contains($0) })
    }
}
