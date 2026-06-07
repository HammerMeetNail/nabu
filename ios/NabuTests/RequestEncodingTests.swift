import XCTest
@testable import Nabu

final class RequestEncodingTests: XCTestCase {
    let encoder: JSONEncoder = {
        let e = JSONEncoder()
        e.keyEncodingStrategy = .convertToSnakeCase
        e.outputFormatting = [.sortedKeys, .prettyPrinted]
        return e
    }()

    func json(_ data: Data) -> [String: Any] {
        try! JSONSerialization.jsonObject(with: data) as! [String: Any]
    }

    // MARK: - Auth

    func testRegisterRequest() throws {
        let req = RegisterRequest(email: "a@b.com", password: "secret123")
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["email"] as? String, "a@b.com")
        XCTAssertEqual(dict["password"] as? String, "secret123")
    }

    func testLoginRequest() throws {
        let req = LoginRequest(email: "a@b.com", password: "secret123")
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["email"] as? String, "a@b.com")
        XCTAssertEqual(dict["password"] as? String, "secret123")
    }

    func testMagicLinkRequest() throws {
        let req = MagicLinkRequest(email: "a@b.com")
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["email"] as? String, "a@b.com")
    }

    func testResetPasswordRequest() throws {
        let req = ResetPasswordRequest(token: "tok123", password: "newpass")
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["token"] as? String, "tok123")
        XCTAssertEqual(dict["password"] as? String, "newpass")
    }

    // MARK: - Household

    func testCreateHouseholdRequest() throws {
        let req = CreateHouseholdRequest(name: "My Home", initials: "MH")
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["name"] as? String, "My Home")
        XCTAssertEqual(dict["initials"] as? String, "MH")
    }

    func testJoinHouseholdRequest() throws {
        let req = JoinHouseholdRequest(inviteCode: "ABC123")
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["invite_code"] as? String, "ABC123")
    }

    func testTransferOwnershipRequest() throws {
        let req = TransferOwnershipRequest(newOwnerId: 2)
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["new_owner_id"] as? Int, 2)
    }

    // MARK: - Chores

    func testCreateChoreRequestFull() throws {
        let req = CreateChoreRequest(
            name: "Walk Dog",
            icon: "🐕",
            color: "#FF0000",
            category: "exercise",
            indicatorLabels: ["morning", "evening"],
            indicatorDefaults: ["morning"]
        )
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["name"] as? String, "Walk Dog")
        XCTAssertEqual(dict["icon"] as? String, "🐕")
        XCTAssertEqual(dict["color"] as? String, "#FF0000")
        XCTAssertEqual(dict["category"] as? String, "exercise")
        XCTAssertEqual(dict["indicator_labels"] as? [String], ["morning", "evening"])
        XCTAssertEqual(dict["indicator_defaults"] as? [String], ["morning"])
    }

    func testCreateChoreRequestMinimal() throws {
        let req = CreateChoreRequest(name: "Simple", icon: nil, color: nil, category: nil, indicatorLabels: nil, indicatorDefaults: nil)
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["name"] as? String, "Simple")
        XCTAssertNil(dict["icon"])
        XCTAssertNil(dict["color"])
    }

    // MARK: - Logs

    func testCreateLogRequestHomeDirectTap() throws {
        let req = CreateLogRequest(
            choreId: 1,
            note: nil,
            indicators: nil,
            date: nil,
            hour: 14,
            completedAt: "2024-12-25T14:30:00Z",
            volumeML: nil,
            userId: nil,
            indicatorVolumes: nil,
            followUpMinutes: nil
        )
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["chore_id"] as? Int, 1)
        XCTAssertEqual(dict["hour"] as? Int, 14)
        XCTAssertEqual(dict["completed_at"] as? String, "2024-12-25T14:30:00Z")
        XCTAssertNil(dict["note"])
    }

    func testCreateLogRequestWithVolume() throws {
        let req = CreateLogRequest(
            choreId: 1,
            note: "big feed",
            indicators: ["🍼 formula"],
            date: "2024-12-25",
            hour: 8,
            completedAt: "2024-12-25T08:00:00Z",
            volumeML: 120,
            userId: nil,
            indicatorVolumes: nil,
            followUpMinutes: nil)
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["volume_ml"] as? Int, 120)
        XCTAssertEqual(dict["indicators"] as? [String], ["🍼 formula"])
    }

    func testCreateLogRequestAnytime() throws {
        let req = CreateLogRequest(
            choreId: 2,
            note: nil,
            indicators: nil,
            date: "2024-12-25",
            hour: nil,
            completedAt: "2024-12-25T12:00:00Z",
            volumeML: nil,
            userId: nil,
            indicatorVolumes: nil,
            followUpMinutes: nil)
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertNil(dict["hour"])
    }

    func testCreateLogRequestOnBehalfOf() throws {
        let req = CreateLogRequest(
            choreId: 1,
            note: nil,
            indicators: nil,
            date: "2024-12-25",
            hour: 10,
            completedAt: "2024-12-25T10:00:00Z",
            volumeML: nil,
            userId: 2,
            indicatorVolumes: nil,
            followUpMinutes: nil)
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["user_id"] as? Int, 2)
    }

    func testUpdateLogRequestSparse() throws {
        let req = UpdateLogRequest(
            note: "updated note",
            indicators: nil,
            volumeML: nil,
            userId: nil,
            completedAt: nil,
            hour: nil,
            date: nil,
            indicatorVolumes: nil)
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["note"] as? String, "updated note")
        XCTAssertNil(dict["indicators"])
        XCTAssertNil(dict["volume_ml"])
    }

    // MARK: - Preferences

    func testPatchUserPreferencesPartial() throws {
        let req = PatchUserPreferencesRequest(
            choreOrder: [3, 1, 2],
            hiddenHomeChoreIds: nil,
            timezone: nil
        )
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["chore_order"] as? [Int], [3, 1, 2])
        XCTAssertNil(dict["hidden_home_chore_ids"])
    }

    func testPatchNotificationPrefs() throws {
        let req = PatchNotificationPrefsRequest(
            pushEnabled: true,
            emailEnabled: nil,
            enabledPushTypes: ["chore_logged"]
        )
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["push_enabled"] as? Bool, true)
        XCTAssertNil(dict["email_enabled"])
        XCTAssertEqual(dict["enabled_push_types"] as? [String], ["chore_logged"])
    }

    // MARK: - Schedule

    func testCreateScheduleRequest() throws {
        let req = CreateScheduleRequest(
            choreId: 1,
            frequencyType: "daily",
            timePeriod: "anytime",
            specificTime: "08:00",
            daysOfWeek: nil,
            intervalDays: nil,
            dayOfMonth: nil,
            monthWeekday: nil,
            monthOfYear: nil,
            startDate: "2024-12-25",
            recurrenceEnd: nil,
            targetCount: nil,
            isActive: true,
            assignedUserId: nil
        )
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["chore_id"] as? Int, 1)
        XCTAssertEqual(dict["frequency_type"] as? String, "daily")
        XCTAssertEqual(dict["specific_time"] as? String, "08:00")
        XCTAssertTrue(dict["is_active"] as? Bool ?? false)
    }
}
