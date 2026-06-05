import XCTest
@testable import Nabu

@MainActor
final class StateTests: XCTestCase {
    func testDefaultValuesMatchPWAInitialState() {
        let state = AppState()

        XCTAssertNil(state.user)
        XCTAssertNil(state.household)
        XCTAssertTrue(state.userHouseholds.isEmpty)
        XCTAssertNil(state.activeHouseholdId)
        XCTAssertTrue(state.members.isEmpty)
        XCTAssertTrue(state.invites.isEmpty)
        XCTAssertTrue(state.chores.isEmpty)
        XCTAssertTrue(state.todayLogs.isEmpty)
        XCTAssertTrue(state.weekLogs.isEmpty)
        XCTAssertTrue(state.schedules.isEmpty)
        XCTAssertTrue(state.latestLogs.isEmpty)
        XCTAssertTrue(state.notifications.isEmpty)
        XCTAssertEqual(state.unreadNotifications, 0)
        XCTAssertNil(state.notificationPrefs)
        XCTAssertTrue(state.availableNotificationTypes.isEmpty)
        XCTAssertTrue(state.choreOrder.isEmpty)
        XCTAssertTrue(state.hiddenHomeChoreIDs.isEmpty)
        XCTAssertEqual(state.currentTab, .home)
        XCTAssertEqual(state.activityView, .history)
        XCTAssertEqual(state.calendarView, .day)
        XCTAssertNil(state.calendarDate)
        XCTAssertEqual(state.homeView, .log)
        XCTAssertNil(state.activeSheet)
        XCTAssertNil(state.toast)
        XCTAssertFalse(state.jiggleMode)
        XCTAssertNil(state.historyChoreFilter)
        XCTAssertFalse(state.historyFilterOpen)
    }

    func testResetClearsAllState() {
        let state = AppState()
        state.currentTab = .stats
        state.reset()

        XCTAssertNil(state.user)
        XCTAssertNil(state.household)
        XCTAssertEqual(state.currentTab, .home)
    }

    func testResetHouseholdScopedPreservesUser() {
        let state = AppState()
        let user = User(id: 1, householdId: 1, email: "test@test.com", displayName: "Test", avatarColor: "#FF0000", emailVerified: true, role: "owner", createdAt: Date())
        state.user = user
        state.household = Household(
            id: 1, name: "Test", initials: "T",
            inviteCode: "ABC", createdAt: Date()
        )
        state.activeHouseholdId = 1
        state.chores = [Chore(
            id: 1, householdId: 1, name: "Chore", icon: "🧹",
            color: "#FF0000", sortOrder: 0, category: "",
            isPredefined: false, predefinedKey: nil, createdBy: nil,
            createdAt: Date(), indicatorLabels: [],
            indicatorDefaults: [], hasVolumeML: false
        )]

        state.resetHouseholdScoped()

        XCTAssertEqual(state.user?.id, 1)
        XCTAssertNil(state.household)
        XCTAssertNil(state.activeHouseholdId)
        XCTAssertTrue(state.chores.isEmpty)
    }
}
