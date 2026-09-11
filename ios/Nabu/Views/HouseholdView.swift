import SwiftUI

struct HouseholdView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @StateObject private var auth = AuthStore(api: APIClient(baseURL: URL(string: "http://localhost:8080")!))
    @State private var showingEdit = false
    @State private var showingInvite = false
    @State private var showingTransfer = false
    @State private var inviteCode: String?
    @State private var householdName: String = ""
    @State private var householdInitials: String = ""
    @State private var isSaving = false
    @State private var exportFile: ExportFile?
    @State private var isExporting = false
    @State private var exportAllDates = true
    @State private var exportStart = Calendar.current.date(byAdding: .month, value: -1, to: TestHooks.reviewDate ?? Date()) ?? Date()
    @State private var exportEnd = TestHooks.reviewDate ?? Date()
    @State private var exportError: String?
    @State private var exportTask: Task<Void, Never>?
    @State private var exportJob = UUID()
    @State private var exportTempURL: URL?
    @EnvironmentObject private var accountDeletion: AccountDeletionModel
    @State private var showingLeaveConfirm = false
    @State private var verificationSent = false
	@State private var showingPassword = false

    private var canExportHousehold: Bool {
        let role = state.members.first(where: { $0.userId == state.user?.id })?.role ?? state.user?.role
        return role == "owner" || role == "admin"
    }

    var body: some View {
        NavigationStack {
            List {
                // Account — first, per the settings hierarchy (C5): the
                // things about *you*, then the household, then device prefs.
                if let user = state.user {
                    Section("Account") {
                        HStack {
                            Text("Email")
                            Spacer()
                            Text(user.email)
                                .foregroundColor(.secondary)
                        }
                        if !user.emailVerified {
                            Button(verificationSent ? "Verification email queued" : "Resend verification email") {
                                Task { await resendVerification() }
                            }
                            .disabled(verificationSent)
                        }
                        Button(user.hasPassword == false ? "Set Password" : "Change Password") {
                            showingPassword = true
                        }
                        Button("Delete Account…", role: .destructive) {
                            accountDeletion.open(api: environment.apiClient)
                        }
                    }
                }

                // Household info
                if let household = state.household {
                    Section("Household") {
                        HStack {
                            Text(household.initials)
                                .font(.title2)
                                .fontWeight(.bold)
                                .foregroundColor(.white)
                                .frame(width: 44, height: 44)
                                .background(DesignColors.brand)
                                .clipShape(RoundedRectangle(cornerRadius: 10))

                            VStack(alignment: .leading) {
                                Text(household.name)
                                    .font(.headline)
                                if state.user?.role == "owner", let code = household.inviteCode, !code.isEmpty {
                                    Text("\(environment.baseURL.absoluteString)/join?code=\(code)")
                                        .font(.caption)
                                        .foregroundColor(.secondary)
                                }
                            }
                        }

                        Button("Edit Household") {
                            householdName = household.name
                            householdInitials = household.initials
                            showingEdit = true
                        }
                    }
                }

                // Members
                if !state.members.isEmpty {
                    Section("Members (\(state.members.count))") {
                        ForEach(state.members) { member in
                            MemberRow(member: member, isCurrentUser: member.userId == state.user?.id,
                                      currentUserRole: state.user?.role ?? "member",
                                      onRoleChange: { newRole in Task { await updateMemberRole(member, role: newRole) } },
                                      onRemove: { Task { await removeMember(member) } })
                        }
                    }
                }

                // Invites
                if state.user?.role == "owner", !state.invites.isEmpty {
                    Section("Invites") {
                        ForEach(state.invites) { invite in
                            HStack {
                                VStack(alignment: .leading) {
                                    Text("\(environment.baseURL.absoluteString)/join?code=\(invite.code)")
                                        .font(.system(.caption, design: .monospaced))
                                        .lineLimit(1)
                                        .minimumScaleFactor(0.7)
                                    if let expires = invite.expiresAt {
                                        Text("Expires \(expires.formatted(date: .abbreviated, time: .omitted))")
                                            .font(.caption)
                                            .foregroundColor(.secondary)
                                    }
                                    Text("\(invite.usedCount)/\(invite.maxUses) used")
                                        .font(.caption)
                                        .foregroundColor(.secondary)
                                }
                                Spacer()
                                Button("Delete", role: .destructive) {
                                    Task { await deleteInvite(invite) }
                                }
                            }
                        }
                    }
                }

                // Invite links are owner-controlled, single-use, and expiring.
                if state.user?.role == "owner" {
                Section {
                    Button {
                        Task { await createInvite() }
                    } label: {
                        Label("Create Single-Use Invite", systemImage: "person.badge.plus")
                    }
                }

                Text("New links expire after 7 days. Removing a member or changing a role revokes existing links.")
                    .font(.caption)
                    .foregroundColor(.secondary)
                }

                // Household actions
                if state.userHouseholds.count > 1 {
                    Section("Your Households") {
                        ForEach(state.userHouseholds) { hh in
                            HStack {
                                VStack(alignment: .leading) {
                                    Text(hh.name)
                                    Text(hh.role.capitalized)
                                        .font(.caption)
                                        .foregroundColor(.secondary)
                                }
                                Spacer()
                                if hh.id == state.activeHouseholdId {
                                    Image(systemName: "checkmark")
                                        .foregroundColor(.accentColor)
                                } else {
                                    Button("Switch") {
                                        Task { await activateHousehold(hh.id) }
                                    }
                                }
                            }
                        }
                    }
                }

                // Dangerous actions
                Section {
                    if state.members.count > 1 && state.user?.role == "owner" {
                        Button("Transfer Ownership") {
                            showingTransfer = true
                        }
                    }
                    Button("Leave Household", role: .destructive) {
                        showingLeaveConfirm = true
                    }
                    // Same confirmation the PWA shows before leaving.
                    .confirmationDialog(
                        "Are you sure you want to leave this household? All your data will remain with the household.",
                        isPresented: $showingLeaveConfirm,
                        titleVisibility: .visible
                    ) {
                        Button("Leave Household", role: .destructive) {
                            Task { await leaveHousehold() }
                        }
                    }
                }

                // Notifications
                Section {
                    NavigationLink {
                        NotificationsView()
                    } label: {
                        HStack {
                            Label("Notifications", systemImage: "bell")
                            Spacer()
                            if !state.hideNotificationBadge && state.unreadNotifications > 0 {
                                Text("\(state.unreadNotifications)")
                                    .font(.caption)
                                    .fontWeight(.bold)
                                    .foregroundColor(.white)
                                    .padding(.horizontal, 8)
                                    .padding(.vertical, 2)
                                    .background(Color.red)
                                    .clipShape(Capsule())
                            }
                        }
                    }

                    NavigationLink {
                        NotificationPreferencesView()
                    } label: {
                        Label("Notification Preferences", systemImage: "bell.badge")
                    }

                    // Parity with the PWA settings toggle: notifications keep
                    // accumulating; only the unread count display is hidden.
                    Toggle("Hide notification badge", isOn: Binding(
                        get: { state.hideNotificationBadge },
                        set: { hide in
                            Task {
                                await PreferencesDataLoader(api: environment.apiClient, state: state)
                                    .setHideNotificationBadge(hide)
                            }
                        }
                    ))
                }

                // Volume unit (display-only preference; volumes stay mL in the API)
                Section {
                    Picker("Volume unit", selection: Binding(
                        get: { state.volumeUnit == "oz" ? "oz" : "ml" },
                        set: { newUnit in
                            Task {
                                await PreferencesDataLoader(api: environment.apiClient, state: state)
                                    .setVolumeUnit(newUnit)
                            }
                        }
                    )) {
                        Text("mL").tag("ml")
                        Text("oz").tag("oz")
                    }
                    .pickerStyle(.segmented)
                } header: {
                    Text("Volume unit")
                } footer: {
                    Text("How feed amounts are shown and entered. Stored values don't change.")
                }

                Section {
                    Toggle("All dates", isOn: $exportAllDates).disabled(isExporting)
                    if !exportAllDates {
                        DatePicker("From", selection: $exportStart, displayedComponents: .date)
                            .disabled(isExporting).accessibilityIdentifier("export-start-date")
                        DatePicker("Through", selection: $exportEnd, displayedComponents: .date)
                            .disabled(isExporting).accessibilityIdentifier("export-end-date")
                    }
                    if let exportError {
                        Text(exportError).foregroundColor(.red).accessibilityIdentifier("export-error")
                    }
                    if isExporting { Button("Cancel export") { cancelExport() } }
                } header: {
                    Text("Export dates")
                } footer: {
                    Text("Each export can contain up to 10,000 records and 16 MB. Choose a smaller date range if needed.")
                }

                if canExportHousehold {
                    Section {
                        Button {
                            startExport(household: true)
                        } label: {
                            HStack {
                                Label("Export household data as CSV", systemImage: "square.and.arrow.up")
                                if isExporting {
                                    Spacer()
                                    ProgressView()
                                }
                            }
                        }
                        .disabled(isExporting)
                    } header: {
                        Text("Export")
                    } footer: {
                        Text("Includes household details, chores, schedules, participants, and activity and notes within the selected dates. Invite codes and account credentials are never included.")
                    }
                }

                // Data export (same all-history window as the PWA's link)
                Section {
                    Button {
                        startExport(household: false)
                    } label: {
                        HStack {
                            Label("Export logs as CSV", systemImage: "square.and.arrow.up")
                            if isExporting {
                                Spacer()
                                ProgressView()
                            }
                        }
                    }
                    .disabled(isExporting)
                } footer: {
                    Text("Download activity within the selected dates as a CSV spreadsheet.")
                }

                // About (A5): the same privacy/support URLs App Store
                // Connect metadata points at.
                Section("About") {
                    Link(destination: environment.baseURL.appendingPathComponent("privacy")) {
                        Label("Privacy Policy", systemImage: "hand.raised")
                    }
                    Link(destination: environment.baseURL.appendingPathComponent("support")) {
                        Label("Support", systemImage: "questionmark.circle")
                    }
                    HStack {
                        Text("Version")
                        Spacer()
                        Text(appVersion)
                            .foregroundColor(.secondary)
                    }
                }

                Section {
                    Button("Sign Out", role: .destructive) {
                        Task {
                            _ = await auth.logout()
                        }
                    }
                }
            }
            .navigationTitle("Settings")
            .refreshable {
                await refreshHousehold()
            }
            // Warning haptics on destructive confirms (C3).
            .sensoryFeedback(.warning, trigger: accountDeletion.isPresented) { _, new in new }
            .sensoryFeedback(.warning, trigger: showingLeaveConfirm) { _, new in new }
        }
        .onAppear {
            auth.configure(api: environment.apiClient)
        }
        .onChange(of: state.revision) { _, _ in
            cancelExport()
            clearExportFile()
        }
        .onDisappear { cancelExport() }
        .sheet(item: $exportFile, onDismiss: clearExportFile) { file in
            ShareSheet(items: [file.url])
        }
        .sheet(isPresented: $showingPassword) {
            PasswordChangeSheet()
        }
        .sheet(isPresented: $showingEdit) {
            NavigationStack {
                Form {
                    Section("Household Name") {
                        TextField("Name", text: $householdName)
                    }
                    Section("Initials") {
                        TextField("Initials (2-4 chars)", text: $householdInitials)
                    }
                }
                .navigationTitle("Edit Household")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) { Button("Cancel") { showingEdit = false } }
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Save") { Task { await saveHousehold() } }
                            .disabled(isSaving)
                    }
                }
            }
        }
        .sheet(isPresented: $showingTransfer) {
            NavigationStack {
                List {
                    ForEach(state.members.filter { $0.userId != state.user?.id }) { member in
                        Button {
                            Task { await transferOwnership(to: member) }
                        } label: {
                            HStack {
                                Text(member.displayName.isEmpty ? member.email : member.displayName)
                                Spacer()
                            }
                        }
                    }
                }
                .navigationTitle("Transfer Ownership")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) { Button("Cancel") { showingTransfer = false } }
                }
            }
        }
        .alert("Invite Code", isPresented: .constant(inviteCode != nil)) {
            Button("Copy") {
                if let code = inviteCode {
                    UIPasteboard.general.string = code
                }
                inviteCode = nil
            }
            Button("OK") { inviteCode = nil }
        } message: {
            if let code = inviteCode {
                Text("Share this code: \(code). It can be used once and expires after 7 days.")
            }
        }
    }

    private var appVersion: String {
        let version = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "1.0"
        let build = Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "1"
        return "\(version) (\(build))"
    }

    private func saveHousehold() async {
        let owner = state.revision
        let api = environment.apiClient.scoped()
        isSaving = true
        let body = UpdateHouseholdRequest(name: householdName, initials: householdInitials)
        do {
            let resp: HouseholdResponse = try await api.patch("/api/household", body: body)
            guard state.revision == owner else { return }
            state.household = resp.household
            showingEdit = false
        } catch {}
        isSaving = false
    }

    private func createInvite() async {
        let owner = state.revision
        let api = environment.apiClient.scoped()
        do {
            let resp: InviteResponse = try await api.postEmpty("/api/household/invites")
            guard state.revision == owner else { return }
            state.invites.append(resp.invite)
            inviteCode = resp.invite.code
        } catch {}
    }

    private func deleteInvite(_ invite: Invite) async {
        let owner = state.revision
        let api = environment.apiClient.scoped()
        do {
            let _: StatusResponse = try await api.delete("/api/household/invites/\(invite.id)")
            guard state.revision == owner else { return }
            state.invites.removeAll { $0.id == invite.id }
        } catch {}
    }

    private func updateMemberRole(_ member: Member, role: String) async {
        let owner = state.revision
        let api = environment.apiClient.scoped()
        let newRole = role
        let body = UpdateMemberRoleRequest(role: newRole)
        do {
            let _: StatusResponse = try await api.patch("/api/household/members/\(member.userId)", body: body)
            guard state.revision == owner else { return }
            if let idx = state.members.firstIndex(where: { $0.userId == member.userId }) {
                let updated = Member(userId: member.userId, email: member.email,
                                     displayName: member.displayName, avatarColor: member.avatarColor,
                                     emailVerified: member.emailVerified, role: newRole)
                state.members[idx] = updated
            }
        } catch {}
    }

    private func removeMember(_ member: Member) async {
        let owner = state.revision
        let api = environment.apiClient.scoped()
        do {
            let _: StatusResponse = try await api.delete("/api/household/members/\(member.userId)")
            guard state.revision == owner else { return }
            await refreshHousehold()
        } catch {}
    }

    private func leaveHousehold() async {
        let api = environment.apiClient.scoped()
        do {
            let _: StatusResponse = try await api.postEmpty("/api/household/leave")
        } catch {}
    }

    private func transferOwnership(to member: Member) async {
        var api = environment.apiClient.scoped()
        let body = TransferOwnershipRequest(newOwnerId: member.userId)
        do {
            let _: StatusResponse = try await api.post("/api/household/transfer", body: body)
            api = environment.apiClient.scoped()
            let owner = state.revision
            showingTransfer = false
            // Reload household data
            let data: HouseholdResponse = try await api.get("/api/household")
            guard state.revision == owner else { return }
            state.household = data.household
            state.members = data.members
            state.historicalMembers = data.historicalMembers
            state.invites = data.invites
        } catch {}
    }

    private func refreshHousehold() async {
        let api = environment.apiClient.scoped()
        let owner = state.revision
        do {
            let data: HouseholdResponse = try await api.get("/api/household")
            guard state.revision == owner else { return }
            state.household = data.household
            state.members = data.members
            state.historicalMembers = data.historicalMembers
            state.invites = data.invites
        } catch {}
    }

    private func startExport(household: Bool) {
        guard !isExporting else { return }
        let owner = state.revision
        let store = ActivityStore(api: environment.apiClient.scoped())
        let job = UUID()
        exportJob = job
        exportError = nil
        isExporting = true
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = .current
        formatter.dateFormat = "yyyy-MM-dd"
        let start = exportAllDates ? nil : formatter.string(from: exportStart)
        let end = exportAllDates ? nil : formatter.string(from: exportEnd)
        exportTask = Task {
            defer {
                if exportJob == job { isExporting = false; exportTask = nil }
            }
            do {
                let url: URL
                if household { url = try await store.exportHouseholdCSV(start: start, end: end) }
                else { url = try await store.exportLogsCSV(start: start, end: end) }
                guard state.revision == owner, exportJob == job, !Task.isCancelled else {
                    try? FileManager.default.removeItem(at: url)
                    return
                }
                clearExportFile()
                exportTempURL = url
                exportFile = ExportFile(url: url)
            } catch {
                guard state.revision == owner, exportJob == job, !Task.isCancelled else { return }
                exportError = error.localizedDescription
            }
        }
    }

    private func cancelExport() {
        exportTask?.cancel()
        exportTask = nil
        exportJob = UUID()
        isExporting = false
        exportError = nil
    }

    private func clearExportFile() {
        if let url = exportTempURL { try? FileManager.default.removeItem(at: url) }
        exportTempURL = nil
        exportFile = nil
    }

    private func resendVerification() async {
        let owner = state.revision
        let api = environment.apiClient.scoped()
        let _: StatusResponse? = try? await api.postEmpty("/api/auth/email/verification/resend")
        guard state.revision == owner else { return }
        verificationSent = true
    }

    private func activateHousehold(_ id: Int) async {
        var api = environment.apiClient.scoped()
        do {
            let _: StatusResponse = try await api.postEmpty("/api/households/\(id)/activate")
            api = environment.apiClient.scoped()
            let owner = state.revision
            let data: HouseholdResponse = try await api.get("/api/household")
            guard state.revision == owner else { return }
            state.household = data.household
            state.members = data.members
            state.historicalMembers = data.historicalMembers
            state.invites = data.invites
            state.activeHouseholdId = data.household.id
            // Reload chores for the newly activated household.
            if let chores: ChoresResponse = try? await api.get("/api/chores") {
                guard state.revision == owner else { return }
                state.chores = chores.chores
            }
        } catch {}
    }
}

