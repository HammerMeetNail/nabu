import AuthenticationServices
import SwiftUI

struct LoginView: View {
    @Environment(AppState.self) var state
    @Environment(\.colorScheme) private var colorScheme
    var auth: AuthStore
    @State private var googleAuth: GoogleOAuthCoordinator
    @State private var appleAuth = SignInWithAppleCoordinator()
    @State private var email = ""
    @State private var password = ""
    @State private var showRegister = false
    @State private var showMagicLink = false
    @State private var showForgotPassword = false

    init(auth: AuthStore, apiBaseURL: URL) {
        self.auth = auth
        self._googleAuth = State(initialValue: GoogleOAuthCoordinator(baseURL: apiBaseURL))
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 24) {
                    // Logo, name and what Nabu is for
                    VStack(spacing: 8) {
                        NabuLogo(size: 64)
                        Text("Nabu")
                            .font(.largeTitle.weight(.bold))
                            .foregroundColor(DesignColors.brand)
                        Text(authTagline)
                            .font(.subheadline)
                            .foregroundColor(DesignColors.textSecondary)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 32)
                    }
                    .padding(.top, 32)

                    if let invite = state.invitePreview {
                        InviteBanner(status: invite)
                    }

                    // Card
                    AuthCard {
                        VStack(spacing: 16) {
                            // Apple and Google first, the quickest way in.
                            // Sign in with Apple — App Store guideline 4.8
                            // requires it at least as prominent as other
                            // third-party sign-in options, hence above Google.
                            SignInWithAppleButton(.signIn) { request in
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
                                PasswordField(placeholder: "Password", text: $password)
                            }

                            if let error = auth.errorMessage ?? googleAuth.errorMessage ?? appleAuth.errorMessage {
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

    private func performAppleSignIn(_ result: Result<ASAuthorization, Error>) {
        Task {
            if let user = await appleAuth.handle(result, api: auth.api) {
                state.user = user
            }
        }
    }
}

/// What a pending invite link is for, above sign-in and sign-up: who sent it
/// and to which household, or that the link is dead (the PWA's /join page).
struct InviteBanner: View {
    let status: InvitePreviewStatus

    var body: some View {
        switch status {
        case .ready(let preview):
            VStack(spacing: 8) {
                if let inviter = preview.inviter {
                    MemberAvatar(name: inviter.displayName, colorHex: inviter.avatarColor, size: 56)
                }
                Text(preview.inviter.map { "\($0.displayName) invited you to join" } ?? "You're invited to join")
                    .font(.subheadline)
                    .foregroundColor(DesignColors.textSecondary)
                Text(preview.household.name)
                    .font(.title2.weight(.bold))
                    .foregroundColor(DesignColors.textPrimary)
                Text("Create an account or sign in to join.")
                    .font(.footnote)
                    .foregroundColor(DesignColors.textSecondary)
            }
            .multilineTextAlignment(.center)
            .padding(.horizontal, 24)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("invite-banner")
        case .invalid:
            Text("This invite link isn't valid any more. Ask whoever sent it for a new one.")
                .font(.callout)
                .foregroundColor(DesignColors.textSecondary)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 24)
                .accessibilityIdentifier("invite-banner-invalid")
        }
    }
}
