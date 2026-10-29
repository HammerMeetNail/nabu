import Combine
import SwiftUI

/// Shown instead of onboarding when an administrator has disabled this
/// account's household membership. The message comes from the server and
/// names the household. Sign-out stays available so a disabled account can
/// leave the app or switch to another household (the PWA keeps its Account
/// section reachable in the same state).
struct AccessSuspendedView: View {
    let message: String
    var onSignOut: () -> Void = {}

    var body: some View {
        VStack(spacing: 16) {
            Spacer(minLength: 64)
            Image(systemName: "person.crop.circle.badge.xmark")
                .font(.system(size: 48))
                .foregroundColor(.secondary)
            Text("Access disabled")
                .font(.title2.weight(.bold))
            Text(message)
                .font(.subheadline)
                .foregroundColor(.secondary)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("access-suspended-message")
            Button("Sign Out", action: onSignOut)
                .buttonStyle(NabuPrimaryButtonStyle())
                .accessibilityIdentifier("access-suspended-sign-out")
            Spacer()
        }
        .padding()
        .frame(maxWidth: .infinity)
    }
}

// MARK: - Activity packs

/// The activity pack picker's state, shared by setup and Manage → Add
/// activities (PWA `onboarding.js`).
@MainActor
final class PackSelection: ObservableObject {
    static let maxNames = 6

    @Published var packs: [ActivityPack]?
    @Published var loadFailed = false
    /// The chosen packs, each with its name fields.
    @Published var names: [String: [String]] = [:]
    @Published var error: String?

    func isOn(_ key: String) -> Bool { names[key] != nil }

    func toggle(_ key: String) {
        names[key] = names[key] == nil ? [""] : nil
        error = nil
    }

    func addName(to key: String) {
        guard var fields = names[key], fields.count < Self.maxNames else { return }
        fields.append("")
        names[key] = fields
    }

    /// The packs to send, in the catalog's order, without blank names.
    var picks: [PackPick] {
        (packs ?? []).compactMap { pack in
            guard let fields = names[pack.key] else { return nil }
            let given = pack.nameLabel == nil ? [] : fields
                .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
                .filter { !$0.isEmpty }
            return PackPick(key: pack.key, names: given.isEmpty ? nil : given)
        }
    }

    /// The first name given twice across the picks, ignoring case. Names
    /// become activity names, so each must differ (the server checks too).
    static func duplicateName(in picks: [PackPick]) -> String? {
        var seen = Set<String>()
        for name in picks.flatMap({ $0.names ?? [] }) where !seen.insert(name.lowercased()).inserted {
            return name
        }
        return nil
    }

    /// Why these picks can't be sent yet, if they can't.
    var problem: String? {
        let picks = self.picks
        if picks.isEmpty { return "Pick at least one" }
        if let twice = Self.duplicateName(in: picks) { return "\"\(twice)\" is used twice; give each one a different name" }
        return nil
    }

    func load(api: APIClient) async {
        loadFailed = false
        do {
            let response: ActivityPacksResponse = try await api.get("/api/packs")
            packs = response.packs
        } catch {
            Log.loading.failure("Load activity packs", error)
            loadFailed = true
        }
    }

    /// Adds the picked packs to the current household. Returns how many
    /// activities were new, or nil with `error` set.
    func add(api: APIClient) async -> Int? {
        do {
            let response: AddPacksResponse = try await api.post("/api/chores/packs", body: AddPacksRequest(packs: picks))
            return response.added.count
        } catch APIError.serverError(_, let message) {
            error = message
        } catch {
            self.error = "Could not add the activities. Please retry."
        }
        return nil
    }
}

/// The pack picker for setup and Manage → Add activities.
struct PackPicker: View {
    @ObservedObject var selection: PackSelection
    let api: APIClient

    var body: some View {
        VStack(spacing: 8) {
            if let packs = selection.packs {
                ForEach(packs) { pack in row(pack) }
            } else if selection.loadFailed {
                Text("Couldn't load the activities.")
                    .foregroundColor(DesignColors.textSecondary)
                Button("Retry") { Task { await selection.load(api: api) } }
                    .buttonStyle(NabuSecondaryButtonStyle())
            } else {
                ProgressView().task { await selection.load(api: api) }
            }
        }
    }