struct PasswordChangeSheet: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @Environment(\.dismiss) private var dismiss
    @StateObject private var auth = AuthStore(api: APIClient(baseURL: URL(string: "http://localhost:8080")!))
    @State private var current = ""
    @State private var password = ""
    @State private var confirmation = ""
    @State private var validationError: String?

    private var hasPassword: Bool { state.user?.hasPassword != false }

    var body: some View {
        NavigationStack {
            Form {
                if hasPassword {
                    SecureField("Current Password", text: $current)
                        .textContentType(.password)
                }
                SecureField("New Password", text: $password)
                    .textContentType(.newPassword)
                SecureField("Confirm New Password", text: $confirmation)
                    .textContentType(.newPassword)
                if let error = validationError ?? auth.errorMessage {
                    Text(error).foregroundColor(DesignColors.danger)
                }
            }
            .navigationTitle(hasPassword ? "Change Password" : "Set Password")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }.disabled(auth.isLoading)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await save() } }
                        .disabled(auth.isLoading || password.isEmpty || confirmation.isEmpty || (hasPassword && current.isEmpty))
                }
            }
            .interactiveDismissDisabled(auth.isLoading)
            .onAppear { auth.configure(api: environment.apiClient) }
        }
    }

    private func save() async {
        let api = environment.apiClient.scoped()
        validationError = nil
        guard password == confirmation else {
            validationError = "New passwords do not match"
            return
        }
        guard (8...72).contains(password.utf8.count) else {
            validationError = "Password must be between 8 and 72 bytes"
            return
        }
        if let user = await auth.changePassword(current: hasPassword ? current : "", new: password) {
            state.user = user
            dismiss()
        }
    }
}

