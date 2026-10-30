import SwiftUI

// Section card views for the Stats tab. Data/semantics mirror the PWA's
// section renderers in `stats.js`; presentation is native per plan §2.1
// (segmented pickers instead of pill bars, native date pickers, Swift Charts).

// MARK: - Shared building blocks

/// Card chrome shared by every stats section.
struct StatsCard<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 10) { content }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding()
            .background(DesignColors.surface)
            .clipShape(RoundedRectangle(cornerRadius: 16))
            .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }
}

struct StatsLoadStatus: View {
    var model: StatsModel
    let resource: String

    var body: some View {
        if model.loading.contains(resource) {
            ProgressView("Loading chart")
                .font(.caption)
        } else if let message = model.errors[resource] {
            HStack {
                Text(message).font(.caption)
                Button("Retry") { Task { await model.retrySection(resource) } }
                    .accessibilityIdentifier("stats-retry-\(resource)")
            }
            .foregroundStyle(DesignColors.textSecondary)
        }
    }
}

/// A Stats card's title and its controls: side by side, or the controls under
/// the title at accessibility sizes, where beside them a title was cut to its
/// first letter ("C…"). The PWA's headers wrap the same way at 200% zoom.
/// `truncates` keeps a long name to one line beside the controls.
struct StatsCardHeader<Controls: View>: View {
    let title: Text
    var truncates = false
    @ViewBuilder var controls: Controls
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let stacks = dynamicTypeSize.isAccessibilitySize
        let layout = stacks ? AnyLayout(VStackLayout(alignment: .leading, spacing: 8)) : AnyLayout(HStackLayout())
        layout {
            title.font(.headline).lineLimit(truncates && !stacks ? 1 : nil)
            if !stacks { Spacer() }
            controls
        }
    }
}

/// Native segmented day/week/month(/all) period control (PWA
/// `renderStatsPeriodToggle` semantics, §2.1 presentation).
struct StatsPeriodToggle: View {
    let period: String
    let includeAll: Bool
    let onChange: (String) -> Void

    private var periods: [String] { includeAll ? ["day", "week", "month", "all"] : ["day", "week", "month"] }
    private static let labels = ["day": "Day", "week": "Week", "month": "Month", "all": "All"]

    var body: some View {
        Picker("Time period", selection: Binding(
            get: { period },
            set: { onChange($0) }
        )) {
            ForEach(periods, id: \.self) { p in
                Text(Self.labels[p] ?? p).tag(p)
            }
        }
        .pickerStyle(.segmented)
        .fixedSize()
    }
}

/// Member split rows with proportional bars (PWA `renderMemberList`).
struct MemberBarList: View {
    let byMember: [(userId: Int, count: Int)]
    let members: [Member]

    var body: some View {
        if byMember.isEmpty {
            Text("No data").font(.caption).foregroundStyle(DesignColors.textSecondary)
        } else {
            let maxCount = byMember.first?.count ?? 1
            VStack(spacing: 4) {
                ForEach(byMember, id: \.userId) { entry in
                    let member = members.first { $0.userId == entry.userId }
                    let name = member.map { $0.displayName.isEmpty ? $0.email : $0.displayName } ?? "User \(entry.userId)"
                    HStack(spacing: 6) {
                        MemberAvatar(name: name, colorHex: member?.avatarColor ?? "#19323C", size: 18)
                        Text(name).font(.system(size: 11)).lineLimit(1)
                        GeometryReader { geo in
                            ZStack(alignment: .leading) {
                                RoundedRectangle(cornerRadius: 2).fill(DesignColors.surfaceSecondary)
                                RoundedRectangle(cornerRadius: 2)
                                    .fill(DesignColors.primary.opacity(0.5))
                                    .frame(width: geo.size.width * (maxCount > 0 ? CGFloat(entry.count) / CGFloat(maxCount) : 0))
                            }
                        }
                        .frame(height: 8)
                        Text("\(entry.count)")
                            .font(.system(size: 10))
                            .foregroundStyle(DesignColors.textSecondary)
                            .frame(width: 24, alignment: .trailing)
                    }
                }
            }
        }
    }
}

