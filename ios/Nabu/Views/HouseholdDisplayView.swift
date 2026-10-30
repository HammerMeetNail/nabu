import SwiftUI

/// The household display (`HouseholdDisplay`, PWA `display.js`): Home's tiles
/// large under the household's members. Tap who you are, then what you did,
/// and it's logged at once for you, with Undo. There are no tabs, Settings or
/// switching accounts; only the PIN leaves it.
struct HouseholdDisplayView: View {
    @Environment(AppState.self) private var state
    @Environment(AppEnvironment.self) private var environment
    @State private var pickedID: Int?
    @State private var pickedAt = Date.distantPast
    @State private var needsPick = false
    @State private var choosing: Chore?
    @State private var leaving = false
    @State private var undo: Undo?
    @State private var failure: Toast?

    private struct Undo: Equatable {
        let logID: Int
        let choreID: Int
        let label: String
    }

    private var members: [Member] { state.members.filter { !$0.suspended } }
    private var chores: [Chore] {
        HouseholdDisplay.chores(state.chores, order: state.choreOrder, hidden: state.hiddenHomeChoreIDs)
    }

    private func picked(at now: Date = Date()) -> Member? {
        guard let pickedID, now.timeIntervalSince(pickedAt) <= HouseholdDisplay.pickIdle else { return nil }
        return members.first { $0.userId == pickedID }
    }

    private func name(_ member: Member) -> String {
        member.displayName.isEmpty ? member.email : member.displayName
    }

