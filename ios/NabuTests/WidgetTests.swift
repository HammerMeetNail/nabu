import XCTest
@testable import Nabu

/// The logic the Lock Screen widget and the Control Center button share with
/// the app (Support/WidgetDataCache.swift): which activity they show, the
/// time since its last log, and the sheet a tap opens.
final class WidgetTests: XCTestCase {

    private let chores = [
        WidgetChoreSnapshot(id: 3, name: "Feed Baby", icon: "🍼", lastCompletedAt: nil),
        WidgetChoreSnapshot(id: 7, name: "Walk Dog", icon: "🐕", lastCompletedAt: nil),
    ]

    func testShowsTheChosenActivity() {
        XCTAssertEqual(WidgetDataCache.activity(id: 7, in: chores)?.name, "Walk Dog")
    }

    func testShowsHomesFirstActivityWhenNoneWasChosen() {
        XCTAssertEqual(WidgetDataCache.activity(id: nil, in: chores)?.name, "Feed Baby")
        XCTAssertNil(WidgetDataCache.activity(id: nil, in: []))
    }

    /// A deleted or hidden activity shows nothing rather than another one.
    func testShowsNothingForAnActivityThatIsGone() {
        XCTAssertNil(WidgetDataCache.activity(id: 99, in: chores))
    }

    /// A tap opens the same pre-filled sheet as the notification "Log now".
    func testQuickLogURLOpensTheActivitysSheet() {
        let url = WidgetDataCache.quickLogURL(choreID: 7)
        XCTAssertEqual(DeepLink.parse(url), .quickLog(.chore(id: 7)))
    }

    func testCompactTimeSince() {
        let now = Date(timeIntervalSince1970: 1_000_000)
        func ago(_ minutes: Double) -> String {
            WidgetTimeText.compact(since: now.addingTimeInterval(-minutes * 60), now: now)
        }
        XCTAssertEqual(ago(0), "now")
        XCTAssertEqual(ago(0.5), "now")
        XCTAssertEqual(ago(1), "1m")
        XCTAssertEqual(ago(59), "59m")
        XCTAssertEqual(ago(60), "1h")
        XCTAssertEqual(ago(130), "2h")
        XCTAssertEqual(ago(47 * 60 + 59), "47h")
        XCTAssertEqual(ago(48 * 60), "2d")
        // A log the clock says is in the future reads as just now.
        XCTAssertEqual(ago(-5), "now")
    }
}
