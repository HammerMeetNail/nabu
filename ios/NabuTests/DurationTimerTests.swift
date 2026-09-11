import XCTest
@testable import Nabu

final class DurationTimerTests: XCTestCase {
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        defaults = UserDefaults(suiteName: "DurationTimerTests")!
        defaults.removePersistentDomain(forName: "DurationTimerTests")
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: "DurationTimerTests")
        super.tearDown()
    }

    func testSaveLoadRoundTrip() {
        let started = Date(timeIntervalSince1970: 1_780_000_000.5)
        let timer = ActiveTimer(choreId: 7, choreName: "Nap", choreIcon: "😴", startedAt: started)
        DurationTimer.save(timer, to: defaults)

        let loaded = DurationTimer.load(from: defaults)
        XCTAssertEqual(loaded?.choreId, 7)
        XCTAssertEqual(loaded?.choreName, "Nap")
        XCTAssertEqual(loaded?.choreIcon, "😴")
        XCTAssertEqual(loaded!.startedAt.timeIntervalSince1970, started.timeIntervalSince1970, accuracy: 0.001)
    }

    func testLoadReturnsNilWhenEmpty() {
        XCTAssertNil(DurationTimer.load(from: defaults))
    }

    func testLoadReturnsNilForCorruptData() {
        defaults.set(Data("not json".utf8), forKey: DurationTimer.defaultsKey)
        XCTAssertNil(DurationTimer.load(from: defaults))
        // Valid JSON but wrong shape (choreId missing) is also rejected.
        defaults.set(Data("{\"startedAt\": 123}".utf8), forKey: DurationTimer.defaultsKey)
        XCTAssertNil(DurationTimer.load(from: defaults))
    }

    func testSaveNilClears() {
        let timer = ActiveTimer(choreId: 1, choreName: "X", choreIcon: "⏱", startedAt: Date())
        DurationTimer.save(timer, to: defaults)
        XCTAssertNotNil(DurationTimer.load(from: defaults))
        DurationTimer.save(nil, to: defaults)
        XCTAssertNil(DurationTimer.load(from: defaults))
    }

    func testElapsedSeconds() {
        let now = Date()
        let timer = ActiveTimer(choreId: 1, choreName: "X", choreIcon: "⏱",
                                startedAt: now.addingTimeInterval(-90))
        XCTAssertEqual(DurationTimer.elapsedSeconds(timer, now: now), 90)
    }

    func testElapsedSecondsClampsToZeroForFutureStart() {
        let now = Date()
        let timer = ActiveTimer(choreId: 1, choreName: "X", choreIcon: "⏱",
                                startedAt: now.addingTimeInterval(60))
        XCTAssertEqual(DurationTimer.elapsedSeconds(timer, now: now), 0)
    }

    func testFormatElapsedMatchesPWA() {
        XCTAssertEqual(DurationTimer.formatElapsed(0), "0:00")
        XCTAssertEqual(DurationTimer.formatElapsed(59), "0:59")
        XCTAssertEqual(DurationTimer.formatElapsed(90), "1:30")
        XCTAssertEqual(DurationTimer.formatElapsed(3599), "59:59")
        XCTAssertEqual(DurationTimer.formatElapsed(3600), "1:00:00")
        XCTAssertEqual(DurationTimer.formatElapsed(3661), "1:01:01")
        XCTAssertEqual(DurationTimer.formatElapsed(-5), "0:00")
    }
    func testStoppedTimerKeepsDurationKeyAndOriginAcrossReload() {
        let origin = LogOrigin(actorID: 4, householdID: 9)
        let timer = ActiveTimer(choreId: 3, choreName: "Sleep", choreIcon: "⏱",
            startedAt: Date(timeIntervalSince1970: 1000), origin: origin,
            idempotencyKey: "frozen", stoppedAt: Date(timeIntervalSince1970: 1090))
        XCTAssertTrue(DurationTimer.save(timer, to: defaults))
        let loaded = DurationTimer.load(from: defaults, origin: origin)
        XCTAssertEqual(loaded, timer)
        XCTAssertEqual(DurationTimer.elapsedSeconds(loaded!, now: Date(timeIntervalSince1970: 5000)), 90)
        XCTAssertNil(DurationTimer.load(from: defaults, origin: LogOrigin(actorID: 5, householdID: 9)))
        XCTAssertNil(DurationTimer.load(from: defaults, origin: LogOrigin(actorID: 4, householdID: 10)))
    }

}
