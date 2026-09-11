import Foundation

/// View model for the Stats tab. Mirrors the PWA's stats data-loading and
/// interaction semantics (`app.js` loadAllStatsData / loadWidgetData /
/// loadChoreAnalyticsData / loadBabyTimeSeries and the stats-* action
/// handlers): same endpoints, same query parameters, same caching keys, same
/// fetch caps. Presentation is native (§2.1); behavior must not diverge.
@MainActor
final class StatsModel: ObservableObject {

    /// Bounds the per-chore/per-widget time-series fan-out on a single Stats
    /// load, so a household with many metric/indicator chores can't trigger
    /// an unbounded burst of full-year-scan requests (PWA
    /// MAX_ANALYTICS_FETCHES).
    static let maxAnalyticsFetches = 15

    private(set) var api: APIClient?
    private(set) var state: AppState?
    private(set) var preferences: PreferencesDataLoader?

    @Published var isLoading = true
    @Published private(set) var loading: Set<String> = []
    @Published private(set) var errors: [String: String] = [:]
    private var stateRevision: UUID?
    private var requests: [String: UUID] = [:]
    private var queries: [String: String] = [:]
    private var pageRequest = UUID()

    // Overview
    @Published var overview: StatsOverview?

    // Heatmap
    @Published var heatmap: [HeatmapEntry] = []

    // Busy hours (+ filters)
    @Published var busyHours: [BusyHour] = []
    @Published var busyHoursStart = ""
    @Published var busyHoursEnd = ""
    @Published var bhChoreId: Int?
    @Published var bhUserId: Int?
    @Published var bhFilterStart = ""
    @Published var bhFilterEnd = ""

    // Leaderboard
    @Published var leaderboardPeriod = "week"
    @Published var leaderboardByPeriod: [String: LeaderboardResponse] = [:]

    // Top chores
    @Published var topChoresPeriod = "month"
    @Published var topChoresUserId: Int = 0
    @Published var topChoresByUserAndPeriod: [String: [TopChore]] = [:]

    // Categories (breakdown endpoint, period-scoped — #84/#85 convergence)
    @Published var categoriesPeriod = "week"
    @Published var categoriesBreakdown: [BreakdownEntry] = []

    // Chores section (period-scoped)
    @Published var choreStatsPeriod = "month"
    @Published var choreStats: [ChoreStat] = []
    @Published var choreStatsStart = ""
    @Published var choreStatsEnd = ""

    // Baby care
    @Published var feedBabyPeriod = "daily"
    @Published var changeBabyPeriod = "daily"
    @Published var feedBabyTS: ChoreTimeSeries?
    @Published var changeBabyTS: ChoreTimeSeries?

    // Feeding gaps (cluster feeding)
    @Published var feedingGaps: [FeedingGap] = []
    @Published var feedingGapsStart = ""
    @Published var feedingGapsEnd = ""
    @Published var feedingGapsExplainerVisible = false

    // Generalized per-chore analytics (chore:<id> sections)
    @Published var choreTimeSeries: [Int: ChoreTimeSeries] = [:]
    @Published var choreAnalyticsPeriod: [Int: String] = [:]

    // Widget data, keyed by widget id. Each entry is the per-chore results
    // the widget renders from (time-series or summary, by widget type).
    @Published var widgetTimeSeries: [String: [ChoreTimeSeries]] = [:]
    @Published var widgetSummaries: [String: [ChoreSummary]] = [:]

    // Customize panel
    @Published var customizeOpen = false
    @Published var widgetWizardOpen = false

    func configure(api: APIClient, state: AppState) {
        // A retained view model must never adopt a later account implicitly.
        guard self.api == nil else { return }
        self.api = api.scoped()
        self.state = state
        stateRevision = state.revision
        self.preferences = PreferencesDataLoader(api: api.scoped(), state: state)
    }

    private var contextIsCurrent: Bool {
        guard let api, let state else { return false }
        return stateRevision == state.revision && api.identity.isCurrent(api.requestContext)
    }

    private func visible(_ section: String) -> Bool {
        section.isEmpty || !(state?.statsSectionHidden ?? []).contains(section)
    }

