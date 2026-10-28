import AppIntents
import Foundation

// App Intents (improvement plan, Phase 3): Siri, Shortcuts, Spotlight and the
// Action button can log an activity, undo that log, and say when an activity
// was last done, without opening the app. They run in the app's process with
// the app's session. A log is the request a Home log sends (date, hour and
// completedAt from now) and goes through the same journal: offline, it waits
// on this device for the identity whose Home was saved here, and syncs when
// the app next reaches the server.

extension Notification.Name {
    /// An intent added or removed a log; a running app refreshes.
    static let nabuIntentChangedLogs = Notification.Name("nabuIntentChangedLogs")
}

// MARK: - Errors

enum NabuIntentError: Error, Equatable, CustomLocalizedStringResourceConvertible {
    case signedOut
    case noHousehold
    case unreachable
    case unavailable
    case activityGone
    case noAmount(String)
    case unknownSubject(String, String)
    case notSaved
    case undoNeedsServer

    var localizedStringResource: LocalizedStringResource {
        switch self {
        case .signedOut: return "Open Nabu and sign in first."
        case .noHousehold: return "Open Nabu and set up your household first."
        case .unreachable: return "Nabu can't reach its server, and this device has no saved Home to log to yet."
        case .unavailable: return "Nabu couldn't do that right now. Try again in a moment."
        case .activityGone: return "That activity isn't in your household any more."
        case .noAmount(let name): return "\(name) doesn't record an amount."
        case .unknownSubject(let subject, let name): return "\(subject) isn't one of \(name)'s choices."
        case .notSaved: return "Nabu couldn't save that log on this device."
        case .undoNeedsServer: return "Nabu needs to reach its server to remove that log."
        }
    }
}

// MARK: - Session

/// Where an intent stands: the session confirmed (the running app's, or by a
/// check like a launch's), or the server out of reach with Home saved on this
/// device, whose identity an offline log waits for. The saved Home is only
/// read, so a later offline launch still opens it.
@MainActor
enum IntentSession {
    enum State {
        case ready(APIClient, User)
        case offline(HomeSnapshot)

        var userID: Int {
            switch self {
            case .ready(_, let user): return user.id
            case .offline(let saved): return saved.user.id
            }
        }

        var householdID: Int? {
            switch self {
            case .ready(_, let user): return user.householdId
            case .offline(let saved): return saved.household.id
            }
        }
    }

    /// The running app's client (AppEnvironment sets it). Intents share its
    /// session and never check it themselves: an answer they adopted would
    /// reset the app's screens mid-launch. The app's own check decides.
    static var appClient: (() -> APIClient?)?

    /// A client for the app's server when iOS started the app only to run an
    /// intent. Tests replace it.
    static var makeClient: () -> APIClient = defaultClient

    static func defaultClient() -> APIClient {
        let baseURL = AppEnvironment.resolveBaseURL()
        return APIClient(baseURL: baseURL, identity: ClientIdentity.shared(for: baseURL))
    }

    /// The journal logs wait in. Tests replace it.
    static var queue: () -> OfflineLogQueue = { OfflineLogQueue.shared }

    static func start() async throws -> State {
        if let api = appClient?() {
            return try await adoptAppSession(api)
        }
        let api = makeClient()
        let snapshot = api.identity.snapshot
        switch snapshot.phase {
        case .ready:
            guard let user = snapshot.user else { throw NabuIntentError.signedOut }
            guard user.householdId != nil else { throw NabuIntentError.noHousehold }
            return .ready(api, user)
        case .anonymous, .logoutPending:
            throw NabuIntentError.signedOut
        case .changing:
            throw NabuIntentError.unavailable
        case .checking:
            break
        }
        switch await AuthStore(api: api).checkSession() {
        case .answered(let user?):
            guard user.householdId != nil else { throw NabuIntentError.noHousehold }
            return .ready(api, user)
        case .answered(nil):
            throw NabuIntentError.signedOut
        case .unreachable:
            guard let saved = api.identity.savedHome() else { throw NabuIntentError.unreachable }
            return .offline(saved)
        case .failed:
            throw NabuIntentError.unavailable
        }
    }

