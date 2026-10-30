import Foundation
import Observation

/// Lives above the identity-gated tabs so canonical session confirmation cannot
/// destroy a deletion failure before the user can read it.
@MainActor
@Observable
final class AccountDeletionModel {
    var isPresented = false
    private(set) var isDeleting = false
    private(set) var errorMessage: String?
    /// Opt-in: also delete every log the user recorded in shared households.
    var deleteLogs = false
    /// Opt-in: strip the user's notes from the logs that stay.
    var deleteNotes = false
    private struct Owner: Equatable {
        let server: URL
        let actorID: Int
        let householdID: Int?
    }
    @ObservationIgnored private var owner: Owner?
    @ObservationIgnored private var intent = UUID()

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
        deleteLogs = false
        deleteNotes = false
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
            let _: StatusResponse = try await api.scoped().delete("/api/me", body: DeleteAccountRequest(confirm: "DELETE", deleteLogs: deleteLogs, deleteNotes: deleteNotes))
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
@Observable
final class AuthStore {
    private(set) var api: APIClient
    var isLoading = false
    var errorMessage: String?
    var registrationNotice: String?

    init(api: APIClient) {
        self.api = api
    }

    /// Until `configure(api:)`: a view's state is made anew each time SwiftUI
    /// rebuilds the view, so its placeholders share one client.
    convenience init() { self.init(api: Self.unconfigured) }
    private static let unconfigured = APIClient(baseURL: URL(string: "http://localhost:8080")!)

    func configure(api: APIClient) {
        self.api = api
    }

    // MARK: - Session

    func loadSession() async -> User? {
        do {
            let response: UserResponse = try await api.get("/api/me")
            return api.identity.managed ? api.identity.snapshot.user : response.user
        } catch {
            Log.api.failure("Load the session", error)
            return nil
        }
    }

    enum SessionCheck: Equatable {
        case answered(User?)
        /// No connection, a timeout, or a 5xx: the server said nothing about
        /// the session (PWA `sessionError(...).unreachable`).
        case unreachable
        case failed
    }

    /// Like `loadSession`, but tells an unreachable server apart from an
    /// answer, so launch can open Home saved on this device instead.
    func checkSession() async -> SessionCheck {
        do {
            let response: UserResponse = try await api.get("/api/me")
            return .answered(api.identity.managed ? api.identity.snapshot.user : response.user)
        } catch APIError.serverError(statusCode: 401, _), APIError.httpError(statusCode: 401) {
            return .answered(nil)
        } catch {
            return Self.serverUnreachable(error) ? .unreachable : .failed
        }
    }

    static func serverUnreachable(_ error: Error) -> Bool {
        switch error {
        case APIError.serverError(let status, _), APIError.httpError(let status): return status >= 500
        default: return LogStore.isNetworkFailure(error)
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
        } catch {
            errorMessage = Log.actions.failureMessage("Login failed", error)
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
        } catch {
            errorMessage = Log.actions.failureMessage("Registration failed", error)
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
        } catch {
            errorMessage = Log.actions.failureMessage("Password reset failed", error)
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
        } catch {
            errorMessage = Log.actions.failureMessage("Password change failed", error)
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
        } catch {
            errorMessage = Log.actions.failureMessage("Failed to create household", error)
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
        } catch {
            errorMessage = Log.actions.failureMessage("Failed to join household", error)
            return nil
        }
    }

    func seedDefaults() async -> Bool {
        do {
            let _: StatusResponse = try await api.postEmpty("/api/chores/seed-defaults")
            return true
        } catch {
            Log.actions.failure("Add the default activities", error)
            return false
        }
    }
}
