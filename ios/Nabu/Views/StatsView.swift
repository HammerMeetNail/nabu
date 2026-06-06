import SwiftUI

struct StatsView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var overview: StatsOverview?
    @State private var heatmap: [HeatmapEntry] = []
    @State private var busyHours: [BusyHour] = []
    @State private var busyHoursStart: String = ""
    @State private var busyHoursEnd: String = ""
    @State private var topChoresByUser: [Int: [TopChore]] = [:]
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

    // Chart tap state: tracks which bar index is selected per chart
    @State private var selectedFeedBar: Int? = nil
    @State private var selectedChangeBar: Int? = nil

    // Chore stats filter state
    @State private var csFilterStart: String = ""
    @State private var csFilterEnd: String = ""

    // Baby care
    @State private var babyPeriod: String = "daily"
    @State private var feedBabyTS: ChoreTimeSeries?
    @State private var changeBabyTS: ChoreTimeSeries?

    private var currentTopChores: [TopChore] {
        topChoresByUser[topChoresUserId ?? 0] ?? []
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
                            if let ov = overview { leaderboardCard(ov.leaderboard) }
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
        let periodLabels = ["daily": "Daily", "weekly": "Weekly", "monthly": "Monthly"]

        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Baby").font(.headline)
                Spacer()
                HStack(spacing: 4) {
                    ForEach(["daily", "weekly", "monthly"], id: \.self) { p in
                        Button {
                            Task { await setBabyPeriod(p) }
                        } label: {
                            Text(periodLabels[p] ?? p)
                                .font(.caption).fontWeight(.medium)
                                .padding(.horizontal, 10).padding(.vertical, 5)
                                .background(babyPeriod == p ? DesignColors.primary : DesignColors.surfaceSecondary)
                                .foregroundColor(babyPeriod == p ? .white : DesignColors.textPrimary)
                                .clipShape(Capsule())
                        }
                    }
                }
            }

            VStack(spacing: 12) {
                if let ts = feedBabyTS {
                    babyColumn(ts, type: "feed")
                        .frame(maxWidth: .infinity)
                }
                if let ts = changeBabyTS {
                    babyColumn(ts, type: "change")
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
    private func babyColumn(_ ts: ChoreTimeSeries, type: String) -> some View {
        let isVolume = type == "feed"

        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 4) {
                Text(ts.choreIcon).font(.body)
                Text(ts.choreName).font(.subheadline).fontWeight(.semibold).lineLimit(1)
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
                babyVolumeChart(ts.periods)
            } else {
                babyIndicatorChart(ts.periods)
            }
        }
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
    private func babyVolumeChart(_ periods: [TimeSeriesPeriod]) -> some View {
        let stackColors: [String: Color] = [
            "🍼 formula": Color(hexUnsafe: "EC4899"),
            "🤱 breast": Color(hexUnsafe: "F59E0B"),
        ]
        let stackKeys = extractStackKeys(periods, volumeMode: true)
        let maxML = max(1, periods.map { $0.totalML ?? 0 }.max() ?? 1)
        let colW: CGFloat = 18
        let spacing: CGFloat = 4
        let chartH: CGFloat = 80

        VStack(alignment: .leading, spacing: 4) {
            ScrollView(.horizontal, showsIndicators: false) {
                VStack(alignment: .leading, spacing: 0) {
                    HStack(alignment: .bottom, spacing: 0) {
                        Text("mL")
                            .font(.system(size: 8))
                            .foregroundColor(DesignColors.textSecondary)
                            .frame(width: 24, height: chartH + 14, alignment: .bottom)
                            .padding(.bottom, 2)

                        HStack(alignment: .bottom, spacing: spacing) {
                            ForEach(Array(periods.enumerated()), id: \.offset) { i, p in
                                babyVolumeBarColumn(p, i: i, maxML: maxML, colW: colW, chartH: chartH, stackKeys: stackKeys, stackColors: stackColors, selectedBar: selectedFeedBar, isSelected: selectedFeedBar == i)
                                    .onTapGesture {
                                        selectedFeedBar = selectedFeedBar == i ? nil : i
                                    }
                            }
                        }
                    }

                    babyVolumeLegend(periods)
                }
            }
        }
    }

    @ViewBuilder
    private func babyVolumeBarColumn(_ p: TimeSeriesPeriod, i: Int, maxML: Int, colW: CGFloat, chartH: CGFloat, stackKeys: [String], stackColors: [String: Color], selectedBar: Int?, isSelected: Bool) -> some View {
        let totalML = p.totalML ?? 0
        let valText = volumeBarLabel(p, stackKeys: stackKeys)
        VStack(spacing: 2) {
            // Value label above bar (shown on selection)
            Text(valText)
                .font(.system(size: 8, weight: .bold))
                .foregroundColor(.white)
                .lineLimit(3)
                .fixedSize(horizontal: false, vertical: true)
                .frame(width: colW * 3)
                .opacity(isSelected ? 1 : 0)
                .offset(y: isSelected ? 0 : 4)

            ZStack(alignment: .bottom) {
                Rectangle()
                    .fill(Color.clear)
                    .frame(width: colW, height: chartH)

                if totalML > 0 {
                    stackedVolumeBars(p, totalML: totalML, maxML: maxML, colW: colW, chartH: chartH, stackKeys: stackKeys, stackColors: stackColors)
                }
            }
            .frame(width: colW, height: chartH)

            Text(formatBabyXLabel(p, period: babyPeriod))
                .font(.system(size: 7))
                .foregroundColor(DesignColors.textSecondary)
                .lineLimit(1)
                .frame(width: colW)
        }
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
            .padding(.leading, 28)
        }
    }

    // MARK: - Baby Indicator Chart

    @ViewBuilder
    private func babyIndicatorChart(_ periods: [TimeSeriesPeriod]) -> some View {
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
        let colW: CGFloat = 18
        let spacing: CGFloat = 4
        let chartH: CGFloat = 80

        VStack(alignment: .leading, spacing: 4) {
            ScrollView(.horizontal, showsIndicators: false) {
                VStack(alignment: .leading, spacing: 0) {
                    HStack(alignment: .bottom, spacing: 0) {
                        Text("cnt")
                            .font(.system(size: 8))
                            .foregroundColor(DesignColors.textSecondary)
                            .frame(width: 24, height: chartH + 14, alignment: .bottom)
                            .padding(.bottom, 2)

                        HStack(alignment: .bottom, spacing: spacing) {
                            ForEach(Array(periods.enumerated()), id: \.offset) { i, p in
                                babyIndicatorBarColumn(p, i: i, maxCount: maxCount, colW: colW, chartH: chartH, indicatorKeys: indicatorKeys, indicatorColors: indicatorColors, selectedBar: selectedChangeBar, isSelected: selectedChangeBar == i)
                                    .onTapGesture {
                                        selectedChangeBar = selectedChangeBar == i ? nil : i
                                    }
                            }
                        }
                    }

                    babyIndicatorLegend(periods, indicatorKeys: indicatorKeys, indicatorColors: indicatorColors)
                }
            }
        }
    }

    @ViewBuilder
    private func babyIndicatorBarColumn(_ p: TimeSeriesPeriod, i: Int, maxCount: Int, colW: CGFloat, chartH: CGFloat, indicatorKeys: [String], indicatorColors: [String: Color], selectedBar: Int?, isSelected: Bool) -> some View {
        let periodTotal = indicatorKeys.reduce(0) { $0 + (p.indicators?[$1] ?? 0) }
        let valText = indicatorBarLabel(p, indicatorKeys: indicatorKeys)

        VStack(spacing: 2) {
            Text(valText)
                .font(.system(size: 8, weight: .bold))
                .foregroundColor(.white)
                .lineLimit(3)
                .fixedSize(horizontal: false, vertical: true)
                .frame(width: colW * 3)
                .opacity(isSelected ? 1 : 0)
                .offset(y: isSelected ? 0 : 4)

            ZStack(alignment: .bottom) {
                Rectangle()
                    .fill(Color.clear)
                    .frame(width: colW, height: chartH)

                if periodTotal > 0 {
                    stackedIndicatorBars(p, maxCount: maxCount, colW: colW, chartH: chartH, indicatorKeys: indicatorKeys, indicatorColors: indicatorColors)
                }
            }
            .frame(width: colW, height: chartH)

            Text(formatBabyXLabel(p, period: babyPeriod))
                .font(.system(size: 7))
                .foregroundColor(DesignColors.textSecondary)
                .lineLimit(1)
                .frame(width: colW)
        }
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
            .padding(.leading, 28)
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
    private func leaderboardCard(_ entries: [LeaderboardEntry]) -> some View {
        VStack(spacing: 8) {
            Text("Leaderboard").font(.headline)
                .frame(maxWidth: .infinity, alignment: .leading)
            Text(formatWeekLabel())
                .font(.caption).foregroundColor(DesignColors.textSecondary)
                .frame(maxWidth: .infinity, alignment: .leading)
            if entries.isEmpty {
                Text("No chores this week")
                    .font(.subheadline).foregroundColor(DesignColors.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .center)
            } else {
                ForEach(Array(entries.enumerated()), id: \.offset) { _, entry in
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

    // MARK: - Top Chores (user pills + ranked list)

    @ViewBuilder
    private var topChoresSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Top Chores").font(.headline)
                .frame(maxWidth: .infinity, alignment: .leading)

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
                Text("No data yet")
                    .font(.subheadline).foregroundColor(DesignColors.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .center)
            } else {
                HStack(spacing: 0) {
                    Spacer()
                    HStack(spacing: 8) {
                        Text("Day").font(.system(size: 10)).foregroundColor(DesignColors.textSecondary)
                            .frame(width: 40, alignment: .center)
                        Text("Wk").font(.system(size: 10)).foregroundColor(DesignColors.textSecondary)
                            .frame(width: 40, alignment: .center)
                        Text("Mo").font(.system(size: 10)).foregroundColor(DesignColors.textSecondary)
                            .frame(width: 40, alignment: .center)
                    }
                }
                .padding(.bottom, 2)
                let maxMonth = chores.map(\.thisMonth).max() ?? 1
                ForEach(Array(chores.enumerated()), id: \.element.choreId) { idx, chore in
                    topChoreRow(chore, rank: idx + 1, maxMonth: maxMonth)
                }
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    @ViewBuilder
    private func topChoreRow(_ chore: TopChore, rank: Int, maxMonth: Int) -> some View {
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
                    let pct = maxMonth > 0 ? CGFloat(chore.thisMonth) / CGFloat(maxMonth) : 0
                    RoundedRectangle(cornerRadius: 3)
                        .fill(DesignColors.accent.opacity(0.8))
                        .frame(width: geo.size.width * pct)
                }
                .frame(height: 6)
                .frame(maxHeight: .infinity)
            }
            Text("\(chore.today)")
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(DesignColors.textPrimary)
                .frame(width: 40, alignment: .center)
            Text("\(chore.thisWeek)")
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(DesignColors.textSecondary)
                .frame(width: 40, alignment: .center)
            Text("\(chore.thisMonth)")
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
            try? await environment.apiClient.get("/api/stats/top-chores")
        }
        let csTask: Task<ChoreStatsResponse?, Never> = Task {
            try? await environment.apiClient.get("/api/stats/chores")
        }

        let ov = await ovTask.value
        let hm = await hmTask.value
        let bh = await bhTask.value
        let tc = await tcTask.value
        let cs = await csTask.value

        overview = ov?.overview
        heatmap = hm?.heatmap ?? []
        busyHours = bh?.busyHours ?? []
        busyHoursStart = bh?.start ?? ""
        busyHoursEnd = bh?.end ?? ""
        topChoresByUser[0] = tc?.topChores ?? []
        choreStats = cs?.choreStats ?? []
        choreStatsStart = cs?.start ?? ""
        choreStatsEnd = cs?.end ?? ""

        // Load baby time series
        await loadBabyTimeSeries()

        isLoading = false
    }

    private func loadBabyTimeSeries() async {
        let period = babyPeriod
        if let fb = feedBabyChore {
            if let data: TimeSeriesResponse = try? await environment.apiClient.get(
                "/api/stats/chores/\(fb.id)/time-series",
                query: [URLQueryItem(name: "period", value: period)]
            ) {
                feedBabyTS = data.timeSeries
            }
        }
        if let cb = changeBabyChore {
            if let data: TimeSeriesResponse = try? await environment.apiClient.get(
                "/api/stats/chores/\(cb.id)/time-series",
                query: [URLQueryItem(name: "period", value: period)]
            ) {
                changeBabyTS = data.timeSeries
            }
        }
    }

    private func setBabyPeriod(_ period: String) async {
        guard period != babyPeriod else { return }
        babyPeriod = period
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
        guard topChoresByUser[userId] == nil else { return }
        if let tc: TopChoresResponse = try? await environment.apiClient.get(
            "/api/stats/top-chores",
            query: [URLQueryItem(name: "userId", value: "\(userId)")]
        ) {
            topChoresByUser[userId] = tc.topChores
        }
    }
}

