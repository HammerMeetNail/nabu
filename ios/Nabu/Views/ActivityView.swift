import SwiftUI

struct ActivityView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var historyLogs: [ChoreLog] = []
    @State private var historyHasMore = false
    @State private var historyBefore: String?

    private let activityStore: ActivityStore
    private let logStore: LogStore

    init(activityStore: ActivityStore, logStore: LogStore) {
        self.activityStore = activityStore
        self.logStore = logStore
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                PillTabBar(
                    selection: $state.activityView,
                    tabs: Array(ActivityViewMode.allCases),
                    labelFor: { $0.title }
                )
                .padding(.horizontal)
                .padding(.vertical, 8)

                switch state.activityView {
                case .history:
                    HistoryListView(
                        activityStore: activityStore, logStore: logStore,
                        logs: $historyLogs, hasMore: $historyHasMore, before: $historyBefore
                    )
                case .day:
                    DayView(activityStore: activityStore, logStore: logStore)
                case .week:
                    WeekView(activityStore: activityStore)
                }
            }
            .navigationTitle("Activity")
            .navigationBarTitleDisplayMode(.inline)
        }
        .task {
            await loadInitialData()
        }
        .onChange(of: state.activityView) { _, newMode in
            Task { await loadDataFor(mode: newMode) }
        }
    }

    private func loadInitialData() async {
        await loadDataFor(mode: state.activityView)
    }

    private func loadDataFor(mode: ActivityViewMode) async {
        switch mode {
        case .history:
            if historyLogs.isEmpty {
                do {
                    let data = try await activityStore.loadHistory()
                    historyLogs = data.logs
                    historyHasMore = data.hasMore
                    historyBefore = data.start
                } catch {}
            }
        case .day:
            let date = state.calendarDate?.value ?? todayISO()
            do {
                let data = try await activityStore.loadToday(date: date)
                state.todayLogs = data.logs
            } catch {}
        case .week:
            if state.weekLogs.isEmpty {
                let date = state.calendarDate?.value ?? todayISO()
                let start = weekStart(from: date)
                do {
                    let data = try await activityStore.loadWeek(start: start)
                    state.weekLogs = data.logs
                } catch {}
            }
        }
    }
}

// MARK: - History List View

struct HistoryListView: View {
    let activityStore: ActivityStore
    let logStore: LogStore
    @Binding var logs: [ChoreLog]
    @Binding var hasMore: Bool
    @Binding var before: String?
    @EnvironmentObject var state: AppState

    @State private var isLoadingMore = false
    @State private var selectedLog: ChoreLog?
    @State private var selectedChore: Chore?

    var body: some View {
        List {
            ForEach(groupedLogs(), id: \.key) { group in
                Section(group.key) {
                    ForEach(group.rows) { log in
                        historyRow(log)
                    }
                }
            }

            if hasMore {
                HStack {
                    Spacer()
                    if isLoadingMore {
                        ProgressView()
                    } else {
                        Button("Load more") {
                            Task { await loadMore() }
                        }
                    }
                    Spacer()
                }
            }
        }
        .listStyle(.plain)
        .sheet(item: $selectedLog) { log in
            if let chore = selectedChore {
                LogSheet(state: state, chore: chore, log: log, logStore: logStore)
            }
        }
    }

    private func groupedLogs() -> [(key: String, rows: [ChoreLog])] {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        var groups: [String: [ChoreLog]] = [:]
        for log in logs {
            let dateStr = f.string(from: log.completedAt)
            groups[dateStr, default: []].append(log)
        }
        return groups.sorted { $0.key > $1.key }.map { ($0.key, $0.value) }
    }

    @ViewBuilder
    private func historyRow(_ log: ChoreLog) -> some View {
        let chore = state.chores.first(where: { $0.id == log.choreId })
        Button {
            selectedChore = chore
            selectedLog = log
        } label: {
            HStack(spacing: 12) {
                Text(chore?.icon ?? "📋")
                    .font(.title3)
                    .frame(width: 32, height: 32)
                    .background(Color(hex: chore?.color ?? "#6B7280") ?? .gray)
                    .clipShape(RoundedRectangle(cornerRadius: 8))

                VStack(alignment: .leading, spacing: 2) {
                    Text(chore?.name ?? "Chore")
                        .font(.subheadline)
                        .fontWeight(.medium)
                        .foregroundColor(.primary)
                    HStack(spacing: 4) {
                        Text(fmtTime(log.completedAt))
                        if let userId = state.members.first(where: { $0.userId == log.userId }) {
                            Text("· \(userId.displayName.isEmpty ? userId.email : userId.displayName)")
                        }
                        if !log.note.isEmpty {
                            Text("· \(log.note)")
                        }
                        if let volume = log.volumeML {
                            Text("· \(volume)mL")
                        }
                        ForEach(log.indicators, id: \.self) { indicator in
                            Text(indicator)
                        }
                    }
                    .font(.caption)
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                }
            }
            .padding(.vertical, 4)
            .overlay(
                Rectangle()
                    .fill(Color(hex: chore?.color ?? "#6B7280") ?? .gray)
                    .frame(width: 3),
                alignment: .leading
            )
        }
    }

