import SwiftUI

struct QuickLogSheet: View {
    @Environment(\.dismiss) private var dismiss
    let state: AppState
    let logStore: LogStore

    @State private var note = ""
    @State private var isSaving = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Add a note...", text: $note, axis: .vertical)
                        .lineLimit(2...4)
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
    }

    private var visibleChores: [Chore] {
        state.chores.filter { !state.hiddenHomeChoreIDs.contains($0.id) }
    }

    private func quickLog(_ chore: Chore) {
        guard !isSaving else { return }
        isSaving = true

        let now = Date()
        let isoFormatter = ISO8601DateFormatter()
        let dateFormatter = DateFormatter()
        dateFormatter.dateFormat = "yyyy-MM-dd"

        let completedAt = isoFormatter.string(from: now)
        let dateStr = dateFormatter.string(from: now)
        let hour = Calendar.current.component(.hour, from: now)

        Task {
            do {
                let response = try await logStore.createLog(
                    choreId: chore.id, note: note, date: dateStr,
                    indicators: [], slotHour: hour,
                    completedAt: completedAt
                )
                state.todayLogs.insert(response.log, at: 0)
                state.latestLogs[chore.id] = response.log
                dismiss()
            } catch {
                isSaving = false
            }
        }
    }
}