    /// Uses the running app's session, waiting up to five seconds while the
    /// app's launch check is still out.
    private static func adoptAppSession(_ api: APIClient) async throws -> State {
        var waits = 0
        while waits < 50, [.checking, .changing].contains(api.identity.snapshot.phase) {
            try await Task.sleep(for: .milliseconds(100))
            waits += 1
        }
        let snapshot = api.identity.snapshot
        switch snapshot.phase {
        case .ready:
            guard let user = snapshot.user else { throw NabuIntentError.signedOut }
            guard user.householdId != nil else { throw NabuIntentError.noHousehold }
            return .ready(api, user)
        case .anonymous, .logoutPending:
            throw NabuIntentError.signedOut
        case .checking, .changing:
            throw NabuIntentError.unavailable
        }
    }

    /// Chores the app loaded a moment ago, for the identity it loaded them as.
    private static var loaded: (revision: UUID, at: Date, chores: [Chore])?

    /// The app passes the chores it just loaded, so the queries Siri makes
    /// after `updateAppShortcutParameters()` need no request of their own.
    static func noteLoaded(_ chores: [Chore], for owner: ClientIdentity.Snapshot) {
        loaded = (owner.revision, Date(), chores)
    }

    /// The household's activities: the app's if it loaded them in the past
    /// minute, else the server's, else those saved with Home on this device.
    static func chores(in state: State) async -> [Chore] {
        switch state {
        case .ready(let api, _):
            if let loaded, loaded.revision == api.identity.snapshot.revision,
               Date().timeIntervalSince(loaded.at) < 60 { return loaded.chores }
            if let data: ChoresResponse = try? await api.get("/api/chores") { return data.chores }
            return api.identity.savedHome()?.chores ?? []
        case .offline(let saved):
            return saved.chores
        }
    }

    static func activities() async throws -> [ActivityEntity] {
        await chores(in: try await start()).map(ActivityEntity.init)
    }

    static func chore(_ id: Int, in state: State) async throws -> Chore {
        guard let chore = await chores(in: state).first(where: { $0.id == id }) else {
            throw NabuIntentError.activityGone
        }
        return chore
    }
}

// MARK: - The last log an intent made

/// Kept so "Undo last log" removes the log an intent just made, and only
/// that: within the hour, for the same account and household.
struct LastIntentLog: Codable, Equatable {
    /// The server's id, or nil while the log waits in the journal.
    let logID: Int?
    let idempotencyKey: String
    let choreID: Int
    let choreName: String
    let actorID: Int
    let householdID: Int
    let at: Date

    static let defaultsKey = "nabuLastIntentLog"
    /// Tests replace it.
    @MainActor static var defaults: UserDefaults = .standard

    @MainActor static func load() -> LastIntentLog? {
        guard let data = defaults.data(forKey: defaultsKey) else { return nil }
        return try? JSONDecoder().decode(LastIntentLog.self, from: data)
    }

    @MainActor func save() {
        if let data = try? JSONEncoder().encode(self) { Self.defaults.set(data, forKey: Self.defaultsKey) }
    }

    @MainActor static func clear() {
        defaults.removeObject(forKey: defaultsKey)
    }
}

// MARK: - What the intents do

/// The intents' work, apart from App Intents' types so tests can call it.
/// Each returns what Siri says.
@MainActor
enum IntentActions {
    static let undoWindow: TimeInterval = 3600

