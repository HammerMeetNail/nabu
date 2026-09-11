import SwiftUI

struct ActivityView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @StateObject private var model: ActivityModel
    @State private var choreFilter: Set<Int> = []
    @State private var showingFilter = false

    private let activityStore: ActivityStore
    private let logStore: LogStore

    init(activityStore: ActivityStore, logStore: LogStore) {
        self.activityStore = activityStore
        self.logStore = logStore
        _model = StateObject(wrappedValue: ActivityModel(store: activityStore))
    }

    private var isSearching: Bool {
        !model.query.trimmingCharacters(in: .whitespaces).isEmpty
    }

    var body: some View {
        NavigationStack {
            // Activity is history-only, matching the PWA (the Day/Week calendar
            // sub-views were removed there for low usage / visual noise).
            HistoryListView(
                activityStore: activityStore, logStore: logStore,
                model: model, choreFilter: $choreFilter
            )
            .navigationTitle("Activity")
            .toolbar {
                ToolbarItem(placement: .navigationBarTrailing) {
                    // Chore chips filter the loaded (windowed) pages; they
                    // don't apply to a flat text search (PWA hides its
                    // filter FAB while searching).
                    if !isSearching {
                        Button {
                            showingFilter = true
                        } label: {
                            Image(systemName: choreFilter.isEmpty
                                ? "line.3.horizontal.decrease.circle"
                                : "line.3.horizontal.decrease.circle.fill")
                                .symbolRenderingMode(.hierarchical)
                        }
                        .accessibilityLabel("Filter activity")
                    }
                }
            }
            .searchable(text: Binding(get: { model.query }, set: { model.selectQuery($0) }), prompt: "Search notes & titles")
            .sheet(isPresented: $showingFilter) {
                ChoreFilterSheet(chores: state.chores, selected: $choreFilter)
            }
        }
        .task(id: state.currentTab.title + ":" + model.query) {
            model.configure(state: state)
            if state.currentTab == .activity { await model.search() }
        }
        .onChange(of: state.todayLogs) { _, _ in
            if state.currentTab == .activity { Task { await model.refresh() } }
        }
    }

}

// MARK: - History List View

struct HistoryListView: View {
    let activityStore: ActivityStore
    let logStore: LogStore
    @ObservedObject var model: ActivityModel
    @Binding var choreFilter: Set<Int>
    @EnvironmentObject var state: AppState

    @State private var selectedLog: ChoreLog?
    @State private var selectedChore: Chore?
    @State private var editingNoteDate: DayNoteTarget?
    @State private var deletingLog: ChoreLog?

    private var isSearching: Bool { !model.query.trimmingCharacters(in: .whitespaces).isEmpty }

    var body: some View {
        List {
            if model.isLoading { ProgressView("Loading Activity") }
            if let errorMessage = model.errorMessage {
                VStack {
                    Text(errorMessage)
                    Button("Retry") { Task { await refresh() } }
                }
            }
            if !model.isLoading && model.errorMessage == nil, let message = emptyMessage {
                ContentUnavailableView {
                    Label(
                        isSearching ? "No results" : model.hasMore ? "No activity in this range" : "No activity yet",
                        systemImage: isSearching ? "magnifyingglass" : "waveform"
                    )
                } description: {
                    Text(message)
                }
                .listRowSeparator(.hidden)
            }

            let groups = groupedLogs()
            ForEach(groups, id: \.key) { group in
                Section {
                    ForEach(group.rows) { log in
                        historyRow(log)
                    }
                } header: {
                    dayHeader(dateKey: group.key, count: group.rows.count)
                }
            }

            if model.hasMore && !isSearching {
                HStack {
                    Spacer()
                    if model.isLoadingMore {
                        ProgressView()
                    } else {
                        Button("Load more") {
                            Task { await loadMore() }
                        }
                    }
                    Spacer()
                }
                // Sentinel for infinite scroll: appearing means the user
                // reached the tail, so auto-load the next page. The button
                // above stays as an explicit fallback (PWA behavior).
                .onAppear {
                    Task { await loadMore() }
                }
            }
        }
        .listStyle(.plain)
        .refreshable {
            await refresh()
        }
        .sheet(item: $selectedLog, onDismiss: { Task { await refresh() } }) { log in
            if let chore = selectedChore {
                LogSheet(state: state, chore: chore, log: log, logStore: logStore)
            }
        }
        .sheet(item: $editingNoteDate) { target in
            DayNoteSheet(date: target.date, activityStore: activityStore)
        }
        .confirmationDialog(
            "Delete this log?",
            isPresented: Binding(
                get: { deletingLog != nil },
                set: { if !$0 { deletingLog = nil } }
            ),
            titleVisibility: .visible
        ) {
            Button("Delete", role: .destructive) {
                if let log = deletingLog {
                    Task { await deleteLog(log) }
                }
                deletingLog = nil
            }
            Button("Cancel", role: .cancel) { deletingLog = nil }
        }
    }

