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
    @ObservedObject var model: StatsModel
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

struct BabyCareSection: View {
    @ObservedObject var model: StatsModel
    let members: [Member]
    let volumeUnit: String

    var body: some View {
        StatsCard {
            Text("Baby").font(.headline)
            if let chore = model.feedBabyChore {
                let ts = model.feedBabyTS ?? emptySeries(chore)
                BabyColumn(model: model, ts: ts, type: "feed", period: model.feedBabyPeriod,
                           members: members, volumeUnit: volumeUnit)
            }
            if let chore = model.changeBabyChore {
                let ts = model.changeBabyTS ?? emptySeries(chore)
                BabyColumn(model: model, ts: ts, type: "change", period: model.changeBabyPeriod,
                           members: members, volumeUnit: volumeUnit)
            }
            if model.feedBabyChore != nil {
                FeedingGapsColumn(model: model, volumeUnit: volumeUnit)
            }
        }
    }

    private func emptySeries(_ chore: Chore) -> ChoreTimeSeries {
        ChoreTimeSeries(choreId: chore.id, choreName: chore.name, choreIcon: chore.icon,
                        metricType: chore.metricType, metricUnit: chore.metricUnit, byMember: [], periods: [])
    }
}

struct BabyColumn: View {
    @ObservedObject var model: StatsModel
    let ts: ChoreTimeSeries
    let type: String // "feed" | "change"
    let period: String // daily | weekly | monthly
    let members: [Member]
    let volumeUnit: String

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 4) {
                Text(ts.choreIcon).font(.body)
                Text(ts.choreName).font(.subheadline).fontWeight(.semibold).lineLimit(1)
                Spacer(minLength: 4)
                Picker("Time period", selection: Binding(
                    get: { period },
                    set: { p in Task { await model.setBabyPeriod(p, type: type) } }
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
    @ObservedObject var model: StatsModel
    let volumeUnit: String

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("🕐 Cluster Feeding").font(.subheadline).fontWeight(.semibold)
                Button {
                    withAnimation { model.feedingGapsExplainerVisible.toggle() }
                } label: {
                    Image(systemName: "info.circle")
                        .font(.caption)
                        .foregroundStyle(DesignColors.textSecondary)
                }
                .accessibilityLabel("How to read this chart")
                Spacer()
                Picker("Range", selection: Binding(
                    get: {
                        [1, 7, 14].first { model.isFeedingGapsQuickActive(days: $0) } ?? 0
                    },
                    set: { days in
                        if days > 0 { Task { await model.setFeedingGapsQuickRange(days: days) } }
                    }
                )) {
                    Text("Day").tag(1)
                    Text("Week").tag(7)
                    Text("2 Weeks").tag(14)
                    if ![1, 7, 14].contains(where: { model.isFeedingGapsQuickActive(days: $0) }) {
                        Text("Custom").tag(0)
                    }
                }
                .pickerStyle(.segmented)
                .fixedSize()
            }

            HStack(spacing: 8) {
                StatsDatePicker(dateString: Binding(
                    get: { model.feedingGapsStart },
                    set: { model.feedingGapsStart = $0; Task { await model.loadFeedingGaps() } }
                ))
                Text("–").font(.caption).foregroundStyle(DesignColors.textSecondary)
                StatsDatePicker(dateString: Binding(
                    get: { model.feedingGapsEnd },
                    set: { model.feedingGapsEnd = $0; Task { await model.loadFeedingGaps() } }
                ))
            }

            if model.feedingGapsExplainerVisible {
                FeedingGapsExplainer()
            }

            if model.feedingGaps.isEmpty {
                Text("No data").font(.caption).foregroundStyle(DesignColors.textSecondary)
            } else {
                FeedingGapsScatter(gaps: model.feedingGaps, volumeUnit: volumeUnit)
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

// MARK: - Generalized per-chore analytics (chore:<id>)

/// Member split plus a metric-appropriate chart for any chore that tracks a
/// metric or has indicator labels (PWA `renderChoreAnalyticsSection`).
struct ChoreAnalyticsSection: View {
    @ObservedObject var model: StatsModel
    let chore: Chore
    let members: [Member]

    var body: some View {
        let period = model.choreAnalyticsPeriod[chore.id] ?? "day"
        let grain = StatsSections.choreAnalyticsGrain(period)
        let ts = model.choreTimeSeries[chore.id]

        StatsCard {
            HStack {
                Text("\(chore.icon) \(chore.name)").font(.headline).lineLimit(1)
                Spacer()
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
            let unit = chore.metricUnit
            PeriodBarChart(
                segments: PeriodBarData.valueSegments(periods) { Double($0.totalML ?? 0) },
                periods: periods, grain: grain,
                unitLabel: unit.isEmpty ? "amount" : unit,
                seriesColors: ["value": DesignColors.primary],
                summaryText: { p in "\(p.totalML ?? 0)\(unit.isEmpty ? "" : " " + unit)" }
            )
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
