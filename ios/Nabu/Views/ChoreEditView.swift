import SwiftUI

let COLOR_SWATCHES = [
    "#F59E0B", "#EC4899", "#8B5CF6", "#10B981", "#6366F1", "#6B7280",
    "#3B82F6", "#06B6D4", "#F97316", "#EF4444", "#14B8A6", "#60A5FA",
    "#FB923C", "#1F2937", "#A78BFA", "#34D399", "#2E86AB", "#19323C",
]

let QUICK_EMOJIS = [
    "🐱", "🐶", "🐰", "🐹", "🐟", "🐦", "🌱", "🌿", "🌸", "🌻",
    "🍽️", "🧹", "🗑️", "🧺", "👕", "🛁", "🛏️", "🚿", "💊", "🧽",
    "🎃", "🍼", "👶", "🛒", "🧴", "💡", "🔧", "📦", "🥣", "🌊",
    "🏠", "⭐", "❤️", "✨", "🧸", "🎯", "🔑", "📋", "🪴", "🫙",
]

struct ChoreEditView: View {
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    let chore: Chore?
    let choreStore: ChoreStore

    @State private var name: String = ""
    @State private var icon: String = "📋"
    @State private var color: String = "#2E86AB"
    @State private var indicatorLabels: [String] = []
    @State private var indicatorDefaults: Set<String> = []
    @State private var followUpEnabled: Bool = true
    @State private var metricType: String = "none"
    @State private var metricUnit: String = "mL"
    @State private var subjects: [String] = []
    @State private var visibility: String = "household"
    @State private var isSaving = false
    @State private var errorMessage: String?
    @State private var reminderEnabled: Bool = false
    @State private var reminderLeadMinutes: Int = 10
    @State private var showPrivateConfirm = false
    @State private var showHouseholdConfirm = false

    private var isNew: Bool { chore == nil }

    private var isAdmin: Bool {
        let role = state.members.first(where: { $0.userId == state.user?.id })?.role ?? state.user?.role ?? "member"
        return role == "owner" || role == "admin"
    }

