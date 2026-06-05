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