struct MemberAvatar: View {
    let name: String
    let colorHex: String
    let size: CGFloat

    var body: some View {
        Circle()
            .fill(Color(hexUnsafe: colorHex.trimmingCharacters(in: CharacterSet(charactersIn: "#"))))
            .frame(width: size, height: size)
            .overlay(
                Text(String(name.prefix(1)).uppercased())
                    .font(.system(size: size * 0.45, weight: .semibold))
                    .foregroundColor(.white)
            )
    }
}

/// Shared "time since last log per chore" rows (the last-done section and
/// last-done widgets). `orderedChores` controls row order.
struct LastDoneList: View {
    let orderedChores: [Chore]
    let latestLogs: [Int: ChoreLog]

    var body: some View {
        VStack(spacing: 6) {
            ForEach(orderedChores) { chore in
                HStack(spacing: 8) {
                    Text(chore.icon).font(.body)
                    Text(chore.name).font(.subheadline).lineLimit(1)
                    Spacer()
                    if let log = latestLogs[chore.id] {
                        Text(formatTimeAgo(log.completedAt))
                            .font(.caption)
                            .foregroundStyle(DesignColors.textSecondary)
                    } else {
                        Text("never")
                            .font(.caption.italic())
                            .foregroundStyle(DesignColors.textSecondary.opacity(0.7))
                    }
                }
            }
        }
    }
}

// MARK: - Last done section

struct LastDoneSection: View {
    let chores: [Chore]
    let latestLogs: [Int: ChoreLog]

    /// Most recent first; never-logged chores last (PWA
    /// `renderLastDoneSection`).
    private var ordered: [Chore] {
        chores.sorted { a, b in
            let ta = latestLogs[a.id]?.completedAt.timeIntervalSince1970 ?? 0
            let tb = latestLogs[b.id]?.completedAt.timeIntervalSince1970 ?? 0
            return ta > tb
        }
    }

    var body: some View {
        StatsCard {
            Text("Last done").font(.headline)
            if chores.isEmpty {
                Text("No chores yet")
                    .font(.subheadline).foregroundStyle(DesignColors.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .center)
            } else {
                LastDoneList(orderedChores: ordered, latestLogs: latestLogs)
            }
        }
    }
}

// MARK: - Baby care section

/// A card for each baby, named for the baby (PWA `renderBabyCareSection`).
/// The household's Feed Baby and Change Baby share one called Baby.
struct BabyCareSection: View {
    var model: StatsModel
    let members: [Member]
    let volumeUnit: String

    var body: some View {
        VStack(spacing: 16) {
            ForEach(model.babyCards, id: \.id) { card in
                VStack(spacing: 8) {
                    StatsCard {
                        Text(card.name.isEmpty ? String(localized: "Baby") : card.name).font(.headline)
                        if let chore = card.feed {
                            BabyColumn(model: model, ts: model.babyTimeSeries[chore.id] ?? emptySeries(chore), type: "feed",
                                       choreId: chore.id, members: members, volumeUnit: volumeUnit)
                        }
                        if let chore = card.change {
                            BabyColumn(model: model, ts: model.babyTimeSeries[chore.id] ?? emptySeries(chore), type: "change",
                                       choreId: chore.id, members: members, volumeUnit: volumeUnit)
                        }
                        if let feed = card.feed {
                            FeedingGapsColumn(model: model, choreId: feed.id, volumeUnit: volumeUnit)
                        }
                    }
                    ForEach(resources(card), id: \.self) { StatsLoadStatus(model: model, resource: $0) }
                }
            }
        }
    }

    private func resources(_ card: StatsSections.BabyCard) -> [String] {
        [card.feed, card.change].compactMap { $0.map { "baby:\($0.id)" } } + (card.feed.map { ["gaps:\($0.id)"] } ?? [])
    }