    private func loadMore() async {
        guard let before = before else { return }
        isLoadingMore = true
        do {
            let data = try await activityStore.loadMoreHistory(before: before)
            logs.append(contentsOf: data.logs)
            hasMore = data.hasMore
            self.before = data.start
        } catch {}
        isLoadingMore = false
    }
}

// MARK: - Day View

struct DayView: View {
    let activityStore: ActivityStore
    let logStore: LogStore
    @EnvironmentObject var state: AppState

    var body: some View {
        let date = state.calendarDate?.value ?? todayISO()
        let logs = state.todayLogs
        let anytimeLogs = logs.filter { $0.slotHour == nil }
        let loggedChoreIds = Set(logs.map(\.choreId))
        let total = state.chores.count
        let done = loggedChoreIds.count

        VStack(spacing: 0) {
            // Navigation
            HStack {
                Button {
                    guard let cd = state.calendarDate else {
                        state.calendarDate = LocalDate(value: shiftISO(todayISO(), by: -1))
                        return
                    }
                    state.calendarDate = LocalDate(value: shiftISO(cd.value, by: -1))
                } label: {
                    Image(systemName: "chevron.left")
                }
                Spacer()
                Text(fmtLongDate(date))
                    .font(.headline)
                Spacer()
                Button {
                    guard let cd = state.calendarDate else {
                        state.calendarDate = LocalDate(value: shiftISO(todayISO(), by: 1))
                        return
                    }
                    state.calendarDate = LocalDate(value: shiftISO(cd.value, by: 1))
                } label: {
                    Image(systemName: "chevron.right")
                }
            }
            .padding(.horizontal)
            .padding(.vertical, 8)
            .onChange(of: state.calendarDate?.value ?? "") { _, newDate in
                guard !newDate.isEmpty else { return }
                Task {
                    do {
                        let data = try await activityStore.loadToday(date: newDate)
                        state.todayLogs = data.logs
                    } catch {}
                }
            }

            // Progress
            if total > 0 {
                VStack(spacing: 4) {
                    ProgressView(value: Double(done), total: Double(total))
                        .tint(.accentColor)
                    Text("\(done) of \(total) done")
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
                .padding(.horizontal)
            }

            // Grid
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(spacing: 0) {
                        // Anytime row
                        if !anytimeLogs.isEmpty {
                            VStack(spacing: 0) {
                                HStack(spacing: 0) {
                                    Text("Anytime")
                                        .font(.caption)
                                        .foregroundColor(.secondary)
                                        .italic()
                                        .frame(width: 52, alignment: .leading)
                                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 120))], spacing: 4) {
                                        ForEach(anytimeLogs) { log in
                                            choreCard(log)
                                        }
                                    }
                                }
                                .padding(.vertical, 4)
                                .padding(.horizontal, 8)
                            }
                            .background(DesignColors.surface)
                            Divider()
                        }

                        // Hour rows
                        ForEach(0..<24, id: \.self) { hour in
                            let hourLogs = logs.filter { $0.slotHour == hour }
                            HStack(spacing: 0) {
                                Text(fmtHour(hour))
                                    .font(.caption2)
                                    .foregroundColor(.secondary)
                                    .frame(width: 52, alignment: .leading)
                                if hourLogs.isEmpty {
                                    Rectangle()
                                        .fill(Color.clear)
                                        .frame(height: 32)
                                } else {
                                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 120))], spacing: 4) {
                                        ForEach(hourLogs) { log in
                                            choreCard(log)
                                        }
                                    }
                                }
                            }
                            .padding(.vertical, 2)
                            .padding(.horizontal, 8)
                            .id(hour)
                            if hour < 23 {
                                Divider()
                            }
                        }
                    }
                }
                .onAppear {
                    let now = Calendar.current.component(.hour, from: Date())
                    proxy.scrollTo(max(now - 2, 0), anchor: .top)
                }
            }
        }
    }

    @ViewBuilder
    private func choreCard(_ log: ChoreLog) -> some View {
        let chore = state.chores.first(where: { $0.id == log.choreId })
        HStack(spacing: 4) {
            Text(chore?.icon ?? "📋")
                .font(.caption)
            Text(chore?.name ?? "Chore")
                .font(.caption2)
                .lineLimit(1)
            if !log.indicators.isEmpty {
                Text(log.indicators.joined(separator: " "))
                    .font(.caption2)
            }
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 4)
        .background(Color(hex: chore?.color ?? "6B7280")?.opacity(0.15) ?? DesignColors.surfaceSecondary)
        .overlay(
            Rectangle()
                .fill(Color(hex: chore?.color ?? "#6B7280") ?? .gray)
                .frame(width: 3),
            alignment: .leading
        )
        .clipShape(RoundedRectangle(cornerRadius: 6))
        .overlay(alignment: .topTrailing) {
            Image(systemName: "checkmark.circle.fill")
                .font(.caption2)
                .foregroundColor(.green)
                .offset(x: 4, y: -4)
        }
    }
}

