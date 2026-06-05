import SwiftUI

struct MagicLinkView: View {
    @ObservedObject var auth: AuthStore
    @State private var email = ""
    @State private var sent = false

    var body: some View {
        VStack(spacing: 20) {
            Spacer()

            if sent {
                Text("Check Your Email")
                    .font(.largeTitle)
                    .fontWeight(.bold)

                Text("We sent a magic link to your email address. Click the link to sign in.")
                    .multilineTextAlignment(.center)
                    .foregroundColor(.secondary)
                    .padding(.horizontal)
            } else {
                Text("Magic Link")
                    .font(.largeTitle)
                    .fontWeight(.bold)

                Text("Enter your email and we'll send you a link to sign in.")
                    .multilineTextAlignment(.center)
                    .foregroundColor(.secondary)
                    .padding(.horizontal)

                VStack(spacing: 12) {
                    TextField("Email", text: $email)
                        .textContentType(.emailAddress)
                        .keyboardType(.emailAddress)
                        .autocapitalization(.none)
                        .disableAutocorrection(true)
                        .textFieldStyle(.roundedBorder)
                }
                .padding(.horizontal)

                Button(action: sendLink) {
                    if auth.isLoading {
                        ProgressView()
                            .tint(.white)
                    } else {
                        Text("Send Magic Link")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.borderedProminent)
                .disabled(email.isEmpty || auth.isLoading)
                .padding(.horizontal)
            }

            Spacer()
        }
        .padding()
    }

    private func sendLink() {
        Task {
            let _ = await auth.requestMagicLink(email: email)
            sent = true
        }
    }
}

struct ForgotPasswordView: View {
    @ObservedObject var auth: AuthStore
    @State private var email = ""
    @State private var sent = false

    var body: some View {
        VStack(spacing: 20) {
            Spacer()

            if sent {
                Text("Check Your Email")
                    .font(.largeTitle)
                    .fontWeight(.bold)

                Text("If an account exists, we sent a password reset link.")
                    .multilineTextAlignment(.center)
                    .foregroundColor(.secondary)
                    .padding(.horizontal)
            } else {
                Text("Forgot Password")
                    .font(.largeTitle)
                    .fontWeight(.bold)

                Text("We'll send you a password reset link.")
                    .multilineTextAlignment(.center)
                    .foregroundColor(.secondary)
                    .padding(.horizontal)

                VStack(spacing: 12) {
                    TextField("Email", text: $email)
                        .textContentType(.emailAddress)
                        .keyboardType(.emailAddress)
                        .autocapitalization(.none)
                        .disableAutocorrection(true)
                        .textFieldStyle(.roundedBorder)
                }
                .padding(.horizontal)

                Button(action: sendReset) {
                    if auth.isLoading {
                        ProgressView()
                            .tint(.white)
                    } else {
                        Text("Send Reset Link")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.borderedProminent)
                .disabled(email.isEmpty || auth.isLoading)
                .padding(.horizontal)
            }

            Spacer()
        }
        .padding()
    }

    private func sendReset() {
        Task {
            let _ = await auth.requestPasswordReset(email: email)
            sent = true
        }
    }
}

struct ResetPasswordView: View {
    @EnvironmentObject var state: AppState
    @ObservedObject var auth: AuthStore
    let token: String
    @State private var password = ""
    @State private var confirmPassword = ""

    var body: some View {
        VStack(spacing: 20) {
            Spacer()

            Text("Reset Password")
                .font(.largeTitle)
                .fontWeight(.bold)

            VStack(spacing: 12) {
                SecureField("New Password (min 8 characters)", text: $password)
                    .textContentType(.newPassword)
                    .textFieldStyle(.roundedBorder)

                SecureField("Confirm Password", text: $confirmPassword)
                    .textContentType(.newPassword)
                    .textFieldStyle(.roundedBorder)
            }
            .padding(.horizontal)

            if let error = auth.errorMessage {
                Text(error)
                    .font(.callout)
                    .foregroundColor(.red)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal)
            }

            Button(action: performReset) {
                if auth.isLoading {
                    ProgressView()
                        .tint(.white)
                } else {
                    Text("Reset Password")
                        .frame(maxWidth: .infinity)
                }
            }
            .buttonStyle(.borderedProminent)
            .disabled(password.count < 8 || password != confirmPassword || auth.isLoading)
            .padding(.horizontal)

            Spacer()
        }
        .padding()
    }

    private func performReset() {
        Task {
            if let user = await auth.resetPassword(token: token, password: password) {
                state.user = user
            }
        }
    }
}

struct VerifyEmailView: View {
    let success: Bool

    var body: some View {
        VStack(spacing: 20) {
            Spacer()

            Text(success ? "Email Verified!" : "Verify Your Email")
                .font(.largeTitle)
                .fontWeight(.bold)

            Text(success
                 ? "Your email has been verified. You can now sign in."
                 : "Check your email for a verification link.")
                .multilineTextAlignment(.center)
                .foregroundColor(.secondary)
                .padding(.horizontal)

            Spacer()
        }
        .padding()
    }
}