    private func row(_ pack: ActivityPack) -> some View {
        let on = selection.isOn(pack.key)
        return VStack(alignment: .leading, spacing: 8) {
            Button { selection.toggle(pack.key) } label: {
                HStack(spacing: 12) {
                    Image(systemName: on ? "checkmark.circle.fill" : "circle")
                        .font(.title3)
                        .foregroundColor(on ? DesignColors.primary : DesignColors.textSecondary)
                    Text(pack.icon).font(.title2)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(pack.title).font(.body.weight(.semibold)).foregroundColor(DesignColors.textPrimary)
                        Text(pack.summary).font(.footnote).foregroundColor(DesignColors.textSecondary)
                    }
                    Spacer(minLength: 0)
                }
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("\(pack.title), \(pack.summary)")
            .accessibilityAddTraits(on ? [.isButton, .isSelected] : .isButton)
            .accessibilityIdentifier("pack-\(pack.key)")
            if on, let label = pack.nameLabel {
                let fields = selection.names[pack.key] ?? [""]
                ForEach(fields.indices, id: \.self) { index in
                    TextField("\(label) (optional)", text: nameBinding(pack.key, index))
                        .textFieldStyle(NabuTextFieldStyle())
                        .textInputAutocapitalization(.words)
                        .autocorrectionDisabled()
                        .accessibilityLabel(fields.count > 1 ? "\(label) \(index + 1)" : label)
                        .accessibilityIdentifier("pack-\(pack.key)-name-\(index)")
                }
                if fields.count < PackSelection.maxNames {
                    Button("Add another") { selection.addName(to: pack.key) }
                        .font(.callout)
                        .frame(minHeight: 44)
                        .accessibilityIdentifier("pack-\(pack.key)-add-name")
                }
            }
        }
        .padding(12)
        .background(RoundedRectangle(cornerRadius: 12)
            .strokeBorder(on ? DesignColors.primary : DesignColors.border, lineWidth: on ? 2 : 1))
    }

    private func nameBinding(_ key: String, _ index: Int) -> Binding<String> {
        Binding(
            get: { selection.names[key].flatMap { index < $0.count ? $0[index] : nil } ?? "" },
            set: { value in
                guard var fields = selection.names[key], index < fields.count else { return }
                fields[index] = String(value.prefix(30))
                selection.names[key] = fields
            })
    }
}

// MARK: - Household setup

/// Setup for a signed-in user without a household (PWA `onboarding.js`):
/// their name, the household's name, then who and what they look after,
/// which picks the activity packs to seed. The invite link comes last.
///
/// ContentView owns it. Creating the household changes the identity, which
/// replaces this screen for a moment and then loads the new household; the
/// setup must outlive both, and keep the main tabs back until it's done.
@MainActor
final class HouseholdSetup: ObservableObject {
    enum Step { case name, household, packs, invite, join }

    @Published var step: Step = .name
    @Published var name = ""
    @Published var color: String?
    @Published private(set) var householdName = ""
    @Published private(set) var initials = ""
    @Published var error: String?
    @Published var saving = false
    /// The household this setup created, whose invite link it shows.
    @Published private(set) var created: Household?
    /// From creating the household until the user leaves the invite step.
    @Published private(set) var isActive = false
    let packs = PackSelection()
    private var initialsEdited = false
    private var startedFor: Int?
    private var packChanges: AnyCancellable?

    init() {
        // The view reads the picker's state too (its error, whether it loaded).
        packChanges = packs.objectWillChange.sink { [weak self] _ in self?.objectWillChange.send() }
    }

    /// Whether setup keeps the main tabs back for this account: from creating
    /// the household until the user leaves the invite step.
    func holdsMainTabs(for user: User?) -> Bool {
        isActive && user != nil && user?.id == startedFor
    }