    /// Message to show when the (filtered) list is empty. Distinguishes "no
    /// match on this page but more pages exist" from "nothing matches at all",
    /// so a paginated view doesn't wrongly claim there's no matching activity.
    private var emptyMessage: String? {
        guard groupedLogs().isEmpty else { return nil }
        if isSearching {
            return "No activity matches your search."
        }
        if !choreFilter.isEmpty {
            return model.hasMore
                ? "No matching activity in this time range. Load more to look further back."
                : "No activity matches the selected chores."
        }
        if model.hasMore { return "No activity in this time range. Load more to look further back." }
        return model.logs.isEmpty ? "No completed chores yet." : nil
    }

    /// Offline-queued logs synthesized as rows (negative ids) so they show
    /// inline with a "pending" badge until the queue replays (PWA Phase 2.1).
    /// Search results skip them — they aren't on the server yet.
    private func pendingRows() -> [ChoreLog] {
        state.pendingLogs.enumerated().map { index, pending in
            ChoreLog(
                id: -(index + 1), householdId: 0,
                userId: pending.userId ?? state.user?.id ?? 0,
                choreId: pending.choreId, completedAt: pending.completedAt,
                note: pending.note, indicators: pending.indicators,
                slotHour: nil, createdAt: pending.completedAt,
                volumeML: pending.volumeML,
                indicatorVolumes: pending.indicatorVolumes.isEmpty ? nil : pending.indicatorVolumes,
                title: pending.title, rating: pending.rating,
                subject: pending.subject
            )
        }
    }

    private func groupedLogs() -> [(key: String, rows: [ChoreLog])] {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        let source: [ChoreLog]
        if let results = model.searchResults {
            source = results
        } else {
            source = filterLogsByChores(pendingRows() + model.logs, selected: choreFilter)
        }
        var groups: [String: [ChoreLog]] = [:]
        for log in source {
            let dateStr = f.string(from: log.completedAt)
            groups[dateStr, default: []].append(log)
        }
        return groups.sorted { $0.key > $1.key }.map { ($0.key, $0.value) }
    }

    /// Day header: label, per-day count chip, and the shared note affordance
    /// (📝 shows the note; ＋ offers to add one) — PWA `hist-date-header`.
    @ViewBuilder
    private func dayHeader(dateKey: String, count: Int) -> some View {
        let note = state.dayNotes[dateKey] ?? ""
        HStack(spacing: 8) {
            Text(fmtDayLabel(dateKey))
            Text("\(count)")
                .font(.caption2)
                .fontWeight(.semibold)
                .padding(.horizontal, 6)
                .padding(.vertical, 1)
                .background(DesignColors.surfaceSecondary)
                .clipShape(Capsule())
                .accessibilityLabel("\(count) logs")
            Spacer()
            Button {
                editingNoteDate = DayNoteTarget(date: dateKey)
            } label: {
                if note.isEmpty {
                    Label("note", systemImage: "plus")
                        .font(.caption2)
                        .foregroundColor(.secondary)
                } else {
                    Label(note, systemImage: "note.text")
                        .font(.caption2)
                        .lineLimit(1)
                        .foregroundColor(.primary)
                }
            }
            .buttonStyle(.borderless)
            .accessibilityLabel(note.isEmpty ? "Add a note for \(fmtDayLabel(dateKey))" : "Edit note: \(note)")
        }
        .textCase(nil)
    }

