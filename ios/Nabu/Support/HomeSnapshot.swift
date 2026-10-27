import Foundation

/// The last confirmed Home data for this device's active identity, so a cold
/// launch that cannot reach the server opens Home and queues logs in the
/// journal (PWA `snapshot.js`). `ClientIdentity` stores it: it is saved only
/// from server-confirmed state of exactly this identity and deleted whenever
/// the identity stops being active, so it never outlives a sign-out, an
/// account or household switch, or an invalidated session.
struct HomeSnapshot: Codable, Equatable {
    static let currentVersion = 1

    var version = HomeSnapshot.currentVersion
    let user: User
    let savedAt: Date
    let household: Household
    let members: [Member]
    let historicalMembers: [HistoricalMember]
    let userHouseholds: [HouseholdWithRole]
    let chores: [Chore]
    /// The latest log of each chore; `AppState.latestLogs` keys them by chore.
    let latestLogs: [ChoreLog]
    let choreOrder: [Int]
    let hiddenHomeChoreIDs: [Int]
    let volumeUnit: String
    let hideNotificationBadge: Bool

    /// Saved for this account, in this household, with this role.
    func belongs(to other: User) -> Bool {
        user.id == other.id && user.householdId == other.householdId && user.role == other.role
            && household.id == other.householdId
    }

    // Plain coders: the file is written and read only by this app, so dates
    // round-trip exactly and no API key strategy applies.
    static let encoder = JSONEncoder()
    static let decoder = JSONDecoder()
}

@MainActor
extension AppState {
    /// Home data loaded for the signed-in identity, or nil when there is no
    /// household to show (none yet, or a disabled membership).
    func homeSnapshot(savedAt: Date = Date()) -> HomeSnapshot? {
        guard let user, let household, user.householdId == household.id,
              accessSuspendedMessage == nil else { return nil }
        return HomeSnapshot(user: user, savedAt: savedAt, household: household, members: members,
                            historicalMembers: historicalMembers, userHouseholds: userHouseholds,
                            chores: chores, latestLogs: Array(latestLogs.values),
                            choreOrder: choreOrder, hiddenHomeChoreIDs: hiddenHomeChoreIDs,
                            volumeUnit: volumeUnit, hideNotificationBadge: hideNotificationBadge)
    }

    /// Restores saved Home data after `adopt` made its identity current.
    func applyHomeSnapshot(_ saved: HomeSnapshot) {
        household = saved.household
        activeHouseholdId = saved.household.id
        members = saved.members
        historicalMembers = saved.historicalMembers
        userHouseholds = saved.userHouseholds
        chores = saved.chores
        latestLogs = Dictionary(saved.latestLogs.map { ($0.choreId, $0) }, uniquingKeysWith: { first, _ in first })
        choreOrder = saved.choreOrder
        hiddenHomeChoreIDs = saved.hiddenHomeChoreIDs
        volumeUnit = saved.volumeUnit
        hideNotificationBadge = saved.hideNotificationBadge
        // `adopt` cleared the widget for the new revision; show the saved times.
        WidgetDataCache.write(chores: WidgetDataCache.snapshots(from: self))
    }
}