    /// Initial load, refresh, retry and controls all acquire the same resource
    /// owner. Data from an older selection cannot publish or clear a newer
    /// request's loading/error state. Failed refreshes retain same-query data.
    private func read<T>(
        _ key: String, section: String, query: String,
        matches: () -> Bool = { true }, clear: () -> Void = {},
        fetch: (APIClient) async throws -> T, publish: (T) -> Void
    ) async {
        guard contextIsCurrent, visible(section), matches(), let api else { return }
        let request = UUID()
        requests[key] = request
        if queries[key] != query { clear() }
        queries[key] = query
        loading.insert(key)
        errors[key] = nil
        func owns() -> Bool { contextIsCurrent && requests[key] == request }
        defer { if owns() { loading.remove(key) } }
        do {
            let data = try await fetch(api)
            guard owns(), visible(section), matches(), !Task.isCancelled else { return }
            publish(data)
        } catch {
            guard owns(), visible(section), matches(), !Task.isCancelled else { return }
            errors[key] = "Could not load this chart. Retry to refresh."
        }
    }

    // MARK: - Derived

    var chores: [Chore] { state?.chores ?? [] }

    var feedBabyChore: Chore? { chores.first { $0.name == "Feed Baby" } }
    var changeBabyChore: Chore? { chores.first { $0.name == "Change Baby" } }

    /// The ordered, visible section keys for the current preferences +
    /// eligible dynamic sections.
    var sectionLayout: [String] {
        let dynamicKeys = StatsSections.eligibleChoreSectionKeys(chores)
            + (state?.statsWidgets ?? []).map { StatsSections.widgetSectionKey($0.id) }
        return StatsSections.resolveLayout(
            userOrder: state?.statsSectionOrder ?? [],
            userHidden: state?.statsSectionHidden ?? [],
            dynamicKeys: dynamicKeys
        )
    }

    /// All section keys (visible and hidden) in customize-panel order.
    var customizeKeys: [String] {
        let dynamicKeys = StatsSections.eligibleChoreSectionKeys(chores)
            + (state?.statsWidgets ?? []).map { StatsSections.widgetSectionKey($0.id) }
        return StatsSections.resolveLayout(
            userOrder: state?.statsSectionOrder ?? [],
            userHidden: [],
            dynamicKeys: dynamicKeys
        )
    }

    var currentLeaderboard: [LeaderboardEntry] {
        leaderboardByPeriod[leaderboardPeriod]?.leaderboard ?? []
    }

    var currentTopChores: [TopChore] {
        topChoresByUserAndPeriod["\(topChoresUserId)-\(topChoresPeriod)"] ?? []
    }

    // MARK: - Load

    /// `showSpinner: false` refreshes in place (pull-to-refresh) without
    /// flipping the whole tab back to the loading state.
    func loadAll(showSpinner: Bool = true) async {
        guard contextIsCurrent else { return }
        let request = UUID()
        pageRequest = request
        if showSpinner { isLoading = true }
        if topChoresUserId == 0 { topChoresUserId = state?.user?.id ?? 0 }

        await withTaskGroup(of: Void.self) { group in
            group.addTask { await self.loadOverview() }
            group.addTask { await self.loadHeatmap() }
            group.addTask { await self.loadBusyHours() }
            group.addTask { await self.loadChoreStats() }
            group.addTask { await self.loadCategories() }
            group.addTask { await self.loadTopChores() }
            group.addTask { await self.loadLeaderboard() }
            group.addTask { await self.loadBabyTimeSeries() }
            group.addTask { await self.loadFeedingGaps() }
            group.addTask { await self.loadChoreAnalytics() }
            group.addTask { await self.loadWidgetData() }
            await group.waitForAll()
        }

        if contextIsCurrent, pageRequest == request { isLoading = false }
    }

    private func loadOverview() async {
        await read("overview", section: "", query: "overview", fetch: { api in
            try await api.get("/api/stats/overview") as OverviewResponse
        }) { data in
            overview = data.overview
        }
    }

    private func loadHeatmap() async {
        await read("activity", section: "activity", query: "heatmap", fetch: { api in
            try await api.get("/api/stats/heatmap") as HeatmapResponse
        }) { data in
            heatmap = data.heatmap
        }
    }

