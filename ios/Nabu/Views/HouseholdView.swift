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

    var body: some View {
        NavigationStack {
            List {
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
                                if let code = household.inviteCode {
                                    Text("\(environment.baseURL.absoluteString)/join/\(code)")
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
                if !state.invites.isEmpty {
                    Section("Invites") {
                        ForEach(state.invites) { invite in
                            HStack {
                                VStack(alignment: .leading) {
                                    Text("\(environment.baseURL.absoluteString)/join/\(invite.code)")
                                        .font(.system(.caption, design: .monospaced))
                                        .lineLimit(1)
                                        .minimumScaleFactor(0.7)
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

                // Invite button
                Section {
                    Button {
                        Task { await createInvite() }
                    } label: {
                        Label("Create Invite Code", systemImage: "person.badge.plus")
                    }
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
                        Task { await leaveHousehold() }
                    }
                }

                // Account
                if let user = state.user {
                    Section("Account") {
                        HStack {
                            Text("Email")
                            Spacer()
                            Text(user.email)
                                .foregroundColor(.secondary)
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
                            if state.unreadNotifications > 0 {
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
                }

                Section {
                    Button("Sign Out", role: .destructive) {
                        Task {
                            await auth.logout()
                            state.reset()
                        }
                    }
                }
            }
            .navigationTitle("Settings")
        }
        .onAppear {
            auth.configure(api: environment.apiClient)
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
                Text("Share this code: \(code)")
            }
        }
    }

    private func saveHousehold() async {
        isSaving = true
        let body = UpdateHouseholdRequest(name: householdName, initials: householdInitials)
        do {
            let resp: HouseholdResponse = try await environment.apiClient.patch("/api/household", body: body)
            state.household = resp.household
            showingEdit = false
        } catch {}
        isSaving = false
    }

    private func createInvite() async {
        do {
            let resp: InviteResponse = try await environment.apiClient.postEmpty("/api/household/invites")
            state.invites.append(resp.invite)
            inviteCode = resp.invite.code
        } catch {}
    }

    private func deleteInvite(_ invite: Invite) async {
        do {
            let _: StatusResponse = try await environment.apiClient.delete("/api/household/invites/\(invite.id)")
            state.invites.removeAll { $0.id == invite.id }
        } catch {}
    }

    private func updateMemberRole(_ member: Member, role: String) async {
        let newRole = member.role == "member" ? "admin" : "member"
        let body = UpdateMemberRoleRequest(role: newRole)
        do {
            let _: StatusResponse = try await environment.apiClient.patch("/api/household/members/\(member.userId)", body: body)
            if let idx = state.members.firstIndex(where: { $0.userId == member.userId }) {
                let updated = Member(userId: member.userId, email: member.email,
                                     displayName: member.displayName, avatarColor: member.avatarColor,
                                     emailVerified: member.emailVerified, role: newRole)
                state.members[idx] = updated
            }
        } catch {}
    }

    private func removeMember(_ member: Member) async {
        do {
            let _: StatusResponse = try await environment.apiClient.delete("/api/household/members/\(member.userId)")
            state.members.removeAll { $0.userId == member.userId }
        } catch {}
    }

    private func leaveHousehold() async {
        do {
            let _: StatusResponse = try await environment.apiClient.postEmpty("/api/household/leave")
            state.resetHouseholdScoped()
        } catch {}
    }

    private func transferOwnership(to member: Member) async {
        let body = TransferOwnershipRequest(newOwnerId: member.userId)
        do {
            let _: StatusResponse = try await environment.apiClient.post("/api/household/transfer", body: body)
            showingTransfer = false
            // Reload household data
            let data: HouseholdResponse = try await environment.apiClient.get("/api/household")
            state.household = data.household
            state.members = data.members
            state.invites = data.invites
        } catch {}
    }

    private func activateHousehold(_ id: Int) async {
        do {
            let _: StatusResponse = try await environment.apiClient.postEmpty("/api/households/\(id)/activate")
            state.resetHouseholdScoped()
            let data: HouseholdResponse = try await environment.apiClient.get("/api/household")
            state.household = data.household
            state.members = data.members
            state.invites = data.invites
            state.activeHouseholdId = data.household.id
        } catch {}
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
