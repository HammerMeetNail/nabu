import Foundation
import UIKit
import UserNotifications

/// Orchestrates the APNs registration lifecycle around the pure
/// `PushRegistrationPhase` state machine: system authorization, token
/// registration with the backend, and unregistration on logout.
///
/// A singleton because UIKit's remote-notification callbacks land on the app
/// delegate, which exists before any SwiftUI state.
@MainActor
final class PushRegistrationController: ObservableObject {
    static let shared = PushRegistrationController()

    @Published private(set) var phase: PushRegistrationPhase = .idle

    private var api: APIClient?
    private let defaults: UserDefaults
    private var tokenRequestOwner: ClientIdentity.Snapshot?

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    func configure(api: APIClient) {
        self.api = api
    }

    /// Silent launch-time sync: if the user already granted notification
    /// permission, refresh the token registration (tokens can change, so
    /// Apple recommends re-registering on every launch). Never prompts —
    /// mirrors the PWA's `maybeSubscribePush()` after login.
    func syncIfAuthorized() async {
        guard let api, api.identity.snapshot.origin != nil else { return }
        let owner = api.identity.snapshot
        let settings = await UNUserNotificationCenter.current().notificationSettings()
        guard settings.authorizationStatus == .authorized
                || settings.authorizationStatus == .provisional else { return }
        guard api.identity.isCurrent(owner) else { return }
        tokenRequestOwner = owner
        apply(.authorizationGranted)
        UIApplication.shared.registerForRemoteNotifications()
    }

    /// The pre-prompt's "Enable" button: this is the only place the system
    /// permission dialog is fired (never cold — the pre-prompt explains why
    /// first). Returns whether the user granted permission.
    @discardableResult
    func requestAuthorizationAndRegister() async -> Bool {
        guard let api, api.identity.snapshot.origin != nil else { return false }
        let owner = api.identity.snapshot
        apply(.enableRequested)
        do {
            let granted = try await UNUserNotificationCenter.current()
                .requestAuthorization(options: [.alert, .sound, .badge])
            guard api.identity.isCurrent(owner) else { return false }
            if granted {
                tokenRequestOwner = owner
                apply(.authorizationGranted)
                UIApplication.shared.registerForRemoteNotifications()
            } else {
                apply(.authorizationDenied)
            }
            return granted
        } catch {
            apply(.authorizationDenied)
            return false
        }
    }

    var authorizationStatus: UNAuthorizationStatus {
        get async {
            await UNUserNotificationCenter.current().notificationSettings().authorizationStatus
        }
    }

    /// `didRegisterForRemoteNotificationsWithDeviceToken` → hex-encode and
    /// register with the backend. Failure is non-fatal: push stays off and
    /// the phase records why.
    func handleDeviceToken(_ deviceToken: Data) async {
        guard let api, let owner = tokenRequestOwner, api.identity.isCurrent(owner), owner.origin != nil else { return }
        let token = PushRegistration.hexToken(from: deviceToken)
        apply(.tokenReceived(token))
        let body = APNsRegisterRequest(
            token: token,
            environment: APNsEnvironment.current,
            bundleId: Bundle.main.bundleIdentifier ?? "com.nabu.app",
            deviceName: "iOS"
        )
        do {
            let _: StatusResponse = try await api.post("/api/mobile/apns/register", body: body)
            guard api.identity.isCurrent(owner) else { return }
            defaults.set(token, forKey: PushRegistration.storedTokenKey)
            apply(.registerSucceeded)
        } catch {
            guard api.identity.isCurrent(owner) else { return }
            apply(.registerFailed("Registration failed. Retry when connected."))
        }
    }

    /// `didFailToRegisterForRemoteNotificationsWithError` — expected on
    /// simulators and when offline; surfaced as state, never as an alert.
    func handleRegistrationFailure(_ error: Error) {
        guard let api, let owner = tokenRequestOwner, api.identity.isCurrent(owner) else { return }
        apply(.registerFailed("Device registration failed."))
    }

    /// Session revocation removes this session's server registrations. Clear
    /// the local token only after the logout response is confirmed.
    func didConfirmLogout() {
        defaults.removeObject(forKey: PushRegistration.storedTokenKey)
        suspend()
    }

    func suspend() {
        tokenRequestOwner = nil
        UIApplication.shared.unregisterForRemoteNotifications()
        UNUserNotificationCenter.current().removeAllDeliveredNotifications()
        apply(.unregistered)
    }

    /// OS background alerts are generic. Before foreground display or an
    /// action, confirm canonical membership and current chore visibility.
    func confirmedOwner(_ data: [AnyHashable: Any]) async -> ClientIdentity.Snapshot? {
        let api = self.api ?? APIClient(baseURL: AppEnvironment.resolveBaseURL(),
            identity: ClientIdentity.shared(for: AppEnvironment.resolveBaseURL()))
        guard api.identity.snapshot.phase != .logoutPending,
              let expectedUser = (data["userId"] as? NSNumber)?.intValue,
              let expectedHousehold = (data["householdId"] as? NSNumber)?.intValue else { return nil }
        guard let response: UserResponse = try? await api.get("/api/me"),
              response.user?.id == expectedUser, response.user?.householdId == expectedHousehold else { return nil }
        let owner = api.identity.snapshot
        guard owner.phase == .ready, owner.user?.id == expectedUser,
              owner.user?.householdId == expectedHousehold else { return nil }
        if let choreID = (data["choreId"] as? NSNumber)?.intValue {
            guard let chores: ChoresResponse = try? await api.scoped(to: owner).get("/api/chores"),
                  chores.chores.contains(where: { $0.id == choreID }) else { return nil }
        }
        return api.identity.isCurrent(owner) ? owner : nil
    }

    func isCurrent(_ owner: ClientIdentity.Snapshot) -> Bool {
        let identity = api?.identity ?? ClientIdentity.shared(for: AppEnvironment.resolveBaseURL())
        return identity.isCurrent(owner)
    }

    func snooze(choreID: Int, owner: ClientIdentity.Snapshot) async {
        let api = self.api ?? APIClient(baseURL: AppEnvironment.resolveBaseURL(),
            identity: ClientIdentity.shared(for: AppEnvironment.resolveBaseURL()))
        guard api.identity.isCurrent(owner) else { return }
        let body = ReminderSnoozeRequest(choreId: choreID, minutes: 30)
        let _: StatusResponse? = try? await api.scoped(to: owner).post("/api/reminders/snooze", body: body)
    }

    private func apply(_ event: PushRegistrationPhase.Event) {
        phase = phase.applying(event)
    }
}
