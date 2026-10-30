import AuthenticationServices
import SwiftUI

struct RegisterView: View {
    @Environment(AppState.self) var state
    @Environment(\.colorScheme) private var colorScheme
    var auth: AuthStore
    var googleAuth: GoogleOAuthCoordinator
    @State private var appleAuth = SignInWithAppleCoordinator()
    @Environment(\.dismiss) private var dismiss
    @State private var email = ""
    @State private var password = ""

    var body: some View {
        ScrollView {
            VStack(spacing: 24) {
                VStack(spacing: 8) {
                    NabuLogo(size: 56)
                    Text("Create Account")
                        .font(.title.weight(.bold))
                        .foregroundColor(DesignColors.textPrimary)
                    Text(authTagline)
                        .font(.subheadline)
                        .foregroundColor(DesignColors.textSecondary)
                        .multilineTextAlignment(.center)
                        .padding(.horizontal, 32)
                }
                .padding(.top, 24)

                if let invite = state.invitePreview {
                    InviteBanner(status: invite)
                }

                AuthCard {
                    VStack(spacing: 16) {
                        // Apple and Google first, the quickest way in.
                        // Sign up with Apple — App Store guideline 4.8
                        // requires it at least as prominent as other
                        // third-party sign-in options, hence above Google.
                        SignInWithAppleButton(.signUp) { request in
                            appleAuth.prepare(request, api: auth.api)
                        } onCompletion: { result in
                            performAppleSignIn(result)
                        }
                        .signInWithAppleButtonStyle(colorScheme == .dark ? .white : .black)
                        .frame(height: 44)
                        .clipShape(RoundedRectangle(cornerRadius: 8))
                        .accessibilityIdentifier("siwa-button")

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

                        OrDivider()

                        LabeledField("Email") {
                            TextField("you@example.com", text: $email)
                                .textContentType(.emailAddress)
                                .keyboardType(.emailAddress)
                                .autocapitalization(.none)
                                .disableAutocorrection(true)
                                .textFieldStyle(NabuTextFieldStyle())
                        }

                        LabeledField("Password") {
                            VStack(alignment: .leading, spacing: 6) {
                                PasswordField(placeholder: "Password", text: $password, contentType: .newPassword)
                                // The rule, checked as it's met.
                                Label("At least 8 characters", systemImage: passwordLongEnough ? "checkmark.circle.fill" : "circle")
                                    .font(.footnote)
                                    .foregroundColor(passwordLongEnough ? DesignColors.success : DesignColors.textSecondary)
                                    .accessibilityElement(children: .ignore)
                                    .accessibilityLabel("At least 8 characters")
                                    .accessibilityValue(passwordLongEnough ? "met" : "not met")
                                    .accessibilityIdentifier("password-rule")
                            }
                        }

                        if let error = auth.errorMessage ?? googleAuth.errorMessage ?? appleAuth.errorMessage {
                            Text(error)
                                .font(.callout)
                                .foregroundColor(DesignColors.danger)
                                .multilineTextAlignment(.center)
                        }

                        if let notice = auth.registrationNotice {
                            Text(notice)
                                .font(.callout)
                                .foregroundColor(.secondary)
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
                        .disabled(email.isEmpty || password.isEmpty
                                  || auth.isLoading || googleAuth.isAuthenticating || !isValid)
                        .accessibilityIdentifier("register-submit-button")


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

    private var passwordLongEnough: Bool { password.count >= 8 }

    /// As the server checks it: at most 72 bytes is bcrypt's limit.
    private var isValid: Bool {
        passwordLongEnough && password.utf8.count <= 72 && email.contains("@")
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

    private func performAppleSignIn(_ result: Result<ASAuthorization, Error>) {
        Task {
            if let user = await appleAuth.handle(result, api: auth.api) {
                state.user = user
            }
        }
    }
}
