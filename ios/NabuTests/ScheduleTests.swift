import XCTest
@testable import Nabu

final class ScheduleTests: XCTestCase {

    // MARK: - FreqType

    func testFreqTypeAllCases() {
        XCTAssertEqual(FreqType.allCases.count, 7)
    }

    func testFreqTypeLabels() {
        XCTAssertEqual(FreqType.once.label, "Does not repeat")
        XCTAssertEqual(FreqType.daily.label, "Every day")
        XCTAssertEqual(FreqType.weekly.label, "Weekly")
        XCTAssertEqual(FreqType.everyNDays.label, "Every N days")
    }

    func testFreqTypeRawValues() {
        XCTAssertEqual(FreqType.everyNDays.rawValue, "every_n_days")
        XCTAssertEqual(FreqType.monthlyByDate.rawValue, "monthly_by_date")
        XCTAssertEqual(FreqType.monthlyByWeekday.rawValue, "monthly_by_weekday")
    }

    // MARK: - fmtScheduleTime

    func testFmtScheduleTimeAM() {
        XCTAssertEqual(fmtScheduleTime("08:00"), "8:00 AM")
        XCTAssertEqual(fmtScheduleTime("00:30"), "12:30 AM")
    }

    func testFmtScheduleTimePM() {
        XCTAssertEqual(fmtScheduleTime("14:30"), "2:30 PM")
        XCTAssertEqual(fmtScheduleTime("12:00"), "12:00 PM")
        XCTAssertEqual(fmtScheduleTime("23:45"), "11:45 PM")
    }

    // MARK: - DAY_NAMES_SHORT

    func testDayNames() {
        XCTAssertEqual(DAY_NAMES_SHORT[0], "Sun")
        XCTAssertEqual(DAY_NAMES_SHORT[6], "Sat")
        XCTAssertEqual(DAY_NAMES_SHORT.count, 7)
    }

    // MARK: - Recurrence summary

    func testRecurrenceSummaryOnce() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1, frequencyType: "once",
                                timePeriod: "anytime", specificTime: "08:00",
                                timesOfDay: [], daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 0, monthWeekday: nil, monthOfYear: 0,
                                recurrenceEnd: nil, startDate: nil, targetCount: 0,
                                isActive: true, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        let summary = recurrenceSummary(sch)
        XCTAssertTrue(summary.contains("Once"))
        XCTAssertTrue(summary.contains("8:00 AM"))
    }

    func testRecurrenceSummaryDaily() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1, frequencyType: "daily",
                                timePeriod: "anytime", specificTime: nil,
                                timesOfDay: [], daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 0, monthWeekday: nil, monthOfYear: 0,
                                recurrenceEnd: nil, startDate: nil, targetCount: 0,
                                isActive: true, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        let summary = recurrenceSummary(sch)
        XCTAssertTrue(summary.contains("Every day"))
    }

    func testRecurrenceSummaryWeekly() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1, frequencyType: "weekly",
                                timePeriod: "anytime", specificTime: "09:00",
                                timesOfDay: [], daysOfWeek: [1, 3, 5], intervalDays: 0,
                                dayOfMonth: 0, monthWeekday: nil, monthOfYear: 0,
                                recurrenceEnd: nil, startDate: nil, targetCount: 0,
                                isActive: true, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        let summary = recurrenceSummary(sch)
        XCTAssertTrue(summary.contains("Every Mon, Wed, Fri"))
        XCTAssertTrue(summary.contains("9:00 AM"))
    }

    func testRecurrenceSummaryWithEnd() {
        let df = DateFormatter()
        df.dateFormat = "yyyy-MM-dd"
        let end = df.date(from: "2026-07-04")!
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1, frequencyType: "daily",
                                timePeriod: "anytime", specificTime: nil,
                                timesOfDay: [], daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 0, monthWeekday: nil, monthOfYear: 0,
                                recurrenceEnd: end, startDate: nil, targetCount: 0,
                                isActive: true, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        let summary = recurrenceSummary(sch)
        XCTAssertTrue(summary.contains("until Jul 4"))
    }

    func testRecurrenceSummaryEveryNDays() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1, frequencyType: "every_n_days",
                                timePeriod: "anytime", specificTime: nil,
                                timesOfDay: [], daysOfWeek: [], intervalDays: 3,
                                dayOfMonth: 0, monthWeekday: nil, monthOfYear: 0,
                                recurrenceEnd: nil, startDate: nil, targetCount: 0,
                                isActive: true, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        let summary = recurrenceSummary(sch)
        XCTAssertTrue(summary.contains("Every 3 days"))
    }

    func testRecurrenceSummaryMonthlyByDate() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1, frequencyType: "monthly_by_date",
                                timePeriod: "anytime", specificTime: nil,
                                timesOfDay: [], daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 15, monthWeekday: nil, monthOfYear: 0,
                                recurrenceEnd: nil, startDate: nil, targetCount: 0,
                                isActive: true, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        let summary = recurrenceSummary(sch)
        XCTAssertTrue(summary.contains("Monthly on the 15th"))
    }

    // MARK: - Week start

    func testWeekStartForMonday() {
        let result = weekStart(from: "2026-06-01") // Monday
        XCTAssertEqual(result, "2026-06-01")
    }

    func testWeekStartForSunday() {
        let result = weekStart(from: "2026-06-07") // Sunday
        // Sunday's Monday should be the previous day
        XCTAssertEqual(result, "2026-06-01")
    }
}
