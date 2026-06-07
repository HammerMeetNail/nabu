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
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
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
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
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
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
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
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
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
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
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
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
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

    // MARK: - isActiveForDay

    func testDailyScheduleAlwaysActive() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1,
                                frequencyType: "daily", timePeriod: "anytime",
                                specificTime: nil, timesOfDay: [],
                                daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 0, monthWeekday: nil,
                                monthOfYear: 0, recurrenceEnd: nil,
                                startDate: nil, targetCount: 0,
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        XCTAssertTrue(isActiveForDay(sch, "2026-04-28"))
        XCTAssertTrue(isActiveForDay(sch, "2026-01-01"))
    }

    func testInactiveScheduleReturnsFalse() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1,
                                frequencyType: "daily", timePeriod: "anytime",
                                specificTime: nil, timesOfDay: [],
                                daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 0, monthWeekday: nil,
                                monthOfYear: 0, recurrenceEnd: nil,
                                startDate: nil, targetCount: 0,
                                isActive: false,
                                isFollowUp: false, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        XCTAssertFalse(isActiveForDay(sch, "2026-04-28"))
    }

    func testWeeklyScheduleMatchesCorrectDays() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1,
                                frequencyType: "weekly", timePeriod: "anytime",
                                specificTime: nil, timesOfDay: [],
                                daysOfWeek: [1, 3], intervalDays: 0,
                                dayOfMonth: 0, monthWeekday: nil,
                                monthOfYear: 0, recurrenceEnd: nil,
                                startDate: nil, targetCount: 0,
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        XCTAssertTrue(isActiveForDay(sch, "2026-04-27"))
        XCTAssertFalse(isActiveForDay(sch, "2026-04-28"))
        XCTAssertTrue(isActiveForDay(sch, "2026-04-29"))
    }

    func testEveryNDaysSchedule() {
        let df = ISO8601DateFormatter()
        df.formatOptions = [.withFullDate]
        let createdAt = df.date(from: "2026-04-01T00:00:00Z")!
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1,
                                frequencyType: "every_n_days", timePeriod: "anytime",
                                specificTime: nil, timesOfDay: [],
                                daysOfWeek: [], intervalDays: 3,
                                dayOfMonth: 0, monthWeekday: nil,
                                monthOfYear: 0, recurrenceEnd: nil,
                                startDate: nil, targetCount: 0,
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
                                createdAt: createdAt, updatedAt: Date())
        XCTAssertTrue(isActiveForDay(sch, "2026-04-01"))
        XCTAssertFalse(isActiveForDay(sch, "2026-04-02"))
        XCTAssertTrue(isActiveForDay(sch, "2026-04-04"))
    }

    func testMonthlyByDateSchedule() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1,
                                frequencyType: "monthly_by_date", timePeriod: "anytime",
                                specificTime: nil, timesOfDay: [],
                                daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 15, monthWeekday: nil,
                                monthOfYear: 0, recurrenceEnd: nil,
                                startDate: nil, targetCount: 0,
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        XCTAssertTrue(isActiveForDay(sch, "2026-04-15"))
        XCTAssertFalse(isActiveForDay(sch, "2026-04-16"))
        XCTAssertTrue(isActiveForDay(sch, "2026-05-15"))
    }

    func testMonthlyByWeekdaySecondMonday() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1,
                                frequencyType: "monthly_by_weekday", timePeriod: "anytime",
                                specificTime: nil, timesOfDay: [],
                                daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 0,
                                monthWeekday: MonthWeekday(week: 2, day: 1),
                                monthOfYear: 0, recurrenceEnd: nil,
                                startDate: nil, targetCount: 0,
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        XCTAssertTrue(isActiveForDay(sch, "2026-04-13"))
        XCTAssertFalse(isActiveForDay(sch, "2026-04-06"))
        XCTAssertFalse(isActiveForDay(sch, "2026-04-20"))
    }

    func testYearlySchedule() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1,
                                frequencyType: "yearly", timePeriod: "anytime",
                                specificTime: nil, timesOfDay: [],
                                daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 28, monthWeekday: nil,
                                monthOfYear: 4, recurrenceEnd: nil,
                                startDate: nil, targetCount: 0,
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        XCTAssertTrue(isActiveForDay(sch, "2026-04-28"))
        XCTAssertFalse(isActiveForDay(sch, "2026-04-29"))
        XCTAssertTrue(isActiveForDay(sch, "2027-04-28"))
    }

    func testRespectsRecurrenceEnd() {
        let df = DateFormatter()
        df.dateFormat = "yyyy-MM-dd"
        let end = df.date(from: "2026-04-30")!
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1,
                                frequencyType: "daily", timePeriod: "anytime",
                                specificTime: nil, timesOfDay: [],
                                daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 0, monthWeekday: nil,
                                monthOfYear: 0, recurrenceEnd: end,
                                startDate: nil, targetCount: 0,
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        XCTAssertTrue(isActiveForDay(sch, "2026-04-28"))
        XCTAssertFalse(isActiveForDay(sch, "2026-05-01"))
    }

    func testOnceScheduleActiveOnlyOnStartDate() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1,
                                frequencyType: "once", timePeriod: "anytime",
                                specificTime: nil, timesOfDay: [],
                                daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 0, monthWeekday: nil,
                                monthOfYear: 0, recurrenceEnd: nil,
                                startDate: "2026-04-30", targetCount: 0,
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        XCTAssertTrue(isActiveForDay(sch, "2026-04-30"))
        XCTAssertFalse(isActiveForDay(sch, "2026-04-29"))
        XCTAssertFalse(isActiveForDay(sch, "2026-05-01"))
    }

    func testOnceScheduleWithNoStartDateReturnsFalse() {
        let sch = ChoreSchedule(id: 1, householdId: 1, choreId: 1,
                                frequencyType: "once", timePeriod: "anytime",
                                specificTime: nil, timesOfDay: [],
                                daysOfWeek: [], intervalDays: 0,
                                dayOfMonth: 0, monthWeekday: nil,
                                monthOfYear: 0, recurrenceEnd: nil,
                                startDate: nil, targetCount: 0,
                                isActive: true,
                                isFollowUp: false, assignedUserId: nil,
                                createdAt: Date(), updatedAt: Date())
        XCTAssertFalse(isActiveForDay(sch, "2026-04-30"))
    }
}
