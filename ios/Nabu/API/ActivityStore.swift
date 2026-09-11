import Foundation

@MainActor
final class ActivityStore {
    let api: APIClient
    private let owner: ClientIdentity.Snapshot

    init(api: APIClient) {
        self.api = api.scoped()
        self.owner = api.requestContext
    }

    func loadActivity(query: String, before: String? = nil) async throws -> HistoryResponse {
        var items: [URLQueryItem] = []
        if !query.isEmpty { items.append(URLQueryItem(name: "q", value: query)) }
        else if let before { items.append(URLQueryItem(name: "before", value: before)) }
        return try await api.get("/api/logs/history", query: items)
    }

    func loadHistory() async throws -> HistoryResponse {
        try await loadActivity(query: "")
    }

    func loadMoreHistory(before: String) async throws -> HistoryResponse {
        try await loadActivity(query: "", before: before)
    }

    /// Flat text search across note/title — spans all history, capped and
    /// newest-first on the server, bypassing the windowed pagination.
    func searchHistory(query: String) async throws -> HistoryResponse {
        try await loadActivity(query: query)
    }

    func loadToday(date: String) async throws -> TodayResponse {
        try await api.get("/api/logs/today", query: [URLQueryItem(name: "date", value: date)])
    }

    // MARK: - Day notes

    /// Loads the household's day notes (server default: last 90 days) as a
    /// date → note map, mirroring the PWA's `state.dayNotes`.
    func loadDayNotes() async throws -> [String: String] {
        let data: DayNotesResponse = try await api.get("/api/day-notes")
        return Dictionary(uniqueKeysWithValues: data.notes.map { ($0.date, $0.note) })
    }

    /// Sets (or clears, when empty) the shared note for a date.
    func setDayNote(date: String, note: String) async throws -> DayNote {
        let data: DayNoteResponse = try await api.put("/api/day-notes/\(date)", body: SetDayNoteRequest(note: note))
        return data.note
    }

    // MARK: - CSV export

    /// A complete, bounded CSV, with inclusive calendar-date filters.
    func exportLogsCSV(start: String? = nil, end: String? = nil) async throws -> URL {
        try await exportCSV(path: "/api/logs/export", prefix: "nabu-logs", start: start, end: end)
    }

    func exportHouseholdCSV(start: String? = nil, end: String? = nil) async throws -> URL {
        try await exportCSV(path: "/api/household/data", prefix: "nabu-household-data", start: start, end: end)
    }

    private func exportCSV(path: String, prefix: String, start: String?, end: String?) async throws -> URL {
        let first = start ?? "0001-01-01", last = end ?? "9999-12-31"
        guard first <= last else {
            throw APIError.serverError(statusCode: 400, message: "Choose an end date on or after the start date.")
        }
        let data = try await api.getData(path, query: [URLQueryItem(name: "start", value: first), URLQueryItem(name: "end", value: last)], expectedContentType: "text/csv")
        try Task.checkCancellation()
        guard api.identity.isCurrent(owner) else { throw APIError.contextChanged }
        guard !data.isEmpty, data.count <= 16 * 1024 * 1024 else {
            throw APIError.serverError(statusCode: 413, message: "The export is too large. Choose a smaller date range.")
        }
        let url = FileManager.default.temporaryDirectory.appendingPathComponent("\(prefix)-\(UUID().uuidString).csv")
        try data.write(to: url, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        return url
    }

}

/// One owner for entry, search, refresh, edit dismissal and pagination. Query
/// bindings invalidate synchronously, before a debounced request is scheduled.
@MainActor
final class ActivityModel: ObservableObject {
    let store: ActivityStore
    private var state: AppState?
    private var revision: UUID?
    @Published var logs: [ChoreLog] = []
    @Published var searchResults: [ChoreLog]?
    @Published private(set) var query = ""
    @Published private(set) var hasMore = false
    @Published private(set) var before: String?
    @Published private(set) var isLoading = false
    @Published private(set) var isLoadingMore = false
    @Published var errorMessage: String?