    func loadBusyHours() async {
        let filters = [bhChoreId.map(String.init) ?? "", bhUserId.map(String.init) ?? "", bhFilterStart, bhFilterEnd]
        var query: [URLQueryItem] = []
        if let cid = bhChoreId { query.append(URLQueryItem(name: "choreId", value: "\(cid)")) }
        if let uid = bhUserId { query.append(URLQueryItem(name: "userId", value: "\(uid)")) }
        if !bhFilterStart.isEmpty { query.append(URLQueryItem(name: "start", value: bhFilterStart)) }
        if !bhFilterEnd.isEmpty { query.append(URLQueryItem(name: "end", value: bhFilterEnd)) }
        await read("busy-hours", section: "busy-hours", query: filters.joined(separator: "|"), matches: {
            filters == [bhChoreId.map(String.init) ?? "", bhUserId.map(String.init) ?? "", bhFilterStart, bhFilterEnd]
        }, clear: {
            busyHours = []; busyHoursStart = ""; busyHoursEnd = ""
        }, fetch: { api in
            try await api.get("/api/stats/busy-hours", query: query) as BusyHoursResponse
        }) { data in
            busyHours = data.busyHours
            busyHoursStart = data.start
            busyHoursEnd = data.end
        }
    }

    private func loadChoreStats() async {
        let period = choreStatsPeriod
        await read("chores", section: "chores", query: period, matches: { choreStatsPeriod == period }, clear: {
            choreStats = []; choreStatsStart = ""; choreStatsEnd = ""
        }, fetch: { api in
            try await api.get("/api/stats/chores", query: [URLQueryItem(name: "period", value: period)]) as ChoreStatsResponse
        }) { data in
            choreStats = data.choreStats
            choreStatsStart = data.start
            choreStatsEnd = data.end
        }
    }

    private func loadCategories() async {
        let period = categoriesPeriod
        await read("categories", section: "categories", query: period, matches: { categoriesPeriod == period }, clear: {
            categoriesBreakdown = []
        }, fetch: { api in
            try await api.get("/api/stats/breakdown", query: [URLQueryItem(name: "period", value: period)]) as BreakdownResponse
        }) { data in
            categoriesBreakdown = data.breakdown
        }
    }

    private func loadTopChores() async {
        let key = "\(topChoresUserId)-\(topChoresPeriod)"
        var query = [URLQueryItem(name: "period", value: topChoresPeriod)]
        if topChoresUserId != 0 {
            query.insert(URLQueryItem(name: "userId", value: "\(topChoresUserId)"), at: 0)
        }
        await read("top-chores", section: "top-chores", query: key, matches: {
            key == "\(topChoresUserId)-\(topChoresPeriod)"
        }, fetch: { api in
            try await api.get("/api/stats/top-chores", query: query) as TopChoresResponse
        }) { data in
            topChoresByUserAndPeriod[key] = data.topChores
        }
    }

    private func loadLeaderboard() async {
        let period = leaderboardPeriod
        await read("leaderboard", section: "leaderboard", query: period, matches: { leaderboardPeriod == period }, fetch: { api in
            try await api.get("/api/stats/leaderboard", query: [URLQueryItem(name: "period", value: period)]) as LeaderboardResponse
        }) { data in
            leaderboardByPeriod[period] = data
        }
    }

    func loadBabyTimeSeries() async {
        await loadBaby(type: "feed")
        await loadBaby(type: "change")
    }

    private func loadBaby(type: String) async {
        let feed = type == "feed"
        guard let chore = feed ? feedBabyChore : changeBabyChore else { return }
        let period = feed ? feedBabyPeriod : changeBabyPeriod
        await read("baby:\(type)", section: "baby", query: "\(chore.id)|\(period)", matches: {
            (feed ? feedBabyPeriod : changeBabyPeriod) == period && (feed ? feedBabyChore : changeBabyChore)?.id == chore.id
        }, clear: {
            if feed { feedBabyTS = nil } else { changeBabyTS = nil }
        }, fetch: { api in
            try await api.get("/api/stats/chores/\(chore.id)/time-series", query: [URLQueryItem(name: "period", value: period)]) as TimeSeriesResponse
        }) { data in
            if feed { feedBabyTS = data.timeSeries } else { changeBabyTS = data.timeSeries }
        }
    }

