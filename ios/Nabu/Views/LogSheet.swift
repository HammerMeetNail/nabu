import SwiftUI

struct LogSheet: View {
    @Environment(\.dismiss) private var dismiss
    let state: AppState
    let chore: Chore
    let log: ChoreLog?
    let logStore: LogStore
    var onUndo: ((Int, String) -> Void)?

    @State private var note = ""
    @State private var selectedIndicators: [String] = []
    @State private var volumeML: Int? = nil
    @State private var selectedUserId: Int?
    @State private var whenDate: Date = Date()
    @State private var isSaving = false
    @State private var errorMessage: String?

    private var isEditing: Bool { log != nil }

    var body: some View {
        NavigationStack {
            Form {
                if hasWhenPicker {
                    Section {
                        DatePicker("When", selection: $whenDate)
                            .datePickerStyle(.compact)
                    }
                }

                if hasIndicators {
                    Section("How did it go?") {
                        chipList
                    }
                }

                if chore.hasVolumeML {
                    Section("Volume") {
                        Picker("Volume", selection: Binding(
                            get: { volumeML },
                            set: { volumeML = $0 }
                        )) {
                            Text("--").tag(Int?.none)
                            ForEach(Array(stride(from: 0, through: 200, by: 5)), id: \.self) { ml in
                                Text("\(ml) mL").tag(Optional(ml))
                            }
                        }
                        .pickerStyle(.menu)
                    }
                }

                if state.members.count > 1 {
                    Section("Done by") {
                        Picker("Done by", selection: Binding(
                            get: { selectedUserId ?? state.user?.id },
                            set: { selectedUserId = $0 }
                        )) {
                            ForEach(state.members) { member in
                                Text(member.displayName.isEmpty ? member.email : member.displayName)
                                    .tag(Optional(member.userId))
                            }
                        }
                        .pickerStyle(.menu)
                    }
                }

                Section("Note") {
                    TextField("Add a note...", text: $note, axis: .vertical)
                        .lineLimit(2...4)
                }

                Section {
                    Button {
                        saveLog()
                    } label: {
                        if isSaving {
                            ProgressView()
                                .frame(maxWidth: .infinity)
                        } else {
                            Text(isEditing ? "Update" : "Log")
                                .frame(maxWidth: .infinity)
                                .fontWeight(.semibold)
                        }
                    }
                    .disabled(isSaving)
                }

                if isEditing, let logId = log?.id {
                    Section {
                        Button("Remove log", role: .destructive) {
                            dismiss()
                            onUndo?(logId, chore.name)
                        }
                    }
                }
            }
            .navigationTitle("\(chore.icon) \(chore.name)")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
        .onAppear {
            setupFromLog()
        }
    }

    private var hasWhenPicker: Bool { true }
    private var hasIndicators: Bool { !chore.indicatorLabels.isEmpty }

    private var chipList: some View {
        LazyVGrid(columns: [GridItem(.adaptive(minimum: 80), spacing: 8)], spacing: 8) {
            ForEach(chore.indicatorLabels, id: \.self) { label in
                Button {
                    toggleIndicator(label)
                } label: {
                    Text(label)
                        .font(.subheadline)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 6)
                        .background(selectedIndicators.contains(label) ? Color.accentColor : DesignColors.surfaceSecondary)
                        .foregroundColor(selectedIndicators.contains(label) ? .white : .primary)
                        .clipShape(Capsule())
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func toggleIndicator(_ label: String) {
        if let idx = selectedIndicators.firstIndex(of: label) {
            selectedIndicators.remove(at: idx)
        } else {
            selectedIndicators.append(label)
        }
    }

    private func setupFromLog() {
        if let log = log {
            note = log.note
            selectedIndicators = log.indicators
            volumeML = log.volumeML
            selectedUserId = log.userId
            whenDate = log.completedAt
        } else {
            selectedIndicators = chore.indicatorDefaults
            if let latestLog = state.latestLogs[chore.id] {
                volumeML = latestLog.volumeML
            }
        }
    }

    private func saveLog() {
        guard !isSaving else { return }
        isSaving = true
        errorMessage = nil

        let isoFormatter = ISO8601DateFormatter()
        let dateFormatter = DateFormatter()
        dateFormatter.dateFormat = "yyyy-MM-dd"

        let completedAtISO = isoFormatter.string(from: whenDate)
        let dateStr = dateFormatter.string(from: whenDate)
        let hour = Calendar.current.component(.hour, from: whenDate)

        Task {
            do {
                if let logId = log?.id {
                    let _ = try await logStore.updateLog(
                        logId: logId, note: note, indicators: selectedIndicators,
                        volumeML: volumeML, userId: selectedUserId,
                        completedAt: completedAtISO, hour: hour, date: dateStr
                    )
                    // Update state
                    if let idx = state.todayLogs.firstIndex(where: { $0.id == logId }) {
                        let updated = state.todayLogs[idx]
                        let newLog = ChoreLog(
                            id: updated.id, householdId: updated.householdId,
                            userId: selectedUserId ?? updated.userId,
                            choreId: updated.choreId, completedAt: whenDate,
                            note: note, indicators: selectedIndicators,
                            slotHour: hour, createdAt: updated.createdAt,
                            volumeML: volumeML
                        )
                        state.todayLogs[idx] = newLog
                    }
                } else {
                    let response = try await logStore.createLog(
                        choreId: chore.id, note: note, date: dateStr,
                        indicators: selectedIndicators, slotHour: hour,
                        completedAt: completedAtISO, volumeML: volumeML,
                        userId: selectedUserId
                    )
                    state.todayLogs.insert(response.log, at: 0)
                    state.latestLogs[chore.id] = response.log
                }
                dismiss()
            } catch {
                errorMessage = "Failed to save log"
                isSaving = false
            }
        }
    }
}