    private func emptySeries(_ chore: Chore) -> ChoreTimeSeries {
        ChoreTimeSeries(choreId: chore.id, choreName: chore.name, choreIcon: chore.icon,
                        metricType: chore.metricType, metricUnit: chore.metricUnit, byMember: [], periods: [])
    }
}

struct BabyColumn: View {
    var model: StatsModel
    let ts: ChoreTimeSeries
    let type: String // "feed" | "change"
    let choreId: Int
    let members: [Member]
    let volumeUnit: String
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var period: String { model.babyPeriod(choreId) } // daily | weekly | monthly

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // At accessibility sizes the picker goes under the title, which
            // may wrap, rather than squeeze it (PWA `.baby-col-header`).
            let stacks = dynamicTypeSize.isAccessibilitySize
            let header = stacks ? AnyLayout(VStackLayout(alignment: .leading, spacing: 8)) : AnyLayout(HStackLayout(spacing: 4))
            header {
                HStack(spacing: 4) {
                    Text(ts.choreIcon).font(.body)
                    Text(ts.choreName).font(.subheadline).fontWeight(.semibold).lineLimit(stacks ? nil : 1)
                }
                if !stacks { Spacer(minLength: 4) }
                Picker("Time period", selection: Binding(
                    get: { period },
                    set: { p in Task { await model.setBabyPeriod(p, choreId: choreId) } }
                )) {
                    Text("Daily").tag("daily")
                    Text("Weekly").tag("weekly")
                    Text("Monthly").tag("monthly")
                }
                .pickerStyle(.segmented)
                .fixedSize()
            }

            MemberBarList(byMember: ts.byMember.map { ($0.userId, $0.count) }, members: members)

            if ts.periods.isEmpty {
                Text("No data").font(.caption2).foregroundStyle(DesignColors.textSecondary)
            } else if type == "feed" {
                VolumePeriodChart(periods: ts.periods, grain: period, volumeUnit: volumeUnit)
                UnitAmountCharts(periods: ts.periods, fallback: "", grain: period, excludeLiquid: true)
            } else {
                IndicatorPeriodChart(periods: ts.periods, grain: period)
            }
        }
    }
}

/// Stacked volume-by-indicator bars + totals legend (PWA
/// `renderVolumeChart`).
struct VolumePeriodChart: View {
    let periods: [TimeSeriesPeriod]
    let grain: String
    let volumeUnit: String

    var body: some View {
        let keys = PeriodBarData.stackKeys(periods, volumeMode: true)
        var colors: [String: Color] = [PeriodBarData.unlabeledKey: Color(uiColor: .systemGray3)]
        let _ = keys.forEach { colors[$0] = IndicatorColor.color(for: $0) }

        VStack(alignment: .leading, spacing: 4) {
            PeriodBarChart(
                segments: PeriodBarData.volumeSegments(periods, unit: volumeUnit),
                periods: periods,
                grain: grain,
                unitLabel: volumeUnit == "oz" ? "oz" : "mL",
                seriesColors: colors,
                summaryText: { p in Self.volumeSummary(p, stackKeys: keys, unit: volumeUnit) }
            )
            ChartTotalsLegend(entries: legendEntries)
        }
    }

    /// PWA `volumeBarLabel`: per-indicator volumes plus the unlabeled remainder.
    static func volumeSummary(_ p: TimeSeriesPeriod, stackKeys: [String], unit: String) -> String {
        var parts: [String] = []
        var attributed = 0
        for key in stackKeys {
            let ml = p.volumeByIndicator?[key] ?? 0
            attributed += ml
            if ml > 0 { parts.append("\(key) \(VolumeUnits.formatVolume(ml, unit: unit))") }
        }
        let unlabeled = (p.totalML ?? 0) - attributed
        if unlabeled > 0 { parts.append("unlabeled \(VolumeUnits.formatVolume(unlabeled, unit: unit))") }
        if parts.isEmpty, let total = p.totalML, total > 0 {
            return VolumeUnits.formatVolume(total, unit: unit)
        }
        return parts.joined(separator: ", ")
    }