    /// Loads the cluster-feeding gap scatter. The stored end date is
    /// inclusive (what the pickers show); the API gets an exclusive end one
    /// day later (PWA `apiExclusiveEnd`). Defaults to the last 7 days.
    func loadFeedingGaps() async {
        guard contextIsCurrent, visible("baby"), let chore = feedBabyChore else { return }
        let today = Date()
        if feedingGapsEnd.isEmpty { feedingGapsEnd = Self.dateString(today) }
        if feedingGapsStart.isEmpty {
            feedingGapsStart = Self.dateString(Calendar.current.date(byAdding: .day, value: -6, to: today) ?? today)
        }
        let query = [
            URLQueryItem(name: "start", value: feedingGapsStart),
            URLQueryItem(name: "end", value: Self.exclusiveEnd(feedingGapsEnd)),
        ]
        let dates = [feedingGapsStart, feedingGapsEnd]
        await read("gaps", section: "baby", query: dates.joined(separator: "|"), matches: {
            dates == [feedingGapsStart, feedingGapsEnd] && feedBabyChore?.id == chore.id
        }, clear: { feedingGaps = [] }, fetch: { api in
            try await api.get("/api/stats/feeding-gaps", query: query) as FeedingGapsResponse
        }) { data in
            feedingGaps = data.feedingGaps
        }
    }

    /// Quick-range buttons on the cluster feeding column: Day / Week / 2 Weeks.
    func setFeedingGapsQuickRange(days: Int) async {
        let end = Date()
        let start = Calendar.current.date(byAdding: .day, value: -(days - 1), to: end) ?? end
        feedingGapsEnd = Self.dateString(end)
        feedingGapsStart = Self.dateString(start)
        await loadFeedingGaps()
    }

    /// Whether a quick-range button is the active one for the current dates
    /// (PWA `isQuickActive`).
    func isFeedingGapsQuickActive(days: Int) -> Bool {
        if feedingGapsStart.isEmpty || feedingGapsEnd.isEmpty { return days == 7 }
        guard let end = Self.parseDate(feedingGapsEnd) else { return false }
        let expected = Calendar.current.date(byAdding: .day, value: -(days - 1), to: end)
        return expected.map { Self.dateString($0) == feedingGapsStart } ?? false
    }

    /// Fetches daily time-series for chores with a generalized analytics
    /// section, skipping hidden sections, capped at `maxAnalyticsFetches`.
    func loadChoreAnalytics() async {
        guard contextIsCurrent else { return }
        let ids = chores.filter(StatsSections.choreHasAnalytics)
            .filter { visible(StatsSections.choreSectionKey($0.id)) }
            .prefix(Self.maxAnalyticsFetches).map(\.id)
        await withTaskGroup(of: Void.self) { group in
            for id in ids { group.addTask { await self.loadChoreAnalytics(choreId: id) } }
        }
    }

    private func loadChoreAnalytics(choreId: Int) async {
        let key = StatsSections.choreSectionKey(choreId)
        let period = choreAnalyticsPeriod[choreId] ?? "day"
        await read(key, section: key, query: period, matches: {
            (choreAnalyticsPeriod[choreId] ?? "day") == period && chores.contains { $0.id == choreId }
        }, clear: { choreTimeSeries[choreId] = nil }, fetch: { api in
            try await api.get("/api/stats/chores/\(choreId)/time-series", query: [
                URLQueryItem(name: "period", value: StatsSections.choreAnalyticsGrain(period))
            ]) as TimeSeriesResponse
        }) { data in choreTimeSeries[choreId] = data.timeSeries }
    }

    func setChoreAnalyticsPeriod(_ period: String, choreId: Int) async {
        guard contextIsCurrent, (choreAnalyticsPeriod[choreId] ?? "day") != period else { return }
        choreAnalyticsPeriod[choreId] = period
        await loadChoreAnalytics(choreId: choreId)
    }

    func loadWidgetData() async {
        guard contextIsCurrent, let api else { return }
        let widgets = (state?.statsWidgets ?? [])
            .filter { visible(StatsSections.widgetSectionKey($0.id)) }
        // Saved widgets already have the server's 20-widget limit. The 15-chore
        // analytics budget must not leave valid rendered cards without values.
        for widget in widgets { await loadData(for: widget, api: api) }
    }