    /// Prefills the name and color once per account. New accounts are named
    /// after their email, which is no name to prefill.
    func start(user: User?) {
        guard let user, startedFor != user.id else { return }
        reset()
        startedFor = user.id
        let emailName = user.email.split(separator: "@").first.map(String.init) ?? ""
        name = user.displayName == emailName ? "" : user.displayName
        let current = user.avatarColor.uppercased()
        color = ProfileEditor.colors.contains { $0.hex == current } ? current : ProfileEditor.colors[0].hex
    }

    func reset() {
        step = .name; name = ""; color = nil; householdName = ""; initials = ""; initialsEdited = false
        error = nil; saving = false; created = nil; isActive = false; startedFor = nil
        packs.names = [:]; packs.error = nil
    }

    /// "Dana O'Connor" → "The O'Connors' home"; "Dana" → "Dana's home"
    /// (PWA `householdNameFor`).
    static func householdName(for name: String) -> String {
        let words = name.split(whereSeparator: \.isWhitespace).map(String.init)
        guard let last = words.last else { return "" }
        let suggestion: String
        if words.count == 1 {
            suggestion = "\(last)'s home"
        } else {
            let lower = last.lowercased()
            let plural = ["s", "x", "z", "ch", "sh"].contains { lower.hasSuffix($0) } ? "\(last)es" : "\(last)s"
            suggestion = "The \(plural)' home"
        }
        return suggestion.count <= 60 ? suggestion : ""
    }

    /// PWA `generateInitials`.
    static func initials(for name: String) -> String {
        let words = name.split(whereSeparator: \.isWhitespace).map(String.init)
        guard let first = words.first else { return "" }
        if words.count == 1 { return (first.count <= 3 ? first : String(first.prefix(2))).uppercased() }
        return words.compactMap(\.first).map(String.init).joined().uppercased()
    }

    /// Filling the name fills the initials, until the user types their own.
    func setHouseholdName(_ value: String) {
        householdName = String(value.prefix(60))
        if !initialsEdited { initials = String(Self.initials(for: householdName).prefix(4)) }
    }

    func setInitials(_ value: String) {
        initials = String(value.prefix(4))
        initialsEdited = true
    }

    func saveName(api: APIClient, state: AppState) async {
        saving = true
        error = await ProfileEditor.save(name: name, color: color, api: api, state: state)
        saving = false
        guard error == nil else { return }
        name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        if householdName.isEmpty { setHouseholdName(Self.householdName(for: name)) }
        step = .household
    }

    func continueFromHousehold() {
        guard !householdName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            error = "Name your household"
            return
        }
        error = nil
        step = .packs
    }

    /// Creates the household once, adds the picked packs, then shows the
    /// invite step. A household created without its packs stays on the
    /// picker to retry them.
    func create(auth: AuthStore, api: APIClient) async {
        if let problem = packs.problem { error = problem; return }
        saving = true
        error = nil
        defer { saving = false }
        if created == nil {
            isActive = true
            let trimmed = householdName.trimmingCharacters(in: .whitespacesAndNewlines)
            let typed = initials.trimmingCharacters(in: .whitespaces)
            guard let household = await auth.createHousehold(name: trimmed, initials: typed.isEmpty ? Self.initials(for: trimmed) : typed) else {
                isActive = false
                error = auth.errorMessage ?? "Could not create the household. Please retry."
                return
            }
            created = household
        }
        if await packs.add(api: api) != nil {
            step = .invite
        } else {
            error = "Your household is ready, but its activities weren't added. \(packs.error ?? "Please retry.")"
        }
    }

    /// Leaves setup for the main tabs once the household's data has reloaded,
    /// so Home shows the new activities.
    func finish(reload: () async -> Void) async {
        saving = true
        await reload()
        reset()
    }
}

