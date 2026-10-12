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

struct OnboardingView: View {
    @EnvironmentObject var state: AppState
    @ObservedObject var auth: AuthStore
    @State private var mode: OnboardingMode = .create

    /// An invite link opened before sign-in lands the user on the Join tab
    /// with the code filled in.
    private var pendingInviteCode: String? { state.pendingInviteCode }

    enum OnboardingMode {
        case create
        case join
    }

    var body: some View {
        ScrollView {
            VStack(spacing: 24) {
                Spacer(minLength: 48)

                VStack(spacing: 4) {
                    Text("Welcome!")
                        .font(.title.weight(.bold))
                        .foregroundColor(DesignColors.textPrimary)
                    Text("You need a household to get started.")
                        .foregroundColor(DesignColors.textSecondary)
                }

                AuthCard {
                    VStack(spacing: 16) {
                        PillTabBar(
                            selection: $mode,
                            tabs: [OnboardingMode.create, OnboardingMode.join],
                            labelFor: { m in
                                switch m {
                                case .create: return "Create"
                                case .join:   return "Join"
                                }
                            }
                        )

                        switch mode {
                        case .create:
                            CreateHouseholdView(auth: auth)
                        case .join:
                            JoinHouseholdView(auth: auth)
                        }
                    }
                }

                Spacer(minLength: 32)
            }
        }
        .pageBackground()
        .onAppear {
            if pendingInviteCode != nil {
                mode = .join
            }
        }
    }
}

struct CreateHouseholdView: View {
    @EnvironmentObject var state: AppState
    @ObservedObject var auth: AuthStore
    @State private var name = ""
    @State private var initials = ""

    var body: some View {
        VStack(spacing: 12) {
            LabeledField("Household Name") {
                TextField("e.g. Smith Family", text: $name)
                    .textFieldStyle(NabuTextFieldStyle())
            }

            LabeledField("Initials (2-3 chars)") {
                TextField("e.g. SF", text: $initials)
                    .textFieldStyle(NabuTextFieldStyle())
                    .onChange(of: initials) { _, newValue in
                        if newValue.count > 3 {
                            initials = String(newValue.prefix(3))
                        }
                    }
            }

            if let error = auth.errorMessage {
                Text(error)
                    .font(.callout)
                    .foregroundColor(DesignColors.danger)
            }

            Button(action: createAndSeed) {
                if auth.isLoading {
                    ProgressView()
                        .tint(.white)
                } else {
                    Text("Create Household")
                }
            }
            .buttonStyle(NabuPrimaryButtonStyle())
            .disabled(name.isEmpty || initials.count < 2 || auth.isLoading)
            .accessibilityIdentifier("create-household-button")
        }
    }

    private func createAndSeed() {
        Task {
            if let household = await auth.createHousehold(name: name, initials: initials) {
                // Seed defaults BEFORE setting state.household so that
                // the onChange handler's loadAppData() picks up the freshly
                // seeded chores, avoiding FK violations on log save.
                let _ = await auth.seedDefaults()
                state.household = household
                state.activeHouseholdId = household.id
            }
        }
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
                // Seed defaults BEFORE setting state.household — same race
                // condition fix as CreateHouseholdView.createAndSeed().
                let _ = await auth.seedDefaults()
                state.household = household
                state.activeHouseholdId = household.id
            }
        }
    }
}
