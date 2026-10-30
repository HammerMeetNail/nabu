import XCTest
@testable import Nabu

/// Ports the PWA's stats section-registry cases (`web/static/js/tests/runner.js`,
/// `stats.js`) so both clients resolve identical layouts from the same
/// preferences.
final class StatsSectionsTests: XCTestCase {

    // MARK: - Registry

    func testCanonicalListMatchesServerRegistry() {
        // Must match internal/userprefs/sections.go and stats.js exactly.
        XCTAssertEqual(StatsSections.all, [
            "overview", "last-done", "baby", "activity", "busy-hours",
            "leaderboard", "top-chores", "categories", "chores", "recap",
        ])
    }

    // MARK: - Dynamic keys

    func testDynamicKeyPredicates() {
        XCTAssertTrue(StatsSections.isChoreSectionKey("chore:12"))
        XCTAssertFalse(StatsSections.isChoreSectionKey("chore:"))
        XCTAssertFalse(StatsSections.isChoreSectionKey("chore:abc"))
        XCTAssertFalse(StatsSections.isChoreSectionKey("widget:12"))

        XCTAssertTrue(StatsSections.isWidgetSectionKey("widget:a1B2-_x"))
        XCTAssertFalse(StatsSections.isWidgetSectionKey("widget:"))
        XCTAssertFalse(StatsSections.isWidgetSectionKey("widget:<script>"))
        XCTAssertFalse(StatsSections.isWidgetSectionKey("widget:" + String(repeating: "a", count: 65)))

        XCTAssertEqual(StatsSections.choreSectionKey(7), "chore:7")
        XCTAssertEqual(StatsSections.widgetSectionKey("u-1"), "widget:u-1")
        XCTAssertEqual(StatsSections.choreId(fromSectionKey: "chore:42"), 42)
        XCTAssertNil(StatsSections.choreId(fromSectionKey: "widget:42"))
    }

    // MARK: - choreHasAnalytics (mirrors runner.js Phase 3 cases)

    func testChoreHasAnalytics() {
        XCTAssertTrue(StatsSections.choreHasAnalytics(makeChore(id: 1, name: "Naps", metricType: "duration")))
        XCTAssertTrue(StatsSections.choreHasAnalytics(makeChore(id: 2, name: "Meds", indicatorLabels: ["am"])))
        XCTAssertFalse(StatsSections.choreHasAnalytics(makeChore(id: 3, name: "Vacuum")))
        // Baby chores are covered by the dedicated baby section, whatever
        // their names; a pet's feeds keep their own.
        XCTAssertFalse(StatsSections.choreHasAnalytics(makeChore(id: 4, name: "Feed Baby", metricType: "amount")))
        XCTAssertFalse(StatsSections.choreHasAnalytics(makeChore(id: 5, name: "Change Baby", indicatorLabels: ["💩 poo"])))
        XCTAssertFalse(StatsSections.choreHasAnalytics(makeChore(id: 6, name: "Bottles", metricType: "amount", templateKey: "Feed Baby", namedFor: "Ada")))
        XCTAssertTrue(StatsSections.choreHasAnalytics(makeChore(id: 7, name: "Feed Mongo", metricType: "amount", templateKey: "Feed Cats", namedFor: "Mongo")))
    }

    /// One card per baby, by template, in the activities' order (runner.js
    /// "babyCards").
    func testBabyCardsGiveEachBabyACard() {
        let cards = StatsSections.babyCards([
            makeChore(id: 1, name: "Feed Ada", templateKey: "Feed Baby", namedFor: "Ada"),
            makeChore(id: 2, name: "Feed Ben", templateKey: "Feed Baby", namedFor: "Ben"),
            makeChore(id: 3, name: "Ada's diapers", templateKey: "Change Baby", namedFor: "Ada"),
            makeChore(id: 4, name: "Change Ben", templateKey: "Change Baby", namedFor: "Ben"),
            makeChore(id: 5, name: "Feed Mongo", templateKey: "Feed Cats", namedFor: "Mongo"),
            // The household's own, renamed, and one with no template, by its name.
            makeChore(id: 6, name: "Night feeds", predefinedKey: "Feed Baby"),
            makeChore(id: 7, name: "Change Baby", predefinedKey: ""),
            makeChore(id: 8, name: "Feed Baby", predefinedKey: "Feed Cats"),
        ])
        XCTAssertEqual(cards.map(\.name), ["Ada", "Ben", ""])
        XCTAssertEqual(cards.map { [$0.feed?.id, $0.change?.id] }, [[1, 3], [2, 4], [6, 7]])
        XCTAssertEqual(cards.map(\.id), [1, 2, 6])
    }

