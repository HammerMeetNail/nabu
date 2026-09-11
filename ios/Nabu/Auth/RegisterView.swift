import AuthenticationServices
import SwiftUI

struct RegisterView: View {
    @EnvironmentObject var state: AppState
    @Environment(\.colorScheme) private var colorScheme
    @ObservedObject var auth: AuthStore
    @ObservedObject var googleAuth: GoogleOAuthCoordinator
    @StateObject private var appleAuth = SignInWithAppleCoordinator()
    @Environment(\.dismiss) private var dismiss
    @State private var email = ""
    @State private var password = ""
    @State private var confirmPassword = ""

    var body: some View {
        ScrollView {
            VStack(spacing: 24) {
                Text("Create Account")
                    .font(.title.weight(.bold))
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
                        .disabled(email.isEmpty || password.isEmpty || confirmPassword.isEmpty
                                  || auth.isLoading || googleAuth.isAuthenticating || !isValid)
                        .accessibilityIdentifier("register-submit-button")

                        OrDivider()

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

    private func performAppleSignIn(_ result: Result<ASAuthorization, Error>) {
        Task {
            if let user = await appleAuth.handle(result, api: auth.api) {
                state.user = user
            }
        }
    }
}
