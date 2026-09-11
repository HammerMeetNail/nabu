import SwiftUI

struct QuickLogSheet: View {
    @Environment(\.dismiss) private var dismiss
    let state: AppState
    let logStore: LogStore

    @State private var note = ""
    @State private var submissionKey = UUID().uuidString
    @State private var chosenChoreID: Int?
    @State private var submittedAt: Date?
    @State private var isSaving = false
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            Form {
                if let error = errorMessage {
                    Section {
                        Text(error)
                            .foregroundColor(.red)
                            .font(.subheadline)
                    }
                }

                Section {
                    TextField("Add a note...", text: $note, axis: .vertical)
                        .lineLimit(2...4)
                        .disabled(chosenChoreID != nil)
                } header: {
                    Text("Log a chore")
                } footer: {
                    Text("Tap a chore to log it instantly.")
                }

                Section {
                    ForEach(visibleChores) { chore in
                        Button {
                            quickLog(chore)
                        } label: {
                            HStack {
                                Text(chore.icon)
                                    .font(.title3)
                                Text(chore.name)
                                    .foregroundColor(.primary)
                                Spacer()
                                if isSaving {
                                    ProgressView()
                                }
                            }
                        }
                        .disabled(isSaving || (chosenChoreID != nil && chosenChoreID != chore.id))
                    }
                }
            }
            .navigationTitle("Quick Log")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }

    private var visibleChores: [Chore] {
        state.chores.filter { !state.hiddenHomeChoreIDs.contains($0.id) }
    }

    private func quickLog(_ chore: Chore) {
        guard !isSaving else { return }
        isSaving = true

        let owner = state.revision
        chosenChoreID = chore.id
        if submittedAt == nil { submittedAt = Date() }
        let now = submittedAt!
        let isoFormatter = ISO8601DateFormatter()
        let dateFormatter = DateFormatter()
        dateFormatter.dateFormat = "yyyy-MM-dd"

        let completedAt = isoFormatter.string(from: now)
        let dateStr = dateFormatter.string(from: now)
        let hour = Calendar.current.component(.hour, from: now)

        Task {
            do {
                let outcome = try await logStore.createLog(
                    choreId: chore.id, note: note, date: dateStr,
                    indicators: [], slotHour: hour,
                    completedAt: completedAt, idempotencyKey: submissionKey
                )
                guard state.revision == owner else { return }
                switch outcome {
                case .created(let response):
                    state.todayLogs.insert(response.log, at: 0)
                    state.latestLogs[chore.id] = response.log
                case .queued(let pending):
                    var row = pending
                    if row.userId == nil { row.userId = state.user?.id }
                    state.pendingLogs.insert(row, at: 0)
                }
                dismiss()
            } catch {
                guard state.revision == owner else { return }
                errorMessage = error.localizedDescription
                isSaving = false
            }
        }
    }
}
