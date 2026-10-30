import Foundation

/// Stats section registry and layout resolution, ported from the PWA
/// (`web/static/js/stats.js`). Must stay in sync with the canonical list in
/// `internal/userprefs/sections.go` — when a section is added there, append
/// it to the END of `all` here too.
enum StatsSections {
    /// Canonical section list and default order.
    static let all = [
        "overview",
        "last-done",
        "baby",
        "activity",
        "busy-hours",
        "leaderboard",
        "top-chores",
        "categories",
        "chores",
        "recap",
    ]

    static let labels: [String: String] = [
        "overview": "Overview cards",
        "last-done": "Last done",
        "baby": "Baby care",
        "activity": "Activity (heatmap)",
        "busy-hours": "Busy hours",
        "leaderboard": "Leaderboard",
        "top-chores": "Top chores",
        "categories": "Categories",
        "chores": "Chores",
        "recap": "Weekly recap",
    ]

    // Dynamic per-entity section keys. A section key is either a static
    // canonical key (above), a per-chore analytics section "chore:<id>", or
    // a user-defined widget "widget:<uuid>".

    static func choreSectionKey(_ id: Int) -> String { "chore:\(id)" }
    static func widgetSectionKey(_ id: String) -> String { "widget:\(id)" }

    static func isChoreSectionKey(_ key: String) -> Bool {
        guard key.hasPrefix("chore:") else { return false }
        let rest = key.dropFirst("chore:".count)
        return !rest.isEmpty && rest.allSatisfy(\.isNumber)
    }

    static func isWidgetSectionKey(_ key: String) -> Bool {
        guard key.hasPrefix("widget:") else { return false }
        let rest = key.dropFirst("widget:".count)
        guard !rest.isEmpty, rest.count <= 64 else { return false }
        return rest.allSatisfy { c in
            c.isASCII && (c.isLetter || c.isNumber || c == "_" || c == "-")
        }
    }

    static func isDynamicSectionKey(_ key: String) -> Bool {
        isChoreSectionKey(key) || isWidgetSectionKey(key)
    }

    /// The chore id from a "chore:<id>" key, or nil.
    static func choreId(fromSectionKey key: String) -> Int? {
        guard isChoreSectionKey(key) else { return nil }
        return Int(key.dropFirst("chore:".count))
    }

    enum BabyKind { case feed, change }

    /// Whether an activity is a baby's feeds or changes, by the template it
    /// started from: a pack's "Feed Ada", or the household's Feed Baby,
    /// however either was renamed. One with no template counts when it is
    /// called Feed Baby or Change Baby, as Stats always read it (PWA
    /// `babyKind`).
    static func babyKind(_ chore: Chore) -> BabyKind? {
        switch chore.template ?? chore.name {
        case "Feed Baby": return .feed
        case "Change Baby": return .change
        default: return nil
        }
    }

    /// One baby's Stats card: their feeds and changes. The household's Feed
    /// Baby and Change Baby share one with no name.
    struct BabyCard: Equatable {
        let name: String
        var feed: Chore?
        var change: Chore?

        var id: Int { (feed ?? change)?.id ?? 0 }
    }

    /// The baby activities on one card per baby, in the activities' order
    /// (PWA `babyCards`).
    static func babyCards(_ chores: [Chore]) -> [BabyCard] {
        var cards: [BabyCard] = []
        for chore in chores {
            guard let kind = babyKind(chore) else { continue }
            let open = cards.firstIndex {
                $0.name == chore.namedFor && (kind == .feed ? $0.feed : $0.change) == nil
            }
            var card = open.map { cards[$0] } ?? BabyCard(name: chore.namedFor)
            if kind == .feed { card.feed = chore } else { card.change = chore }
            if let open { cards[open] = card } else { cards.append(card) }
        }
        return cards
    }

    /// Whether a chore is rich enough to warrant its own generalized
    /// analytics section: it tracks a metric or has indicator labels. A
    /// baby's feeds and changes are excluded because the "baby" section
    /// already renders them.
    static func choreHasAnalytics(_ chore: Chore) -> Bool {
        if babyKind(chore) != nil { return false }
        let hasMetric = !chore.metricType.isEmpty && chore.metricType != "none"
        return hasMetric || !chore.indicatorLabels.isEmpty
    }

    /// Ordered list of per-chore section keys that should be auto-available
    /// on the stats page for the given chores.
    static func eligibleChoreSectionKeys(_ chores: [Chore]) -> [String] {
        chores.filter(choreHasAnalytics).map { choreSectionKey($0.id) }
    }

    /// Merges the user's stored order with the canonical registry plus any
    /// dynamic keys (per-chore/widget). Static keys not present in the user's
    /// order are appended; then eligible dynamic keys are appended so
    /// newly-configured chores/widgets appear automatically. Hidden sections
    /// are excluded. Stored dynamic keys that are no longer eligible are
    /// dropped.
    static func resolveLayout(userOrder: [String], userHidden: [String], dynamicKeys: [String] = []) -> [String] {
        let hidden = Set(userHidden)
        let dynamic = Set(dynamicKeys)
        func valid(_ k: String) -> Bool { all.contains(k) || dynamic.contains(k) }
        var seen = Set<String>()
        var out: [String] = []
        for k in userOrder where valid(k) && !hidden.contains(k) && !seen.contains(k) {
            out.append(k); seen.insert(k)
        }
        for k in all where !seen.contains(k) && !hidden.contains(k) {
            out.append(k); seen.insert(k)
        }
        for k in dynamicKeys where !seen.contains(k) && !hidden.contains(k) {
            out.append(k); seen.insert(k)
        }
        return out
    }

    /// Display label for a section key, including the dynamic per-chore
    /// ("chore:<id>") and widget ("widget:<uuid>") keys.
    static func label(for key: String, chores: [Chore], widgets: [StatsWidget]) -> String {
        if let l = labels[key] { return l }
        if let id = choreId(fromSectionKey: key) {
            if let c = chores.first(where: { $0.id == id }) {
                return "\(c.icon) \(c.name)"
            }
            return "Chore"
        }
        if isWidgetSectionKey(key) {
            let id = String(key.dropFirst("widget:".count))
            if let w = widgets.first(where: { $0.id == id }) {
                return w.title.isEmpty ? "Widget" : w.title
            }
            return "Widget"
        }
        return key
    }

    /// Maps a per-chore section's day/week/month period to the time-series
    /// grain the endpoint understands (daily/weekly/monthly buckets).
    static func choreAnalyticsGrain(_ period: String) -> String {
        switch period {
        case "week": return "weekly"
        case "month": return "monthly"
        default: return "daily"
        }
    }

    /// The time-series grain a widget's data is fetched at, derived from its
    /// period so month/all pull enough history: day/week use the daily series
    /// (14 days), month/all use the monthly series (6 months).
    static func widgetGrain(_ widget: StatsWidget) -> String {
        (widget.period == "month" || widget.period == "all") ? "monthly" : "daily"
    }
}
