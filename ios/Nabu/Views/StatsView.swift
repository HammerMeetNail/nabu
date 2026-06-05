import SwiftUI

struct StatsView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var overview: StatsOverview?
    @State private var heatmap: [HeatmapEntry] = []
    @State private var busyHours: [BusyHour] = []
    @State private var topChoresByUser: [Int: [TopChore]] = [:]  // key 0 = all users
    @State private var choreStats: [ChoreStat] = []
    @State private var topChoresUserId: Int? = nil
    @State private var isLoading = true

    private var currentTopChores: [TopChore] {
        topChoresByUser[topChoresUserId ?? 0] ?? []
    }

    private var activeChoreStats: [ChoreStat] {
        choreStats.filter { $0.totalThisWeek > 0 || $0.totalThisMonth > 0 }
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

    // MARK: - Heatmap (green GitHub-style grid)

    @ViewBuilder
    private var heatmapCard: some View {
        let weeks = buildHeatmapWeeks()
        let maxCount = heatmap.map(\.count).max() ?? 1

        VStack(alignment: .leading, spacing: 10) {
            Text("Activity").font(.headline)

            HStack(alignment: .top, spacing: 4) {
                // Row labels: Sun–Sat
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

            // Legend
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

        // Match JS: dayOfWeek = today.getDay() (0=Sun); startDate = today - (dayOfWeek + 19*7)
        let weekday = cal.component(.weekday, from: today)  // 1=Sun, 7=Sat
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

    // MARK: - Leaderboard (avatar circle + name + count)

    @ViewBuilder
    private func leaderboardCard(_ entries: [LeaderboardEntry]) -> some View {
        VStack(spacing: 8) {
            Text("Leaderboard").font(.headline)
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

            // User filter pills (only when household has >1 member)
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
                // Column headers
                HStack {
                    Spacer()
                    Text("Day").font(.caption).foregroundColor(DesignColors.textSecondary)
                        .frame(width: 28, alignment: .center)
                    Text("Wk").font(.caption).foregroundColor(DesignColors.textSecondary)
                        .frame(width: 28, alignment: .center)
                    Text("Mo").font(.caption).foregroundColor(DesignColors.textSecondary)
                        .frame(width: 28, alignment: .center)
                }
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
        HStack(spacing: 6) {
            Text("\(rank)")
                .font(.caption).fontWeight(.bold)
                .foregroundColor(DesignColors.textSecondary)
                .frame(width: 16)
            Text(chore.choreIcon).font(.body).frame(width: 24)
            Text(chore.choreName).font(.subheadline).lineLimit(1)
            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    RoundedRectangle(cornerRadius: 3)
                        .fill(DesignColors.surfaceSecondary)
                    let pct = maxMonth > 0 ? CGFloat(chore.thisMonth) / CGFloat(maxMonth) : 0
                    RoundedRectangle(cornerRadius: 3)
                        .fill(DesignColors.primary.opacity(0.6))
                        .frame(width: geo.size.width * pct)
                }
                .frame(maxHeight: .infinity, alignment: .center)
            }
            .frame(height: 20)
            Text("\(chore.today)")
                .font(.caption).foregroundColor(DesignColors.textSecondary)
                .frame(width: 28, alignment: .center)
            Text("\(chore.thisWeek)")
                .font(.caption).foregroundColor(DesignColors.textSecondary)
                .frame(width: 28, alignment: .center)
            Text("\(chore.thisMonth)")
                .font(.caption).foregroundColor(DesignColors.textSecondary)
                .frame(width: 28, alignment: .center)
        }
    }

    // MARK: - Categories (horizontal progress bars)

    @ViewBuilder
    private func categoriesCard(_ entries: [BreakdownEntry]) -> some View {
        let barMax = entries.map(\.count).max() ?? 1
        VStack(spacing: 8) {
            Text("Categories").font(.headline)
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

    // MARK: - Per-Chore Stats

    @ViewBuilder
    private var choreStatsSection: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("Chores").font(.headline)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.bottom, 10)
            ForEach(Array(activeChoreStats.enumerated()), id: \.element.choreId) { idx, cs in
                choreStatCard(cs)
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
    private func choreStatCard(_ cs: ChoreStat) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            // Header
            HStack(spacing: 8) {
                Text(cs.choreIcon).font(.body)
                Text(cs.choreName).font(.subheadline).fontWeight(.semibold).lineLimit(1)
                Spacer()
                Text("\(cs.totalThisWeek)/wk")
                    .font(.caption).foregroundColor(DesignColors.primary)
                Text("\(cs.totalThisMonth)/mo")
                    .font(.caption).foregroundColor(DesignColors.textSecondary)
            }
            // Indicator chips
            if cs.hasIndicators, let indCounts = cs.indicatorCounts, !indCounts.isEmpty {
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
            // Volume mini-chart
            if cs.hasVolume, let volHistory = cs.volumeHistory, !volHistory.isEmpty {
                let maxVol = volHistory.map(\.totalML).max() ?? 1
                HStack(alignment: .bottom, spacing: 2) {
                    ForEach(volHistory.suffix(14), id: \.date) { point in
                        let h = maxVol > 0 ? CGFloat(point.totalML) / CGFloat(maxVol) * 30 : 1
                        RoundedRectangle(cornerRadius: 2)
                            .fill(DesignColors.primary.opacity(0.6))
                            .frame(width: 6, height: max(h, 1))
                    }
                }
                .frame(height: 32)
                if let avg = cs.avgVolume {
                    Text("Avg \(Int(avg.rounded()))mL / feed")
                        .font(.caption2).foregroundColor(DesignColors.textSecondary)
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

        // Launch all requests concurrently, independently — one failure doesn't drop the rest
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
        topChoresByUser[0] = tc?.topChores ?? []
        choreStats = cs?.choreStats ?? []
        isLoading = false
    }

    private func setTopChoresUser(_ userId: Int) async {
        // Tap again to deselect
        if topChoresUserId == userId {
            topChoresUserId = nil
            return
        }
        topChoresUserId = userId
        // Skip if already cached
        guard topChoresByUser[userId] == nil else { return }
        if let tc: TopChoresResponse = try? await environment.apiClient.get(
            "/api/stats/top-chores",
            query: [URLQueryItem(name: "userId", value: "\(userId)")]
        ) {
            topChoresByUser[userId] = tc.topChores
        }
    }
}
