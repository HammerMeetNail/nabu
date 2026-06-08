import SwiftUI

struct NotificationPreferencesView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var prefs: ReminderPreference?
    @State private var types: [NotificationTypeInfo] = []
    @State private var loading = true
    @State private var saving = false
    @State private var lastKnownEnabledTypes: [String] = []

    private let leadTimes = [5, 10, 15, 30, 60]

    var body: some View {
        Group {
            if loading {
                ProgressView()
            } else if let prefs = prefs {
                List {
                    Section {
                        VStack(alignment: .leading, spacing: 4) {
                            Text("Applies to your account across all households.")
                                .font(.caption)
                                .foregroundColor(.secondary)
                        }
                    }

                    Section {
                        HStack {
                            Text("Push Notifications")
                            Spacer()
                            Toggle("Push Notifications", isOn: Binding(
                                get: { prefs.pushEnabled },
                                set: { newValue in
                                    Task { await togglePushEnabled(newValue) }
                                }
                            ))
                            .labelsHidden()
                        }
                    }

                    if prefs.pushEnabled && !types.isEmpty {
                        Section {
                            ForEach(types) { type in
                                let isEnabled = typeEnabled(type.type)
                                HStack {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(type.label)
                                            .font(.subheadline)
                                        Text(type.description)
                                            .font(.caption)
                                            .foregroundColor(.secondary)
                                    }
                                    Spacer()
                                    Toggle(type.label, isOn: Binding(
                                        get: { isEnabled },
                                        set: { newValue in
                                            Task { await toggleType(type.type, enabled: newValue) }
                                        }
                                    ))
                                    .labelsHidden()
                                    .disabled(saving)
                                }

                                if type.type == "schedule_reminder" && isEnabled {
                                    HStack {
                                        VStack(alignment: .leading, spacing: 2) {
                                            Text("Reminder lead time")
                                                .font(.subheadline)
                                            Text("Minutes before a scheduled chore's time")
                                                .font(.caption)
                                                .foregroundColor(.secondary)
                                        }
                                        Spacer()
                                        Picker("Lead time", selection: Binding(
                                            get: { prefs.defaultReminderLeadMinutes },
                                            set: { newValue in
                                                Task { await savePrefs(PatchNotificationPrefsRequest(
                                                    pushEnabled: nil, emailEnabled: nil,
                                                    enabledPushTypes: nil,
                                                    defaultReminderLeadMinutes: newValue
                                                )) }
                                            }
                                        )) {
                                            ForEach(leadTimes, id: \.self) { m in
                                                Text("\(m) min").tag(m)
                                            }
                                        }
                                        .pickerStyle(.menu)
                                    }
                                    .padding(.leading, 16)
                                }
                            }
                        }
                    }
                }
                .navigationTitle("Notifications")
            }
        }
        .task {
            await loadPrefs()
        }
    }

    private var allTypes: [String] {
        types.map(\.type)
    }

    private func typeEnabled(_ type: String) -> Bool {
        guard let prefs = prefs else { return false }
        if !prefs.pushEnabled { return false }
        if prefs.enabledPushTypes.isEmpty { return true }
        return prefs.enabledPushTypes.contains(type)
    }

    private func loadPrefs() async {
        loading = true
        do {
            let data: NotificationPrefsResponse = try await environment.apiClient.get("/api/notification-preferences")
            prefs = data.preferences
            types = data.availableTypes
            state.notificationPrefs = data.preferences
            state.availableNotificationTypes = data.availableTypes
            if data.preferences.enabledPushTypes.isEmpty {
                lastKnownEnabledTypes = data.availableTypes.map(\.type)
            } else {
                lastKnownEnabledTypes = data.preferences.enabledPushTypes
            }
        } catch {
            prefs = ReminderPreference(
                userId: 0, pushEnabled: true, emailEnabled: false,
                quietHoursStart: "", quietHoursEnd: "", timezone: "UTC",
                enabledPushTypes: [], defaultReminderLeadMinutes: 10
            )
            types = []
        }
        loading = false
    }

    private func savePrefs(_ update: PatchNotificationPrefsRequest) async {
        saving = true
        do {
            let data: NotificationPrefsResponse = try await environment.apiClient.patch(
                "/api/notification-preferences", body: update)
            prefs = data.preferences
            state.notificationPrefs = data.preferences
        } catch {}
        saving = false
    }

    private func togglePushEnabled(_ enabled: Bool) async {
        let types: [String]
        if enabled {
            types = lastKnownEnabledTypes
        } else {
            lastKnownEnabledTypes = prefs?.enabledPushTypes ?? allTypes
            if lastKnownEnabledTypes.isEmpty && (prefs?.pushEnabled ?? true) {
                lastKnownEnabledTypes = allTypes
            }
            types = []
        }
        await savePrefs(PatchNotificationPrefsRequest(
            pushEnabled: enabled, emailEnabled: nil, enabledPushTypes: types, defaultReminderLeadMinutes: nil))
    }

    private func toggleType(_ type: String, enabled: Bool) async {
        var currentTypes = prefs?.enabledPushTypes ?? []
        if currentTypes.isEmpty && (prefs?.pushEnabled ?? true) {
            currentTypes = allTypes
        }

        let newTypes: [String]
        if enabled {
            newTypes = currentTypes + [type]
        } else {
            newTypes = currentTypes.filter { $0 != type }
        }

        let pushEnabled = !newTypes.isEmpty
        await savePrefs(PatchNotificationPrefsRequest(
            pushEnabled: pushEnabled, emailEnabled: nil, enabledPushTypes: newTypes, defaultReminderLeadMinutes: nil))
    }
}
