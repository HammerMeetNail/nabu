import Foundation

/// Lives above the identity-gated tabs so canonical session confirmation cannot
/// destroy a deletion failure before the user can read it.
@MainActor
final class AccountDeletionModel: ObservableObject {
    @Published var isPresented = false
    @Published private(set) var isDeleting = false
    @Published private(set) var errorMessage: String?
    private struct Owner: Equatable {
        let server: URL
        let actorID: Int
        let householdID: Int?
    }
    private var owner: Owner?
    private var intent = UUID()

    private func currentOwner(_ api: APIClient) -> Owner? {
        let snapshot = api.identity.snapshot
        guard snapshot.phase == .ready, let user = snapshot.user else { return nil }
        return Owner(server: api.baseURL, actorID: user.id, householdID: user.householdId)
    }

    func matches(_ api: APIClient) -> Bool { owner != nil && owner == currentOwner(api) }

    func open(api: APIClient) {
        guard let current = currentOwner(api) else { return }
        intent = UUID()
        owner = current
        errorMessage = nil
        isDeleting = false
        isPresented = true
    }

    func cancel() {
        intent = UUID()
        isPresented = false
        isDeleting = false
        errorMessage = nil
        owner = nil
    }

    func reconcile(api: APIClient) {
        guard isPresented, api.identity.snapshot.phase != .changing else { return }
        if !matches(api) { cancel() }
    }

    func delete(api: APIClient) async {
        guard isPresented, !isDeleting, matches(api) else { return }
        let token = intent
        isDeleting = true
        errorMessage = nil
        defer { if intent == token { isDeleting = false } }
        do {
            let _: StatusResponse = try await api.scoped().delete("/api/me", body: DeleteAccountRequest(confirm: "DELETE"))
            if intent == token { cancel() }
        } catch {
            guard intent == token, isPresented else { return }
            guard matches(api) else { cancel(); return }
            if case APIError.serverError(_, let message) = error { errorMessage = message }
            else { errorMessage = "Account deletion failed. Please try again." }
        }
    }
}

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
