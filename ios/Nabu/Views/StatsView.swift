import SwiftUI

struct StatsView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var overview: StatsOverview?
    @State private var heatmap: [HeatmapEntry] = []
    @State private var busyHours: [BusyHour] = []
    @State private var topChores: [TopChore] = []
    @State private var isLoading = true

    var body: some View {
        NavigationStack {
            Group {
                if isLoading {
                    ProgressView()
                        .frame(maxHeight: .infinity)
                } else if let overview = overview {
                    ScrollView {
                        VStack(spacing: 16) {
                            overviewRow(overview)
                            if !heatmap.isEmpty {
                                heatmapCard
                            }
                            streakCard(overview.streaks)
                            leaderboardCard(overview.leaderboard)
                            if !busyHours.isEmpty {
                                busyHoursCard
                            }
                            breakdownCard(overview.breakdown)
                        }
                        .padding()
                    }
                } else {
                    VStack(spacing: 16) {
                        Text("📊")
                            .font(.system(size: 48))
                        Text("No stats yet")
                            .font(.title3)
                            .fontWeight(.semibold)
                        Text("Complete some chores to see your stats.")
                            .foregroundColor(.secondary)
                    }
                    .frame(maxHeight: .infinity)
                }
            }
            .navigationTitle("Stats")
        }
        .task {
            await loadStats()
        }
    }

    // MARK: - Overview row: TODAY / THIS WEEK / DAY STREAK / TOP CHORE

    @ViewBuilder
    private func overviewRow(_ overview: StatsOverview) -> some View {
        let todayCount = state.todayLogs.count
        let weekCount = state.weekLogs.count
        let streak = overview.streaks.current
        let topChoreName = topChores.first?.choreIcon ?? "—"

        HStack(spacing: 8) {
            statTile(value: "\(todayCount)", label: "TODAY")
            statTile(value: "\(weekCount)", label: "THIS WEEK")
            statTile(value: "\(streak)", label: "DAY STREAK")
            statTile(value: topChoreName, label: "TOP CHORE")
        }
    }

    private func statTile(value: String, label: String) -> some View {
        VStack(spacing: 4) {
            Text(value)
                .font(.title2)
                .fontWeight(.bold)
                .foregroundColor(DesignColors.primary)
                .lineLimit(1)
                .minimumScaleFactor(0.6)
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

    // MARK: - Heatmap

    @ViewBuilder
    private var heatmapCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Activity Heatmap")
                .font(.headline)

            let weeks = heatmapWeeks()
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(alignment: .top, spacing: 3) {
                    ForEach(Array(weeks.enumerated()), id: \.offset) { _, week in
                        VStack(spacing: 3) {
                            ForEach(week, id: \.date) { entry in
                                heatmapCell(entry)
                            }
                        }
                    }
                }
            }

            HStack {
                Text("Less")
                    .font(.caption2)
                    .foregroundColor(.secondary)
                ForEach([0.0, 0.25, 0.5, 0.75, 1.0], id: \.self) { opacity in
                    RoundedRectangle(cornerRadius: 2)
                        .fill(DesignColors.primary.opacity(opacity == 0 ? 0.08 : opacity))
                        .frame(width: 10, height: 10)
                }
                Text("More")
                    .font(.caption2)
                    .foregroundColor(.secondary)
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    private func heatmapCell(_ entry: HeatmapEntry) -> some View {
        let maxCount = heatmap.map(\.count).max() ?? 1
        let intensity = maxCount > 0 ? Double(entry.count) / Double(maxCount) : 0
        return RoundedRectangle(cornerRadius: 2)
            .fill(entry.count == 0
                  ? DesignColors.surfaceSecondary
                  : DesignColors.primary.opacity(0.15 + intensity * 0.85))
            .frame(width: 10, height: 10)
    }

    private func heatmapWeeks() -> [[HeatmapEntry]] {
        guard !heatmap.isEmpty else { return [] }
        var weeks: [[HeatmapEntry]] = []
        var current: [HeatmapEntry] = []
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        for entry in heatmap {
            guard let date = f.date(from: entry.date) else { continue }
            let weekday = Calendar.current.component(.weekday, from: date)
            if weekday == 2 && !current.isEmpty {
                weeks.append(current)
                current = []
            }
            current.append(entry)
        }
        if !current.isEmpty { weeks.append(current) }
        return weeks
    }

    // MARK: - Streak card

    @ViewBuilder
    private func streakCard(_ streaks: Streaks) -> some View {
        VStack(spacing: 8) {
            Text("Streaks")
                .font(.headline)
                .frame(maxWidth: .infinity, alignment: .leading)

            HStack(spacing: 24) {
                VStack {
                    Text("\(streaks.current)")
                        .font(.system(size: 36, weight: .bold))
                        .foregroundColor(DesignColors.primary)
                    Text("Current")
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
                VStack {
                    Text("\(streaks.longest)")
                        .font(.system(size: 36, weight: .bold))
                        .foregroundColor(.orange)
                    Text("Longest")
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
                Spacer()
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    // MARK: - Leaderboard

    @ViewBuilder
    private func leaderboardCard(_ entries: [LeaderboardEntry]) -> some View {
        VStack(spacing: 8) {
            Text("Leaderboard")
                .font(.headline)
                .frame(maxWidth: .infinity, alignment: .leading)

            ForEach(Array(entries.enumerated()), id: \.offset) { idx, entry in
                HStack {
                    Text("\(idx + 1)")
                        .font(.caption)
                        .fontWeight(.bold)
                        .foregroundColor(.secondary)
                        .frame(width: 24)

                    if let member = state.members.first(where: { $0.userId == entry.userId }) {
                        Text(member.displayName.isEmpty ? member.email : member.displayName)
                            .font(.subheadline)
                    } else {
                        Text("User \(entry.userId)")
                            .font(.subheadline)
                    }

                    Spacer()

                    Text("\(entry.count)")
                        .font(.subheadline)
                        .fontWeight(.semibold)
                        .foregroundColor(DesignColors.primary)
                }
            }
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    // MARK: - Busy Hours

    @ViewBuilder
    private var busyHoursCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Busy Hours")
                .font(.headline)

            let maxCount = busyHours.map(\.count).max() ?? 1
            HStack(alignment: .bottom, spacing: 3) {
                ForEach(busyHours, id: \.hour) { entry in
                    let height = maxCount > 0 ? CGFloat(entry.count) / CGFloat(maxCount) * 48 : 2
                    VStack(spacing: 2) {
                        RoundedRectangle(cornerRadius: 3)
                            .fill(DesignColors.primary.opacity(0.7))
                            .frame(height: max(height, 2))
                        Text(entry.hour % 6 == 0 ? "\(entry.hour)" : "")
                            .font(.system(size: 8))
                            .foregroundColor(.secondary)
                    }
                    .frame(maxWidth: .infinity)
                }
            }
            .frame(height: 64)
        }
        .padding()
        .background(DesignColors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }

    // MARK: - Breakdown

    @ViewBuilder
    private func breakdownCard(_ entries: [BreakdownEntry]) -> some View {
        VStack(spacing: 8) {
            Text("By Category")
                .font(.headline)
                .frame(maxWidth: .infinity, alignment: .leading)

            ForEach(entries, id: \.category) { entry in
                HStack {
                    Text(entry.category)
                        .font(.subheadline)
                    Spacer()
                    Text("\(entry.count)")
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                }
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
        do {
            async let ovReq: OverviewResponse = environment.apiClient.get("/api/stats/overview")
            async let hmReq: HeatmapResponse = environment.apiClient.get("/api/stats/heatmap")
            async let bhReq: BusyHoursResponse = environment.apiClient.get("/api/stats/busy-hours")
            async let tcReq: TopChoresResponse = environment.apiClient.get("/api/stats/top-chores")

            let ov = try await ovReq
            let hm = try await hmReq
            let bh = try await bhReq
            let tc = try await tcReq

            overview = ov.overview
            heatmap = hm.heatmap
            busyHours = bh.busyHours
            topChores = tc.topChores
        } catch {
            // Try overview alone so partial data still renders
            if let ov = try? await environment.apiClient.get("/api/stats/overview") as OverviewResponse {
                overview = ov.overview
            }
        }
        isLoading = false
    }
}
