import CryptoKit
import Foundation
import Security

/// The household display (PWA `display.js`): a shared tablet shows Home's
/// tiles large under a row of the household's members. Someone taps who they
/// are, then what they did, and it's logged at once for them, with Undo. An
/// owner or admin turns it on with a PIN this device keeps as a salted
/// SHA-256, per account and household; the PIN is what turns it off.
enum HouseholdDisplay {
    struct Setting: Codable, Equatable {
        let salt: String
        let hash: String
        /// Wrong PINs and the pause after them, kept with the setting so
        /// neither closing the form nor relaunching resets them.
        var tries: Int?
        var pausedUntil: Date?
    }

    /// Who did it resets after two quiet minutes, so the next person can't
    /// log as the last one by mistake.
    static let pickIdle: TimeInterval = 120
    static let maxPINTries = 5
    static let pinPause: TimeInterval = 30
    private static let prefix = "nabu-display:"

    static func key(_ origin: LogOrigin) -> String {
        "\(prefix)\(origin.actorID):\(origin.householdID)"
    }

    /// The signed-in account and its household, which the setting belongs to.
    @MainActor static func origin(of state: AppState) -> LogOrigin? {
        guard let user = state.user, let household = user.householdId else { return nil }
        return LogOrigin(actorID: user.id, householdID: household)
    }

    static func setting(for origin: LogOrigin?, defaults: UserDefaults = .standard) -> Setting? {
        guard let origin, let data = defaults.data(forKey: key(origin)) else { return nil }
        return try? JSONDecoder().decode(Setting.self, from: data)
    }

    static func validPIN(_ pin: String) -> Bool {
        (4...8).contains(pin.count) && pin.allSatisfy { ("0"..."9").contains($0) }
    }

    static func hash(pin: String, salt: String) -> String {
        SHA256.hash(data: Data("\(salt):\(pin)".utf8)).map { String(format: "%02x", $0) }.joined()
    }

    /// Turns the display on for `origin`. False for a PIN that isn't 4 to 8
    /// digits, or when the device can't make a salt.
    @discardableResult
    static func turnOn(pin: String, for origin: LogOrigin, defaults: UserDefaults = .standard) -> Bool {
        guard validPIN(pin) else { return false }
        var bytes = [UInt8](repeating: 0, count: 16)
        guard SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes) == errSecSuccess else { return false }
        let salt = bytes.map { String(format: "%02x", $0) }.joined()
        guard let data = try? JSONEncoder().encode(Setting(salt: salt, hash: hash(pin: pin, salt: salt))) else { return false }
        defaults.set(data, forKey: key(origin))
        return true
    }

    /// One try at the PIN that turns the display off: nil when it did,
    /// otherwise what to say (`Leaving.attempt`).
    static func leave(pin: String, for origin: LogOrigin, now: Date = Date(), defaults: UserDefaults = .standard) -> String? {
        guard var setting = setting(for: origin, defaults: defaults) else { return nil }
        var attempts = Leaving(tries: setting.tries ?? 0, pausedUntil: setting.pausedUntil)
        let correct = validPIN(pin) && hash(pin: pin, salt: setting.salt) == setting.hash
        if let message = attempts.attempt(correct: correct, now: now) {
            setting.tries = attempts.tries
            setting.pausedUntil = attempts.pausedUntil
            if let data = try? JSONEncoder().encode(setting) { defaults.set(data, forKey: key(origin)) }
            return message
        }
        defaults.removeObject(forKey: key(origin))
        return nil
    }

#if DEBUG
    /// `-resetState`: no UI test starts on another test's display.
    static func removeAll(defaults: UserDefaults = .standard) {
        for key in defaults.dictionaryRepresentation().keys where key.hasPrefix(prefix) {
            defaults.removeObject(forKey: key)
        }
    }
#endif

    /// Whether the account may turn a device into a display: owners and admins.
    static func canManage(user: User?, members: [Member]) -> Bool {
        guard let user, user.householdId != nil else { return false }
        let role = members.first { $0.userId == user.id }?.role ?? user.role
        return role == "owner" || role == "admin"
    }

    /// The display's tiles: Home's, in Home's order, without hidden ones or
    /// Admins-only activities, since anyone in the room can see the display.
    static func chores(_ chores: [Chore], order: [Int], hidden: [Int]) -> [Chore] {
        let shown = chores.filter { !hidden.contains($0.id) && $0.visibility != "admins" }
        guard !order.isEmpty else { return shown.sorted { $0.id < $1.id } }
        let position = Dictionary(order.enumerated().map { ($1, $0) }, uniquingKeysWith: { first, _ in first })
        return shown.enumerated()
            .sorted { (position[$0.element.id] ?? Int.max, $0.offset) < (position[$1.element.id] ?? Int.max, $1.offset) }
            .map(\.element)
    }

    /// Tries at the PIN that leaves the display (PWA `tryLeave`). After
    /// `maxPINTries` wrong ones, even the right PIN waits out the pause.
    struct Leaving: Equatable {
        var tries = 0
        var pausedUntil: Date?

        /// Nil when the display may close; otherwise what to say.
        mutating func attempt(correct: Bool, now: Date = Date()) -> String? {
            if let pausedUntil, pausedUntil > now {
                return "Too many tries. Wait \(Int(pausedUntil.timeIntervalSince(now).rounded(.up))) seconds."
            }
            if correct { return nil }
            tries += 1
            if tries < HouseholdDisplay.maxPINTries { return "That PIN isn't right." }
            tries = 0
            pausedUntil = now.addingTimeInterval(HouseholdDisplay.pinPause)
            return "Too many tries. Wait \(Int(HouseholdDisplay.pinPause)) seconds."
        }
    }
}