    /// PWA volume-chart legend: formula/breast feed counts + unlabeled volume.
    private var legendEntries: [ChartTotalsLegend.Entry] {
        var entries: [ChartTotalsLegend.Entry] = []
        let formulaTotal = periods.reduce(0) { $0 + ($1.indicators?["🍼 formula"] ?? 0) }
        let breastTotal = periods.reduce(0) { $0 + ($1.indicators?["🤱 breast"] ?? 0) }
        let unlabeledML = periods.reduce(0) { sum, p in
            sum + (p.totalML ?? 0) - (p.volumeByIndicator ?? [:]).values.reduce(0, +)
        }
        if formulaTotal > 0 {
            entries.append(.init(label: "🍼 \(formulaTotal) total", color: IndicatorColor.color(for: "🍼 formula")))
        }
        if breastTotal > 0 {
            entries.append(.init(label: "🤱 \(breastTotal) total", color: IndicatorColor.color(for: "🤱 breast")))
        }
        if unlabeledML > 0 {
            entries.append(.init(label: "unlabeled \(VolumeUnits.formatVolume(unlabeledML, unit: volumeUnit))", color: Color(uiColor: .systemGray3)))
        }
        return entries
    }
}

/// Stacked indicator-count bars + totals legend (PWA `renderIndicatorChart`).
struct IndicatorPeriodChart: View {
    let periods: [TimeSeriesPeriod]
    let grain: String

    var body: some View {
        let keys = PeriodBarData.stackKeys(periods, volumeMode: false)
        VStack(alignment: .leading, spacing: 4) {
            PeriodBarChart(
                segments: PeriodBarData.indicatorSegments(periods),
                periods: periods,
                grain: grain,
                unitLabel: "count",
                seriesColors: [:],
                summaryText: { p in Self.indicatorSummary(p, keys: keys) }
            )
            ChartTotalsLegend(entries: keys.map { key in
                let total = periods.reduce(0) { $0 + ($1.indicators?[key] ?? 0) }
                return .init(label: "\(key) \(total) total", color: IndicatorColor.color(for: key))
            })
        }
    }

    static func indicatorSummary(_ p: TimeSeriesPeriod, keys: [String]) -> String {
        keys.compactMap { key in
            guard let c = p.indicators?[key], c > 0 else { return nil }
            return "\(key) \(c)"
        }.joined(separator: ", ")
    }
}

// MARK: - Cluster feeding (feeding gaps)

struct FeedingGapsColumn: View {
    var model: StatsModel
    /// The baby's feed activity.
    let choreId: Int
    let volumeUnit: String

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var explainerVisible: Bool { model.feedingGapsExplainers.contains(choreId) }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            let stacks = dynamicTypeSize.isAccessibilitySize
            let header = stacks ? AnyLayout(VStackLayout(alignment: .leading, spacing: 8)) : AnyLayout(HStackLayout())
            // At accessibility sizes the range picker goes under the title.
            header {
                HStack {
                    Text("🕐 Cluster Feeding").font(.subheadline).fontWeight(.semibold)
                    Button {
                        withAnimation {
                            if explainerVisible {
                                model.feedingGapsExplainers.remove(choreId)
                            } else {
                                model.feedingGapsExplainers.insert(choreId)
                            }
                        }
                    } label: {
                        Image(systemName: "info.circle")
                            .font(.caption)
                            .foregroundStyle(DesignColors.textSecondary)
                    }
                    .accessibilityLabel("How to read this chart")
                }
                if !stacks { Spacer() }
                Picker("Range", selection: Binding(
                    get: {
                        [1, 7, 14].first { model.isFeedingGapsQuickActive(days: $0, choreId: choreId) } ?? 0
                    },
                    set: { days in
                        if days > 0 { Task { await model.setFeedingGapsQuickRange(days: days, choreId: choreId) } }
                    }
                )) {
                    Text("Day").tag(1)
                    Text("Week").tag(7)
                    Text("2 Weeks").tag(14)
                    if ![1, 7, 14].contains(where: { model.isFeedingGapsQuickActive(days: $0, choreId: choreId) }) {
                        Text("Custom").tag(0)
                    }
                }
                .pickerStyle(.segmented)
                .fixedSize()
            }

