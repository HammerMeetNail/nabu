import SwiftUI

struct HomeView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var showingQuickLog = false
    @State private var selectedChore: Chore?   // non-nil drives the log sheet
    @State private var editingLog: ChoreLog?
    @State private var undoLogId: Int?
    @State private var undoChoreName: String?
    private let logStore: LogStore

    init(logStore: LogStore) {
        self.logStore = logStore
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                headerTabs
                if state.homeView == .manage {
                    ManageChoresView(choreStore: ChoreStore(api: environment.apiClient))
                } else {
                    homeGridContent
                }
            }
            .navigationTitle("Home")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .navigationBarTrailing) {
                    if state.homeView == .log {
                        Button {
                            showingQuickLog = true
                        } label: {
                            Image(systemName: "plus.circle.fill")
                                .font(.title2)
                        }
                        .accessibilityIdentifier("quick-log-button")
                    }
                }
                ToolbarItem(placement: .navigationBarLeading) {
                    if state.homeView == .log {
                        Button {
                            state.jiggleMode.toggle()
                        } label: {
                            Image(systemName: state.jiggleMode ? "checkmark" : "pencil")
                        }
                        .accessibilityIdentifier("jiggle-button")
                    }
                }
            }
            .sheet(isPresented: $showingQuickLog) {
                QuickLogSheet(state: state, logStore: logStore)
            }
            // Use .sheet(item:) so the chore is always non-nil in the closure — no
            // if-let race between selectedChore being set and the closure being evaluated.
            .sheet(item: $selectedChore) { chore in
                LogSheet(
                    state: state,
                    chore: chore,
                    log: editingLog,
                    logStore: logStore,
                    onUndo: { logId, choreName in
                        undoLogId = logId
                        undoChoreName = choreName
                        selectedChore = nil   // dismisses the sheet
                    }
                )
            }
            .overlay(alignment: .bottom) {
                if let logId = undoLogId, let name = undoChoreName {
                    UndoToast(choreName: name) {
                        Task {
                            do {
                                let _: StatusResponse = try await logStore.deleteLog(logId: logId)
                                state.todayLogs.removeAll { $0.id == logId }
                                state.latestLogs.removeValue(forKey: logId)
                            } catch {
                                // Silent failure
                            }
                            undoLogId = nil
                            undoChoreName = nil
                        }
                    } onDismiss: {
                        undoLogId = nil
                        undoChoreName = nil
                    }
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
        }
    }

    private var headerTabs: some View {
        PillTabBar(
            selection: $state.homeView,
            tabs: Array(HomeViewMode.allCases),
            labelFor: { $0.title }
        )
        .padding(.horizontal)
        .padding(.vertical, 8)
    }

    private var homeGridContent: some View {
        let chores = sortedChores()
        if chores.isEmpty {
            return AnyView(
                Text("No chores yet. Go to Manage to add some.")
                    .foregroundColor(.secondary)
                    .padding()
            )
        }
        return AnyView(
            ScrollView {
                HomeGrid(
                    chores: chores,
                    latestLogs: state.latestLogs,
                    isJiggling: state.jiggleMode,
                    onTap: { chore in
                        editingLog = nil
                        selectedChore = chore
                    },
                    onLongPress: { chore in
                        editingLog = nil
                        selectedChore = chore
                    }
                )
                .padding()
            }
        )
    }

    private func sortedChores() -> [Chore] {
        let visible = state.chores.filter { !state.hiddenHomeChoreIDs.contains($0.id) }
        if state.choreOrder.isEmpty {
            return visible.sorted { $0.id < $1.id }
        }
        let orderMap = Dictionary(uniqueKeysWithValues: state.choreOrder.enumerated().map { ($1, $0) })
        return visible.sorted {
            (orderMap[$0.id] ?? Int.max) < (orderMap[$1.id] ?? Int.max)
        }
    }
}