    private func fmtDayLabel(_ dateKey: String) -> String {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        guard let d = f.date(from: dateKey) else { return dateKey }
        f.dateFormat = "EEE, MMM d"
        return f.string(from: d)
    }

    @ViewBuilder
    private func historyRow(_ log: ChoreLog) -> some View {
        let chore = state.chores.first(where: { $0.id == log.choreId })
        let isPending = log.id < 0
        let unit = state.volumeUnit
        Button {
            // Pending rows aren't yet on the server, so they're not tappable.
            guard !isPending else { return }
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
                    HStack(spacing: 6) {
                        Text(chore?.name ?? "Chore")
                            .font(.subheadline)
                            .fontWeight(.medium)
                            .foregroundColor(.primary)
                        if isPending {
                            Label("pending", systemImage: "clock")
                                .font(.caption2)
                                .foregroundColor(.secondary)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(DesignColors.surfaceSecondary)
                                .clipShape(Capsule())
                                .accessibilityLabel("Pending sync")
                        }
                    }
                    if let title = log.title, !title.isEmpty {
                        Text(title)
                            .font(.caption)
                            .fontWeight(.medium)
                            .foregroundColor(.primary)
                            .lineLimit(1)
                    }
                    HStack(spacing: 4) {
                        Text(fmtTime(log.completedAt))
                        if let userId = state.authorMembers.first(where: { $0.userId == log.userId }) {
                            Text("· \(userId.displayName.isEmpty ? userId.email : userId.displayName)")
                        }
                        if let subject = log.subject, !subject.isEmpty {
                            Text("· \(subject)")
                                .fontWeight(.medium)
                        }
                        if !log.note.isEmpty {
                            Text("· \(log.note)")
                        }
                        let volKeys = Set(log.indicatorVolumes?.keys.map { $0 } ?? [])
                        if volKeys.isEmpty, let volume = log.volumeML {
                            Text("· \(formatChoreAmount(volume, chore: chore, volumeUnit: unit))")
                        }
                        let volParts = (log.indicatorVolumes ?? [:]).sorted(by: { $0.key < $1.key }).map { k, v in
                            "\(k.split(separator: " ").first ?? "") \(formatChoreAmount(v, chore: chore, volumeUnit: unit))"
                        }
                        if !volParts.isEmpty {
                            Text("· \(volParts.joined(separator: " "))")
                        }
                        ForEach(log.indicators.filter { !volKeys.contains($0) }, id: \.self) { indicator in
                            Text(indicator.split(separator: " ").first.map(String.init) ?? "")
                        }
                        if let rating = log.rating, rating > 0 {
                            Text("· ★\(String(format: "%.1f", Double(rating) / 10))")
                        }
                    }
                    .font(.caption)
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                }
            }
            .padding(.vertical, 4)
            .opacity(isPending ? 0.7 : 1)
            .overlay(
                Rectangle()
                    .fill(Color(hex: chore?.color ?? "#6B7280") ?? .gray)
                    .frame(width: 3),
                alignment: .leading
            )
        }
        .swipeActions(edge: .trailing) {
            if !isPending {
                Button(role: .destructive) {
                    deletingLog = log
                } label: {
                    Label("Delete", systemImage: "trash")
                }
            }
        }
    }

    private func loadMore() async { await model.loadMore() }
    private func refresh() async { await model.refresh() }

    private func deleteLog(_ log: ChoreLog) async {
        let owner = state.revision
        do {
            let _: StatusResponse = try await logStore.deleteLog(logId: log.id)
            guard state.revision == owner else { return }
            model.confirmDeletion(log.id)
            await model.refresh()
        } catch {
            guard state.revision == owner else { return }
            model.errorMessage = "Could not remove this log. Retry when connected."
        }
    }

}