    init(store: ActivityStore) { self.store = store }
    func configure(state: AppState) {
        guard self.state == nil else { return }
        self.state = state
        revision = state.revision
    }
    private var current: Bool {
        state?.revision == revision && state != nil && store.api.identity.isCurrent(store.api.requestContext)
    }
    private var normalizedQuery: String { query.trimmingCharacters(in: .whitespaces) }

    func selectQuery(_ value: String) {
        guard current, query != value else { return }
        query = value
        _ = state?.beginOperation("activity")
        isLoading = true
        isLoadingMore = false
        errorMessage = nil
        searchResults = normalizedQuery.isEmpty ? nil : []
    }

    func confirmDeletion(_ id: Int) {
        guard current else { return }
        _ = state?.beginOperation("activity")
        isLoading = false
        isLoadingMore = false
        logs.removeAll { $0.id == id }
        searchResults?.removeAll { $0.id == id }
        state?.todayLogs.removeAll { $0.id == id }
    }

    func search() async {
        guard current, let state else { return }
        let owner = state.beginOperation("activity")
        let selected = normalizedQuery
        isLoading = true
        isLoadingMore = false
        errorMessage = nil
        if !selected.isEmpty { try? await Task.sleep(nanoseconds: 300_000_000) }
        guard current, state.owns(owner), selected == normalizedQuery, !Task.isCancelled else { return }
        await refresh()
    }

    func refresh() async {
        guard current, let state else { return }
        let owner = state.beginOperation("activity")
        let selected = normalizedQuery
        isLoading = true
        isLoadingMore = false
        errorMessage = nil
        defer { if current && state.owns(owner) { isLoading = false } }
        do {
            let data = try await store.loadActivity(query: selected)
            guard current, state.owns(owner), selected == normalizedQuery, !Task.isCancelled else { return }
            if selected.isEmpty {
                logs = data.logs
                hasMore = data.hasMore
                before = data.start
                searchResults = nil
                if let notes = try? await store.loadDayNotes(), current, state.owns(owner), !Task.isCancelled {
                    state.dayNotes = notes
                }
            } else { searchResults = data.logs }
        } catch {
            guard current, state.owns(owner), selected == normalizedQuery, !Task.isCancelled else { return }
            errorMessage = "Could not load Activity. Retry when connected."
        }
    }

    func loadMore() async {
        guard current, let state, let cursor = before, hasMore,
              !isLoading, !isLoadingMore, normalizedQuery.isEmpty else { return }
        let owner = state.beginOperation("activity")
        isLoadingMore = true
        errorMessage = nil
        defer { if current && state.owns(owner) { isLoadingMore = false } }
        do {
            let data = try await store.loadActivity(query: "", before: cursor)
            guard current, state.owns(owner), normalizedQuery.isEmpty, before == cursor, !Task.isCancelled else { return }
            var existing = Set(logs.map(\.id))
            logs.append(contentsOf: data.logs.filter { existing.insert($0.id).inserted })
            hasMore = data.hasMore
            before = data.start
        } catch {
            guard current, state.owns(owner), normalizedQuery.isEmpty, !Task.isCancelled else { return }
            errorMessage = "Could not load more Activity. Retry when connected."
        }
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

func fmtTime(_ date: Date) -> String {
    let f = DateFormatter()
    f.dateFormat = "HH:mm"
    return f.string(from: date)
}

// MARK: - Activity filtering

/// Filters history logs by a set of selected chore IDs. An empty selection
/// means "no filter" — every log is returned. This mirrors the PWA activity
/// filter's additive semantics: nothing selected shows all activity, and each
/// selected chore adds its logs to the view.
func filterLogsByChores(_ logs: [ChoreLog], selected: Set<Int>) -> [ChoreLog] {
    guard !selected.isEmpty else { return logs }
    return logs.filter { selected.contains($0.choreId) }
}
