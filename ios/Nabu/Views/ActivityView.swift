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
            // Activity is history-only, matching the PWA (the Day/Week calendar
            // sub-views were removed there for low usage / visual noise).
            HistoryListView(
                activityStore: activityStore, logStore: logStore,
                logs: $historyLogs, hasMore: $historyHasMore, before: $historyBefore
            )
            .navigationTitle("Activity")
            .navigationBarTitleDisplayMode(.inline)
        }
        .task {
            await loadHistory()
        }
    }

    private func loadHistory() async {
        guard historyLogs.isEmpty else { return }
        do {
            let data = try await activityStore.loadHistory()
            historyLogs = data.logs
            historyHasMore = data.hasMore
            historyBefore = data.start
        } catch {}
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
                        let volKeys = Set(log.indicatorVolumes?.keys.map { $0 } ?? [])
                        if volKeys.isEmpty, let volume = log.volumeML {
                            Text("· \(volume)mL")
                        }
                        let volParts = (log.indicatorVolumes ?? [:]).map { k, v in
                            "\(k.split(separator: " ").first ?? "") \(v)mL"
                        }
                        if !volParts.isEmpty {
                            Text("· \(volParts.joined(separator: " "))")
                        }
                        ForEach(log.indicators.filter { !volKeys.contains($0) }, id: \.self) { indicator in
                            Text(indicator.split(separator: " ").first.map(String.init) ?? "")
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
