import Foundation

@MainActor
final class ActivityStore {
    let api: APIClient

    init(api: APIClient) {
        self.api = api
    }

    func loadHistory() async throws -> HistoryResponse {
        try await api.get("/api/logs/history")
    }

    func loadMoreHistory(before: String) async throws -> HistoryResponse {
        try await api.get("/api/logs/history", query: [URLQueryItem(name: "before", value: before)])
    }

    func loadToday(date: String) async throws -> TodayResponse {
        try await api.get("/api/logs/today", query: [URLQueryItem(name: "date", value: date)])
    }

    func loadWeek(start: String) async throws -> LogsResponse {
        try await api.get("/api/logs/week", query: [URLQueryItem(name: "start", value: start)])
    }
}

// MARK: - Date helpers

func todayISO() -> String {
    let f = DateFormatter()
    f.dateFormat = "yyyy-MM-dd"
    return f.string(from: Date())
}

func shiftISO(_ dateStr: String, by days: Int) -> String {
    let f = DateFormatter()
    f.dateFormat = "yyyy-MM-dd"
    guard let d = f.date(from: dateStr),
          let shifted = Calendar.current.date(byAdding: .day, value: days, to: d) else {
        return dateStr
    }
    return f.string(from: shifted)
}

func weekStart(from dateStr: String) -> String {
    let f = DateFormatter()
    f.dateFormat = "yyyy-MM-dd"
    guard let d = f.date(from: dateStr) else { return dateStr }
    let weekday = Calendar.current.component(.weekday, from: d)
    let daysFromMonday = weekday == 1 ? -6 : 2 - weekday
    guard let monday = Calendar.current.date(byAdding: .day, value: daysFromMonday, to: d) else {
        return dateStr
    }
    return f.string(from: monday)
}

func fmtHour(_ h: Int) -> String {
    switch h {
    case 0: return "12 AM"
    case 1...11: return "\(h) AM"
    case 12: return "12 PM"
    case 13...23: return "\(h - 12) PM"
    default: return "\(h)"
    }
}

func fmtShortDate(_ dateStr: String) -> String {
    let f = DateFormatter()
    f.dateFormat = "yyyy-MM-dd"
    guard let d = f.date(from: dateStr) else { return dateStr }
    f.dateFormat = "E, d"
    return f.string(from: d)
}

func fmtLongDate(_ dateStr: String) -> String {
    let f = DateFormatter()
    f.dateFormat = "yyyy-MM-dd"
    guard let d = f.date(from: dateStr) else { return dateStr }
    f.dateFormat = "EEEE, MMMM d"
    return f.string(from: d)
}

func fmtTime(_ date: Date) -> String {
    let f = DateFormatter()
    f.dateFormat = "HH:mm"
    return f.string(from: date)
}