    func loadData(for widget: StatsWidget, api: APIClient) async {
        // Ignore a caller's live API copy: every child uses the model's bound API.
        if widget.type == "last-done" { return }
        let key = StatsSections.widgetSectionKey(widget.id)
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let fingerprint = (try? encoder.encode(widget)) ?? Data()
        var seen = Set<Int>()
        let ids = widget.choreIds.filter { seen.insert($0).inserted }.prefix(Self.maxAnalyticsFetches)
        let matches = { [self] in
            guard let current = state?.statsWidgets.first(where: { $0.id == widget.id }) else { return false }
            return current == widget
        }
        if widget.type == "timeseries" {
            await read(key, section: key, query: fingerprint.base64EncodedString(), matches: matches,
                       clear: { widgetTimeSeries[widget.id] = nil }, fetch: { api in
                var results: [ChoreTimeSeries] = []
                for id in ids {
                    guard contextIsCurrent, matches(), visible(key), !Task.isCancelled else { throw APIError.contextChanged }
                    let data: TimeSeriesResponse = try await api.get("/api/stats/chores/\(id)/time-series", query: [
                        URLQueryItem(name: "period", value: StatsSections.widgetGrain(widget))
                    ])
                    results.append(data.timeSeries)
                }
                return results
            }) { widgetTimeSeries[widget.id] = $0 }
        } else {
            await read(key, section: key, query: fingerprint.base64EncodedString(), matches: matches,
                       clear: { widgetSummaries[widget.id] = nil }, fetch: { api in
                var results: [ChoreSummary] = []
                for id in ids {
                    guard contextIsCurrent, matches(), visible(key), !Task.isCancelled else { throw APIError.contextChanged }
                    let data: ChoreSummaryResponse = try await api.get("/api/stats/chores/\(id)/summary", query: [
                        URLQueryItem(name: "period", value: widget.period.isEmpty ? "week" : widget.period)
                    ])
                    results.append(data.summary)
                }
                return results
            }) { widgetSummaries[widget.id] = $0 }
        }
    }

    /// Every visible card keeps a retry path, including an empty first result.
    func retrySection(_ key: String) async {
        switch key {
        case "overview", "recap": await loadOverview()
        case "activity": await loadHeatmap()
        case "busy-hours": await loadBusyHours()
        case "chores": await loadChoreStats()
        case "categories": await loadCategories()
        case "leaderboard": await loadLeaderboard()
        case "top-chores": await loadTopChores()
        case "baby": await loadBabyTimeSeries(); await loadFeedingGaps()
        case "baby:feed": await loadBaby(type: "feed")
        case "baby:change": await loadBaby(type: "change")
        case "gaps": await loadFeedingGaps()
        default:
            if let id = StatsSections.choreId(fromSectionKey: key) {
                await loadChoreAnalytics(choreId: id)
            } else if let api, let widget = state?.statsWidgets.first(where: { StatsSections.widgetSectionKey($0.id) == key }) {
                await loadData(for: widget, api: api)
            }
        }
    }

    // MARK: - Period toggles

    func setLeaderboardPeriod(_ period: String) async {
        guard contextIsCurrent, period != leaderboardPeriod else { return }
        leaderboardPeriod = period
        await loadLeaderboard()
    }

    func setTopChoresPeriod(_ period: String) async {
        guard contextIsCurrent, period != topChoresPeriod else { return }
        topChoresPeriod = period
        await loadTopChores()
    }

    func setTopChoresUser(_ userId: Int) async {
        guard contextIsCurrent, userId != topChoresUserId else { return }
        topChoresUserId = userId
        await loadTopChores()
    }

    func setCategoriesPeriod(_ period: String) async {
        guard contextIsCurrent, period != categoriesPeriod else { return }
        categoriesPeriod = period
        await loadCategories()
    }

    func setChoreStatsPeriod(_ period: String) async {
        guard contextIsCurrent, period != choreStatsPeriod else { return }
        choreStatsPeriod = period
        await loadChoreStats()
    }

    func setBabyPeriod(_ period: String, type: String) async {
        if type == "feed" {
            guard contextIsCurrent, period != feedBabyPeriod else { return }
            feedBabyPeriod = period
        } else {
            guard contextIsCurrent, period != changeBabyPeriod else { return }
            changeBabyPeriod = period
        }
        await loadBaby(type: type)
    }

