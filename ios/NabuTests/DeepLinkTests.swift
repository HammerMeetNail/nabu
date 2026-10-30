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

    // MARK: - What a quick-log target opens

    private func chore(_ id: Int, _ name: String, category: String = "chores", predefinedKey: String? = nil) -> Chore {
        Chore(id: id, householdId: 1, name: name, icon: "📋", color: "#000000",
              sortOrder: 0, category: category, isPredefined: predefinedKey != nil,
              predefinedKey: predefinedKey, createdBy: nil, createdAt: Date(),
              indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false, metricType: "none")
    }

    func testFeedBabyOpensFeedBaby() {
        let feed = chore(4, "Feed Baby", category: "feeding", predefinedKey: "Feed Baby")
        let chores = [chore(1, "Feed Cats", category: "feeding"), feed]
        XCTAssertEqual(QuickLogTarget.predefined(key: "Feed Baby").chore(in: chores, home: chores)?.id, 4)
    }

    /// A pack's "Feed Ada" came from Feed Baby: the Log Feed shortcut opens
    /// it before Home's first feeding tile, a pet's (PWA `quickLogChore`).
    func testFeedBabyOpensANamedBabysFeedsBeforeAPets() {
        let feedCats = chore(1, "Feed Cats", category: "feeding", predefinedKey: "Feed Cats")
        let bottles = Chore(id: 5, householdId: 1, name: "Bottles", icon: "🍼", color: "#000000", sortOrder: 1,
                            category: "feeding", isPredefined: false, predefinedKey: "", createdBy: nil, createdAt: Date(),
                            indicatorLabels: [], indicatorDefaults: [], hasVolumeML: true, metricType: "amount",
                            templateKey: "Feed Baby", namedFor: "Ada")
        let chores = [feedCats, bottles]
        XCTAssertEqual(QuickLogTarget.predefined(key: "Feed Baby").chore(in: chores, home: chores)?.id, 5)
    }

    /// Packs name a baby's activities after the baby, and a household can
    /// have only pets: the Log Feed shortcut opens Home's first feeding tile.
    func testFeedBabyFallsBackToHomesFirstFeedingActivity() {
        let vacuum = chore(1, "Vacuum")
        let feedCats = chore(2, "Feed Cats", category: "feeding")
        let feedAda = chore(3, "Feed Ada", category: "feeding")
        let chores = [vacuum, feedCats, feedAda]
        let target = QuickLogTarget.predefined(key: "Feed Baby")
        XCTAssertEqual(target.chore(in: chores, home: [vacuum, feedAda, feedCats])?.name, "Feed Ada")
        // A feeding activity hidden from Home isn't one of its tiles.
        XCTAssertEqual(target.chore(in: chores, home: [vacuum, feedCats])?.name, "Feed Cats")
        XCTAssertNil(target.chore(in: [vacuum], home: [vacuum]))
    }

    func testOtherPredefinedTargetsDontFallBack() {
        let chores = [chore(2, "Feed Cats", category: "feeding")]
        XCTAssertNil(QuickLogTarget.predefined(key: "Change Baby").chore(in: chores, home: chores))
    }

    func testChoreTargetOpensThatActivity() {
        let chores = [chore(1, "Vacuum"), chore(2, "Feed Cats", category: "feeding")]
        XCTAssertEqual(QuickLogTarget.chore(id: 2).chore(in: chores, home: [])?.name, "Feed Cats")
        XCTAssertNil(QuickLogTarget.chore(id: 9).chore(in: chores, home: chores))
    }
}
