import SwiftUI

/// The Stats tab. Renders the section registry in the user's resolved order
/// (static sections, generalized `chore:<id>` analytics, `widget:<uuid>`
/// cards) with a customize panel for reorder/hide and widget management —
/// behavior parity with the PWA's stats page (`stats.js`), native
/// presentation per plan §2.1.
struct StatsView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @StateObject private var model = StatsModel()
    @State private var customizeOpen = false

    private var volumeUnit: String { state.volumeUnit == "oz" ? "oz" : "ml" }

    private func fmtVol(_ ml: Int) -> String {
        VolumeUnits.formatVolume(ml, unit: volumeUnit)
    }

    var body: some View {
        NavigationStack {
            Group {
                if model.isLoading {
                    SkeletonCards()
                } else {
                    ScrollView {
                        VStack(spacing: 16) {
                            if model.overview == nil {
                                InlineErrorView(message: "Stats couldn't be loaded. Try again.") {
                                    await model.loadAll(showSpinner: false)
                                }
                            }
                            if state.latestLogs.isEmpty {
                                Text("Log a chore to start your stats. You can still adjust the charts below.")
                                    .foregroundStyle(DesignColors.textSecondary)
                                Button("Log Your First Chore") { state.currentTab = .home }
                            }
                            ForEach(model.sectionLayout, id: \.self) { key in
                                VStack(spacing: 8) {
                                    sectionView(for: key)
                                    if key == "baby" {
                                        StatsLoadStatus(model: model, resource: "baby:feed")
                                        StatsLoadStatus(model: model, resource: "baby:change")
                                        StatsLoadStatus(model: model, resource: "gaps")
                                    } else {
                                        StatsLoadStatus(model: model, resource: key == "recap" ? "overview" : key)
                                    }
                                }
                            }
                        }
                        .padding()
                    }
                    .refreshable {
                        await model.loadAll(showSpinner: false)
                    }
                }
            }
            .navigationTitle("Stats")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        customizeOpen = true
                    } label: {
                        Image(systemName: "slider.horizontal.3")
                            .symbolRenderingMode(.hierarchical)
                    }
                    .accessibilityLabel("Customize stats")
                    .accessibilityIdentifier("stats-customize")
                }
            }
            .sheet(isPresented: $customizeOpen) {
                CustomizeStatsView(
                    model: model,
                    chores: state.chores,
                    widgets: state.statsWidgets,
                    hidden: Set(state.statsSectionHidden)
                )
                .sheet(isPresented: $model.widgetWizardOpen) {
                    WidgetWizardView(model: model, chores: state.chores)
                }
            }
        }
        .task {
            model.configure(api: environment.apiClient, state: state)
            await model.loadAll()
        }
    }

    // MARK: - Section dispatch

    @ViewBuilder
    private func sectionView(for key: String) -> some View {
        switch key {
        case "overview":
            overviewRow
        case "last-done":
            LastDoneSection(chores: state.chores, latestLogs: state.latestLogs)
        case "baby":
            if model.feedBabyChore != nil || model.changeBabyChore != nil {
                BabyCareSection(model: model, members: state.authorMembers, volumeUnit: volumeUnit)
            }
        case "activity":
            heatmapSection
        case "busy-hours":
            busyHoursSection
        case "leaderboard":
            leaderboardSection
        case "top-chores":
            topChoresSection
        case "categories":
            categoriesSection
        case "chores":
            choresSection
        case "recap":
            if let recap = model.overview?.recap, recap.totalChores > 0 {
                recapCard(recap)
            }
        default:
            if let choreId = StatsSections.choreId(fromSectionKey: key),
               let chore = state.chores.first(where: { $0.id == choreId }) {
                ChoreAnalyticsSection(model: model, chore: chore, members: state.authorMembers)
            } else if StatsSections.isWidgetSectionKey(key),
                      let widget = state.statsWidgets.first(where: { StatsSections.widgetSectionKey($0.id) == key }) {
                WidgetCard(model: model, widget: widget, chores: state.chores,
                           members: state.authorMembers, latestLogs: state.latestLogs)
            }
        }
    }

    // MARK: - Overview cards

    @ViewBuilder
    private var overviewRow: some View {
        let todayCount = state.todayLogs.count
        let weekCount = model.overview?.recap.totalChores ?? 0
        let streak = model.overview?.streaks.current ?? 0
        let topName: String = {
            guard let first = model.choreStats.first, first.totalThisWeek > 0 else { return "-" }
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

    // MARK: - Activity heatmap

    private var heatmapSection: some View {
        StatsCard {
            Text("Activity").font(.headline)
            HeatmapChart(heatmap: model.heatmap)
        }
    }

    // MARK: - Busy hours

    private var busyHoursSection: some View {
        StatsCard {
            Text("Busy Hours").font(.headline)

            if !model.busyHoursStart.isEmpty, !model.busyHoursEnd.isEmpty {
                Text(StatsFormat.rangeLabel(model.busyHoursStart, model.busyHoursEnd))
                    .font(.caption).foregroundColor(DesignColors.textSecondary)
            }

            HStack(spacing: 8) {
                Picker("Chore", selection: $model.bhChoreId) {
                    Text("All chores").tag(nil as Int?)
                    ForEach(state.chores) { c in
                        Text(c.name).tag(c.id as Int?)
                    }
                }
                .pickerStyle(.menu)
                .onChange(of: model.bhChoreId) { Task { await model.loadBusyHours() } }

                Picker("Member", selection: $model.bhUserId) {
                    Text("All members").tag(nil as Int?)
                    ForEach(state.members) { m in
                        Text(m.displayName.isEmpty ? m.email : m.displayName).tag(m.userId as Int?)
                    }
                }
                .pickerStyle(.menu)
                .onChange(of: model.bhUserId) { Task { await model.loadBusyHours() } }
            }

            HStack(spacing: 8) {
                StatsDatePicker(dateString: Binding(
                    get: { model.bhFilterStart.isEmpty ? model.busyHoursStart : model.bhFilterStart },
                    set: { model.bhFilterStart = $0; Task { await model.loadBusyHours() } }
                ))
                Text("–").font(.caption).foregroundColor(DesignColors.textSecondary)
                StatsDatePicker(dateString: Binding(
                    get: { model.bhFilterEnd.isEmpty ? model.busyHoursEnd : model.bhFilterEnd },
                    set: { model.bhFilterEnd = $0; Task { await model.loadBusyHours() } }
                ))
            }

            BusyHoursChart(busyHours: model.busyHours)
        }
    }

    // MARK: - Leaderboard

    private var leaderboardRangeLabel: String {
        if model.leaderboardPeriod == "all" { return "All time" }
        if let resp = model.leaderboardByPeriod[model.leaderboardPeriod],
           let s = resp.start, let e = resp.end, !s.isEmpty, !e.isEmpty {
            return StatsFormat.rangeLabel(s, e)
        }
        return ""
    }

    private var leaderboardEmptyLabel: String {
        switch model.leaderboardPeriod {
        case "all": return "No chores logged yet"
        case "day": return "No chores today"
        case "month": return "No chores this month"
        default: return "No chores this week"
        }
    }

    private var leaderboardSection: some View {
        StatsCard {
            HStack {
                Text("Leaderboard").font(.headline)
                Spacer()
                StatsPeriodToggle(period: model.leaderboardPeriod, includeAll: true) { p in
                    Task { await model.setLeaderboardPeriod(p) }
                }
            }
            if !leaderboardRangeLabel.isEmpty {
                Text(leaderboardRangeLabel)
                    .font(.caption).foregroundColor(DesignColors.textSecondary)
            }
            if model.currentLeaderboard.isEmpty {
                Text(leaderboardEmptyLabel)
                    .font(.subheadline).foregroundColor(DesignColors.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .center)
            } else {
                ForEach(model.currentLeaderboard, id: \.userId) { entry in
                    let member = state.authorMembers.first { $0.userId == entry.userId }
                    let name = member.map { $0.displayName.isEmpty ? $0.email : $0.displayName } ?? "User \(entry.userId)"
                    HStack(spacing: 8) {
                        MemberAvatar(name: name, colorHex: member?.avatarColor ?? "#19323C", size: 28)
                        Text(name).font(.subheadline).lineLimit(1)
                        Spacer()
                        Text("\(entry.count) chores")
                            .font(.subheadline).foregroundColor(DesignColors.textSecondary)
                    }
                }
            }
        }
    }

    // MARK: - Top chores

    private var topChoresEmptyLabel: String {
        switch model.topChoresPeriod {
        case "all": return "No chores logged yet"
        case "day": return "No chores today"
        case "month": return "No chores this month"
        default: return "No chores this week"
        }
    }

    private var topChoresSection: some View {
        StatsCard {
            HStack {
                Text("Top Chores").font(.headline)
                Spacer()
                StatsPeriodToggle(period: model.topChoresPeriod, includeAll: true) { p in
                    Task { await model.setTopChoresPeriod(p) }
                }
            }

            if state.members.count > 1 {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(state.members) { member in
                            let name = member.displayName.isEmpty ? member.email : member.displayName
                            let isActive = model.topChoresUserId == member.userId
                            Button {
                                Task { await model.setTopChoresUser(member.userId) }
                            } label: {
                                HStack(spacing: 4) {
                                    MemberAvatar(name: name, colorHex: member.avatarColor, size: 20)
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

            let chores = model.currentTopChores
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

    // MARK: - Categories

    private var categoriesSection: some View {
        let entries = model.categoriesBreakdown

        return StatsCard {
            HStack {
                Text("Categories").font(.headline)
                Spacer()
                StatsPeriodToggle(period: model.categoriesPeriod, includeAll: false) { p in
                    Task { await model.setCategoriesPeriod(p) }
                }
            }
            if entries.isEmpty {
                Text("No data yet")
                    .font(.subheadline).foregroundColor(DesignColors.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .center)
            } else {
                let barMax = entries.map(\.count).max() ?? 1
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
        }
    }

    // MARK: - Chores (period-scoped, #84/#85 convergence)

    private var activeChoreStats: [ChoreStat] {
        model.choreStats.filter { $0.totalInRange > 0 }
    }

    private var choreStatsPeriodLabel: String {
        switch model.choreStatsPeriod {
        case "day": return "today"
        case "week": return "this week"
        default: return "this month"
        }
    }

    private var choresSection: some View {
        StatsCard {
            HStack {
                Text("Chores").font(.headline)
                Spacer()
                StatsPeriodToggle(period: model.choreStatsPeriod, includeAll: false) { p in
                    Task { await model.setChoreStatsPeriod(p) }
                }
            }

            let stats = activeChoreStats
            if stats.isEmpty {
                Text("No chores in this period").foregroundStyle(DesignColors.textSecondary)
            }
            ForEach(Array(stats.enumerated()), id: \.element.choreId) { idx, cs in
                if cs.hasIndicators || cs.hasVolume {
                    DisclosureGroup {
                        choreStatDetails(cs).padding(.top, 6)
                    } label: {
                        choreStatHeader(cs)
                    }
                } else {
                    choreStatHeader(cs)
                }
                if idx < stats.count - 1 {
                    Divider().padding(.vertical, 4)
                }
            }
        }
    }

    @ViewBuilder
    private func choreStatHeader(_ cs: ChoreStat) -> some View {
        HStack(spacing: 8) {
            Text(cs.choreIcon).font(.body)
            Text(cs.choreName).font(.subheadline).fontWeight(.semibold).lineLimit(1)
            Spacer()
            Text("\(cs.totalInRange) \(choreStatsPeriodLabel)")
                .font(.caption).foregroundColor(DesignColors.primary)
        }
        .padding(.vertical, 2)
    }

    @ViewBuilder
    private func choreStatDetails(_ cs: ChoreStat) -> some View {
        VStack(alignment: .leading, spacing: 8) {
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

            if cs.hasVolume, let volHistory = cs.volumeHistory, !volHistory.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Volume (\(choreStatsPeriodLabel))")
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
                        Text("Avg \(fmtVol(Int(avg.rounded()))) / feed")
                            .font(.caption2).foregroundColor(DesignColors.textSecondary)
                    }
                }
            }
        }
    }

    // MARK: - Weekly recap

    private func recapCard(_ recap: Recap) -> some View {
        StatsCard {
            Text("Weekly Recap").font(.headline)
            Text("This week you completed \(recap.totalChores) chores.")
                .font(.subheadline)
            if !recap.mostActiveDay.isEmpty {
                Text("Most active: \(recap.mostActiveDay)")
                    .font(.subheadline).foregroundColor(DesignColors.textSecondary)
            }
        }
    }
}