    // MARK: - Widgets (customize)

    /// Adds a widget from the wizard. The server assigns the id and echoes
    /// the normalized list; new widgets default to period "week" and expose
    /// a day/week/month toggle on the card.
    func addWidget(title: String, type: String, metric: String, choreIds: [Int]) async -> Bool {
        guard contextIsCurrent, let state, let preferences else { return false }
        let trimmed = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let widget = StatsWidget(
            id: "", type: type, choreIds: choreIds, metric: metric,
            agg: "", period: "week", grain: "",
            title: trimmed.isEmpty ? "Widget" : trimmed
        )
        let saved = await preferences.saveStatsWidgets(state.statsWidgets + [widget])
        guard saved, contextIsCurrent else { return false }
        await loadWidgetData()
        return true
    }

    func removeWidget(id: String) async -> Bool {
        guard contextIsCurrent, let state, let preferences else { return false }
        let widgets = state.statsWidgets.filter { $0.id != id }
        return await preferences.saveStatsWidgets(widgets)
    }

    /// Day/week/month toggle on a widget card: persists the new period into
    /// the stored widget, then refetches that widget's data.
    func setWidgetPeriod(_ period: String, widgetId: String) async -> Bool {
        guard contextIsCurrent, let api, let state, let preferences else { return false }
        guard let current = state.statsWidgets.first(where: { $0.id == widgetId }),
              current.period != period else { return true }
        let widgets = state.statsWidgets.map { w in
            w.id == widgetId
                ? StatsWidget(id: w.id, type: w.type, choreIds: w.choreIds, metric: w.metric,
                              agg: w.agg, period: period, grain: w.grain, title: w.title)
                : w
        }
        let saved = await preferences.saveStatsWidgets(widgets)
        guard saved, contextIsCurrent else { return false }
        if let updated = state.statsWidgets.first(where: { $0.id == widgetId }) {
            await loadData(for: updated, api: api)
        }
        return true
    }

    // MARK: - Sections (customize)

    /// Persists a reorder from the customize panel. Saves the full ordered
    /// key list (visible + hidden + dynamic) plus any missing canonical keys,
    /// like the PWA's drop handler.
    func moveSections(from source: IndexSet, to destination: Int) async {
        guard contextIsCurrent, let preferences else { return }
        var keys = customizeKeys
        let moved = source.sorted().map { keys[$0] }
        let insertion = destination - source.filter { $0 < destination }.count
        for index in source.sorted(by: >) { keys.remove(at: index) }
        keys.insert(contentsOf: moved, at: insertion)
        var seen = Set<String>()
        let all = (keys + StatsSections.all).filter { seen.insert($0).inserted }
        await preferences.saveStatsSectionOrder(all)
    }

    func setSectionVisible(_ key: String, visible: Bool) async {
        guard contextIsCurrent, let state, let preferences else { return }
        var hidden = state.statsSectionHidden
        if visible {
            hidden.removeAll { $0 == key }
        } else if !hidden.contains(key) {
            hidden.append(key)
        }
        if !visible {
            let keys = key == "baby" ? ["baby:feed", "baby:change", "gaps"] : [key]
            for resource in keys {
                requests[resource] = UUID()
                loading.remove(resource)
                errors[resource] = nil
            }
        }
        let saved = await preferences.saveStatsSectionHidden(hidden)
        // Newly-revealed sections may have never fetched their data.
        if saved && visible && contextIsCurrent { await retrySection(key) }
    }

    // MARK: - Date helpers

    nonisolated static func dateString(_ date: Date) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        return f.string(from: date)
    }

    nonisolated static func parseDate(_ s: String) -> Date? {
        guard !s.isEmpty else { return nil }
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        return f.date(from: s)
    }

    /// The stats date pickers show an inclusive end date; the feeding-gaps
    /// API takes an exclusive end (PWA `apiExclusiveEnd`).
    nonisolated static func exclusiveEnd(_ inclusiveEnd: String) -> String {
        guard let d = parseDate(inclusiveEnd),
              let next = Calendar.current.date(byAdding: .day, value: 1, to: d) else {
            return inclusiveEnd
        }
        return dateString(next)
    }
}
