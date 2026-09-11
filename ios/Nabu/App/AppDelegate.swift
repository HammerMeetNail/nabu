import UIKit
import UserNotifications

/// UIKit delegate hooks for remote notifications: APNs token callbacks, the
/// notification action categories, and routing of notification taps/actions.
/// Behavioral parity with the PWA service worker's `push`/`notificationclick`
/// handlers (web/static/service-worker.js).
final class AppDelegate: NSObject, UIApplicationDelegate {
    /// Set by NabuApp once SwiftUI state exists; a "Log now" tap or quick
    /// action that arrives earlier (cold launch) is buffered.
    weak var appState: AppState?
    private var bufferedQuickLog: QuickLogTarget?

    enum ShortcutIdentifiers {
        static let logFeed = "com.nabu.app.shortcut.log-feed"
        static let logChore = "com.nabu.app.shortcut.log-chore"
        static let activity = "com.nabu.app.shortcut.activity"
    }

    enum NotificationIdentifiers {
        /// Must match the `category` field the reminder push carries
        /// (internal/reminder/scheduler.go → aps.category).
        static let reminderCategory = "NABU_REMINDER"
        static let logNowAction = "NABU_LOG_NOW"
        static let snoozeAction = "NABU_SNOOZE_30"
    }

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        registerNotificationCategories()
        return true
    }

    /// Parity with the PWA reminder actions: "✓ Log now" and "⏰ Snooze 30m".
    private func registerNotificationCategories() {
        let logNow = UNNotificationAction(
            identifier: NotificationIdentifiers.logNowAction,
            title: "✓ Log now",
            options: [.foreground]
        )
        let snooze = UNNotificationAction(
            identifier: NotificationIdentifiers.snoozeAction,
            title: "⏰ Snooze 30m",
            options: []
        )
        let reminder = UNNotificationCategory(
            identifier: NotificationIdentifiers.reminderCategory,
            actions: [logNow, snooze],
            intentIdentifiers: []
        )
        UNUserNotificationCenter.current().setNotificationCategories([reminder])
    }

    func application(
        _ application: UIApplication,
        didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data
    ) {
        Task { @MainActor in
            await PushRegistrationController.shared.handleDeviceToken(deviceToken)
        }
    }

    func application(
        _ application: UIApplication,
        didFailToRegisterForRemoteNotificationsWithError error: Error
    ) {
        Task { @MainActor in
            PushRegistrationController.shared.handleRegistrationFailure(error)
        }
    }

    // MARK: - Scene configuration

    /// Installs the scene delegate that receives Home-Screen quick actions;
    /// SwiftUI keeps managing the window as usual.
    func application(
        _ application: UIApplication,
        configurationForConnecting connectingSceneSession: UISceneSession,
        options: UIScene.ConnectionOptions
    ) -> UISceneConfiguration {
        let config = UISceneConfiguration(name: nil, sessionRole: connectingSceneSession.role)
        config.delegateClass = SceneDelegate.self
        return config
    }

    // MARK: - Quick actions (parity with the PWA manifest shortcuts)

    @MainActor
    func handleShortcut(_ item: UIApplicationShortcutItem) {
        switch item.type {
        case ShortcutIdentifiers.logFeed:
            routeQuickLog(.predefined(key: "Feed Baby"))
        case ShortcutIdentifiers.logChore:
            // "Log chore" lands on the home grid, one tap from logging.
            appState?.currentTab = .home
            appState?.homeView = .log
        case ShortcutIdentifiers.activity:
            appState?.currentTab = .activity
        default:
            break
        }
    }

    // MARK: - Deep-link buffering

    /// Routes a quick-log target to the home log sheet, or buffers it until
    /// NabuApp attaches the app state on a cold launch.
    @MainActor
    func routeQuickLog(_ target: QuickLogTarget) {
        guard let appState else {
            bufferedQuickLog = target
            return
        }
        appState.currentTab = .home
        appState.homeView = .log
        appState.pendingQuickLog = target
    }

    @MainActor
    func attach(appState: AppState) {
        self.appState = appState
        if let target = bufferedQuickLog {
            bufferedQuickLog = nil
            routeQuickLog(target)
        }
    }
}

/// Receives Home-Screen quick actions (scene-based apps get them here, not
/// on the app delegate) and forwards to the app delegate's router.
final class SceneDelegate: NSObject, UIWindowSceneDelegate {
    func scene(
        _ scene: UIScene,
        willConnectTo session: UISceneSession,
        options connectionOptions: UIScene.ConnectionOptions
    ) {
        if let shortcut = connectionOptions.shortcutItem {
            forward(shortcut)
        }
    }

    func windowScene(
        _ windowScene: UIWindowScene,
        performActionFor shortcutItem: UIApplicationShortcutItem,
        completionHandler: @escaping (Bool) -> Void
    ) {
        forward(shortcutItem)
        completionHandler(true)
    }

    private func forward(_ item: UIApplicationShortcutItem) {
        Task { @MainActor in
            (UIApplication.shared.delegate as? AppDelegate)?.handleShortcut(item)
        }
    }
}

extension AppDelegate: UNUserNotificationCenterDelegate {
    /// Foreground presentation: show reminders while the app is open, the
    /// same way the PWA's service worker shows them over an open tab.
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        guard let owner = await PushRegistrationController.shared.confirmedOwner(notification.request.content.userInfo),
              await PushRegistrationController.shared.isCurrent(owner) else { return [] }
        return [.banner, .list, .sound]
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse
    ) async {
        let userInfo = response.notification.request.content.userInfo
        guard let owner = await PushRegistrationController.shared.confirmedOwner(userInfo) else { return }
        let choreId = (userInfo["choreId"] as? NSNumber)?.intValue

        switch response.actionIdentifier {
        case NotificationIdentifiers.snoozeAction:
            // Silently reschedules without opening the app — the endpoint is
            // CSRF-exempt and session-authenticated, exactly like the PWA
            // service worker's snooze fetch.
            guard let choreId else { return }
            await PushRegistrationController.shared.snooze(choreID: choreId, owner: owner)
        case NotificationIdentifiers.logNowAction:
            // Deep-links to the pre-filled log sheet — parity with the PWA's
            // `/?quicklog=chore:<id>`.
            guard let choreId else { return }
            await routeNotificationQuickLog(choreID: choreId, owner: owner)
        default:
            // A plain body tap just opens/focuses the app.
            break
        }
    }

    @MainActor
    private func routeNotificationQuickLog(choreID: Int, owner: ClientIdentity.Snapshot) {
        guard PushRegistrationController.shared.isCurrent(owner) else { return }
        routeQuickLog(.chore(id: choreID))
    }
}
