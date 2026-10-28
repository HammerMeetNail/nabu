import Foundation
import UserNotifications

enum TestHooks {
    /// The launch arguments a test passes. A release build reads none, so no
    /// hook below can change how it behaves.
    static var arguments: [String] {
#if DEBUG
        ProcessInfo.processInfo.arguments
#else
        []
#endif
    }

    /// The environment a test sets, for the same hooks. Empty in a release
    /// build.
    static var environment: [String: String] {
#if DEBUG
        ProcessInfo.processInfo.environment
#else
        [:]
#endif
    }

    /// Fixed picker dates for reviewed screen baselines; ordinary launches
    /// continue to use the current local date and time.
    static var reviewDate: Date? {
        let args = arguments
        if let index = args.firstIndex(of: "-reviewDate"), index + 1 < args.count {
            return ISO8601DateFormatter().date(from: args[index + 1])
        }
        return nil
    }

    static var baseURLOverride: String? {
        let args = arguments
        guard let index = args.firstIndex(of: "-nabuBaseURL"),
              index + 1 < args.count else {
            return nil
        }
        return args[index + 1]
    }

    static var resetState: Bool {
        arguments.contains("-resetState")
    }

    static var disableAnimations: Bool {
        arguments.contains("-disableAnimations")
    }

    static var useMockAPI: Bool {
        arguments.contains("-useMockAPI")
    }

    /// Every request fails as if the server could not be reached, so a UI
    /// test can relaunch a real server's account into offline launch.
    static var serverUnreachable: Bool {
        arguments.contains("-serverUnreachable")
    }

    /// Notification permission as UI tests see it: `-pushUndecided` treats it
    /// as undecided, so the notifications offer shows; other UI test launches
    /// (seeded, or registered against a real server) treat it as decided, so
    /// the offer never covers their screens.
    static var pushAuthorizationOverride: UNAuthorizationStatus? {
        let args = arguments
        if args.contains("-pushUndecided") { return .notDetermined }
        if args.contains("-seedHomeForUITest") || args.contains("-nabuAutoRegister") { return .denied }
        return nil
    }

    /// Skips the auth/session check and seeds a logged-in state with a single
    /// chore on the home grid.  Used by XCUITests for the home tab.
    static var seedHomeForUITest: Bool {
        arguments.contains("-seedHomeForUITest")
    }
}