struct OnboardingView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @ObservedObject var auth: AuthStore
    @ObservedObject var setup: HouseholdSetup
    /// Reloads the household's data before the main tabs show.
    var reload: () async -> Void = {}
    @State private var copied = false

    var body: some View {
        ScrollView {
            VStack(spacing: 24) {
                Spacer(minLength: 32)
                AuthCard {
                    VStack(alignment: .leading, spacing: 16) {
                        switch setup.step {
                        case .name: nameStep
                        case .household: householdStep
                        case .packs: packsStep
                        case .invite: inviteStep
                        case .join: joinStep
                        }
                    }
                }
                Spacer(minLength: 32)
            }
        }
        .pageBackground()
        .onAppear {
            setup.start(user: state.user)
            // An invite link opened before sign-in lands on joining, code filled in.
            if state.pendingInviteCode != nil, setup.step == .name { setup.step = .join }
        }
    }

    private func stepLabel(_ number: Int) -> some View {
        Text("Step \(number) of 3")
            .font(.footnote)
            .foregroundColor(DesignColors.textSecondary)
    }

    @ViewBuilder private var errorText: some View {
        if let error = setup.error ?? setup.packs.error {
            Text(error)
                .font(.callout)
                .foregroundColor(DesignColors.danger)
                .accessibilityIdentifier("onboarding-error")
        }
    }

    @ViewBuilder private var nameStep: some View {
        stepLabel(1)
        Text("Welcome to Nabu").font(.title.weight(.bold)).foregroundColor(DesignColors.textPrimary)
        LabeledField("What should your household call you?") {
            TextField("Your name", text: $setup.name)
                .textFieldStyle(NabuTextFieldStyle())
                .textContentType(.name)
                .accessibilityIdentifier("onboarding-name")
                .onChange(of: setup.name) { _, value in
                    if value.count > 50 { setup.name = String(value.prefix(50)) }
                }
        }
        Text("Your color").font(.subheadline.weight(.medium)).foregroundColor(DesignColors.textSecondary)
        LazyVGrid(columns: [GridItem(.adaptive(minimum: 44))], spacing: 8) {
            ForEach(ProfileEditor.colors, id: \.hex) { option in
                Button { setup.color = option.hex } label: {
                    Circle()
                        .fill(Color(hex: option.hex) ?? .gray)
                        .frame(width: 40, height: 40)
                        .overlay(Circle().strokeBorder(Color.primary, lineWidth: setup.color == option.hex ? 3 : 0))
                        .frame(width: 44, height: 44)
                }
                .buttonStyle(.plain)
                .accessibilityLabel(option.name)
                .accessibilityAddTraits(setup.color == option.hex ? .isSelected : [])
            }
        }
        errorText
        Button {
            Task { await setup.saveName(api: environment.apiClient, state: state) }
        } label: {
            if setup.saving { ProgressView().tint(.white) } else { Text("Continue") }
        }
        .buttonStyle(NabuPrimaryButtonStyle())
        .disabled(setup.saving)
        .accessibilityIdentifier("onboarding-continue")
        Button("Joining someone? Enter an invite code") {
            setup.error = nil
            auth.errorMessage = nil
            setup.step = .join
        }
            .font(.callout)
            .frame(maxWidth: .infinity, minHeight: 44)
            .accessibilityIdentifier("onboarding-join")
    }

    @ViewBuilder private var householdStep: some View {
        stepLabel(2)
        Text("Name your household").font(.title2.weight(.bold)).foregroundColor(DesignColors.textPrimary)
        LabeledField("Household name") {
            TextField("Our home", text: Binding(get: { setup.householdName }, set: { setup.setHouseholdName($0) }))
                .textFieldStyle(NabuTextFieldStyle())
                .accessibilityIdentifier("onboarding-household")
        }
        LabeledField("Initials (shown in the header)") {
            TextField("OH", text: Binding(get: { setup.initials }, set: { setup.setInitials($0) }))
                .textFieldStyle(NabuTextFieldStyle())
                .textInputAutocapitalization(.characters)
                .autocorrectionDisabled()
                .accessibilityIdentifier("onboarding-initials")
        }
        errorText
        HStack(spacing: 8) {
            Button("Back") { setup.error = nil; setup.step = .name }
                .buttonStyle(NabuSecondaryButtonStyle())
            Button("Continue") { setup.continueFromHousehold() }
                .buttonStyle(NabuPrimaryButtonStyle())
                .accessibilityIdentifier("onboarding-continue")
        }
    }

    @ViewBuilder private var packsStep: some View {
        stepLabel(3)
        Text("Who and what do you look after?").font(.title2.weight(.bold)).foregroundColor(DesignColors.textPrimary)
        Text("Pick everything that applies. You can add more later in Manage.")
            .font(.subheadline)
            .foregroundColor(DesignColors.textSecondary)
        PackPicker(selection: setup.packs, api: environment.apiClient)
        errorText
        HStack(spacing: 8) {
            if setup.created == nil {
                Button("Back") { setup.error = nil; setup.step = .household }
                    .buttonStyle(NabuSecondaryButtonStyle())
            }
            Button {
                Task { await setup.create(auth: auth, api: environment.apiClient) }
            } label: {
                if setup.saving { ProgressView().tint(.white) }
                else { Text(setup.created == nil ? "Create household" : "Add activities") }
            }
            .buttonStyle(NabuPrimaryButtonStyle())
            .disabled(setup.saving || setup.packs.packs == nil)
            .accessibilityIdentifier("onboarding-create")
        }
    }

    @ViewBuilder private var inviteStep: some View {
        let code = setup.created?.inviteCode ?? ""
        let link = "\(environment.baseURL.absoluteString)/join?code=\(code)"
        Text("Invite your household").font(.title2.weight(.bold)).foregroundColor(DesignColors.textPrimary)
        Text("Nabu works best when everyone logs. Send this link to the people you share the care with, or let them scan the code.")
            .foregroundColor(DesignColors.textSecondary)
        QRCodeView(text: link)
            .frame(maxWidth: .infinity)
        Text(link)
            .font(.footnote.monospaced())
            .textSelection(.enabled)
            .accessibilityIdentifier("onboarding-invite-link")
        if let url = URL(string: link) {
            ShareLink(item: url) {
                Label("Share link", systemImage: "square.and.arrow.up").frame(maxWidth: .infinity)
            }
            .buttonStyle(NabuPrimaryButtonStyle())
            .accessibilityIdentifier("onboarding-share")
        }
        Button(copied ? "Copied" : "Copy link") {
            UIPasteboard.general.string = link
            copied = true
        }
        .buttonStyle(NabuSecondaryButtonStyle())
        Button("Go to Home") {
            Task { await setup.finish(reload: reload) }
        }
        .frame(maxWidth: .infinity, minHeight: 44)
        .disabled(setup.saving)
        .accessibilityIdentifier("onboarding-done")
    }

    @ViewBuilder private var joinStep: some View {
        Text("Join a household").font(.title2.weight(.bold)).foregroundColor(DesignColors.textPrimary)
        Text("Enter the code from your invite, or open the invite link on this device.")
            .font(.subheadline)
            .foregroundColor(DesignColors.textSecondary)
        JoinHouseholdView(auth: auth)
        Button("Set up a new household instead") { setup.step = .name }
            .font(.callout)
            .frame(maxWidth: .infinity, minHeight: 44)
            .accessibilityIdentifier("onboarding-new-household")
    }
}

