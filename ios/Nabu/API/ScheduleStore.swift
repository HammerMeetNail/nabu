import Foundation

@MainActor
final class ScheduleStore {
    let api: APIClient

    init(api: APIClient) {
        self.api = api
    }

    func loadSchedules() async throws -> [ChoreSchedule] {
        let data: SchedulesResponse = try await api.get("/api/schedules")
        return data.schedules
    }

    func createSchedule(body: CreateScheduleRequest) async throws -> ScheduleResponse {
        try await api.post("/api/schedules", body: body)
    }

    func updateSchedule(id: Int, body: PatchScheduleRequest) async throws -> ScheduleResponse {
        try await api.patch("/api/schedules/\(id)", body: body)
    }

    func deleteSchedule(id: Int) async throws -> StatusResponse {
        try await api.delete("/api/schedules/\(id)")
    }
}

// MARK: - Frequency helpers

enum FreqType: String, CaseIterable {
    case once, daily, weekly, everyNDays = "every_n_days"
    case monthlyByDate = "monthly_by_date"
    case monthlyByWeekday = "monthly_by_weekday"
    case yearly

    var label: String {
        switch self {
        case .once: return "Does not repeat"
        case .daily: return "Every day"
        case .weekly: return "Weekly"
        case .everyNDays: return "Every N days"
        case .monthlyByDate: return "Monthly (same date)"
        case .monthlyByWeekday: return "Monthly (same weekday)"
        case .yearly: return "Every year"
        }
    }
}

let DAY_NAMES_SHORT = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]

func recurrenceSummary(_ sch: ChoreSchedule) -> String {
    let freq = sch.frequencyType
    let time = sch.specificTime.map { fmtScheduleTime($0) } ?? ""

    var parts: [String] = []
    switch freq {
    case "once": parts.append("Once")
    case "daily": parts.append("Every day")
    case "weekly":
        if sch.daysOfWeek.isEmpty {
            parts.append("Weekly")
        } else {
            let days = sch.daysOfWeek.sorted().map { DAY_NAMES_SHORT[$0] }
            parts.append("Every \(days.joined(separator: ", "))")
        }
    case "every_n_days": parts.append("Every \(sch.intervalDays) days")
    case "monthly_by_date": parts.append("Monthly on the \(sch.dayOfMonth)\(ordinalSuffix(sch.dayOfMonth))")
    case "monthly_by_weekday":
        if let mw = sch.monthWeekday {
            parts.append("Monthly (\(DAY_NAMES_SHORT[mw.day]) of week \(mw.week))")
        } else {
            parts.append("Monthly (same weekday)")
        }
    case "yearly": parts.append("Annually on \(sch.monthOfYear)/\(sch.dayOfMonth)")
    default: parts.append(freq)
    }

    if !time.isEmpty {
        parts.append(time)
    }
    if let end = sch.recurrenceEnd {
        let f = DateFormatter()
        f.dateFormat = "MMM d"
        parts.append("until \(f.string(from: end))")
    }

    return parts.joined(separator: " · ")
}

func fmtScheduleTime(_ hhmm: String) -> String {
    let parts = hhmm.split(separator: ":")
    guard parts.count == 2,
          let h = Int(parts[0]), let m = Int(parts[1]) else { return hhmm }
    let ampm = h >= 12 ? "PM" : "AM"
    let h12 = h % 12 == 0 ? 12 : h % 12
    return "\(h12):\(String(format: "%02d", m)) \(ampm)"
}

private func ordinalSuffix(_ n: Int) -> String {
    switch n {
    case 11, 12, 13: return "th"
    default:
        switch n % 10 {
        case 1: return "st"
        case 2: return "nd"
        case 3: return "rd"
        default: return "th"
        }
    }
}

func isActiveForDay(_ sch: ChoreSchedule, _ isoDate: String) -> Bool {
    guard sch.isActive else { return false }

    let f = DateFormatter()
    f.dateFormat = "yyyy-MM-dd"
    f.locale = Locale(identifier: "en_US_POSIX")

    guard let date = f.date(from: isoDate) else { return false }

    if let end = sch.recurrenceEnd, date > end { return false }

    let cal = Calendar(identifier: .gregorian)
    let wd = cal.component(.weekday, from: date) - 1

    switch sch.frequencyType {
    case "once":
        guard let start = sch.startDate else { return false }
        return isoDate == String(start.prefix(10))
    case "daily":
        return true
    case "weekly":
        return sch.daysOfWeek.contains(wd)
    case "every_n_days":
        guard sch.intervalDays > 0 else { return false }
        let originStr: String
        if let start = sch.startDate {
            originStr = String(start.prefix(10))
        } else {
            let df = ISO8601DateFormatter()
            df.formatOptions = [.withFullDate]
            originStr = df.string(from: sch.createdAt)
        }
        guard let origin = f.date(from: originStr) else { return false }
        let diffDays = cal.dateComponents([.day], from: origin, to: date).day ?? 0
        return diffDays >= 0 && diffDays % sch.intervalDays == 0
    case "monthly_by_date":
        return cal.component(.day, from: date) == sch.dayOfMonth
    case "monthly_by_weekday":
        guard let mw = sch.monthWeekday else { return false }
        if cal.component(.weekday, from: date) - 1 != mw.day { return false }
        let month = cal.component(.month, from: date)
        let year = cal.component(.year, from: date)
        var count = 0
        for day in 1...cal.component(.day, from: date) {
            let comps = DateComponents(year: year, month: month, day: day)
            if let d = cal.date(from: comps), cal.component(.weekday, from: d) - 1 == mw.day {
                count += 1
            }
        }
        return count == mw.week
    case "yearly":
        return cal.component(.day, from: date) == sch.dayOfMonth
            && cal.component(.month, from: date) == sch.monthOfYear
    default:
        return false
    }
}