    private var scheduleReminderTypeEnabled: Bool {
        let prefs = state.notificationPrefs
        guard let prefs = prefs, prefs.pushEnabled != false else { return false }
        let types = prefs.enabledPushTypes
        if types.isEmpty { return true }
        return types.contains("schedule_reminder")
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("Name") {
                    TextField("Chore name", text: $name)
                }

                Section("Icon") {
                    HStack {
                        Text(icon)
                            .font(.largeTitle)
                            .frame(width: 50, height: 50)
                            .background(Color(hex: color) ?? .gray)
                            .clipShape(RoundedRectangle(cornerRadius: 10))

                        TextField("Emoji", text: $icon)
                    }

                    emojiGrid
                }

                Section("Color") {
                    colorGrid
                }

                if isAdmin {
                    Section {
                        Picker("Visible to", selection: $visibility) {
                            Text("Everyone").tag("household")
                            Text("Owners and admins").tag("admins")
                        }
                        .pickerStyle(.segmented)
                    } header: {
                        Text("Visible to")
                    } footer: {
                        Text(visibility == "admins" ? "Visible only to household owners and admins." : "Visible to all household members.")
                    }
                }

                Section {
                    Picker("Track a value", selection: $metricType) {
                        Text("Nothing").tag("none")
                        Text("Amount (mL/oz/g…)").tag("amount")
                        Text("Rating (stars)").tag("rating")
                        Text("Duration (timer)").tag("duration")
                    }
                    .pickerStyle(.menu)

                    if metricType == "amount" {
                        HStack {
                            Menu("Unit") {
                                ForEach(AmountUnits.common, id: \.self) { unit in Button(unit) { metricUnit = unit } }
                            }.accessibilityIdentifier("chore-unit-presets")
                            TextField("mL", text: $metricUnit)
                                .multilineTextAlignment(.trailing)
                                .onChange(of: metricUnit) { _, newValue in
                                    if newValue.count > 12 {
                                        metricUnit = String(newValue.prefix(12))
                                    }
                                }
                        }
                    }
                } header: {
                    Text("Track a value")
                } footer: {
                    Text("Log a number, rating, or elapsed time with this chore")
                }

                Section {
                    ForEach(subjects.indices, id: \.self) { idx in
                        HStack {
                            TextField("e.g. 👶 Alice", text: Binding(
                                get: { subjects[idx] },
                                set: { subjects[idx] = String($0.prefix(30)) }
                            ))
                            .textFieldStyle(.roundedBorder)

                            Button {
                                subjects.remove(at: idx)
                            } label: {
                                Image(systemName: "xmark.circle.fill")
                                    .foregroundColor(.secondary)
                            }
                            .accessibilityLabel("Remove subject")
                        }
                    }

                    Button {
                        subjects.append("")
                    } label: {
                        Label("Add subject", systemImage: "plus")
                    }
                } header: {
                    Text("Subjects")
                } footer: {
                    Text("Optional tags to distinguish who a log is about (e.g. twins)")
                }

                Section {
                    ForEach(indicatorLabels.indices, id: \.self) { idx in
                        HStack {
                            TextField("e.g. 💩 poo", text: Binding(
                                get: { indicatorLabels[idx] },
                                set: { indicatorLabels[idx] = $0 }
                            ))
                            .textFieldStyle(.roundedBorder)

                            Button {
                                if indicatorDefaults.contains(indicatorLabels[idx]) {
                                    indicatorDefaults.remove(indicatorLabels[idx])
                                } else {
                                    indicatorDefaults.insert(indicatorLabels[idx])
                                }
                            } label: {
                                Text("default")
                                    .font(.caption)
                                    .foregroundColor(indicatorDefaults.contains(indicatorLabels[idx]) ? .accentColor : .secondary)
                            }

                            Button {
                                indicatorDefaults.remove(indicatorLabels[idx])
                                indicatorLabels.remove(at: idx)
                            } label: {
                                Image(systemName: "xmark.circle.fill")
                                    .foregroundColor(.secondary)
                            }
                        }
                    }

                    Button {
                        indicatorLabels.append("")
                    } label: {
                        Label("Add label", systemImage: "plus")
                    }
                } header: {
                    Text("Indicator labels")
                } footer: {
                    Text("Optional tags logged with this chore")
                }

                if !isNew {
                    Section {
                        Toggle("Enable follow-up scheduling", isOn: $followUpEnabled)
                    } header: {
                        Text("Follow-up")
                    } footer: {
                        Text("Schedule next occurrence after logging")
                    }

                    if scheduleReminderTypeEnabled {
                        Section {
                            Toggle("Remind me", isOn: Binding(
                                get: { reminderEnabled },
                                set: { newValue in
                                    reminderEnabled = newValue
                                    saveReminderPref()
                                }
                            ))
                            if reminderEnabled {
                                Picker("Lead time", selection: Binding(
                                    get: { reminderLeadMinutes },
                                    set: { newValue in
                                        reminderLeadMinutes = newValue
                                        saveReminderPref()
                                    }
                                )) {
                                    ForEach([5, 10, 15, 30, 60], id: \.self) { m in
                                        Text("\(m) min before").tag(m)
                                    }
                                }
                            }
                        } header: {
                            Text("Reminder")
                        } footer: {
                            Text("Get a push notification before this chore")
                        }
                    }

                    Section {
                        if chore?.isPredefined == true {
                            Button("Restore default") {
                                Task { await restoreDefaults() }
                            }
                        } else {
                            Button("Delete chore", role: .destructive) {
                                Task { await deleteChore() }
                            }
                        }
                    }
                }
            }
            .navigationTitle(isNew ? "Add Chore" : "Edit Chore")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await save() } }
                        .disabled(name.trimmingCharacters(in: .whitespaces).isEmpty || isSaving)
                }
            }
        }
        .onAppear {
            if let chore = chore {
                name = chore.name
                icon = chore.icon
                color = chore.color
                indicatorLabels = chore.indicatorLabels
                indicatorDefaults = Set(chore.indicatorDefaults)
                followUpEnabled = chore.followUpEnabled
                metricType = chore.metricType
                metricUnit = chore.metricUnit.isEmpty ? "mL" : chore.metricUnit
                subjects = chore.subjects
                visibility = chore.visibility
                loadReminderPref()
            }
        }
        .alert("Make \"\(name)\" Admins only?", isPresented: $showPrivateConfirm) {
            Button("Make Admins only", role: .destructive) { Task { await performSave() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("It will disappear, along with its activity and statistics, for regular members. Information they saw while it was shared cannot be recalled.")
        }
        .alert("Make \"\(name)\" visible to everyone?", isPresented: $showHouseholdConfirm) {
            Button("Make Household") { Task { await performSave() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("All members will gain access to the entire retained history.")
        }
    }

    private func loadReminderPref() {
        guard let choreId = chore?.id else { return }
        Task {
            do {
                let data: ChoreReminderPrefsResponse = try await environment.apiClient.get("/api/chore-reminder-prefs")
                await MainActor.run {
                    state.choreReminderPrefs = data.prefs
                    let pref = data.prefs.first(where: { $0.choreId == choreId })
                    reminderEnabled = pref?.enabled ?? false
                    reminderLeadMinutes = pref?.leadMinutes ?? state.notificationPrefs?.defaultReminderLeadMinutes ?? 10
                }
            } catch {}
        }
    }

    private func saveReminderPref() {
        guard let choreId = chore?.id else { return }
        let leadMinutes = reminderLeadMinutes
        let enabled = reminderEnabled
        struct Body: Codable {
            let enabled: Bool
            let leadMinutes: Int
        }
        let body = Body(enabled: enabled, leadMinutes: leadMinutes)
        Task {
            do {
                let data: ChoreReminderPrefResponse = try await environment.apiClient.patch("/api/chore-reminder-prefs/\(choreId)", body: body)
                await MainActor.run {
                    if let idx = state.choreReminderPrefs.firstIndex(where: { $0.choreId == choreId }) {
                        state.choreReminderPrefs[idx] = data.pref
                    } else {
                        state.choreReminderPrefs.append(data.pref)
                    }
                }
            } catch {}
        }
    }

    private var emojiGrid: some View {
        LazyVGrid(columns: Array(repeating: GridItem(.flexible()), count: 10)) {
            ForEach(QUICK_EMOJIS, id: \.self) { emoji in
                Button {
                    icon = emoji
                } label: {
                    Text(emoji)
                        .font(.title3)
                        .padding(4)
                        .background(icon == emoji ? Color.accentColor.opacity(0.2) : Color.clear)
                        .clipShape(RoundedRectangle(cornerRadius: 6))
                }
                .buttonStyle(.plain)
            }
        }
    }

    private var colorGrid: some View {
        LazyVGrid(columns: Array(repeating: GridItem(.flexible()), count: 6)) {
            ForEach(COLOR_SWATCHES, id: \.self) { swatch in
                Button {
                    color = swatch
                } label: {
                    Circle()
                        .fill(Color(hex: swatch) ?? .gray)
                        .frame(width: 36, height: 36)
                        .overlay(
                            Circle()
                                .stroke(color == swatch ? Color.primary : Color.clear, lineWidth: 3)
                        )
                        .padding(4)
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func save() async {
        // Handle visibility transition confirmations before performing save.
        if !isNew, let oldVis = chore?.visibility {
            let newVis = isAdmin ? visibility : oldVis
            if oldVis != newVis {
                if newVis == "admins" {
                    showPrivateConfirm = true
                    return
                } else if oldVis == "admins" && newVis == "household" {
                    showHouseholdConfirm = true
                    return
                }
            }
        }
        await performSave()
    }

    private func performSave() async {
        let trimmedName = name.trimmingCharacters(in: .whitespaces)
        guard !trimmedName.isEmpty else { return }
        let cleanedLabels = indicatorLabels.map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
        let cleanedDefaults = cleanedLabels.filter { indicatorDefaults.contains($0) }
        let trimmedUnit = metricUnit.trimmingCharacters(in: .whitespaces)
        let finalMetricUnit = metricType == "amount" ? (trimmedUnit.isEmpty ? "mL" : trimmedUnit) : ""
        let cleanedSubjects = subjects.map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
        let visToSend: String? = isAdmin ? visibility : nil

        isSaving = true
        do {
            if isNew {
                let response = try await choreStore.createChore(
                    name: trimmedName, icon: icon, color: color,
                    indicatorLabels: cleanedLabels, indicatorDefaults: cleanedDefaults,
                    followUpEnabled: followUpEnabled,
                    metricType: metricType, metricUnit: finalMetricUnit,
                    subjects: cleanedSubjects, visibility: visToSend
                )
                state.chores.append(response.chore)
                var newOrder = state.choreOrder
                newOrder.append(response.chore.id)
                let patch = PatchUserPreferencesRequest(choreOrder: newOrder)
                Task {
                    let _: UserPreferencesResponse? = try? await environment.apiClient.patch("/api/preferences", body: patch)
                }
                state.choreOrder = newOrder
            } else if let choreId = chore?.id {
                let response = try await choreStore.updateChore(
                    choreId: choreId, name: trimmedName, icon: icon, color: color,
                    indicatorLabels: cleanedLabels, indicatorDefaults: cleanedDefaults,
                    followUpEnabled: followUpEnabled,
                    metricType: metricType, metricUnit: finalMetricUnit,
                    subjects: cleanedSubjects, visibility: visToSend
                )
                if let idx = state.chores.firstIndex(where: { $0.id == choreId }) {
                    state.chores[idx] = response.chore
                }
            }
            dismiss()
        } catch {
            errorMessage = "Failed to save chore"
            isSaving = false
        }
    }

    private func deleteChore() async {
        guard let choreId = chore?.id else { return }
        do {
            let _ = try await choreStore.deleteChore(choreId: choreId)
            state.chores.removeAll { $0.id == choreId }
            state.choreOrder.removeAll { $0 == choreId }
            state.hiddenHomeChoreIDs.removeAll { $0 == choreId }
            dismiss()
        } catch {
            errorMessage = "Failed to delete chore"
        }
    }

    private func restoreDefaults() async {
        guard let choreId = chore?.id else { return }
        do {
            let response = try await choreStore.restoreDefault(choreId: choreId)
            if let idx = state.chores.firstIndex(where: { $0.id == choreId }) {
                state.chores[idx] = response.chore
            }
            dismiss()
        } catch {
            errorMessage = "Failed to restore default"
        }
    }
}