// MARK: - Week View

struct WeekView: View {
    let activityStore: ActivityStore
    @EnvironmentObject var state: AppState

    var body: some View {
        let date = state.calendarDate?.value ?? todayISO()
        let monday = weekStart(from: date)
        let days = (0..<7).map { shiftISO(monday, by: $0) }
        let logs = state.weekLogs

        VStack(spacing: 0) {
            // Navigation
            HStack {
                Button {
                    guard let cd = state.calendarDate else {
                        state.calendarDate = LocalDate(value: shiftISO(monday, by: -7))
                        return
                    }
                    state.calendarDate = LocalDate(value: shiftISO(weekStart(from: cd.value), by: -7))
                } label: {
                    Image(systemName: "chevron.left")
                }
                Spacer()
                Text("\(fmtShortDate(days.first ?? monday)) – \(fmtShortDate(days.last ?? monday))")
                    .font(.headline)
                Spacer()
                Button {
                    guard let cd = state.calendarDate else {
                        state.calendarDate = LocalDate(value: shiftISO(monday, by: 7))
                        return
                    }
                    state.calendarDate = LocalDate(value: shiftISO(weekStart(from: cd.value), by: 7))
                } label: {
                    Image(systemName: "chevron.right")
                }
            }
            .padding(.horizontal)
            .padding(.vertical, 8)
            .onChange(of: state.calendarDate?.value ?? "") { _, _ in
                let date = state.calendarDate?.value ?? todayISO()
                let start = weekStart(from: date)
                Task {
                    do {
                        let data = try await activityStore.loadWeek(start: start)
                        state.weekLogs = data.logs
                    } catch {}
                }
            }

            // Grid
            ScrollView([.horizontal, .vertical]) {
                VStack(spacing: 0) {
                    // Header
                    HStack(spacing: 0) {
                        Text("")
                            .frame(width: 52)
                        ForEach(days, id: \.self) { day in
                            Text(fmtShortDate(day))
                                .font(.caption)
                                .frame(maxWidth: .infinity)
                        }
                    }
                    .padding(.vertical, 4)

                    Divider()

                    // Anytime row
                    let anytimeLogs = logs.filter { $0.slotHour == nil }
                    if !anytimeLogs.isEmpty {
                        HStack(spacing: 0) {
                            Text("Anytime")
                                .font(.caption2)
                                .foregroundColor(.secondary)
                                .italic()
                                .frame(width: 52, alignment: .leading)
                            ForEach(days, id: \.self) { day in
                                VStack(spacing: 2) {
                                    ForEach(anytimeLogs.filter { log in
                                        let f = DateFormatter()
                                        f.dateFormat = "yyyy-MM-dd"
                                        return f.string(from: log.completedAt) == day
                                    }) { log in
                                        weekCell(log)
                                    }
                                }
                                .frame(maxWidth: .infinity, minHeight: 36)
                            }
                        }
                        .padding(.vertical, 4)
                        .background(DesignColors.surfaceSecondary.opacity(0.3))
                        Divider()
                    }

                    // Hour rows
                    ForEach(0..<24, id: \.self) { hour in
                        HStack(spacing: 0) {
                            Text(fmtHour(hour))
                                .font(.caption2)
                                .foregroundColor(.secondary)
                                .frame(width: 52, alignment: .leading)
                            ForEach(days, id: \.self) { day in
                                VStack(spacing: 2) {
                                    ForEach(logs.filter { log in
                                        log.slotHour == hour && {
                                            let f = DateFormatter()
                                            f.dateFormat = "yyyy-MM-dd"
                                            return f.string(from: log.completedAt) == day
                                        }()
                                    }) { log in
                                        weekCell(log)
                                    }
                                }
                                .frame(maxWidth: .infinity, minHeight: 36)
                            }
                        }
                        .padding(.vertical, 1)
                        if hour < 23 {
                            Divider()
                        }
                    }
                }
            }
        }
    }

    @ViewBuilder
    private func weekCell(_ log: ChoreLog) -> some View {
        let chore = state.chores.first(where: { $0.id == log.choreId })
        HStack(spacing: 2) {
            Text(chore?.icon ?? "")
                .font(.caption2)
            Text(chore?.name ?? "")
                .font(.system(size: 8))
                .lineLimit(1)
        }
        .padding(2)
        .frame(maxWidth: .infinity)
        .background(Color(hex: chore?.color ?? "#6B7280")?.opacity(0.13) ?? Color.clear)
        .overlay(
            Rectangle()
                .fill(Color(hex: chore?.color ?? "#6B7280") ?? .gray)
                .frame(width: 2),
            alignment: .leading
        )
        .clipShape(RoundedRectangle(cornerRadius: 3))
    }
}