    func testEligibleChoreSectionKeys() {
        let chores = [
            makeChore(id: 1, name: "Naps", metricType: "duration"),
            makeChore(id: 2, name: "Vacuum"),
            makeChore(id: 3, name: "Meds", indicatorLabels: ["am", "pm"]),
        ]
        XCTAssertEqual(StatsSections.eligibleChoreSectionKeys(chores), ["chore:1", "chore:3"])
    }

    // MARK: - resolveLayout

    func testResolveLayoutDefaultOrder() {
        XCTAssertEqual(
            StatsSections.resolveLayout(userOrder: [], userHidden: []),
            StatsSections.all
        )
    }

    func testResolveLayoutUserOrderFirstThenAppendsMissing() {
        let out = StatsSections.resolveLayout(
            userOrder: ["recap", "overview"], userHidden: []
        )
        XCTAssertEqual(Array(out.prefix(2)), ["recap", "overview"])
        XCTAssertEqual(Set(out), Set(StatsSections.all))
        XCTAssertEqual(out.count, StatsSections.all.count)
    }

    func testResolveLayoutExcludesHiddenAndUnknownAndDuplicates() {
        let out = StatsSections.resolveLayout(
            userOrder: ["baby", "bogus", "baby", "overview"],
            userHidden: ["overview", "recap"]
        )
        XCTAssertEqual(out.first, "baby")
        XCTAssertFalse(out.contains("overview"))
        XCTAssertFalse(out.contains("recap"))
        XCTAssertFalse(out.contains("bogus"))
        XCTAssertEqual(out.filter { $0 == "baby" }.count, 1)
    }

    func testResolveLayoutAppendsEligibleDynamicKeysAndDropsStale() {
        let out = StatsSections.resolveLayout(
            userOrder: ["chore:9", "overview", "widget:gone"],
            userHidden: ["chore:3"],
            dynamicKeys: ["chore:9", "chore:3", "widget:u1"]
        )
        // Stored eligible dynamic key keeps its position; stale one is dropped.
        XCTAssertEqual(Array(out.prefix(2)), ["chore:9", "overview"])
        XCTAssertFalse(out.contains("widget:gone"))
        XCTAssertFalse(out.contains("chore:3")) // hidden
        XCTAssertEqual(out.last, "widget:u1")   // new dynamic keys appended
    }

    // MARK: - Labels

    func testSectionLabels() {
        let chores = [makeChore(id: 2, name: "Meds", indicatorLabels: ["am"])]
        let widgets = [makeWidget(id: "u1", title: "Bottles this week")]
        XCTAssertEqual(StatsSections.label(for: "last-done", chores: chores, widgets: widgets), "Last done")
        XCTAssertEqual(StatsSections.label(for: "chore:2", chores: chores, widgets: widgets), "📋 Meds")
        XCTAssertEqual(StatsSections.label(for: "chore:99", chores: chores, widgets: widgets), "Chore")
        XCTAssertEqual(StatsSections.label(for: "widget:u1", chores: chores, widgets: widgets), "Bottles this week")
        XCTAssertEqual(StatsSections.label(for: "widget:nope", chores: chores, widgets: widgets), "Widget")
    }

    // MARK: - Grains

    func testChoreAnalyticsGrain() {
        XCTAssertEqual(StatsSections.choreAnalyticsGrain("day"), "daily")
        XCTAssertEqual(StatsSections.choreAnalyticsGrain("week"), "weekly")
        XCTAssertEqual(StatsSections.choreAnalyticsGrain("month"), "monthly")
        XCTAssertEqual(StatsSections.choreAnalyticsGrain("bogus"), "daily")
    }

    func testWidgetGrain() {
        XCTAssertEqual(StatsSections.widgetGrain(makeWidget(period: "day")), "daily")
        XCTAssertEqual(StatsSections.widgetGrain(makeWidget(period: "week")), "daily")
        XCTAssertEqual(StatsSections.widgetGrain(makeWidget(period: "month")), "monthly")
        XCTAssertEqual(StatsSections.widgetGrain(makeWidget(period: "all")), "monthly")
    }
}

private func makeChore(id: Int, name: String, metricType: String = "none", indicatorLabels: [String] = [],
                       predefinedKey: String? = nil, templateKey: String = "", namedFor: String = "") -> Chore {
    Chore(
        id: id, householdId: 1, name: name, icon: "📋", color: "#000000",
        sortOrder: 0, category: "test", isPredefined: predefinedKey?.isEmpty == false,
        predefinedKey: predefinedKey, createdBy: nil, createdAt: Date(),
        indicatorLabels: indicatorLabels, indicatorDefaults: [], hasVolumeML: false,
        metricType: metricType, templateKey: templateKey, namedFor: namedFor
    )
}

func makeWidget(id: String = "w1", type: String = "total", choreIds: [Int] = [1], metric: String = "count", period: String = "week", title: String = "Widget") -> StatsWidget {
    StatsWidget(id: id, type: type, choreIds: choreIds, metric: metric, agg: "sum", period: period, grain: "daily", title: title)
}