            StatsDateRange(start: Binding(
                get: { model.feedingGapsRange(choreId).start },
                set: {
                    model.setFeedingGapsDate($0, start: true, choreId: choreId)
                    Task { await model.loadFeedingGaps(choreId: choreId) }
                }
            ), end: Binding(
                get: { model.feedingGapsRange(choreId).end },
                set: {
                    model.setFeedingGapsDate($0, start: false, choreId: choreId)
                    Task { await model.loadFeedingGaps(choreId: choreId) }
                }
            ))

            if explainerVisible {
                FeedingGapsExplainer()
            }

            if let gaps = model.feedingGaps[choreId], !gaps.isEmpty {
                FeedingGapsScatter(gaps: gaps, volumeUnit: volumeUnit)
            } else {
                Text("No data").font(.caption).foregroundStyle(DesignColors.textSecondary)
            }
        }
    }
}

/// The "how to read this chart" explainer (PWA copy, plain text).
struct FeedingGapsExplainer: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Cluster feeding = 2+ feeds within 2 hours.").fontWeight(.semibold)
                + Text(" Each dot is one inter-feed gap. The red line marks 2 hours: dots below it are short gaps, dots above it are typical spacing.")
            explainerRow(kind: .smallTopOff, title: "Small top-off",
                         detail: "Follow-up was ≤ 50% of the preceding feed (tiny snack).")
            explainerRow(kind: .closeFeed, title: "Close feed",
                         detail: "Within 3 hours and not a clear growth spike (≤ the preceding feed, or a follow-up to a top-off).")
            explainerRow(kind: .fullFeed, title: "Growing / spaced",
                         detail: "> 3 hours apart, or baby took more than last time.")
        }
        .font(.caption2)
        .foregroundStyle(DesignColors.textSecondary)
        .padding(8)
        .background(DesignColors.surfaceSecondary)
        .clipShape(RoundedRectangle(cornerRadius: 8))
    }

    private func explainerRow(kind: FeedingGapsScatter.Kind, title: String, detail: String) -> some View {
        HStack(alignment: .top, spacing: 6) {
            Circle().fill(kind.color).frame(width: 8, height: 8).padding(.top, 3)
            Text(title).fontWeight(.semibold)
            Text(detail)
        }
    }
}

/// Compact yyyy-MM-dd string-backed date picker used by stats filters.
/// A start and an end date, side by side, or one under the other at
/// accessibility sizes: side by side they are wider than the screen there,
/// which pushed every Stats card past its edges (PWA `.feeding-gaps-dates`).
struct StatsDateRange: View {
    @Binding var start: String
    @Binding var end: String
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: 8) {
                StatsDatePicker(dateString: $start)
                StatsDatePicker(dateString: $end)
            }
        } else {
            HStack(spacing: 8) {
                StatsDatePicker(dateString: $start)
                Text("–").font(.caption).foregroundStyle(DesignColors.textSecondary)
                StatsDatePicker(dateString: $end)
            }
        }
    }
}

struct StatsDatePicker: View {
    @Binding var dateString: String

    var body: some View {
        DatePicker("", selection: Binding(
            get: { StatsModel.parseDate(dateString) ?? Date() },
            set: { dateString = StatsModel.dateString($0) }
        ), displayedComponents: .date)
        .labelsHidden()
    }
}

/// A filter shown as its chosen option and a chevron, as a menu-style picker
/// is, whose label wraps and grows with the text: a menu-style picker stays
/// one line high, which cut the top and bottom off a wrapped "All members" at
/// accessibility sizes.
struct StatsChoice: View {
    let title: String
    @Binding var selection: Int?
    /// The options, in order; the first is the one for nothing chosen.
    let options: [(id: Int?, name: String)]

    private var chosen: String { options.first { $0.id == selection }?.name ?? options[0].name }

