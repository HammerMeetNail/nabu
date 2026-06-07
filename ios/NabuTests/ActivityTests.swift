import XCTest
@testable import Nabu

final class ActivityTests: XCTestCase {

    // MARK: - ActivityViewMode

    func testActivityViewModeAllCases() {
        XCTAssertEqual(ActivityViewMode.allCases.count, 3)
        XCTAssertTrue(ActivityViewMode.allCases.contains(.history))
        XCTAssertTrue(ActivityViewMode.allCases.contains(.day))
        XCTAssertTrue(ActivityViewMode.allCases.contains(.week))
    }

    func testActivityViewModeTitles() {
        XCTAssertEqual(ActivityViewMode.history.title, "History")
        XCTAssertEqual(ActivityViewMode.day.title, "Day")
        XCTAssertEqual(ActivityViewMode.week.title, "Week")
    }

    // MARK: - Date helpers

    func testTodayISO() {
        let result = todayISO()
        XCTAssertTrue(result.matches("^\\d{4}-\\d{2}-\\d{2}$"))
    }

    func testShiftISODate() {
        let result = shiftISO("2026-06-01", by: 1)
        XCTAssertEqual(result, "2026-06-02")
    }

    func testShiftISODateBackward() {
        let result = shiftISO("2026-06-01", by: -1)
        XCTAssertEqual(result, "2026-05-31")
    }

    func testWeekStartYieldsMonday() {
        let result = weekStart(from: "2026-06-03") // Wednesday
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        guard let d = f.date(from: result) else { return XCTFail("Invalid date") }
        let weekday = Calendar.current.component(.weekday, from: d)
        XCTAssertEqual(weekday, 2, "Expected Monday (weekday=2), got \(weekday)")
    }

    // MARK: - fmtHour

    func testFmtHourMidnight() { XCTAssertEqual(fmtHour(0), "12 AM") }
    func testFmtHourOneAM() { XCTAssertEqual(fmtHour(1), "1 AM") }
    func testFmtHourElevenAM() { XCTAssertEqual(fmtHour(11), "11 AM") }
    func testFmtHourNoon() { XCTAssertEqual(fmtHour(12), "12 PM") }
    func testFmtHourOnePM() { XCTAssertEqual(fmtHour(13), "1 PM") }
    func testFmtHourElevenPM() { XCTAssertEqual(fmtHour(23), "11 PM") }

    // MARK: - Slot hour filtering

    func testAnytimeLogsHaveNullSlotHour() {
        let log = ChoreLog(id: 1, householdId: 1, userId: 1, choreId: 1,
                           completedAt: Date(), note: "", indicators: [],
                           slotHour: nil, createdAt: Date(), volumeML: nil,
                           indicatorVolumes: nil)
        XCTAssertNil(log.slotHour)
    }

    func testTimedLogsHaveSlotHour() {
        let log = ChoreLog(id: 1, householdId: 1, userId: 1, choreId: 1,
                           completedAt: Date(), note: "", indicators: [],
                           slotHour: 9, createdAt: Date(), volumeML: nil,
                           indicatorVolumes: nil)
        XCTAssertEqual(log.slotHour, 9)
    }

    func testFilterAnytimeLogs() {
        let anytime = ChoreLog(id: 1, householdId: 1, userId: 1, choreId: 1,
                                completedAt: Date(), note: "", indicators: [],
                                slotHour: nil, createdAt: Date(), volumeML: nil,
                                indicatorVolumes: nil)
        let timed = ChoreLog(id: 2, householdId: 1, userId: 1, choreId: 2,
                              completedAt: Date(), note: "", indicators: [],
                              slotHour: 9, createdAt: Date(), volumeML: nil,
                              indicatorVolumes: nil)
        let logs = [anytime, timed]
        let anytimeLogs = logs.filter { $0.slotHour == nil }
        XCTAssertEqual(anytimeLogs.count, 1)
    }

    // MARK: - Log grouping by date

    func testGroupLogsByDate() {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        let d1 = f.date(from: "2026-06-02")!
        let d2 = f.date(from: "2026-06-01")!

        let log1 = ChoreLog(id: 1, householdId: 1, userId: 1, choreId: 1,
                            completedAt: d1, note: "", indicators: [],
                            slotHour: 9, createdAt: Date(), volumeML: nil,
                            indicatorVolumes: nil)
        let log2 = ChoreLog(id: 2, householdId: 1, userId: 1, choreId: 2,
                            completedAt: d2, note: "", indicators: [],
                            slotHour: 10, createdAt: Date(), volumeML: nil,
                            indicatorVolumes: nil)

        let logs = [log1, log2]
        var groups: [String: [ChoreLog]] = [:]
        for log in logs {
            let dateStr = f.string(from: log.completedAt)
            groups[dateStr, default: []].append(log)
        }
        let sorted = groups.sorted { $0.key > $1.key }
        XCTAssertEqual(sorted.count, 2)
        XCTAssertEqual(sorted[0].key, "2026-06-02")
        XCTAssertEqual(sorted[1].key, "2026-06-01")
    }
}

private extension String {
    func matches(_ pattern: String) -> Bool {
        range(of: pattern, options: .regularExpression) != nil
    }
}
