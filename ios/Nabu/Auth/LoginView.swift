import SwiftUI

struct LoginView: View {
    @EnvironmentObject var state: AppState
    @ObservedObject var auth: AuthStore
    @StateObject private var googleAuth: GoogleOAuthCoordinator
    @State private var email = ""
    @State private var password = ""
    @State private var showRegister = false
    @State private var showMagicLink = false
    @State private var showForgotPassword = false

    init(auth: AuthStore, apiBaseURL: URL) {
        self.auth = auth
        self._googleAuth = StateObject(wrappedValue: GoogleOAuthCoordinator(baseURL: apiBaseURL))
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 24) {
                    // Logo
                    Text("Nabu")
                        .font(.system(size: 36, weight: .bold))
                        .foregroundColor(DesignColors.brand)
                        .padding(.top, 48)

                    // Card
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
                                SecureField("Password", text: $password)
                                    .textContentType(.password)
                                    .textFieldStyle(NabuTextFieldStyle())
                            }

                            if let error = auth.errorMessage ?? googleAuth.errorMessage {
                                Text(error)
                                    .font(.callout)
                                    .foregroundColor(DesignColors.danger)
                                    .multilineTextAlignment(.center)
                            }

                            // Sign In (primary)
                            Button(action: performLogin) {
                                if auth.isLoading {
                                    ProgressView()
                                        .tint(.white)
                                } else {
                                    Text("Sign In")
                                }
                            }
                            .buttonStyle(NabuPrimaryButtonStyle())
                            .disabled(email.isEmpty || password.isEmpty || auth.isLoading || googleAuth.isAuthenticating)

                            // Magic link link
                            Button("Sign in with magic link") {
                                showMagicLink = true
                            }
                            .font(.subheadline)
                            .foregroundColor(DesignColors.primary)

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

                            OrDivider()

                            // Create Account (secondary)
                            Button("Create Account") {
                                showRegister = true
                            }
                            .buttonStyle(NabuSecondaryButtonStyle())

                            // Forgot password
                            Button("Forgot password?") {
                                showForgotPassword = true
                            }
                            .font(.subheadline)
                            .foregroundColor(DesignColors.textSecondary)
                        }
                    }

                    Spacer(minLength: 32)
                }
            }
            .pageBackground()
            .navigationDestination(isPresented: $showRegister) {
                RegisterView(auth: auth, googleAuth: googleAuth)
            }
            .navigationDestination(isPresented: $showMagicLink) {
                MagicLinkView(auth: auth)
            }
            .navigationDestination(isPresented: $showForgotPassword) {
                ForgotPasswordView(auth: auth)
            }
        }
    }

    private func performLogin() {
        Task {
            if let user = await auth.login(email: email, password: password) {
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
