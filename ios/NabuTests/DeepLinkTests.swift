import XCTest
@testable import Nabu

/// Parsing tests for universal links and the quicklog deep link. The handled
/// paths must stay in sync with the backend's apple-app-site-association
/// (internal/app/server.go) and the PWA's route handling.
final class DeepLinkTests: XCTestCase {

    private func parse(_ string: String) -> DeepLink? {
        DeepLink.parse(URL(string: string)!)
    }

    func testVerifyEmail() {
        XCTAssertEqual(
            parse("https://nabu-app.com/verify-email?token=tok123"),
            .verifyEmail(token: "tok123")
        )
    }

    func testVerifyEmailWithoutTokenIsIgnored() {
        XCTAssertNil(parse("https://nabu-app.com/verify-email"))
        XCTAssertNil(parse("https://nabu-app.com/verify-email?token="))
    }

    func testMagicLogin() {
        XCTAssertEqual(
            parse("https://nabu-app.com/magic-login?token=abc"),
            .magicLogin(token: "abc")
        )
    }

    func testJoinHousehold() {
        XCTAssertEqual(
            parse("https://nabu-app.com/join?code=ABC123"),
            .joinHousehold(code: "ABC123")
        )
    }

    func testQuickLogChore() {
        // Parity with the PWA's /?quicklog=chore:<id> notification deep link.
        XCTAssertEqual(
            parse("https://nabu-app.com/?quicklog=chore:42"),
            .quickLog(.chore(id: 42))
        )
    }

    func testQuickLogManifestShortcutTargets() {
        // Parity with the PWA manifest shortcuts (Log feed / Log chore /
        // Activity), reused by the iOS Home-Screen quick actions.
        XCTAssertEqual(
            parse("https://nabu-app.com/?quicklog=feed-baby"),
            .quickLog(.predefined(key: "Feed Baby"))
        )
        XCTAssertEqual(parse("https://nabu-app.com/?quicklog=chore"), .showHomeLog)
        XCTAssertEqual(parse("https://nabu-app.com/?quicklog=activity"), .showActivity)
    }

    func testQuickLogMalformedIdIsIgnored() {
        XCTAssertNil(parse("https://nabu-app.com/?quicklog=chore:abc"))
        XCTAssertNil(parse("https://nabu-app.com/?quicklog=unknown"))
    }

    func testUnrelatedURLsAreIgnored() {
        XCTAssertNil(parse("https://nabu-app.com/reset-password?token=t"))
        XCTAssertNil(parse("https://nabu-app.com/settings"))
        XCTAssertNil(parse("nabu://callback"))
    }

    func testTokenWithURLEncodedCharactersDecodes() {
        XCTAssertEqual(
            parse("https://nabu-app.com/verify-email?token=a%2Bb"),
            .verifyEmail(token: "a+b")
        )
    }

    // MARK: - Handling a link again

    /// A widget, a control or a notification sends the same quick-log link
    /// every time; each tap must open the sheet, not just the first.
    func testQuickLogLinksAreHandledEveryTime() {
        var handled = HandledLinks()
        let url = URL(string: "https://nabu-app.com/?quicklog=chore:7")!
        let link = DeepLink.parse(url)!
        XCTAssertTrue(handled.shouldHandle(link, from: url))
        XCTAssertTrue(handled.shouldHandle(link, from: url))
        XCTAssertFalse(link.isOneTime)
    }

    /// onOpenURL and Handoff can deliver a token link at once; it is spent once.
    func testOneTimeLinksAreHandledOnce() {
        var handled = HandledLinks()
        for string in ["https://nabu-app.com/verify-email?token=t1",
                       "https://nabu-app.com/magic-login?token=t2",
                       "https://nabu-app.com/join?code=ABC123"] {
            let url = URL(string: string)!
            let link = DeepLink.parse(url)!
            XCTAssertTrue(link.isOneTime, string)
            XCTAssertTrue(handled.shouldHandle(link, from: url), string)
            XCTAssertFalse(handled.shouldHandle(link, from: url), string)
        }
    }

    func testOtherAppLinksAreHandledEveryTime() {
        var handled = HandledLinks()
        for string in ["https://nabu-app.com/?quicklog=activity", "https://nabu-app.com/?quicklog=chore"] {
            let url = URL(string: string)!
            let link = DeepLink.parse(url)!
            XCTAssertTrue(handled.shouldHandle(link, from: url), string)
            XCTAssertTrue(handled.shouldHandle(link, from: url), string)
        }
    }
}
