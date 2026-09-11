import Foundation

/// The single active duration timer, ported from the PWA's
/// `web/static/js/timer.js`. Persisted in `UserDefaults` (the PWA uses
/// localStorage) so it survives relaunch; the elapsed-time chip and
/// stop-and-log wiring live in the views.
struct ActiveTimer: Codable, Equatable {
    let choreId: Int
    let choreName: String
    let choreIcon: String
    /// Wall-clock start. Stored as milliseconds since epoch, matching the
    /// PWA's `Date.now()` shape.
    let startedAt: Date
    let origin: LogOrigin?
    let idempotencyKey: String
    var stoppedAt: Date?

    enum CodingKeys: String, CodingKey {
        case choreId, choreName, choreIcon, startedAt, origin, idempotencyKey, stoppedAt
    }

    init(choreId: Int, choreName: String, choreIcon: String, startedAt: Date, origin: LogOrigin? = nil,
         idempotencyKey: String = UUID().uuidString, stoppedAt: Date? = nil) {
        self.choreId = choreId
        self.choreName = choreName
        self.choreIcon = choreIcon
        self.startedAt = startedAt
        self.origin = origin
        self.idempotencyKey = idempotencyKey
        self.stoppedAt = stoppedAt
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        choreId = try container.decode(Int.self, forKey: .choreId)
        choreName = try container.decodeIfPresent(String.self, forKey: .choreName) ?? ""
        choreIcon = try container.decodeIfPresent(String.self, forKey: .choreIcon) ?? "⏱"
        let ms = try container.decode(Double.self, forKey: .startedAt)
        startedAt = Date(timeIntervalSince1970: ms / 1000)
        origin = try container.decodeIfPresent(LogOrigin.self, forKey: .origin)
        idempotencyKey = try container.decodeIfPresent(String.self, forKey: .idempotencyKey) ?? UUID().uuidString
        stoppedAt = try container.decodeIfPresent(Date.self, forKey: .stoppedAt)
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(choreId, forKey: .choreId)
        try container.encode(choreName, forKey: .choreName)
        try container.encode(choreIcon, forKey: .choreIcon)
        try container.encode(startedAt.timeIntervalSince1970 * 1000, forKey: .startedAt)
        try container.encodeIfPresent(origin, forKey: .origin)
        try container.encode(idempotencyKey, forKey: .idempotencyKey)
        try container.encodeIfPresent(stoppedAt, forKey: .stoppedAt)
    }
}

enum DurationTimer {
    static let defaultsKey = "nabu_active_timer"

    /// Returns the persisted active timer, or nil. Validates shape so a
    /// corrupt/partial value never crashes the caller.
    static func load(from defaults: UserDefaults? = nil, origin: LogOrigin? = nil) -> ActiveTimer? {
        let data: Data?
        if let defaults { data = defaults.data(forKey: key(origin)) }
        else { data = try? Data(contentsOf: file(origin)) }
        guard let data, let timer = try? JSONDecoder().decode(ActiveTimer.self, from: data), timer.origin == origin else { return nil }
        return timer
    }

    /// Persists (or clears, when nil) the active timer.
    @discardableResult
    static func save(_ timer: ActiveTimer?, to defaults: UserDefaults? = nil, origin: LogOrigin? = nil) -> Bool {
        let owner = timer?.origin ?? origin
        do {
            if let timer {
                let data = try JSONEncoder().encode(timer)
                if let defaults { defaults.set(data, forKey: key(owner)) }
                else {
                    let target = file(owner)
                    try FileManager.default.createDirectory(at: target.deletingLastPathComponent(), withIntermediateDirectories: true)
                    try data.write(to: target, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
                }
            } else if let defaults { defaults.removeObject(forKey: key(owner)) }
            else if FileManager.default.fileExists(atPath: file(owner).path) { try FileManager.default.removeItem(at: file(owner)) }
            return true
        } catch { return false }
    }

    private static func key(_ origin: LogOrigin?) -> String {
        guard let origin else { return defaultsKey }
        return "\(defaultsKey)_\(origin.actorID)_\(origin.householdID)"
    }
    private static func file(_ origin: LogOrigin?) -> URL {
        FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
            .appendingPathComponent(key(origin) + ".json")
    }

    /// Whole seconds elapsed since the timer started.
    static func elapsedSeconds(_ timer: ActiveTimer, now: Date = Date()) -> Int {
        max(0, Int((timer.stoppedAt ?? now).timeIntervalSince(timer.startedAt)))
    }

    /// Renders seconds as m:ss (or h:mm:ss past an hour).
    static func formatElapsed(_ seconds: Int) -> String {
        let s = max(0, seconds)
        let h = s / 3600
        let m = (s % 3600) / 60
        let ss = s % 60
        if h > 0 {
            return String(format: "%d:%02d:%02d", h, m, ss)
        }
        return String(format: "%d:%02d", m, ss)
    }
}