    /// Logs the activity now, like tapping its Home tile and then Log: the
    /// date, hour and completedAt are now, the indicators are the activity's
    /// defaults, and the log is the signed-in member's.
    static func log(activityID: Int, amount: Int?, subject: String?, now: Date = Date()) async throws -> String {
        let state = try await IntentSession.start()
        let chore = try await IntentSession.chore(activityID, in: state)
        if amount != nil && !chore.hasVolumeML { throw NabuIntentError.noAmount(chore.name) }
        if let subject, !chore.subjects.contains(subject) {
            throw NabuIntentError.unknownSubject(subject, chore.name)
        }
        guard let householdID = state.householdID else { throw NabuIntentError.noHousehold }
        let dateFormatter = DateFormatter()
        dateFormatter.locale = Locale(identifier: "en_US_POSIX")
        dateFormatter.calendar = Calendar(identifier: .gregorian)
        dateFormatter.dateFormat = "yyyy-MM-dd"
        let date = dateFormatter.string(from: now)
        let hour = Calendar.current.component(.hour, from: now)
        let completedAt = ISO8601DateFormatter().string(from: now)
        let unit = chore.hasVolumeML && !chore.metricUnit.isEmpty ? chore.metricUnit : nil
        let key = UUID().uuidString
        let queued = "Saved \(chore.name) on this device. It syncs when Nabu is back online."

        switch state {
        case .ready(let api, let user):
            let outcome = try await LogStore(api: api, offlineQueue: IntentSession.queue()).createLog(
                choreId: chore.id, date: date, indicators: chore.indicatorDefaults, slotHour: hour,
                completedAt: completedAt, volumeML: amount, userId: user.id, subject: subject,
                idempotencyKey: key, metricUnit: unit)
            switch outcome {
            case .created(let response):
                remember(logID: response.log.id, key: key, chore: chore, actorID: user.id, householdID: householdID, at: now)
                showInWidget(choreID: chore.id, lastCompletedAt: response.log.completedAt)
                return "Logged \(chore.name)."
            case .queued:
                remember(logID: nil, key: key, chore: chore, actorID: user.id, householdID: householdID, at: now)
                showInWidget(choreID: chore.id, lastCompletedAt: now)
                return queued
            }
        case .offline(let saved):
            let body = CreateLogRequest(
                choreId: chore.id, note: "", indicators: chore.indicatorDefaults, date: date, hour: hour,
                completedAt: completedAt, volumeML: amount, userId: saved.user.id, indicatorVolumes: nil,
                followUpMinutes: nil, followUpTime: nil, subject: subject, idempotencyKey: key, metricUnit: unit)
            let origin = LogOrigin(actorID: saved.user.id, householdID: householdID)
            guard IntentSession.queue().enqueue(body, origin: origin) else { throw NabuIntentError.notSaved }
            remember(logID: nil, key: key, chore: chore, actorID: saved.user.id, householdID: householdID, at: now)
            showInWidget(choreID: chore.id, lastCompletedAt: now)
            return queued
        }
    }

    /// Removes the last log an intent made in the past hour, as Undo does
    /// after a log in the app. One still waiting on this device is dropped.
    static func undoLast(now: Date = Date()) async throws -> String {
        let nothing = "There's no log from the past hour to undo."
        guard let last = LastIntentLog.load(), now.timeIntervalSince(last.at) < undoWindow else { return nothing }
        let state = try await IntentSession.start()
        guard state.userID == last.actorID, state.householdID == last.householdID else { return nothing }
        let origin = LogOrigin(actorID: last.actorID, householdID: last.householdID)
        let queue = IntentSession.queue()
        if queue.item(key: last.idempotencyKey, origin: origin) != nil {
            guard queue.discard(key: last.idempotencyKey, origin: origin) else { throw NabuIntentError.unavailable }
            LastIntentLog.clear()
            showInWidget(choreID: last.choreID, lastCompletedAt: latestSaved(choreID: last.choreID, in: state))
            return "Removed the \(last.choreName) log."
        }
        guard let logID = last.logID else {
            // It synced from the journal; its id on the server isn't known here.
            LastIntentLog.clear()
            return "That \(last.choreName) log has synced. Remove it from Activity in Nabu."
        }
        guard case .ready(let api, _) = state else { throw NabuIntentError.undoNeedsServer }
        do {
            let response = try await LogStore(api: api, offlineQueue: queue).deleteLog(logId: logID)
            LastIntentLog.clear()
            if response.reportsLatest { showInWidget(choreID: last.choreID, lastCompletedAt: response.latest?.completedAt) }
            return "Removed the \(last.choreName) log."
        } catch APIError.serverError(statusCode: 404, _), APIError.httpError(statusCode: 404) {
            LastIntentLog.clear()
            return "That \(last.choreName) log was already removed."
        } catch where LogStore.isNetworkFailure(error) {
            throw NabuIntentError.undoNeedsServer
        }
    }