    var body: some View {
        Menu {
            Picker(title, selection: $selection) {
                ForEach(options.indices, id: \.self) { i in
                    Text(options[i].name).tag(options[i].id)
                }
            }
        } label: {
            HStack(spacing: 4) {
                Text(chosen).multilineTextAlignment(.leading)
                Image(systemName: "chevron.up.chevron.down").imageScale(.small)
            }
            .frame(minHeight: 44)
            .contentShape(Rectangle())
        }
        .accessibilityLabel(title)
        .accessibilityValue(chosen)
    }
}

// MARK: - Generalized per-chore analytics (chore:<id>)

/// Member split plus a metric-appropriate chart for any chore that tracks a
/// metric or has indicator labels (PWA `renderChoreAnalyticsSection`).
struct ChoreAnalyticsSection: View {
    var model: StatsModel
    let chore: Chore
    let members: [Member]

    var body: some View {
        let period = model.choreAnalyticsPeriod[chore.id] ?? "day"
        let grain = StatsSections.choreAnalyticsGrain(period)
        let ts = model.choreTimeSeries[chore.id]

        StatsCard {
            StatsCardHeader(title: Text("\(chore.icon) \(chore.name)"), truncates: true) {
                StatsPeriodToggle(period: period, includeAll: false) { p in
                    Task { await model.setChoreAnalyticsPeriod(p, choreId: chore.id) }
                }
            }
            MemberBarList(byMember: (ts?.byMember ?? []).map { ($0.userId, $0.count) }, members: members)
            if let periods = ts?.periods, !periods.isEmpty {
                metricChart(periods: periods, grain: grain)
            } else {
                Text("No data").font(.caption).foregroundStyle(DesignColors.textSecondary)
            }
        }
    }

    /// Metric-appropriate chart: amount → unit totals, duration → minutes,
    /// indicators → stacked counts, else occurrence counts.
    @ViewBuilder
    private func metricChart(periods: [TimeSeriesPeriod], grain: String) -> some View {
        switch chore.metricType {
        case "amount":
            UnitAmountCharts(periods: periods, fallback: chore.metricUnit, grain: grain)
        case "duration":
            PeriodBarChart(
                segments: PeriodBarData.valueSegments(periods) { (Double($0.totalDuration ?? 0) / 60.0).rounded() },
                periods: periods, grain: grain,
                unitLabel: "min",
                seriesColors: ["value": DesignColors.primary],
                summaryText: { p in "\(Int((Double(p.totalDuration ?? 0) / 60.0).rounded())) min" }
            )
        default:
            if !chore.indicatorLabels.isEmpty {
                IndicatorPeriodChart(periods: periods, grain: grain)
            } else {
                PeriodBarChart(
                    segments: PeriodBarData.valueSegments(periods) { Double($0.count) },
                    periods: periods, grain: grain,
                    unitLabel: "count",
                    seriesColors: ["value": DesignColors.primary],
                    summaryText: { p in "\(p.count)" }
                )
            }
        }
    }
}


struct UnitAmountCharts: View {
    let periods: [TimeSeriesPeriod]
    let fallback: String
    let grain: String
    var excludeLiquid = false
    private var units: [String] {
        let found = Set(periods.flatMap { Array(($0.amountsByUnit ?? [:]).keys) }).filter { !excludeLiquid || $0 != "mL" }.sorted()
        return found.isEmpty && !excludeLiquid ? [fallback] : found
    }
    var body: some View {
        ForEach(units, id: \.self) { unit in
            PeriodBarChart(
                segments: PeriodBarData.valueSegments(periods) { Double($0.amountsByUnit?[unit] ?? ($0.amountsByUnit == nil ? $0.totalML ?? 0 : 0)) },
                periods: periods, grain: grain, unitLabel: unit.isEmpty ? "amount" : unit,
                seriesColors: ["value": DesignColors.primary],
                summaryText: { p in "\(p.amountsByUnit?[unit] ?? (p.amountsByUnit == nil ? p.totalML ?? 0 : 0)) \(unit)" }
            )
        }
    }
}
