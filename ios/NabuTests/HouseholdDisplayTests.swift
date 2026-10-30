import XCTest
@testable import Nabu

/// The household display's setting and rules (PWA `display.js`): a salted
/// PIN per account and household on this device, who can turn it on, which
/// tiles it shows, and the pause after wrong PINs.
@MainActor
final class HouseholdDisplayTests: XCTestCase {
    private var defaults: UserDefaults!
    private let origin = LogOrigin(actorID: 1, householdID: 9)

    override func setUp() async throws {
        defaults = UserDefaults(suiteName: "HouseholdDisplayTests-\(UUID().uuidString)")
    }

    private func member(_ id: Int, role: String, suspended: Bool = false) -> Member {
        Member(userId: id, email: "m\(id)@example.com", displayName: "M\(id)", avatarColor: "#2E86AB",
               emailVerified: true, role: role, suspended: suspended)
    }

    private func user(_ id: Int, role: String, household: Int? = 9) -> User {
        User(id: id, householdId: household, email: "m\(id)@example.com", displayName: "M\(id)",
             avatarColor: "#2E86AB", emailVerified: true, role: role, createdAt: Date())
    }

    func testAPINIsFourToEightDigits() {
        XCTAssertTrue(HouseholdDisplay.validPIN("1234"))
        XCTAssertTrue(HouseholdDisplay.validPIN("12345678"))
        for pin in ["", "123", "123456789", "12a4", "１２３４", "12 34"] {
            XCTAssertFalse(HouseholdDisplay.validPIN(pin), pin)
        }
    }

    func testTheDeviceKeepsASaltedHashPerAccountAndHousehold() throws {
        XCTAssertFalse(HouseholdDisplay.turnOn(pin: "12", for: origin, defaults: defaults))
        XCTAssertTrue(HouseholdDisplay.turnOn(pin: "2468", for: origin, defaults: defaults))
        let setting = try XCTUnwrap(HouseholdDisplay.setting(for: origin, defaults: defaults))
        XCTAssertEqual(setting.hash, HouseholdDisplay.hash(pin: "2468", salt: setting.salt))
        XCTAssertNotEqual(setting.hash, HouseholdDisplay.hash(pin: "2468", salt: "other"))
        XCTAssertFalse(String(data: try XCTUnwrap(defaults.data(forKey: HouseholdDisplay.key(origin))), encoding: .utf8)!
            .contains("2468"), "the PIN itself is never stored")
        XCTAssertNil(HouseholdDisplay.setting(for: LogOrigin(actorID: 2, householdID: 9), defaults: defaults))
        XCTAssertNil(HouseholdDisplay.setting(for: LogOrigin(actorID: 1, householdID: 8), defaults: defaults))
    }

    func testOnlyThePINTurnsItOffAndWrongOnesPauseEvenAcrossTries() throws {
        HouseholdDisplay.turnOn(pin: "2468", for: origin, defaults: defaults)
        let now = Date(timeIntervalSince1970: 1_000_000)
        for _ in 1..<HouseholdDisplay.maxPINTries {
            XCTAssertEqual(HouseholdDisplay.leave(pin: "1111", for: origin, now: now, defaults: defaults), "That PIN isn't right.")
        }
        XCTAssertEqual(HouseholdDisplay.leave(pin: "1111", for: origin, now: now, defaults: defaults), "Too many tries. Wait 30 seconds.")
        // Kept with the setting: a new form or a relaunch doesn't end the pause.
        XCTAssertEqual(HouseholdDisplay.leave(pin: "2468", for: origin, now: now.addingTimeInterval(1), defaults: defaults),
                       "Too many tries. Wait 29 seconds.")
        XCTAssertNotNil(HouseholdDisplay.setting(for: origin, defaults: defaults))
        XCTAssertNil(HouseholdDisplay.leave(pin: "2468", for: origin, now: now.addingTimeInterval(HouseholdDisplay.pinPause), defaults: defaults))
        XCTAssertNil(HouseholdDisplay.setting(for: origin, defaults: defaults))
    }

    func testOwnersAndAdminsCanTurnItOn() {
        let members = [member(1, role: "owner"), member(2, role: "admin"), member(3, role: "member")]
        XCTAssertTrue(HouseholdDisplay.canManage(user: user(1, role: "owner"), members: members))
        XCTAssertTrue(HouseholdDisplay.canManage(user: user(2, role: "member"), members: members), "the membership's role wins")
        XCTAssertFalse(HouseholdDisplay.canManage(user: user(3, role: "member"), members: members))
        XCTAssertFalse(HouseholdDisplay.canManage(user: user(1, role: "owner", household: nil), members: members))
        XCTAssertFalse(HouseholdDisplay.canManage(user: nil, members: members))
    }

    func testTheDisplayShowsHomesTilesWithoutHiddenOrAdminsOnlyOnes() {
        func chore(_ id: Int, visibility: String) -> Chore {
            Chore(id: id, householdId: 9, name: "C\(id)", icon: "🧹", color: "#2E86AB", sortOrder: 0, category: "",
                  isPredefined: false, predefinedKey: nil, createdBy: nil, createdAt: Date(),
                  indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false, visibility: visibility)
        }
        let chores = [chore(1, visibility: "household"), chore(2, visibility: "admins"),
                      chore(3, visibility: "household"), chore(4, visibility: "household")]
        XCTAssertEqual(HouseholdDisplay.chores(chores, order: [3, 1, 2], hidden: [4]).map(\.id), [3, 1])
        XCTAssertEqual(HouseholdDisplay.chores(chores, order: [], hidden: []).map(\.id), [1, 3, 4])
        XCTAssertEqual(HouseholdDisplay.chores(chores, order: [3, 3], hidden: []).map(\.id), [3, 1, 4],
                       "a repeated id in the order doesn't trap")
    }

    func testAdoptingAnIdentityReadsItsSetting() {
        let state = AppState()
        let identity = ClientIdentity(managed: true)
        identity.accept(user(1, role: "owner"))
        XCTAssertTrue(HouseholdDisplay.turnOn(pin: "2468", for: origin))
        defer { HouseholdDisplay.removeAll() }
        state.adopt(identity.snapshot)
        XCTAssertTrue(state.householdDisplayOn)
        identity.accept(user(2, role: "owner"))
        state.adopt(identity.snapshot)
        XCTAssertFalse(state.householdDisplayOn, "another account on this device")
    }
}
