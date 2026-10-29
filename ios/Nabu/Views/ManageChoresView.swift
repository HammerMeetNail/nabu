import SwiftUI

struct ManageChoresView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var showingAddSheet = false
    @State private var showingPacks = false
    @State private var editingChore: Chore?

    private let choreStore: ChoreStore

    init(choreStore: ChoreStore) {
        self.choreStore = choreStore
    }

    var body: some View {
        VStack(spacing: 0) {
            Button {
                showingPacks = true
            } label: {
                Label("Add activities", systemImage: "square.grid.2x2")
            }
            .buttonStyle(.bordered)
            .padding(.top, 8)
            .accessibilityIdentifier("add-packs")
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
                        PreferencesDataLoader(api: environment.apiClient, state: state).saveChoreOrder(ids)
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
        .sheet(isPresented: $showingPacks) {
            AddPacksSheet {
                await ChoreDataLoader(api: environment.apiClient, state: state).loadChoreData()
            }
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

            if chore.visibility == "admins" {
                Label("Admins only", systemImage: "lock.fill")
                    .font(.caption2)
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(DesignColors.surfaceSecondary)
                    .clipShape(Capsule())
            }

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
        PreferencesDataLoader(api: environment.apiClient, state: state).saveHiddenHomeChores(Array(hidden))
    }
}

/// Manage → Add activities: the setup's pack picker for this household
/// (PWA `renderPacksSheet`). Activities the household has are kept.
struct AddPacksSheet: View {
    @EnvironmentObject var environment: AppEnvironment
    @Environment(\.dismiss) private var dismiss
    @StateObject private var selection = PackSelection()
    @State private var saving = false
    @State private var message: String?
    /// Reloads the household's activities after an add.
    let reload: () async -> Void

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Text("Add a pack, or name a new baby or pet to give them their own. Activities you have already are kept as they are.")
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                    PackPicker(selection: selection, api: environment.apiClient)
                    if let message = message ?? selection.error {
                        Text(message)
                            .foregroundColor(selection.error == nil && message == Self.nothingNew ? .secondary : DesignColors.danger)
                            .accessibilityIdentifier("add-packs-message")
                    }
                }
                .padding()
            }
            .navigationTitle("Add activities")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Add") { Task { await add() } }
                        .disabled(saving || selection.packs == nil)
                        .accessibilityIdentifier("add-packs-confirm")
                }
            }
        }
    }

    static let nothingNew = "You have these activities already"

    private func add() async {
        if let problem = selection.problem { message = problem; return }
        message = nil
        saving = true
        defer { saving = false }
        guard let added = await selection.add(api: environment.apiClient) else { return }
        if added == 0 { message = Self.nothingNew; return }
        await reload()
        dismiss()
    }
}