    /// Says when the activity was last logged, and by whom.
    static func lastDone(activityID: Int, now: Date = Date()) async throws -> String {
        let state = try await IntentSession.start()
        let chore = try await IntentSession.chore(activityID, in: state)
        var latest: ChoreLog?
        var members: [Member] = []
        switch state {
        case .ready(let api, _):
            let saved = api.identity.savedHome()
            members = saved?.members ?? []
            if let data: LatestLogsResponse = try? await api.get("/api/logs/latest-per-chore") {
                latest = data.latestLogs[String(chore.id)]
            } else {
                latest = saved?.latestLogs.first { $0.choreId == chore.id }
            }
        case .offline(let saved):
            members = saved.members
            latest = saved.latestLogs.first { $0.choreId == chore.id }
        }
        guard let latest else { return "\(chore.name) hasn't been logged yet." }
        let when = RelativeDateTimeFormatter().localizedString(for: latest.completedAt, relativeTo: now)
        if latest.userId == state.userID { return "You last logged \(chore.name) \(when)." }
        if let name = members.first(where: { $0.userId == latest.userId })?.displayName, !name.isEmpty {
            return "\(name) last logged \(chore.name) \(when)."
        }
        return "\(chore.name) was last logged \(when)."
    }

    private static func remember(logID: Int?, key: String, chore: Chore, actorID: Int, householdID: Int, at: Date) {
        LastIntentLog(logID: logID, idempotencyKey: key, choreID: chore.id, choreName: chore.name,
                      actorID: actorID, householdID: householdID, at: at).save()
        NotificationCenter.default.post(name: .nabuIntentChangedLogs, object: nil)
    }

    /// The widget shows the change now rather than after the app's next load.
    private static func showInWidget(choreID: Int, lastCompletedAt: Date?) {
        let snapshots = WidgetDataCache.read()
        guard snapshots.contains(where: { $0.id == choreID }) else { return }
        WidgetDataCache.write(chores: snapshots.map {
            $0.id == choreID ? WidgetChoreSnapshot(id: $0.id, name: $0.name, icon: $0.icon, lastCompletedAt: lastCompletedAt) : $0
        })
        NotificationCenter.default.post(name: .nabuIntentChangedLogs, object: nil)
    }

    private static func latestSaved(choreID: Int, in state: IntentSession.State) -> Date? {
        let saved: HomeSnapshot?
        switch state {
        case .ready(let api, _): saved = api.identity.savedHome()
        case .offline(let home): saved = home
        }
        return saved?.latestLogs.first { $0.choreId == choreID }?.completedAt
    }
}

// MARK: - Activities

/// An activity, as Siri and Shortcuts offer it.
struct ActivityEntity: AppEntity {
    static let typeDisplayRepresentation = TypeDisplayRepresentation(name: "Activity")
    static let defaultQuery = ActivityQuery()

    let id: Int
    let name: String
    let subjects: [String]

    var displayRepresentation: DisplayRepresentation { DisplayRepresentation(title: "\(name)") }

    init(id: Int, name: String, subjects: [String] = []) {
        self.id = id
        self.name = name
        self.subjects = subjects
    }

    init(_ chore: Chore) {
        self.init(id: chore.id, name: chore.name, subjects: chore.subjects)
    }
}

struct ActivityQuery: EntityStringQuery {
    func entities(for identifiers: [Int]) async throws -> [ActivityEntity] {
        try await activities().filter { identifiers.contains($0.id) }
    }

    func entities(matching string: String) async throws -> [ActivityEntity] {
        let wanted = string.trimmingCharacters(in: .whitespacesAndNewlines)
        return try await activities().filter { $0.name.localizedCaseInsensitiveContains(wanted) }
    }

    func suggestedEntities() async throws -> [ActivityEntity] {
        try await activities()
    }