// MARK: - Day Note Sheet

/// Edit sheet for the shared per-day diary note. 500-char cap; saving an
/// empty note clears the day's entry — PWA `renderDayNoteSheet` semantics.
struct DayNoteSheet: View {
    let date: String
    let activityStore: ActivityStore
    @EnvironmentObject var state: AppState
    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @State private var isSaving = false
    @FocusState private var focused: Bool

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("e.g. first solid food!", text: $text, axis: .vertical)
                        .lineLimit(3...6)
                        .focused($focused)
                        .onChange(of: text) { _, newValue in
                            if newValue.count > 500 {
                                text = String(newValue.prefix(500))
                            }
                        }
                } header: {
                    Text("Note for this day (shared)")
                } footer: {
                    Text("\(text.count)/500")
                }
            }
            .navigationTitle(fmtLongDate(date))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await save() } }
                        .disabled(isSaving)
                }
            }
        }
        .presentationDetents([.medium])
        .onAppear {
            text = state.dayNotes[date] ?? ""
            focused = true
        }
    }

    private func save() async {
        let owner = state.revision
        isSaving = true
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        do {
            _ = try await activityStore.setDayNote(date: date, note: trimmed)
            guard state.revision == owner else { return }
            if trimmed.isEmpty {
                state.dayNotes.removeValue(forKey: date)
            } else {
                state.dayNotes[date] = trimmed
            }
            dismiss()
        } catch {}
        isSaving = false
    }

    private func fmtLongDate(_ dateKey: String) -> String {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        guard let d = f.date(from: dateKey) else { return dateKey }
        f.dateFormat = "EEEE, MMMM d"
        return f.string(from: d)
    }
}

// MARK: - Chore Filter Sheet

/// Multi-select chore filter presented from the Activity toolbar. Selection is
/// additive: tapping "All activity" clears the filter, tapping a chore toggles
/// it. Chores are listed in a single alphabetically-sorted list with large,
/// tap-friendly rows.
struct ChoreFilterSheet: View {
    let chores: [Chore]
    @Binding var selected: Set<Int>
    @Environment(\.dismiss) private var dismiss

    private var sortedChores: [Chore] {
        chores.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
    }

    var body: some View {
        NavigationStack {
            List {
                Button {
                    selected.removeAll()
                } label: {
                    HStack {
                        Text("All activity")
                            .fontWeight(.semibold)
                            .foregroundColor(.primary)
                        Spacer()
                        if selected.isEmpty {
                            Image(systemName: "checkmark")
                                .foregroundColor(.accentColor)
                        }
                    }
                    .padding(.vertical, 4)
                }

                ForEach(sortedChores) { chore in
                    Button {
                        if selected.contains(chore.id) {
                            selected.remove(chore.id)
                        } else {
                            selected.insert(chore.id)
                        }
                    } label: {
                        HStack(spacing: 12) {
                            Text(chore.icon)
                                .font(.title3)
                                .frame(width: 28)
                            Text(chore.name)
                                .foregroundColor(.primary)
                            Spacer()
                            if selected.contains(chore.id) {
                                Image(systemName: "checkmark")
                                    .foregroundColor(.accentColor)
                            }
                        }
                        .padding(.vertical, 6)
                    }
                }
            }
            .navigationTitle("Filter activity")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
        }
    }
}

/// Wraps a "YYYY-MM-DD" so it can drive `.sheet(item:)` for the note editor.
struct DayNoteTarget: Identifiable {
    let date: String
    var id: String { date }
}
