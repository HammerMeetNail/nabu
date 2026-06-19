import SwiftUI

struct StatsView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var overview: StatsOverview?
    @State private var heatmap: [HeatmapEntry] = []
    @State private var busyHours: [BusyHour] = []
    @State private var busyHoursStart: String = ""
    @State private var busyHoursEnd: String = ""
    @State private var leaderboardByPeriod: [String: LeaderboardResponse] = [:]
    @State private var topChoresByUserAndPeriod: [String: [TopChore]] = [:]
    @State private var leaderboardPeriod: String = "week"
    @State private var topChoresPeriod: String = "month"
    @State private var choreStats: [ChoreStat] = []
    @State private var choreStatsStart: String = ""
    @State private var choreStatsEnd: String = ""
    @State private var topChoresUserId: Int? = nil
    @State private var isLoading = true

    // Busy hours filter state
    @State private var bhChoreId: Int? = nil
    @State private var bhUserId: Int? = nil
    @State private var bhFilterStart: String = ""
    @State private var bhFilterEnd: String = ""



    // Chore stats filter state
    @State private var csFilterStart: String = ""
    @State private var csFilterEnd: String = ""

    // Baby care
    @State private var feedBabyPeriod: String = "daily"
    @State private var changeBabyPeriod: String = "daily"
    @State private var feedBabyTS: ChoreTimeSeries?
    @State private var changeBabyTS: ChoreTimeSeries?
    @State private var selectedFeedBar: Int? = nil
    @State private var selectedChangeBar: Int? = nil

    private var currentTopChores: [TopChore] {
        let uid = topChoresUserId ?? 0
        let key = "\(uid)-\(topChoresPeriod)"
        return topChoresByUserAndPeriod[key] ?? []
    }

    private var currentLeaderboard: [LeaderboardEntry] {
        if let resp = leaderboardByPeriod[leaderboardPeriod] {
            return resp.leaderboard
        }
        if leaderboardPeriod == "week", let ov = overview {
            return ov.leaderboard
        }
        return []
    }

    private var leaderboardRangeLabel: String {
        if leaderboardPeriod == "all" { return "All time" }
        if let resp = leaderboardByPeriod[leaderboardPeriod],
           let s = resp.start, let e = resp.end,
           !s.isEmpty, !e.isEmpty {
            return formatRangeLabel(s, e)
        }
        return leaderboardPeriod == "week" ? formatWeekLabel() : ""
    }

    private var leaderboardEmptyLabel: String {
        switch leaderboardPeriod {
        case "all": return "No chores logged yet"
        case "day": return "No chores today"
        case "month": return "No chores this month"
        default: return "No chores this week"
        }
    }

    private var topChoresEmptyLabel: String {
        switch topChoresPeriod {
        case "all": return "No chores logged yet"
        case "day": return "No chores today"
        case "month": return "No chores this month"
        default: return "No chores this week"
        }
    }

    private var activeChoreStats: [ChoreStat] {
        choreStats.filter { $0.totalThisWeek > 0 || $0.totalThisMonth > 0 }
    }

    private var feedBabyChore: Chore? {
        state.chores.first { $0.name == "Feed Baby" }
    }

    private var changeBabyChore: Chore? {
        state.chores.first { $0.name == "Change Baby" }
    }

    var body: some View {
        NavigationStack {
            Group {
                if isLoading {
                    ProgressView().frame(maxHeight: .infinity)
                } else {
                    ScrollView {
                        VStack(spacing: 16) {
                            overviewRow
                            if feedBabyTS != nil || changeBabyTS != nil {
                                babyCareSection
                            }
                            if !heatmap.isEmpty { heatmapCard }
                            if !busyHours.isEmpty { busyHoursCard }
                            leaderboardSection
                            topChoresSection
                            if let ov = overview, !ov.breakdown.isEmpty { categoriesCard(ov.breakdown) }
                            if !activeChoreStats.isEmpty { choreStatsSection }
                            if let ov = overview, ov.recap.totalChores > 0 { recapCard(ov.recap) }
                        }
                        .padding()
                    }
                }
            }
            .navigationTitle("Stats")
        }
        .task { await loadStats() }
    }

    // MARK: - Overview Row

    @ViewBuilder
    private var overviewRow: some View {
        let todayCount = state.todayLogs.count
        let weekCount = overview?.recap.totalChores ?? 0
        let streak = overview?.streaks.current ?? 0
        let topName: String = {
            guard let first = choreStats.first, first.totalThisWeek > 0 else { return "-" }
            return first.choreName
        }()

        HStack(spacing: 8) {
            statTile(value: "\(todayCount)", label: "TODAY")
            statTile(value: "\(weekCount)", label: "THIS WEEK")
            statTile(value: "\(streak)", label: "DAY STREAK")
            statTile(value: topName, label: "TOP CHORE")
        }
    }

    private func statTile(value: String, label: String) -> some View {
        VStack(spacing: 4) {
            Text(value)
                .font(.title2).fontWeight(.bold)
                .foregroundColor(DesignColors.primary)
                .lineLimit(1).minimumScaleFactor(0.5)
            Text(label)
                .font(.system(size: 9, weight: .semibold))
                .foregroundColor(DesignColors.textSecondary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 12)
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 12))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    // MARK: - Baby Care Section

    @ViewBuilder
    private var babyCareSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Baby").font(.headline)

            VStack(spacing: 12) {
                if let ts = feedBabyTS {
                    babyColumn(ts, type: "feed", period: feedBabyPeriod)
                        .frame(maxWidth: .infinity)
                }
                if let ts = changeBabyTS {
                    babyColumn(ts, type: "change", period: changeBabyPeriod)
                        .frame(maxWidth: .infinity)
                }
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    @ViewBuilder
    private func babyColumn(_ ts: ChoreTimeSeries, type: String, period: String) -> some View {
        let isVolume = type == "feed"
        let periodLabels = ["daily": "Daily", "weekly": "Weekly", "monthly": "Monthly"]

        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 4) {
                Text(ts.choreIcon).font(.body)
                Text(ts.choreName).font(.subheadline).fontWeight(.semibold).lineLimit(1)
                Spacer(minLength: 4)
                HStack(spacing: 4) {
                    ForEach(["daily", "weekly", "monthly"], id: \.self) { p in
                        Button {
                            Task { await setBabyPeriod(p, type: type) }
                        } label: {
                            Text(periodLabels[p] ?? p)
                                .font(.system(size: 10, weight: .medium))
                                .padding(.horizontal, 7).padding(.vertical, 3)
                                .background(period == p ? DesignColors.primary : DesignColors.surfaceSecondary)
                                .foregroundColor(period == p ? .white : DesignColors.textPrimary)
                                .clipShape(Capsule())
                        }
                    }
                }
            }

            // By-member breakdown
            if !ts.byMember.isEmpty {
                let maxCount = ts.byMember.first?.count ?? 1
                ForEach(ts.byMember, id: \.userId) { entry in
                    let member = state.members.first { $0.userId == entry.userId }
                    let name = member.map { $0.displayName.isEmpty ? $0.email : $0.displayName } ?? "User \(entry.userId)"
                    let initial = String(name.prefix(1)).uppercased()
                    let hex = (member?.avatarColor ?? "#19323C")
                        .trimmingCharacters(in: CharacterSet(charactersIn: "#"))
                    let pct = maxCount > 0 ? CGFloat(entry.count) / CGFloat(maxCount) : 0
                    HStack(spacing: 6) {
                        Circle()
                            .fill(Color(hexUnsafe: hex))
                            .frame(width: 18, height: 18)
                            .overlay(
                                Text(initial)
                                    .font(.system(size: 9, weight: .semibold))
                                    .foregroundColor(.white)
                            )
                        Text(name).font(.system(size: 11)).lineLimit(1)
                        GeometryReader { geo in
                            ZStack(alignment: .leading) {
                                RoundedRectangle(cornerRadius: 2)
                                    .fill(DesignColors.surfaceSecondary)
                                RoundedRectangle(cornerRadius: 2)
                                    .fill(DesignColors.primary.opacity(0.5))
                                    .frame(width: geo.size.width * pct)
                            }
                        }
                        .frame(height: 8)
                        Text("\(entry.count)").font(.system(size: 10))
                            .foregroundColor(DesignColors.textSecondary)
                            .frame(width: 20, alignment: .trailing)
                    }
                }
            }

            if ts.periods.isEmpty {
                Text("No data")
                    .font(.caption2).foregroundColor(DesignColors.textSecondary)
            } else if isVolume {
                babyVolumeChart(ts.periods, period: period, selectedBar: $selectedFeedBar)
            } else {
                babyIndicatorChart(ts.periods, period: period, selectedBar: $selectedChangeBar)
            }
        }
    }

    private func niceAxisStep(_ max: Int) -> Int {
        if max <= 2 { return 1 }
        if max <= 10 { return 2 }
        if max <= 25 { return 5 }
        if max <= 100 { return 25 }
        let magnitude = Int(pow(10.0, floor(log10(Double(max)))))
        let residual = Double(max) / Double(magnitude)
        if residual <= 2 { return magnitude / 2 }
        if residual <= 5 { return magnitude }
        return magnitude * 2
    }

    private func computeTicks(max: Int) -> [Int] {
        let step = niceAxisStep(max)
        var ticks: [Int] = []
        var v = 0
        while v <= max + step / 2 { ticks.append(v); v += step }
        return ticks
    }

    private func buildVolumeSegments(_ p: TimeSeriesPeriod, maxML: Int, chartH: CGFloat, stackKeys: [String]) -> [(key: String, ml: Int, offset: CGFloat)] {
        var segments: [(key: String, ml: Int, offset: CGFloat)] = []
        var runningOffset: CGFloat = 0
        for key in stackKeys {
            let ml = p.volumeByIndicator?[key] ?? 0
            if ml > 0 {
                segments.append((key, ml, runningOffset))
                runningOffset += CGFloat(ml) / CGFloat(maxML) * chartH
            }
        }
        return segments
    }

    private func buildIndicatorSegments(_ p: TimeSeriesPeriod, maxCount: Int, chartH: CGFloat, indicatorKeys: [String]) -> [(key: String, count: Int, offset: CGFloat)] {
        var segments: [(key: String, count: Int, offset: CGFloat)] = []
        var runningOffset: CGFloat = 0
        for key in indicatorKeys {
            let count = p.indicators?[key] ?? 0
            if count > 0 {
                segments.append((key, count, runningOffset))
                runningOffset += CGFloat(count) / CGFloat(maxCount) * chartH
            }
        }
        return segments
    }

    // MARK: - Baby Volume Chart

    @ViewBuilder
    private func babyVolumeChart(_ periods: [TimeSeriesPeriod], period: String, selectedBar: Binding<Int?>) -> some View {
        let stackColors: [String: Color] = [
            "🍼 formula": Color(hexUnsafe: "EC4899"),
            "🤱 breast": Color(hexUnsafe: "F59E0B"),
        ]
        let stackKeys = extractStackKeys(periods, volumeMode: true)
        let maxML = max(1, periods.map { $0.totalML ?? 0 }.max() ?? 1)
        let count = periods.count
        let spacing: CGFloat = 4
        let chartH: CGFloat = 80
        let leftMargin: CGFloat = 38
        let rightMargin: CGFloat = 6

        let ticks = computeTicks(max: maxML)

        GeometryReader { geo in
            let availableWidth = geo.size.width - leftMargin - rightMargin
            let totalSpacing = spacing * CGFloat(max(count - 1, 0))
            let colW = count > 0 ? max(8, (availableWidth - totalSpacing) / CGFloat(count)) : 18
            let xAxisH: CGFloat = 14

            VStack(alignment: .leading, spacing: 0) {
                HStack(alignment: .bottom, spacing: 0) {
                    ZStack(alignment: .topLeading) {
                        ForEach(ticks, id: \.self) { t in
                            let y = chartH - CGFloat(t) / CGFloat(maxML) * chartH
                            Text("\(t)")
                                .font(.system(size: 9))
                                .foregroundColor(Color(hexUnsafe: "9ca3af"))
                                .frame(width: leftMargin - 4, alignment: .trailing)
                                .position(x: (leftMargin - 4) / 2, y: y + 4)
                        }
                        Text("mL")
                            .font(.system(size: 9))
                            .foregroundColor(Color(hexUnsafe: "9ca3af"))
                            .rotationEffect(.degrees(-90))
                            .fixedSize()
                            .position(x: 8, y: chartH / 2)
                    }
                    .frame(width: leftMargin, height: chartH + xAxisH)

                    ZStack(alignment: .bottomLeading) {
                        ForEach(ticks, id: \.self) { t in
                            let y = xAxisH + CGFloat(t) / CGFloat(maxML) * chartH
                            Rectangle()
                                .fill(Color(hexUnsafe: "e5e7eb"))
                                .frame(width: availableWidth, height: 0.5)
                                .offset(y: -y)
                        }
                        Rectangle()
                            .fill(Color(hexUnsafe: "d1d5db"))
                            .frame(width: availableWidth, height: 1)
                            .offset(y: -xAxisH)

                        HStack(alignment: .bottom, spacing: spacing) {
                            ForEach(Array(periods.enumerated()), id: \.offset) { i, p in
                                let showXLabel = period != "daily" || i % 2 == 0
                                let rawLabel = volumeBarLabel(p, stackKeys: stackKeys)
                                let estW = CGFloat(rawLabel.count) * 7
                                let colCenter = CGFloat(i) * colW + colW / 2
                                let labelAlign: Alignment = colCenter + estW / 2 > availableWidth ? .topTrailing
                                    : colCenter - estW / 2 < 0 ? .topLeading
                                    : .top
                                babyVolumeBarColumn(p, i: i, maxML: maxML, colW: colW, chartH: chartH, stackKeys: stackKeys, stackColors: stackColors, period: period, selectedBar: selectedBar, showXLabel: showXLabel, labelAlign: labelAlign)
                            }
                        }
                    }
                    .frame(width: availableWidth, height: chartH + xAxisH)
                }

                babyVolumeLegend(periods)
                    .padding(.leading, leftMargin)
                    .padding(.top, 4)
            }
        }
        .frame(height: chartH + 60)
    }

    @ViewBuilder
    private func babyVolumeBarColumn(_ p: TimeSeriesPeriod, i: Int, maxML: Int, colW: CGFloat, chartH: CGFloat, stackKeys: [String], stackColors: [String: Color], period: String, selectedBar: Binding<Int?>, showXLabel: Bool, labelAlign: Alignment = .top) -> some View {
        let totalML = p.totalML ?? 0
        let valText = volumeBarLabel(p, stackKeys: stackKeys)
        VStack(spacing: 2) {
            ZStack(alignment: .bottom) {
                Rectangle()
                    .fill(Color.clear)
                    .frame(width: colW, height: chartH)

                if totalML > 0 {
                    stackedVolumeBars(p, totalML: totalML, maxML: maxML, colW: colW, chartH: chartH, stackKeys: stackKeys, stackColors: stackColors)
                }
            }
            .frame(width: colW, height: chartH)
            .overlay(alignment: labelAlign) {
                if selectedBar.wrappedValue == i, !valText.isEmpty {
                    Text(valText)
                        .font(.system(size: 7, weight: .bold))
                        .foregroundColor(.white)
                        .shadow(color: Color(hexUnsafe: "374151"), radius: 0, x: 0.75, y: 0.75)
                        .fixedSize(horizontal: true, vertical: false)
                        .offset(y: -14)
                        .allowsHitTesting(false)
                }
            }
            .onTapGesture {
                if selectedBar.wrappedValue == i {
                    selectedBar.wrappedValue = nil
                } else {
                    selectedBar.wrappedValue = i
                }
            }

            if showXLabel {
                Text(formatBabyXLabel(p, period: period))
                    .font(.system(size: 7))
                    .foregroundColor(Color(hexUnsafe: "9ca3af"))
                    .lineLimit(1)
                    .frame(width: colW)
            } else {
                Color.clear.frame(width: colW, height: 14)
            }
        }
        .frame(width: colW)
    }

    private func volumeBarLabel(_ p: TimeSeriesPeriod, stackKeys: [String]) -> String {
        let parts = stackKeys.compactMap { key -> String? in
            guard let ml = p.volumeByIndicator?[key], ml > 0 else { return nil }
            return "\(key) \(ml)mL"
        }
        let attrML = stackKeys.reduce(0) { $0 + (p.volumeByIndicator?[$1] ?? 0) }
        let unlabeledML = (p.totalML ?? 0) - attrML
        if unlabeledML > 0 {
            return (parts + ["unlabeled \(unlabeledML)mL"]).joined(separator: ", ")
        }
        if parts.isEmpty, let total = p.totalML, total > 0 {
            return "\(total)mL"
        }
        return parts.joined(separator: ", ")
    }

    @ViewBuilder
    private func stackedVolumeBars(_ p: TimeSeriesPeriod, totalML: Int, maxML: Int, colW: CGFloat, chartH: CGFloat, stackKeys: [String], stackColors: [String: Color]) -> some View {
        let segments = buildVolumeSegments(p, maxML: maxML, chartH: chartH, stackKeys: stackKeys)
        let attrML = segments.reduce(0) { $0 + $1.ml }
        let unlabeledML = totalML - attrML
        let finalOffset = segments.reduce(0) { $0 + CGFloat($1.ml) / CGFloat(maxML) * chartH }

        ZStack(alignment: .bottom) {
            ForEach(Array(segments.enumerated()), id: \.offset) { _, seg in
                let segH = CGFloat(seg.ml) / CGFloat(maxML) * chartH
                RoundedRectangle(cornerRadius: 2)
                    .fill(stackColors[seg.key] ?? Color(hexUnsafe: "6B7280"))
                    .frame(width: colW - 2, height: max(segH, 1))
                    .offset(y: -seg.offset)
            }
            if unlabeledML > 0 {
                let segH = CGFloat(unlabeledML) / CGFloat(maxML) * chartH
                RoundedRectangle(cornerRadius: 2)
                    .fill(Color(hexUnsafe: "d1d5db").opacity(0.6))
                    .frame(width: colW - 2, height: max(segH, 1))
                    .offset(y: -finalOffset)
            }
        }
    }

    @ViewBuilder
    private func babyVolumeLegend(_ periods: [TimeSeriesPeriod]) -> some View {
        let formulaTotal = periods.reduce(0) { $0 + ($1.indicators?["🍼 formula"] ?? 0) }
        let breastTotal = periods.reduce(0) { $0 + ($1.indicators?["🤱 breast"] ?? 0) }
        let unlabeledTotalML = periods.reduce(0) { s, p in
            let attr = (p.volumeByIndicator ?? [:]).values.reduce(0, +)
            return s + (p.totalML ?? 0) - attr
        }

        if formulaTotal > 0 || breastTotal > 0 || unlabeledTotalML > 0 {
            HStack(spacing: 12) {
                if formulaTotal > 0 {
                    HStack(spacing: 2) {
                        RoundedRectangle(cornerRadius: 2)
                            .fill(Color(hexUnsafe: "EC4899")).frame(width: 8, height: 8)
                        Text("🍼 \(formulaTotal) total").font(.system(size: 8)).foregroundColor(Color(hexUnsafe: "6b7280"))
                    }
                }
                if breastTotal > 0 {
                    HStack(spacing: 2) {
                        RoundedRectangle(cornerRadius: 2)
                            .fill(Color(hexUnsafe: "F59E0B")).frame(width: 8, height: 8)
                        Text("🤱 \(breastTotal) total").font(.system(size: 8)).foregroundColor(Color(hexUnsafe: "6b7280"))
                    }
                }
                if unlabeledTotalML > 0 {
                    HStack(spacing: 2) {
                        RoundedRectangle(cornerRadius: 2)
                            .fill(Color(hexUnsafe: "d1d5db").opacity(0.6)).frame(width: 8, height: 8)
                        Text("unlabeled \(unlabeledTotalML)mL").font(.system(size: 8)).foregroundColor(Color(hexUnsafe: "9ca3af"))
                    }
                }
                Spacer()
            }
            .padding(.leading, 38)
        }
    }

    // MARK: - Baby Indicator Chart

    @ViewBuilder
    private func babyIndicatorChart(_ periods: [TimeSeriesPeriod], period: String, selectedBar: Binding<Int?>) -> some View {
        let indicatorColors: [String: Color] = [
            "💩 poo": Color(hexUnsafe: "8B4513"),
            "💛 pee": Color(hexUnsafe: "FACC15"),
            "🍼 formula": Color(hexUnsafe: "EC4899"),
            "🤱 breast": Color(hexUnsafe: "F59E0B"),
        ]
        let indicatorKeys = extractStackKeys(periods, volumeMode: false)
        let maxCount = max(1, periods.map { p in
            indicatorKeys.reduce(0) { $0 + (p.indicators?[$1] ?? 0) }
        }.max() ?? 1)
        let count = periods.count
        let spacing: CGFloat = 4
        let chartH: CGFloat = 80
        let leftMargin: CGFloat = 38
        let rightMargin: CGFloat = 6

        let ticks = computeTicks(max: maxCount)

        GeometryReader { geo in
            let availableWidth = geo.size.width - leftMargin - rightMargin
            let totalSpacing = spacing * CGFloat(max(count - 1, 0))
            let colW = count > 0 ? max(8, (availableWidth - totalSpacing) / CGFloat(count)) : 18
            let xAxisH: CGFloat = 14

            VStack(alignment: .leading, spacing: 0) {
                HStack(alignment: .bottom, spacing: 0) {
                    ZStack(alignment: .topLeading) {
                        ForEach(ticks, id: \.self) { t in
                            let y = chartH - CGFloat(t) / CGFloat(maxCount) * chartH
                            Text("\(t)")
                                .font(.system(size: 9))
                                .foregroundColor(Color(hexUnsafe: "9ca3af"))
                                .frame(width: leftMargin - 4, alignment: .trailing)
                                .position(x: (leftMargin - 4) / 2, y: y + 4)
                        }
                        Text("cnt")
                            .font(.system(size: 9))
                            .foregroundColor(Color(hexUnsafe: "9ca3af"))
                            .rotationEffect(.degrees(-90))
                            .fixedSize()
                            .position(x: 8, y: chartH / 2)
                    }
                    .frame(width: leftMargin, height: chartH + xAxisH)

                    ZStack(alignment: .bottomLeading) {
                        ForEach(ticks, id: \.self) { t in
                            let y = xAxisH + CGFloat(t) / CGFloat(maxCount) * chartH
                            Rectangle()
                                .fill(Color(hexUnsafe: "e5e7eb"))
                                .frame(width: availableWidth, height: 0.5)
                                .offset(y: -y)
                        }
                        Rectangle()
                            .fill(Color(hexUnsafe: "d1d5db"))
                            .frame(width: availableWidth, height: 1)
                            .offset(y: -xAxisH)

                        HStack(alignment: .bottom, spacing: spacing) {
                            ForEach(Array(periods.enumerated()), id: \.offset) { i, p in
                                let showXLabel = period != "daily" || i % 2 == 0
                                let rawLabel = indicatorBarLabel(p, indicatorKeys: indicatorKeys)
                                let estW = CGFloat(rawLabel.count) * 7
                                let colCenter = CGFloat(i) * colW + colW / 2
                                let labelAlign: Alignment = colCenter + estW / 2 > availableWidth ? .topTrailing
                                    : colCenter - estW / 2 < 0 ? .topLeading
                                    : .top
                                babyIndicatorBarColumn(p, i: i, maxCount: maxCount, colW: colW, chartH: chartH, indicatorKeys: indicatorKeys, indicatorColors: indicatorColors, period: period, selectedBar: selectedBar, showXLabel: showXLabel, labelAlign: labelAlign)
                            }
                        }
                    }
                    .frame(width: availableWidth, height: chartH + xAxisH)
                }

                babyIndicatorLegend(periods, indicatorKeys: indicatorKeys, indicatorColors: indicatorColors)
                    .padding(.leading, leftMargin)
                    .padding(.top, 4)
            }
        }
        .frame(height: chartH + 60)
    }

    @ViewBuilder
    private func babyIndicatorBarColumn(_ p: TimeSeriesPeriod, i: Int, maxCount: Int, colW: CGFloat, chartH: CGFloat, indicatorKeys: [String], indicatorColors: [String: Color], period: String, selectedBar: Binding<Int?>, showXLabel: Bool, labelAlign: Alignment = .top) -> some View {
        let periodTotal = indicatorKeys.reduce(0) { $0 + (p.indicators?[$1] ?? 0) }
        let valText = indicatorBarLabel(p, indicatorKeys: indicatorKeys)

        VStack(spacing: 2) {
            ZStack(alignment: .bottom) {
                Rectangle()
                    .fill(Color.clear)
                    .frame(width: colW, height: chartH)

                if periodTotal > 0 {
                    stackedIndicatorBars(p, maxCount: maxCount, colW: colW, chartH: chartH, indicatorKeys: indicatorKeys, indicatorColors: indicatorColors)
                }
            }
            .frame(width: colW, height: chartH)
            .overlay(alignment: labelAlign) {
                if selectedBar.wrappedValue == i, !valText.isEmpty {
                    Text(valText)
                        .font(.system(size: 7, weight: .bold))
                        .foregroundColor(.white)
                        .shadow(color: Color(hexUnsafe: "374151"), radius: 0, x: 0.75, y: 0.75)
                        .fixedSize(horizontal: true, vertical: false)
                        .offset(y: -14)
                        .allowsHitTesting(false)
                }
            }
            .onTapGesture {
                if selectedBar.wrappedValue == i {
                    selectedBar.wrappedValue = nil
                } else {
                    selectedBar.wrappedValue = i
                }
            }

            if showXLabel {
                Text(formatBabyXLabel(p, period: period))
                    .font(.system(size: 7))
                    .foregroundColor(Color(hexUnsafe: "9ca3af"))
                    .lineLimit(1)
                    .frame(width: colW)
            } else {
                Color.clear.frame(width: colW, height: 14)
            }
        }
        .frame(width: colW)
    }

    private func indicatorBarLabel(_ p: TimeSeriesPeriod, indicatorKeys: [String]) -> String {
        let parts = indicatorKeys.compactMap { key -> String? in
            guard let count = p.indicators?[key], count > 0 else { return nil }
            return "\(key) \(count)"
        }
        return parts.joined(separator: ", ")
    }

    @ViewBuilder
    private func stackedIndicatorBars(_ p: TimeSeriesPeriod, maxCount: Int, colW: CGFloat, chartH: CGFloat, indicatorKeys: [String], indicatorColors: [String: Color]) -> some View {
        let segments = buildIndicatorSegments(p, maxCount: maxCount, chartH: chartH, indicatorKeys: indicatorKeys)

        if indicatorKeys.count > 1 {
            ZStack(alignment: .bottom) {
                ForEach(Array(segments.enumerated()), id: \.offset) { _, seg in
                    let segH = CGFloat(seg.count) / CGFloat(maxCount) * chartH
                    RoundedRectangle(cornerRadius: 2)
                        .fill(indicatorColors[seg.key] ?? Color(hexUnsafe: "6B7280"))
                        .frame(width: colW - 2, height: max(segH, 1))
                        .offset(y: -seg.offset)
                }
            }
        } else if let first = indicatorKeys.first {
            let periodTotal = indicatorKeys.reduce(0) { $0 + (p.indicators?[$1] ?? 0) }
            let segH = CGFloat(periodTotal) / CGFloat(maxCount) * chartH
            RoundedRectangle(cornerRadius: 2)
                .fill(indicatorColors[first] ?? Color(hexUnsafe: "6B7280"))
                .frame(width: colW - 2, height: max(segH, 1))
        }
    }

    @ViewBuilder
    private func babyIndicatorLegend(_ periods: [TimeSeriesPeriod], indicatorKeys: [String], indicatorColors: [String: Color]) -> some View {
        if !indicatorKeys.isEmpty {
            HStack(spacing: 12) {
                ForEach(indicatorKeys, id: \.self) { key in
                    let total = periods.reduce(0) { $0 + ($1.indicators?[key] ?? 0) }
                    HStack(spacing: 2) {
                        RoundedRectangle(cornerRadius: 2)
                            .fill(indicatorColors[key] ?? Color(hexUnsafe: "6B7280"))
                            .frame(width: 8, height: 8)
                        Text("\(key) \(total) total").font(.system(size: 8))
                            .foregroundColor(Color(hexUnsafe: "6b7280"))
                    }
                }
                Spacer()
            }
            .padding(.leading, 38)
        }
    }

    private func extractStackKeys(_ periods: [TimeSeriesPeriod], volumeMode: Bool) -> [String] {
        var seen = Set<String>()
        var keys: [String] = []
        for p in periods {
            let source = volumeMode ? (p.volumeByIndicator ?? [:]) : (p.indicators ?? [:])
            for k in source.keys {
                if !seen.contains(k) { seen.insert(k); keys.append(k) }
            }
        }
        return keys
    }

    private func formatBabyXLabel(_ p: TimeSeriesPeriod, period: String) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        guard let date = f.date(from: p.start) else { return "" }
        switch period {
        case "weekly":
            f.dateFormat = "MMM d"
            return f.string(from: date)
        case "monthly":
            f.dateFormat = "MMM"
            return f.string(from: date)
        default:
            return "\(Calendar.current.component(.day, from: date))"
        }
    }

    // MARK: - Heatmap (green GitHub-style grid)

    @ViewBuilder
    private var heatmapCard: some View {
        let weeks = buildHeatmapWeeks()
        let maxCount = heatmap.map(\.count).max() ?? 1

        VStack(alignment: .leading, spacing: 10) {
            Text("Activity").font(.headline)

            HStack(alignment: .top, spacing: 4) {
                let dayLabels = ["S", "M", "T", "W", "T", "F", "S"]
                VStack(spacing: 3) {
                    ForEach(Array(dayLabels.enumerated()), id: \.offset) { _, lbl in
                        Text(lbl)
                            .font(.system(size: 8))
                            .foregroundColor(DesignColors.textSecondary)
                            .frame(width: 10, height: 10)
                    }
                }

                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(alignment: .top, spacing: 3) {
                        ForEach(Array(weeks.enumerated()), id: \.offset) { _, week in
                            VStack(spacing: 3) {
                                ForEach(week, id: \.date) { entry in
                                    RoundedRectangle(cornerRadius: 2)
                                        .fill(heatmapColor(count: entry.count, maxCount: maxCount))
                                        .frame(width: 10, height: 10)
                                }
                                if week.count < 7 {
                                    ForEach(0..<(7 - week.count), id: \.self) { _ in
                                        Color.clear.frame(width: 10, height: 10)
                                    }
                                }
                            }
                        }
                    }
                }
            }

            HStack(spacing: 4) {
                Text("Less").font(.caption2).foregroundColor(.secondary)
                let lMax = max(4, maxCount)
                ForEach([0, 1, 2, 3, 4], id: \.self) { i in
                    let sample = i == 0 ? 0 : Int(ceil(Double(lMax) * Double(i) / 4.0))
                    RoundedRectangle(cornerRadius: 2)
                        .fill(heatmapColor(count: sample, maxCount: lMax))
                        .frame(width: 10, height: 10)
                }
                Text("More").font(.caption2).foregroundColor(.secondary)
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    private func heatmapColor(count: Int, maxCount: Int) -> Color {
        if count == 0 { return Color(hexUnsafe: "e8e5df") }
        let intensity = maxCount > 0 ? Double(count) / Double(maxCount) : 0
        if intensity <= 0.25 { return Color(hexUnsafe: "c6e48b") }
        if intensity <= 0.50 { return Color(hexUnsafe: "7bc96f") }
        if intensity <= 0.75 { return Color(hexUnsafe: "239a3b") }
        return Color(hexUnsafe: "196127")
    }

    private func buildHeatmapWeeks() -> [[HeatmapEntry]] {
        var cal = Calendar(identifier: .gregorian)
        cal.firstWeekday = 1
        let comps = cal.dateComponents([.year, .month, .day], from: Date())
        guard let today = cal.date(from: comps) else { return [] }

        let weekday = cal.component(.weekday, from: today)
        let dayOfWeek = weekday - 1
        guard let startDate = cal.date(byAdding: .day, value: -(dayOfWeek + 19 * 7), to: today) else { return [] }

        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        f.locale = Locale(identifier: "en_US_POSIX")
        let cellMap = Dictionary(uniqueKeysWithValues: heatmap.map { ($0.date, $0.count) })

        var weeks: [[HeatmapEntry]] = []
        var current = startDate
        while current <= today {
            var week: [HeatmapEntry] = []
            for _ in 0..<7 {
                guard current <= today else { break }
                let ds = f.string(from: current)
                week.append(HeatmapEntry(date: ds, count: cellMap[ds] ?? 0))
                current = cal.date(byAdding: .day, value: 1, to: current) ?? current
            }
            if !week.isEmpty { weeks.append(week) }
        }
        return weeks
    }

    // MARK: - Busy Hours (horizontal bars, matching PWA)

    @ViewBuilder
    private var busyHoursCard: some View {
        let maxCount = busyHours.map(\.count).max() ?? 1
        VStack(alignment: .leading, spacing: 8) {
            Text("Busy Hours").font(.headline)

            // Date range label
            if !busyHoursStart.isEmpty, !busyHoursEnd.isEmpty {
                Text(formatRangeLabel(busyHoursStart, busyHoursEnd))
                    .font(.caption).foregroundColor(DesignColors.textSecondary)
            }

            // Filters
            HStack(spacing: 8) {
                Picker("Chore", selection: $bhChoreId) {
                    Text("All chores").tag(nil as Int?)
                    ForEach(state.chores) { c in
                        Text(c.name).tag(c.id as Int?)
                    }
                }
                .pickerStyle(.menu)
                .font(.caption)
                .onChange(of: bhChoreId) { Task { await loadBusyHoursFiltered() } }

                Picker("Member", selection: $bhUserId) {
                    Text("All members").tag(nil as Int?)
                    ForEach(state.members) { m in
                        Text(m.displayName.isEmpty ? m.email : m.displayName).tag(m.userId as Int?)
                    }
                }
                .pickerStyle(.menu)
                .font(.caption)
                .onChange(of: bhUserId) { Task { await loadBusyHoursFiltered() } }
            }

            HStack(spacing: 8) {
                DatePicker("", selection: Binding(
                    get: { parseDateStr(bhFilterStart) ?? Date() },
                    set: { bhFilterStart = formatDateStr($0); Task { await loadBusyHoursFiltered() } }
                ), displayedComponents: .date)
                .labelsHidden()
                .environment(\.locale, Locale(identifier: "en_US_POSIX"))
                Text("–")
                    .font(.caption).foregroundColor(DesignColors.textSecondary)
                DatePicker("", selection: Binding(
                    get: { parseDateStr(bhFilterEnd) ?? Date() },
                    set: { bhFilterEnd = formatDateStr($0); Task { await loadBusyHoursFiltered() } }
                ), displayedComponents: .date)
                .labelsHidden()
                .environment(\.locale, Locale(identifier: "en_US_POSIX"))
            }

            ForEach(busyHours, id: \.hour) { entry in
                HStack(spacing: 6) {
                    Text(formatHour(entry.hour))
                        .font(.system(size: 11, weight: .medium, design: .monospaced))
                        .foregroundColor(DesignColors.textSecondary)
                        .frame(width: 28, alignment: .trailing)
                    GeometryReader { geo in
                        ZStack(alignment: .leading) {
                            RoundedRectangle(cornerRadius: 3)
                                .fill(DesignColors.surfaceSecondary)
                            let pct = maxCount > 0 ? CGFloat(entry.count) / CGFloat(maxCount) : 0
                            RoundedRectangle(cornerRadius: 3)
                                .fill(DesignColors.primary.opacity(0.75))
                                .frame(width: geo.size.width * pct)
                        }
                    }
                    .frame(height: 10)
                    Text("\(entry.count)")
                        .font(.system(size: 11))
                        .foregroundColor(DesignColors.textSecondary)
                        .frame(width: 24, alignment: .trailing)
                }
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    private func formatHour(_ h: Int) -> String {
        if h == 0 { return "12a" }
        if h < 12 { return "\(h)a" }
        if h == 12 { return "12p" }
        return "\(h - 12)p"
    }

    private func formatRangeLabel(_ start: String, _ end: String) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        guard let s = f.date(from: start), let e = f.date(from: end) else { return "" }
        f.dateFormat = "MMM d"
        return "\(f.string(from: s)) – \(f.string(from: e))"
    }

    private func formatWeekLabel() -> String {
        let cal = Calendar.current
        let now = Date()
        let weekday = cal.component(.weekday, from: now)
        let daysFromMonday = (weekday + 5) % 7
        guard let monday = cal.date(byAdding: .day, value: -daysFromMonday, to: now),
              let sunday = cal.date(byAdding: .day, value: 6, to: monday) else { return "" }
        let f = DateFormatter()
        f.dateFormat = "MMM d"
        return "\(f.string(from: monday)) – \(f.string(from: sunday))"
    }

    private func parseDateStr(_ s: String) -> Date? {
        guard !s.isEmpty else { return nil }
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        return f.date(from: s)
    }

    private func formatDateStr(_ d: Date) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        return f.string(from: d)
    }

    // MARK: - Leaderboard (avatar circle + name + count)

    @ViewBuilder
    private var leaderboardSection: some View {
        VStack(spacing: 8) {
            HStack {
                Text("Leaderboard").font(.headline)
                Spacer()
                statsPeriodToggle(period: leaderboardPeriod, section: "leaderboard")
            }
            Text(leaderboardRangeLabel)
                .font(.caption).foregroundColor(DesignColors.textSecondary)
                .frame(maxWidth: .infinity, alignment: .leading)
            if currentLeaderboard.isEmpty {
                Text(leaderboardEmptyLabel)
                    .font(.subheadline).foregroundColor(DesignColors.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .center)
            } else {
                ForEach(Array(currentLeaderboard.enumerated()), id: \.offset) { _, entry in
                    let member = state.members.first { $0.userId == entry.userId }
                    let name = member.map { $0.displayName.isEmpty ? $0.email : $0.displayName } ?? "User \(entry.userId)"
                    let initial = String(name.prefix(1)).uppercased()
                    let colorHex = (member?.avatarColor ?? "#19323C")
                        .trimmingCharacters(in: CharacterSet(charactersIn: "#"))
                    HStack(spacing: 8) {
                        Circle()
                            .fill(Color(hexUnsafe: colorHex))
                            .frame(width: 28, height: 28)
                            .overlay(
                                Text(initial)
                                    .font(.system(size: 12, weight: .semibold))
                                    .foregroundColor(.white)
                            )
                        Text(name).font(.subheadline).lineLimit(1)
                        Spacer()
                        Text("\(entry.count) chores")
                            .font(.subheadline).foregroundColor(DesignColors.textSecondary)
                    }
                }
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    @ViewBuilder
    private func statsPeriodToggle(period: String, section: String) -> some View {
        let labels: [String: String] = ["day": "Day", "week": "Week", "month": "Month", "all": "All"]
        let periods = ["day", "week", "month", "all"]
        HStack(spacing: 6) {
            ForEach(periods, id: \.self) { p in
                Button {
                    Task { await setStatsPeriod(section: section, period: p) }
                } label: {
                    Text(labels[p] ?? p)
                        .font(.caption)
                        .padding(.horizontal, 10).padding(.vertical, 4)
                        .background(period == p ? DesignColors.primary : DesignColors.surfaceSecondary)
                        .foregroundColor(period == p ? .white : DesignColors.textPrimary)
                        .clipShape(Capsule())
                }
            }
        }
    }

    // MARK: - Top Chores (user pills + ranked list)

    @ViewBuilder
    private var topChoresSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Top Chores").font(.headline)
                Spacer()
                statsPeriodToggle(period: topChoresPeriod, section: "top-chores")
            }

            if state.members.count > 1 {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(state.members) { member in
                            let name = member.displayName.isEmpty ? member.email : member.displayName
                            let initial = String(name.prefix(1)).uppercased()
                            let colorHex = member.avatarColor
                                .trimmingCharacters(in: CharacterSet(charactersIn: "#"))
                            let isActive = topChoresUserId == member.userId
                            Button {
                                Task { await setTopChoresUser(member.userId) }
                            } label: {
                                HStack(spacing: 4) {
                                    Circle()
                                        .fill(Color(hexUnsafe: colorHex))
                                        .frame(width: 20, height: 20)
                                        .overlay(
                                            Text(initial)
                                                .font(.system(size: 10, weight: .semibold))
                                                .foregroundColor(.white)
                                        )
                                    Text(name).font(.caption).lineLimit(1)
                                }
                                .padding(.horizontal, 10).padding(.vertical, 6)
                                .background(isActive ? DesignColors.primary : DesignColors.surfaceSecondary)
                                .foregroundColor(isActive ? .white : DesignColors.textPrimary)
                                .clipShape(Capsule())
                            }
                        }
                    }
                }
            }

            let chores = currentTopChores
            if chores.isEmpty {
                Text(topChoresEmptyLabel)
                    .font(.subheadline).foregroundColor(DesignColors.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .center)
            } else {
                let maxCount = chores.map(\.count).max() ?? 1
                ForEach(Array(chores.enumerated()), id: \.element.choreId) { idx, chore in
                    topChoreRow(chore, rank: idx + 1, maxCount: maxCount)
                }
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    @ViewBuilder
    private func topChoreRow(_ chore: TopChore, rank: Int, maxCount: Int) -> some View {
        HStack(spacing: 8) {
            Text("\(rank)")
                .font(.system(size: 12)).fontWeight(.bold)
                .foregroundColor(DesignColors.textSecondary)
                .frame(width: 18)
            Text(chore.choreIcon).font(.body).frame(width: 24)
            Text(chore.choreName)
                .font(.subheadline).fontWeight(.medium).lineLimit(1)
            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    RoundedRectangle(cornerRadius: 3)
                        .fill(DesignColors.surfaceSecondary)
                    let pct = maxCount > 0 ? CGFloat(chore.count) / CGFloat(maxCount) : 0
                    RoundedRectangle(cornerRadius: 3)
                        .fill(DesignColors.accent.opacity(0.8))
                        .frame(width: geo.size.width * pct)
                }
                .frame(height: 6)
                .frame(maxHeight: .infinity)
            }
            Text("\(chore.count)")
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(DesignColors.accent)
                .frame(width: 40, alignment: .center)
        }
    }

    // MARK: - Categories (horizontal progress bars)

    @ViewBuilder
    private func categoriesCard(_ entries: [BreakdownEntry]) -> some View {
        let barMax = entries.map(\.count).max() ?? 1
        VStack(spacing: 8) {
            Text("Categories").font(.headline)
                .frame(maxWidth: .infinity, alignment: .leading)
            Text(formatWeekLabel())
                .font(.caption).foregroundColor(DesignColors.textSecondary)
                .frame(maxWidth: .infinity, alignment: .leading)
            ForEach(entries, id: \.category) { entry in
                HStack(spacing: 8) {
                    Text(entry.category).font(.subheadline)
                        .frame(width: 90, alignment: .leading).lineLimit(1)
                    GeometryReader { geo in
                        ZStack(alignment: .leading) {
                            RoundedRectangle(cornerRadius: 4).fill(DesignColors.surfaceSecondary)
                            let pct = barMax > 0 ? CGFloat(entry.count) / CGFloat(barMax) : 0
                            RoundedRectangle(cornerRadius: 4)
                                .fill(DesignColors.primary.opacity(0.75))
                                .frame(width: geo.size.width * pct)
                        }
                        .frame(maxHeight: .infinity, alignment: .center)
                    }
                    .frame(height: 18)
                    Text("\(entry.count)").font(.subheadline).foregroundColor(DesignColors.textSecondary)
                        .frame(width: 28, alignment: .trailing)
                }
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    // MARK: - Per-Chore Stats (with date selectors and collapsible cards)

    @ViewBuilder
    private var choreStatsSection: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("Chores").font(.headline)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.bottom, 8)

            // Date range label + pickers
            if !choreStatsStart.isEmpty, !choreStatsEnd.isEmpty {
                Text(formatRangeLabel(choreStatsStart, choreStatsEnd))
                    .font(.caption).foregroundColor(DesignColors.textSecondary)
                    .padding(.bottom, 4)
            }
            HStack(spacing: 8) {
                DatePicker("", selection: Binding(
                    get: { parseDateStr(csFilterStart) ?? Date() },
                    set: { csFilterStart = formatDateStr($0); Task { await loadChoreStatsFiltered() } }
                ), displayedComponents: .date)
                .labelsHidden()
                .environment(\.locale, Locale(identifier: "en_US_POSIX"))
                Text("–")
                    .font(.caption).foregroundColor(DesignColors.textSecondary)
                DatePicker("", selection: Binding(
                    get: { parseDateStr(csFilterEnd) ?? Date() },
                    set: { csFilterEnd = formatDateStr($0); Task { await loadChoreStatsFiltered() } }
                ), displayedComponents: .date)
                .labelsHidden()
                .environment(\.locale, Locale(identifier: "en_US_POSIX"))
            }
            .padding(.bottom, 10)

            ForEach(Array(activeChoreStats.enumerated()), id: \.element.choreId) { idx, cs in
                let hasDetails = cs.hasIndicators || cs.hasVolume
                if hasDetails {
                    DisclosureGroup {
                        choreStatDetails(cs)
                            .padding(.top, 6)
                    } label: {
                        choreStatHeader(cs)
                    }
                } else {
                    choreStatHeader(cs)
                }
                if idx < activeChoreStats.count - 1 {
                    Divider().padding(.vertical, 4)
                }
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    @ViewBuilder
    private func choreStatHeader(_ cs: ChoreStat) -> some View {
        HStack(spacing: 8) {
            Text(cs.choreIcon).font(.body)
            Text(cs.choreName).font(.subheadline).fontWeight(.semibold).lineLimit(1)
            Spacer()
            Text("\(cs.totalThisWeek)/wk")
                .font(.caption).foregroundColor(DesignColors.primary)
            Text("\(cs.totalThisMonth)/mo")
                .font(.caption).foregroundColor(DesignColors.textSecondary)
        }
        .padding(.vertical, 2)
    }

    @ViewBuilder
    private func choreStatDetails(_ cs: ChoreStat) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            // Indicator chips
            if cs.hasIndicators, let indCounts = cs.indicatorCounts, !indCounts.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Indicators")
                        .font(.caption).foregroundColor(DesignColors.textSecondary)
                    let pairs = indCounts.sorted(by: { $0.key < $1.key })
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 4) {
                            ForEach(pairs, id: \.key) { kv in
                                Text("\(kv.key): \(kv.value)")
                                    .font(.caption2)
                                    .padding(.horizontal, 6).padding(.vertical, 2)
                                    .background(DesignColors.primary.opacity(0.12))
                                    .foregroundColor(DesignColors.primary)
                                    .clipShape(Capsule())
                            }
                        }
                    }
                }
            }

            // Volume chart
            if cs.hasVolume, let volHistory = cs.volumeHistory, !volHistory.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Volume (30d)")
                        .font(.caption).foregroundColor(DesignColors.textSecondary)
                    let maxVol = volHistory.map(\.totalML).max() ?? 1
                    HStack(alignment: .bottom, spacing: 2) {
                        ForEach(volHistory.suffix(14), id: \.date) { point in
                            let h = maxVol > 0 ? CGFloat(point.totalML) / CGFloat(maxVol) * 40 : 1
                            RoundedRectangle(cornerRadius: 2)
                                .fill(DesignColors.primary.opacity(0.6))
                                .frame(width: 6, height: max(h, 1))
                        }
                    }
                    .frame(height: 42)
                    if let avg = cs.avgVolume {
                        Text("Avg \(Int(avg.rounded()))mL / feed")
                            .font(.caption2).foregroundColor(DesignColors.textSecondary)
                    }
                }
            }
        }
    }

    // MARK: - Weekly Recap

    @ViewBuilder
    private func recapCard(_ recap: Recap) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Weekly Recap").font(.headline)
                .frame(maxWidth: .infinity, alignment: .leading)
            Text("This week you completed \(recap.totalChores) chores.")
                .font(.subheadline)
            if !recap.mostActiveDay.isEmpty {
                Text("Most active: \(recap.mostActiveDay)")
                    .font(.subheadline).foregroundColor(DesignColors.textSecondary)
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    // MARK: - Load

    private func loadStats() async {
        isLoading = true

        let ovTask: Task<OverviewResponse?, Never> = Task {
            try? await environment.apiClient.get("/api/stats/overview")
        }
        let hmTask: Task<HeatmapResponse?, Never> = Task {
            try? await environment.apiClient.get("/api/stats/heatmap")
        }
        let bhTask: Task<BusyHoursResponse?, Never> = Task {
            try? await environment.apiClient.get("/api/stats/busy-hours")
        }
        let tcTask: Task<TopChoresResponse?, Never> = Task {
            try? await environment.apiClient.get(
                "/api/stats/top-chores",
                query: [URLQueryItem(name: "period", value: topChoresPeriod)]
            )
        }
        let lbTask: Task<LeaderboardResponse?, Never> = Task {
            try? await environment.apiClient.get(
                "/api/stats/leaderboard",
                query: [URLQueryItem(name: "period", value: leaderboardPeriod)]
            )
        }
        let csTask: Task<ChoreStatsResponse?, Never> = Task {
            try? await environment.apiClient.get("/api/stats/chores")
        }

        let ov = await ovTask.value
        let hm = await hmTask.value
        let bh = await bhTask.value
        let tc = await tcTask.value
        let lb = await lbTask.value
        let cs = await csTask.value

        overview = ov?.overview
        heatmap = hm?.heatmap ?? []
        busyHours = bh?.busyHours ?? []
        busyHoursStart = bh?.start ?? ""
        busyHoursEnd = bh?.end ?? ""
        if let lb = lb {
            leaderboardByPeriod[leaderboardPeriod] = lb
        }
        topChoresByUserAndPeriod["0-\(topChoresPeriod)"] = tc?.topChores ?? []
        choreStats = cs?.choreStats ?? []
        choreStatsStart = cs?.start ?? ""
        choreStatsEnd = cs?.end ?? ""

        // Load baby time series
        await loadBabyTimeSeries()

        isLoading = false
    }

    private func loadBabyTimeSeries() async {
        if let fb = feedBabyChore {
            if let data: TimeSeriesResponse = try? await environment.apiClient.get(
                "/api/stats/chores/\(fb.id)/time-series",
                query: [URLQueryItem(name: "period", value: feedBabyPeriod)]
            ) {
                feedBabyTS = data.timeSeries
            }
        }
        if let cb = changeBabyChore {
            if let data: TimeSeriesResponse = try? await environment.apiClient.get(
                "/api/stats/chores/\(cb.id)/time-series",
                query: [URLQueryItem(name: "period", value: changeBabyPeriod)]
            ) {
                changeBabyTS = data.timeSeries
            }
        }
    }

    private func setBabyPeriod(_ period: String, type: String) async {
        if type == "feed" {
            guard period != feedBabyPeriod else { return }
            feedBabyPeriod = period
            selectedFeedBar = nil
        } else {
            guard period != changeBabyPeriod else { return }
            changeBabyPeriod = period
            selectedChangeBar = nil
        }
        await loadBabyTimeSeries()
    }

    private func loadBusyHoursFiltered() async {
        var query: [URLQueryItem] = []
        if let cid = bhChoreId { query.append(URLQueryItem(name: "choreId", value: "\(cid)")) }
        if let uid = bhUserId { query.append(URLQueryItem(name: "userId", value: "\(uid)")) }
        if !bhFilterStart.isEmpty { query.append(URLQueryItem(name: "start", value: bhFilterStart)) }
        if !bhFilterEnd.isEmpty { query.append(URLQueryItem(name: "end", value: bhFilterEnd)) }

        if let data: BusyHoursResponse = try? await environment.apiClient.get(
            "/api/stats/busy-hours", query: query
        ) {
            busyHours = data.busyHours
            busyHoursStart = data.start
            busyHoursEnd = data.end
        }
    }

    private func loadChoreStatsFiltered() async {
        var query: [URLQueryItem] = []
        if !csFilterStart.isEmpty { query.append(URLQueryItem(name: "start", value: csFilterStart)) }
        if !csFilterEnd.isEmpty { query.append(URLQueryItem(name: "end", value: csFilterEnd)) }

        if let data: ChoreStatsResponse = try? await environment.apiClient.get(
            "/api/stats/chores", query: query
        ) {
            choreStats = data.choreStats
            choreStatsStart = data.start
            choreStatsEnd = data.end
        }
    }

    private func setTopChoresUser(_ userId: Int) async {
        if topChoresUserId == userId {
            topChoresUserId = nil
            return
        }
        topChoresUserId = userId
        let key = "\(userId)-\(topChoresPeriod)"
        guard topChoresByUserAndPeriod[key] == nil else { return }
        if let tc: TopChoresResponse = try? await environment.apiClient.get(
            "/api/stats/top-chores",
            query: [
                URLQueryItem(name: "userId", value: "\(userId)"),
                URLQueryItem(name: "period", value: topChoresPeriod),
            ]
        ) {
            topChoresByUserAndPeriod[key] = tc.topChores
        }
    }

    private func setStatsPeriod(section: String, period: String) async {
        if section == "leaderboard" {
            guard period != leaderboardPeriod else { return }
            leaderboardPeriod = period
            if leaderboardByPeriod[period] == nil {
                if let resp: LeaderboardResponse = try? await environment.apiClient.get(
                    "/api/stats/leaderboard",
                    query: [URLQueryItem(name: "period", value: period)]
                ) {
                    leaderboardByPeriod[period] = resp
                }
            }
        } else if section == "top-chores" {
            guard period != topChoresPeriod else { return }
            topChoresPeriod = period
            let uid = topChoresUserId ?? 0
            let key = "\(uid)-\(period)"
            if topChoresByUserAndPeriod[key] == nil {
                var query = [URLQueryItem(name: "period", value: period)]
                if let uid = topChoresUserId {
                    query.append(URLQueryItem(name: "userId", value: "\(uid)"))
                }
                if let tc: TopChoresResponse = try? await environment.apiClient.get(
                    "/api/stats/top-chores",
                    query: query
                ) {
                    topChoresByUserAndPeriod[key] = tc.topChores
                }
            }
        }
    }
}