    private func activities() async throws -> [ActivityEntity] {
        try await IntentSession.activities()
    }
}

/// The names an activity asks "who?" between ("Cat Meds": Mongo, Roger).
struct SubjectOptionsProvider: DynamicOptionsProvider {
    @IntentParameterDependency<LogActivityIntent>(\.$activity)
    var intent

    func results() async throws -> [String] {
        intent?.activity.subjects ?? []
    }
}

// MARK: - Intents

struct LogActivityIntent: AppIntent {
    static let title: LocalizedStringResource = "Log Activity"
    static let description = IntentDescription("Logs an activity now, like tapping its tile on Home and then Log.")
    static let openAppWhenRun = false
    // Allowed from a locked device: a 3 a.m. feed is logged from the
    // nightstand. It only adds a log, and says nothing about earlier ones.
    static let authenticationPolicy: IntentAuthenticationPolicy = .alwaysAllowed

    @Parameter(title: "Activity")
    var activity: ActivityEntity

    @Parameter(title: "Amount", description: "In the activity's own unit, such as ml or mg.",
               inclusiveRange: (0, 100000))
    var amount: Int?

    @Parameter(title: "Who", description: "For an activity that asks who it was for.",
               optionsProvider: SubjectOptionsProvider())
    var subject: String?

    static var parameterSummary: some ParameterSummary {
        Summary("Log \(\.$activity)") {
            \.$amount
            \.$subject
        }
    }

    init() {}

    init(activity: ActivityEntity) {
        self.activity = activity
    }

    @MainActor
    func perform() async throws -> some IntentResult & ProvidesDialog {
        let message = try await IntentActions.log(activityID: activity.id, amount: amount, subject: subject)
        return .result(dialog: "\(message)")
    }
}

struct UndoLastLogIntent: AppIntent {
    static let title: LocalizedStringResource = "Undo Last Log"
    static let description = IntentDescription("Removes the last log made with Siri or Shortcuts in the past hour.")
    static let openAppWhenRun = false
    // It removes data, so not from a locked device.
    static let authenticationPolicy: IntentAuthenticationPolicy = .requiresAuthentication

    @MainActor
    func perform() async throws -> some IntentResult & ProvidesDialog {
        let message = try await IntentActions.undoLast()
        return .result(dialog: "\(message)")
    }
}

struct LastDoneIntent: AppIntent {
    static let title: LocalizedStringResource = "When Was It Last Done"
    static let description = IntentDescription("Says when an activity was last logged, and by whom.")
    static let openAppWhenRun = false
    // It reads the household's logs, so not from a locked device.
    static let authenticationPolicy: IntentAuthenticationPolicy = .requiresAuthentication

    @Parameter(title: "Activity")
    var activity: ActivityEntity

    static var parameterSummary: some ParameterSummary {
        Summary("When was \(\.$activity) last done")
    }

    init() {}

    init(activity: ActivityEntity) {
        self.activity = activity
    }

    @MainActor
    func perform() async throws -> some IntentResult & ProvidesDialog {
        let message = try await IntentActions.lastDone(activityID: activity.id)
        return .result(dialog: "\(message)")
    }
}

// MARK: - Siri phrases

struct NabuShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(intent: LogActivityIntent(), phrases: [
            "Log \(\.$activity) in \(.applicationName)",
            "Log \(\.$activity) with \(.applicationName)",
            "\(.applicationName) log \(\.$activity)",
        ], shortTitle: "Log Activity", systemImageName: "plus.circle")
        AppShortcut(intent: LastDoneIntent(), phrases: [
            "When was \(\.$activity) last done in \(.applicationName)",
            "Ask \(.applicationName) when \(\.$activity) was last done",
        ], shortTitle: "Last Done", systemImageName: "clock")
        AppShortcut(intent: UndoLastLogIntent(), phrases: [
            "Undo the last \(.applicationName) log",
            "Undo my last log in \(.applicationName)",
        ], shortTitle: "Undo Last Log", systemImageName: "arrow.uturn.backward")
    }
}
