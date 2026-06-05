import SwiftUI

struct RegisterView: View {
    @EnvironmentObject var state: AppState
    @ObservedObject var auth: AuthStore
    @ObservedObject var googleAuth: GoogleOAuthCoordinator
    @Environment(\.dismiss) private var dismiss
    @State private var email = ""
    @State private var password = ""
    @State private var confirmPassword = ""

    var body: some View {
        ScrollView {
            VStack(spacing: 24) {
                Text("Create Account")
                    .font(.system(size: 28, weight: .bold))
                    .foregroundColor(DesignColors.textPrimary)
                    .padding(.top, 48)

                AuthCard {
                    VStack(spacing: 16) {
                        LabeledField("Email") {
                            TextField("you@example.com", text: $email)
                                .textContentType(.emailAddress)
                                .keyboardType(.emailAddress)
                                .autocapitalization(.none)
                                .disableAutocorrection(true)
                                .textFieldStyle(NabuTextFieldStyle())
                        }

                        LabeledField("Password") {
                            SecureField("Password (min 8 characters)", text: $password)
                                .textContentType(.newPassword)
                                .textFieldStyle(NabuTextFieldStyle())
                        }

                        LabeledField("Confirm Password") {
                            SecureField("Confirm Password", text: $confirmPassword)
                                .textContentType(.newPassword)
                                .textFieldStyle(NabuTextFieldStyle())
                        }

                        if let error = auth.errorMessage ?? googleAuth.errorMessage {
                            Text(error)
                                .font(.callout)
                                .foregroundColor(DesignColors.danger)
                                .multilineTextAlignment(.center)
                        }

                        // Create Account (primary)
                        Button(action: performRegister) {
                            if auth.isLoading {
                                ProgressView()
                                    .tint(.white)
                            } else {
                                Text("Create Account")
                            }
                        }
                        .buttonStyle(NabuPrimaryButtonStyle())
                        .disabled(email.isEmpty || password.isEmpty || confirmPassword.isEmpty
                                  || auth.isLoading || googleAuth.isAuthenticating || !isValid)

                        OrDivider()

                        // Google
                        Button(action: performGoogleSignIn) {
                            HStack(spacing: 10) {
                                if googleAuth.isAuthenticating {
                                    ProgressView()
                                } else {
                                    GoogleIconSimple()
                                    Text("Continue with Google")
                                }
                            }
                        }
                        .buttonStyle(NabuGoogleButtonStyle())
                        .disabled(auth.isLoading || googleAuth.isAuthenticating)

                        // Back to sign in
                        Button("Already have an account? Sign in") {
                            dismiss()
                        }
                        .font(.subheadline)
                        .foregroundColor(DesignColors.primary)
                    }
                }

                Spacer(minLength: 32)
            }
        }
        .pageBackground()
        .navigationTitle("")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var isValid: Bool {
        password.count >= 8 && password == confirmPassword && email.contains("@")
    }

    private func performRegister() {
        Task {
            if let user = await auth.register(email: email, password: password) {
                state.user = user
            }
        }
    }

    private func performGoogleSignIn() {
        Task {
            if let user = await googleAuth.authenticate() {
                state.user = user
            }
        }
    }
}
