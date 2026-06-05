import SwiftUI

struct ManageChoresView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var showingAddSheet = false
    @State private var editingChore: Chore?

    private let choreStore: ChoreStore

    init(choreStore: ChoreStore) {
        self.choreStore = choreStore
    }

    var body: some View {
        VStack(spacing: 0) {
            if sortedChores.isEmpty {
                emptyState
            } else {
                Text("Drag to reorder · Tap eye to show/hide")
                    .font(.caption)
                    .foregroundColor(.secondary)
                    .padding(.vertical, 8)

                List {
                    ForEach(sortedChores) { chore in
                        choreRow(chore)
                    }
                    .onMove { source, destination in
                        var ids = sortedChores.map(\.id)
                        ids.move(fromOffsets: source, toOffset: destination)
                        state.choreOrder = ids
                        let patch = PatchUserPreferencesRequest(choreOrder: ids)
                        Task {
                            let _: UserPreferencesResponse? = try? await environment.apiClient.patch("/api/preferences", body: patch)
                        }
                    }
                }
                .listStyle(.plain)
            }
        }
        .overlay(alignment: .bottomTrailing) {
            Button {
                showingAddSheet = true
            } label: {
                Image(systemName: "plus.circle.fill")
                    .font(.system(size: 48))
                    .symbolRenderingMode(.hierarchical)
                    .foregroundColor(.accentColor)
            }
            .padding()
        }
        .sheet(isPresented: $showingAddSheet) {
            ChoreEditView(chore: nil, choreStore: choreStore)
        }
        .sheet(item: $editingChore) { chore in
            ChoreEditView(chore: chore, choreStore: choreStore)
        }
    }

    private var sortedChores: [Chore] {
        let chores = state.chores
        if state.choreOrder.isEmpty {
            return chores.sorted { $0.id < $1.id }
        }
        let orderMap = Dictionary(uniqueKeysWithValues: state.choreOrder.enumerated().map { ($1, $0) })
        return chores.sorted {
            (orderMap[$0.id] ?? Int.max) < (orderMap[$1.id] ?? Int.max)
        }
    }

    private var emptyState: some View {
        VStack(spacing: 16) {
            Spacer()
            Text("📋")
                .font(.system(size: 48))
            Text("No chores yet")
                .font(.title3)
                .fontWeight(.semibold)
            Text("Tap + to add your first chore.")
                .foregroundColor(.secondary)
            Button {
                showingAddSheet = true
            } label: {
                Label("Add Chore", systemImage: "plus")
            }
            .buttonStyle(.borderedProminent)
            Spacer()
        }
        .frame(maxWidth: .infinity)
    }

    @ViewBuilder
    private func choreRow(_ chore: Chore) -> some View {
        let isHidden = state.hiddenHomeChoreIDs.contains(chore.id)

        HStack(spacing: 12) {
            Text(chore.icon)
                .font(.title3)
                .frame(width: 36, height: 36)
                .background(Color(hex: chore.color) ?? .gray)
                .clipShape(RoundedRectangle(cornerRadius: 8))

            Text(chore.name)
                .lineLimit(1)

            Spacer()

            Text(chore.isPredefined ? "Default" : "Custom")
                .font(.caption2)
                .foregroundColor(.secondary)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)
                .background(DesignColors.surfaceSecondary)
                .clipShape(Capsule())

            HStack(spacing: 8) {
                Button {
                    toggleHomeVisibility(chore.id)
                } label: {
                    Image(systemName: isHidden ? "eye.slash" : "eye")
                        .foregroundColor(isHidden ? .secondary : .accentColor)
                }
                .buttonStyle(.plain)

                Button {
                    editingChore = chore
                } label: {
                    Image(systemName: "pencil")
                        .foregroundColor(.secondary)
                }
                .buttonStyle(.plain)
            }
        }
        .opacity(isHidden ? 0.5 : 1.0)
    }

    @MainActor
    private func toggleHomeVisibility(_ choreId: Int) {
        var hidden = Set(state.hiddenHomeChoreIDs)
        if hidden.contains(choreId) {
            hidden.remove(choreId)
        } else {
            hidden.insert(choreId)
        }
        let newHidden = Array(hidden)
        state.hiddenHomeChoreIDs = newHidden
        let patch = PatchUserPreferencesRequest(hiddenHomeChoreIds: newHidden)
        Task {
            let _: UserPreferencesResponse? = try? await environment.apiClient.patch("/api/preferences", body: patch)
        }
    }
}
