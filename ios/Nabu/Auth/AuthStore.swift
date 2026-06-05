import Foundation

@MainActor
final class AuthStore: ObservableObject {
    private(set) var api: APIClient
    @Published var isLoading = false
    @Published var errorMessage: String?

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
            return response.user
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
            return response.user
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
        defer { isLoading = false }

        do {
            let req = RegisterRequest(email: email, password: password)
            let response: UserResponse = try await api.post("/api/auth/register", body: req)
            return response.user
        } catch let error as APIError {
            errorMessage = error.errorDescription ?? "Registration failed"
            return nil
        } catch {
            errorMessage = "Registration failed"
            return nil
        }
    }

    func logout() async {
        do {
            let _: StatusResponse = try await api.postEmpty("/api/auth/logout")
        } catch {}
        api.cookieStore.clearAll()
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
            return response.user
        } catch let error as APIError {
            errorMessage = error.errorDescription ?? "Password reset failed"
            return nil
        } catch {
            errorMessage = "Password reset failed"
            return nil
        }
    }

    func changePassword(current: String, new: String) async -> Bool {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            let req = ChangePasswordRequest(currentPassword: current, newPassword: new)
            let _: UserResponse = try await api.post("/api/auth/password", body: req)
            return true
        } catch let error as APIError {
            errorMessage = error.errorDescription ?? "Password change failed"
            return false
        } catch {
            errorMessage = "Password change failed"
            return false
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
