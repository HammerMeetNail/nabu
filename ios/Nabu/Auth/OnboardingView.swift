import SwiftUI

struct OnboardingView: View {
    @EnvironmentObject var state: AppState
    @ObservedObject var auth: AuthStore
    @State private var mode: OnboardingMode = .create

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
                        .font(.system(size: 28, weight: .bold))
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
        }
    }

    private func createAndSeed() {
        Task {
            if let household = await auth.createHousehold(name: name, initials: initials) {
                state.household = household
                state.activeHouseholdId = household.id
                let _ = await auth.seedDefaults()
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
    }

    private func join() {
        Task {
            if let household = await auth.joinHousehold(code: code) {
                state.household = household
                state.activeHouseholdId = household.id
                let _ = await auth.seedDefaults()
            }
        }
    }
}