// MARK: - CSV export support

/// Wraps the downloaded export so it can drive `.sheet(item:)`.
struct ExportFile: Identifiable {
    let url: URL
    var id: String { url.absoluteString }
}

/// Native share sheet for the exported CSV file.
struct ShareSheet: UIViewControllerRepresentable {
    let items: [Any]

    func makeUIViewController(context: Context) -> UIActivityViewController {
        let controller = UIActivityViewController(activityItems: items, applicationActivities: nil)
        controller.view.accessibilityIdentifier = "export-share-sheet"
        return controller
    }

    func updateUIViewController(_ uiViewController: UIActivityViewController, context: Context) {}
}

// MARK: - Delete Account Sheet

/// In-app account deletion (App Store guideline 5.1.1(v)). The server
/// requires the typed confirmation {"confirm":"DELETE"}; a sole owner of a
/// multi-member household gets a 409 telling them to transfer ownership
/// first, surfaced verbatim below the button.
struct DeleteAccountSheet: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject private var accountDeletion: AccountDeletionModel
    @State private var confirmText = ""

    private var confirmed: Bool {
        confirmText.trimmingCharacters(in: .whitespaces) == "DELETE"
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text("Deleting your account permanently removes your data. Households where you are the only member are deleted with all their logs. This cannot be undone.")
                        .font(.subheadline)
                    Text("If you are the only owner of a household with other members, transfer ownership (or remove the other members) first.")
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                }

                Section {
                    TextField("Type DELETE to confirm", text: $confirmText)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.characters)

                    Button(role: .destructive) {
                        Task { await accountDeletion.delete(api: environment.apiClient) }
                    } label: {
                        if accountDeletion.isDeleting {
                            ProgressView()
                        } else {
                            Text("Delete My Account")
                                .frame(maxWidth: .infinity)
                        }
                    }
                    .disabled(!confirmed || accountDeletion.isDeleting)
                } footer: {
                    if let message = accountDeletion.errorMessage {
                        Text(message)
                            .foregroundColor(.red)
                    }
                }
            }
            .navigationTitle("Delete Account")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") {
                        if accountDeletion.matches(environment.apiClient) { state.currentTab = .settings }
                        accountDeletion.cancel()
                    }
                }
            }
        }
    }

}