struct JoinHouseholdView: View {
    @EnvironmentObject var state: AppState
    @ObservedObject var auth: AuthStore
    @State private var code = ""

    var body: some View {
        VStack(spacing: 12) {
            LabeledField("Invite Code") {
                TextField("Enter code", text: $code)
                    .textFieldStyle(NabuTextFieldStyle())
                    .autocapitalization(.allCharacters)
                    .disableAutocorrection(true)
                    .onChange(of: code) { _, newValue in
                        code = newValue.uppercased()
                    }
            }

            if let error = auth.errorMessage {
                Text(error)
                    .font(.callout)
                    .foregroundColor(DesignColors.danger)
            }

            Button(action: join) {
                if auth.isLoading {
                    ProgressView()
                        .tint(.white)
                } else {
                    Text("Join Household")
                }
            }
            .buttonStyle(NabuPrimaryButtonStyle())
            .disabled(code.isEmpty || auth.isLoading)
        }
        .onAppear {
            // Prefill from a /join universal link opened before sign-in.
            if code.isEmpty, let pending = state.pendingInviteCode {
                code = pending.uppercased()
            }
        }
    }

    private func join() {
        Task {
            if let household = await auth.joinHousehold(code: code) {
                state.pendingInviteCode = nil
                // The household has its activities already: joining seeds none.
                state.household = household
                state.activeHouseholdId = household.id
            }
        }
    }
}
