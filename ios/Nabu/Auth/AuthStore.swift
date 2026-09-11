import Foundation

@MainActor
final class AuthStore: ObservableObject {
    private(set) var api: APIClient
    @Published var isLoading = false
    @Published var errorMessage: String?
    @Published var registrationNotice: String?

    init(api: APIClient) {
        self.api = api
    }

    func configure(api: APIClient) {
        self.api = api
    }

    // MARK: - Session

    func loadSession() async -> User? {
        do {
            let response: UserResponse = try await api.get("/api/me")
            return api.identity.managed ? api.identity.snapshot.user : response.user
        } catch {
            return nil
        }
    }

    // MARK: - Login / Register

    func login(email: String, password: String) async -> User? {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            let req = LoginRequest(email: email, password: password)
            let response: UserResponse = try await api.post("/api/auth/login", body: req)
            return api.identity.managed ? api.identity.snapshot.user : response.user
        } catch let error as APIError {
            errorMessage = error.errorDescription ?? "Login failed"
            return nil
        } catch {
            errorMessage = "Login failed"
            return nil
        }
    }

    func register(email: String, password: String) async -> User? {
        isLoading = true
        errorMessage = nil
        registrationNotice = nil
        defer { isLoading = false }

        do {
            let req = RegisterRequest(email: email, password: password)
            let response: UserResponse = try await api.post("/api/auth/register", body: req)
            if response.user == nil {
                registrationNotice = "If this email is new, check your inbox. You can also sign in or request a magic link."
            }
            return api.identity.managed ? api.identity.snapshot.user : response.user
        } catch let error as APIError {
            errorMessage = error.errorDescription ?? "Registration failed"
            return nil
        } catch {
            errorMessage = "Registration failed"
            return nil
        }
    }

    @discardableResult
    func logout() async -> Bool {
        do {
            // The server revokes the session and all of its delivery bindings
            // atomically. Local cookies remain available until it confirms.
            let _: StatusResponse = try await api.postEmpty("/api/auth/logout")
            api.cookieStore.clearAll()
            PushRegistrationController.shared.didConfirmLogout()
            return true
        } catch {
            errorMessage = "Sign-out is unfinished. Your data is hidden; retry to revoke this session."
            return false
        }
    }

    // MARK: - Magic Link

    func requestMagicLink(email: String) async -> Bool {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            let req = MagicLinkRequest(email: email)
            let _: StatusResponse = try await api.post("/api/auth/magic-link/request", body: req)
            return true
        } catch {
            errorMessage = nil
            return true // anti-enumeration: always return success
        }
    }

    // MARK: - Password Reset

    func requestPasswordReset(email: String) async -> Bool {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            let req = ForgotPasswordRequest(email: email)
            let _: StatusResponse = try await api.post("/api/auth/password/forgot", body: req)
            return true
        } catch {
            errorMessage = nil
            return true // anti-enumeration: always return success
        }
    }

    func resetPassword(token: String, password: String) async -> User? {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            let req = ResetPasswordRequest(token: token, password: password)
            let response: UserResponse = try await api.post("/api/auth/password/reset", body: req)
            return api.identity.managed ? api.identity.snapshot.user : response.user
        } catch let error as APIError {
            errorMessage = error.errorDescription ?? "Password reset failed"
            return nil
        } catch {
            errorMessage = "Password reset failed"
            return nil
        }
    }

    func changePassword(current: String, new: String) async -> User? {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            let req = ChangePasswordRequest(currentPassword: current, newPassword: new)
            let response: UserResponse = try await api.post("/api/auth/password", body: req)
            return api.identity.managed ? api.identity.snapshot.user : response.user
        } catch let error as APIError {
            errorMessage = error.errorDescription ?? "Password change failed"
            return nil
        } catch {
            errorMessage = "Password change failed"
            return nil
        }
    }
}

// MARK: - Household Operations

extension AuthStore {
    func createHousehold(name: String, initials: String) async -> Household? {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            let req = CreateHouseholdRequest(name: name, initials: initials)
            let response: HouseholdOnlyResponse = try await api.post("/api/household", body: req)
            return response.household
        } catch let error as APIError {
            errorMessage = error.errorDescription ?? "Failed to create household"
            return nil
        } catch {
            errorMessage = "Failed to create household"
            return nil
        }
    }

    func joinHousehold(code: String) async -> Household? {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            let req = JoinHouseholdRequest(inviteCode: code)
            let response: HouseholdOnlyResponse = try await api.post("/api/household/join", body: req)
            return response.household
        } catch let error as APIError {
            errorMessage = error.errorDescription ?? "Failed to join household"
            return nil
        } catch {
            errorMessage = "Failed to join household"
            return nil
        }
    }

    func seedDefaults() async -> Bool {
        do {
            let _: StatusResponse = try await api.postEmpty("/api/chores/seed-defaults")
            return true
        } catch {
            return false
        }
    }
}