// MARK: - Member Row

struct MemberRow: View {
    @EnvironmentObject var state: AppState
    let member: Member
    let isCurrentUser: Bool
    let currentUserRole: String
    let onRoleChange: (String) -> Void
    let onRemove: () -> Void

    @State private var showingRemoveConfirm = false

    var body: some View {
        HStack {
            Circle()
                .fill(Color(hex: member.avatarColor) ?? .gray)
                .frame(width: 36, height: 36)
                .overlay(
                    Text(String(member.displayName.prefix(1).uppercased()))
                        .font(.caption)
                        .foregroundColor(.white)
                )

            VStack(alignment: .leading) {
                Text(member.displayName.isEmpty ? member.email : member.displayName)
                    .font(.subheadline)
                Text(member.email)
                    .font(.caption)
                    .foregroundColor(.secondary)
            }

            Spacer()

            Text(member.role.capitalized)
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 8)
                .padding(.vertical, 2)
                .background(DesignColors.surfaceSecondary)
                .clipShape(Capsule())

            if currentUserRole == "owner" && !isCurrentUser {
                Menu {
                    Button {
                        onRoleChange(member.role == "member" ? "admin" : "member")
                    } label: {
                        Label(member.role == "member" ? "Make Admin" : "Make Member",
                              systemImage: "person.badge.shield.checkmark")
                    }

                    Button(role: .destructive) {
                        showingRemoveConfirm = true
                    } label: {
                        Label("Remove Member", systemImage: "person.badge.minus")
                    }
                } label: {
                    Image(systemName: "ellipsis.circle")
                        .foregroundColor(.secondary)
                }
                .alert("Remove \(member.displayName.isEmpty ? member.email : member.displayName)?", isPresented: $showingRemoveConfirm) {
                    Button("Remove", role: .destructive) { onRemove() }
                    Button("Cancel", role: .cancel) {}
                }
            }
        }
    }
}