    var body: some View {
        let who = picked()
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 12) {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(members) { person($0, picked: $0.userId == who?.userId) }
                    }
                }
                Button { leaving = true } label: {
                    Image(systemName: "lock.fill")
                        .font(.title3)
                        .frame(width: 44, height: 44)
                }
                .buttonStyle(.bordered)
                .clipShape(Circle())
                .accessibilityLabel("Leave the household display")
                .accessibilityIdentifier("display-leave")
            }
            Text(who.map { "Logging for \(name($0))" }
                 ?? (needsPick ? "Tap who you are first" : "Tap who you are, then what you did"))
                .font(.title3.weight(who == nil && needsPick ? .semibold : .regular))
                .foregroundStyle(who == nil && needsPick ? DesignColors.danger : .secondary)
                .accessibilityIdentifier("display-prompt")
            ScrollView {
                // "Time ago" ticks over each minute, as on Home.
                TimelineView(.periodic(from: .now, by: 60)) { _ in
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 150), spacing: 16)], spacing: 16) {
                        ForEach(chores) { tile($0) }
                    }
                }
            }
        }
        .padding()
        .pageBackground()
        .overlay(alignment: .bottom) {
            if let undo {
                UndoToast(choreName: undo.label) {
                    take(back: undo)
                } onDismiss: {
                    if self.undo == undo { self.undo = nil }
                }
                .id(undo.logID)
            }
        }
        .confirmationDialog(choosing.map { "\($0.icon) \($0.name): which one?" } ?? "",
                            isPresented: Binding(get: { choosing != nil }, set: { if !$0 { choosing = nil } }),
                            titleVisibility: .visible, presenting: choosing) { chore in
            ForEach(chore.subjects, id: \.self) { subject in
                Button(subject) { log(chore, subject: subject) }
            }
        }
        .sheet(isPresented: $leaving) { DisplayPINSheet(purpose: .leave) }
        .errorToast($failure)
        .task(id: pickedAt) {
            // Who did it resets after two quiet minutes.
            guard pickedID != nil else { return }
            try? await Task.sleep(for: .seconds(HouseholdDisplay.pickIdle))
            if !Task.isCancelled { pickedID = nil }
        }
        .task {
            // A display stays up for days: it shows what others log too.
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(60))
                guard !Task.isCancelled else { return }
                await LogDataLoader(api: environment.apiClient, state: state).loadLatestLogsData()
            }
        }
    }

    private func person(_ member: Member, picked: Bool) -> some View {
        Button {
            pick(member)
        } label: {
            VStack(spacing: 4) {
                MemberAvatar(name: name(member), colorHex: member.avatarColor, size: 56)
                    .accessibilityHidden(true)
                Text(name(member))
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                    .frame(maxWidth: 96)
            }
            .padding(8)
            .background(picked ? DesignColors.surface : .clear, in: RoundedRectangle(cornerRadius: 16))
            .overlay(RoundedRectangle(cornerRadius: 16).stroke(picked ? DesignColors.primary : .clear, lineWidth: 2))
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(picked ? .isSelected : [])
        .accessibilityIdentifier("display-person-\(member.userId)")
    }

    private func tile(_ chore: Chore) -> some View {
        Button {
            log(chore)
        } label: {
            HStack(spacing: 0) {
                Rectangle()
                    .fill(Color(hex: chore.color) ?? .accentColor)
                    .frame(width: 6)
                VStack(spacing: 8) {
                    Text(chore.icon)
                        .font(.system(size: 48))
                        .dynamicTypeSize(...DynamicTypeSize.accessibility1)
                    Text(chore.name)
                        .font(.headline)
                        .multilineTextAlignment(.center)
                    Text(state.latestLogs[chore.id].map { formatTimeAgo($0.completedAt) } ?? "never")
                        .font(.subheadline)
                        .foregroundStyle(.primary.opacity(0.75))
                }
                .frame(maxWidth: .infinity, minHeight: 140)
                .padding(.vertical, 12)
                .padding(.horizontal, 8)
            }
            .background(DesignColors.surface)
            .clipShape(RoundedRectangle(cornerRadius: 18))
            .shadow(color: .black.opacity(0.08), radius: 2, y: 1)
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("display-tile-\(chore.id)")
    }

    private func pick(_ member: Member) {
        pickedID = member.userId
        pickedAt = Date()
        needsPick = false
    }

    /// Logs `chore` now for whoever is picked, with the body a Home log
    /// sends (`slotHour` and `completedAt` from now, its default indicators).
    private func log(_ chore: Chore, subject: String? = nil) {
        guard let who = picked() else {
            needsPick = true
            choosing = nil
            return
        }
        if subject == nil, !chore.subjects.isEmpty {
            choosing = chore
            return
        }
        choosing = nil
        pick(who)
        let owner = state.revision
        let now = Date()
        let day = DateFormatter()
        day.locale = Locale(identifier: "en_US_POSIX")
        day.calendar = Calendar(identifier: .gregorian)
        day.dateFormat = "yyyy-MM-dd"
        let store = LogStore(api: environment.apiClient)
        Task {
            do {
                let outcome = try await store.createLog(
                    choreId: chore.id, date: day.string(from: now), indicators: chore.indicatorDefaults,
                    slotHour: Calendar.current.component(.hour, from: now),
                    completedAt: ISO8601DateFormatter().string(from: now), userId: who.userId, subject: subject)
                guard state.revision == owner else { return }
                switch outcome {
                case .created(let response):
                    state.todayLogs.insert(response.log, at: 0)
                    state.applyLatestLog(state.latestAfterSave(response, choreId: chore.id), choreId: chore.id)
                    // Named from the server's answer: who the log is for.
                    let loggedFor = members.first { $0.userId == response.log.userId }.map(name) ?? name(who)
                    let which = response.log.subject.map { " (\($0))" } ?? ""
                    undo = Undo(logID: response.log.id, choreID: chore.id, label: "\(chore.name)\(which) for \(loggedFor)")
                case .queued(let pending):
                    var row = pending
                    if row.userId == nil { row.userId = who.userId }
                    state.pendingLogs.insert(row, at: 0)
                    failure = Toast(message: "Saved \(chore.name) on this device. It syncs when Nabu is back online.")
                }
            } catch {
                guard state.revision == owner else { return }
                failure = Toast(message: Log.actions.failureMessage("Failed to log chore", error))
            }
        }
    }

    private func take(back undo: Undo) {
        self.undo = nil
        let owner = state.revision
        let store = LogStore(api: environment.apiClient)
        Task {
            do {
                let response = try await store.deleteLog(logId: undo.logID)
                guard state.revision == owner else { return }
                state.todayLogs.removeAll { $0.id == undo.logID }
                if response.reportsLatest {
                    state.applyLatestLog(response.latest, choreId: undo.choreID)
                } else {
                    await LogDataLoader(api: store.api, state: state).loadLatestLogsData()
                }
            } catch {
                guard state.revision == owner else { return }
                failure = Toast(message: Log.actions.failureMessage("Failed to undo", error))
            }
        }
    }
}

/// The household display's PIN: choosing it in Settings turns the display
/// on, and entering it on the display turns it off.
struct DisplayPINSheet: View {
    enum Purpose { case turnOn, leave }
    let purpose: Purpose
    @Environment(AppState.self) private var state
    @Environment(\.dismiss) private var dismiss
    @State private var pin = ""
    @State private var again = ""
    @State private var error: String?
    @FocusState private var focused: Bool

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    SecureField(purpose == .turnOn ? "Choose a PIN, 4 to 8 digits" : "PIN", text: $pin)
                        .keyboardType(.numberPad)
                        .focused($focused)
                        .accessibilityIdentifier("display-pin")
                    if purpose == .turnOn {
                        SecureField("Enter it again", text: $again)
                            .keyboardType(.numberPad)
                            .accessibilityIdentifier("display-pin-again")
                    }
                } footer: {
                    if let error {
                        Text(error).foregroundColor(.red).accessibilityIdentifier("display-pin-error")
                    } else if purpose == .turnOn {
                        Text("Only this device keeps the PIN, and only the PIN leaves the display.")
                    }
                }
            }
            .navigationTitle(purpose == .turnOn ? "Household Display" : "Leave the Display")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(purpose == .turnOn ? "Turn On" : "Leave") { submit() }
                        .accessibilityIdentifier("display-pin-submit")
                }
            }
            .onAppear { focused = true }
        }
        .presentationDetents([.medium])
    }

    private func submit() {
        guard let origin = HouseholdDisplay.origin(of: state) else { return }
        switch purpose {
        case .turnOn:
            guard HouseholdDisplay.canManage(user: state.user, members: state.members) else {
                error = "Only owners and admins can turn on the household display."
                return
            }
            guard HouseholdDisplay.validPIN(pin) else { error = "Use 4 to 8 digits."; return }
            guard pin == again else { error = "The PINs don't match."; return }
            guard HouseholdDisplay.turnOn(pin: pin, for: origin) else {
                error = "This device couldn't save the setting."
                return
            }
            dismiss()
            state.householdDisplayOn = true
        case .leave:
            if let message = HouseholdDisplay.leave(pin: pin, for: origin) {
                error = message
                pin = ""
                return
            }
            dismiss()
            state.householdDisplayOn = false
        }
    }
}
