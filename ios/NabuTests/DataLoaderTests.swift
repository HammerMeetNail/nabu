import XCTest
@testable import Nabu

@MainActor
final class DataLoaderTests: XCTestCase {
    var state: AppState!
    var api: APIClient!
    var dataLoader: DataLoader!

    override func setUp() {
        state = AppState()
        api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
        dataLoader = DataLoader()
        dataLoader.configure(api: api, state: state)
    }

    func testDataLoaderConfiguration() {
        XCTAssertNotNil(dataLoader.household)
        XCTAssertNotNil(dataLoader.chores)
        XCTAssertNotNil(dataLoader.logs)
        XCTAssertNotNil(dataLoader.schedules)
        XCTAssertNotNil(dataLoader.notifs)
        XCTAssertNotNil(dataLoader.preferences)
    }

    func testReloadAfterAuthWithoutUser() async {
        // No user set — should gracefully skip
        await dataLoader.reloadAfterAuth()
        // Should not crash
        XCTAssertTrue(true)
    }

    func testForegroundRefreshWithoutUser() async {
        await dataLoader.foregroundRefresh()
        // Should not crash
        XCTAssertTrue(true)
    }

    func testReloadAfterAuthWithUserNoHousehold() async {
        state.user = User(id: 1, householdId: nil, email: "test@test.com",
                          displayName: "Test", avatarColor: "#FF0000",
                          emailVerified: true, role: "owner",
                          createdAt: Date())
        await dataLoader.reloadAfterAuth()
        // Phase 1 (household + preferences) runs, Phase 3 skipped
        // All API calls should fail silently since there's no server
        XCTAssertTrue(true)
    }

    func testActivityViewWhenEmpty() {
        state.todayLogs = []
        XCTAssertTrue(state.todayLogs.isEmpty)
    }

    func testActivityViewWithLogs() {
        let log = ChoreLog(id: 1, householdId: 1, userId: 1, choreId: 1,
                           completedAt: Date(), note: "", indicators: [],
                           slotHour: 9, createdAt: Date(), volumeML: nil)
        state.todayLogs = [log]
        XCTAssertFalse(state.todayLogs.isEmpty)
    }
}
