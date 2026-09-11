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

    func testLogUnitEncodingAndLegacyQueueDecoding() throws {
        let legacy = Data(#"{"choreId":1,"volumeML":5}"#.utf8)
        let decoded = try apiDecoder.decode(CreateLogRequest.self, from: legacy)
        XCTAssertNil(decoded.metricUnit)
        let request = CreateLogRequest(choreId: 1, note: nil, indicators: nil, date: nil, hour: nil, completedAt: nil, volumeML: 5, userId: nil, indicatorVolumes: nil, followUpMinutes: nil, followUpTime: nil, metricUnit: "mg")
        XCTAssertEqual(json(try apiEncoder.encode(request))["metricUnit"] as? String, "mg")
        let patch = UpdateLogRequest(note: nil, indicators: nil, volumeML: nil, userId: nil, completedAt: nil, hour: nil, date: nil, indicatorVolumes: nil, metricUnit: "g")
        let body = json(try apiEncoder.encode(patch))
        XCTAssertEqual(body["metricUnit"] as? String, "g")
        XCTAssertNil(body["volumeML"])
        XCTAssertTrue(AmountUnits.options("reps").contains("reps"))
        for unit in ["mL", "mg", "g"] { XCTAssertTrue(AmountUnits.common.contains(unit)) }
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

    func testChangePasswordUsesServerKeysWithProductionEncoder() throws {
        for current in ["", "existing-password"] {
            let request = ChangePasswordRequest(currentPassword: current, newPassword: "replacement-password")
            let body = json(try apiEncoder.encode(request))
            XCTAssertEqual(Set(body.keys), ["current_password", "new_password"])
            XCTAssertEqual(body["current_password"] as? String, current)
            XCTAssertEqual(body["new_password"] as? String, "replacement-password")
        }
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
            indicatorDefaults: ["morning"],
            followUpEnabled: true
        )
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["name"] as? String, "Walk Dog")
        XCTAssertEqual(dict["icon"] as? String, "🐕")
        XCTAssertEqual(dict["color"] as? String, "#FF0000")
        XCTAssertEqual(dict["category"] as? String, "exercise")
        XCTAssertEqual(dict["indicator_labels"] as? [String], ["morning", "evening"])
        XCTAssertEqual(dict["indicator_defaults"] as? [String], ["morning"])
        XCTAssertEqual(dict["follow_up_enabled"] as? Bool, true)
    }

    func testCreateChoreRequestMinimal() throws {
        let req = CreateChoreRequest(name: "Simple", icon: nil, color: nil, category: nil, indicatorLabels: nil, indicatorDefaults: nil, followUpEnabled: nil)
        let data = try encoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["name"] as? String, "Simple")
        XCTAssertNil(dict["icon"])
        XCTAssertNil(dict["color"])
        XCTAssertNil(dict["follow_up_enabled"])
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
            followUpMinutes: nil,
            followUpTime: nil
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
            followUpMinutes: nil,
            followUpTime: nil)
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
            followUpMinutes: nil,
            followUpTime: nil)
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
            followUpMinutes: nil,
            followUpTime: nil)
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

    func testNullableLogMetricsDistinguishOmissionFromClear() throws {
        let clear = UpdateLogRequest(note: nil, indicators: nil, volumeML: .some(nil),
            userId: nil, completedAt: nil, hour: nil, date: nil, indicatorVolumes: [:],
            rating: .some(nil), title: .some(nil), durationSeconds: .some(nil), subject: .some(nil))
        let encoded = json(try apiEncoder.encode(clear))
        for key in ["volumeML", "rating", "title", "durationSeconds", "subject"] {
            XCTAssertTrue(encoded[key] is NSNull, key)
        }
        XCTAssertEqual((encoded["indicatorVolumes"] as? [String: Int])?.count, 0)
        let omit = UpdateLogRequest(note: "only note", indicators: nil, volumeML: nil,
            userId: nil, completedAt: nil, hour: nil, date: nil, indicatorVolumes: nil)
        let omitted = json(try apiEncoder.encode(omit))
        XCTAssertEqual(omitted.count, 1)
        XCTAssertEqual(omitted["note"] as? String, "only note")
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
            enabledPushTypes: ["chore_logged"],
            defaultReminderLeadMinutes: nil
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

    // MARK: - Wire-format tests for migration 035-039 fields
    // These use the real apiEncoder: the Go server expects camelCase keys.

    func testCreateLogRequestOfflineAndMetricFieldsWireFormat() throws {
        let req = CreateLogRequest(
            choreId: 5,
            note: nil,
            indicators: nil,
            date: "2026-07-02",
            hour: 9,
            completedAt: "2026-07-02T09:15:00Z",
            volumeML: nil,
            userId: nil,
            indicatorVolumes: nil,
            followUpMinutes: nil,
            followUpTime: nil,
            rating: 40,
            durationSeconds: 1800,
            subject: "Ada",
            idempotencyKey: "client-key-123"
        )
        let data = try apiEncoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["idempotencyKey"] as? String, "client-key-123")
        XCTAssertEqual(dict["durationSeconds"] as? Int, 1800)
        XCTAssertEqual(dict["subject"] as? String, "Ada")
        XCTAssertEqual(dict["rating"] as? Int, 40)
        XCTAssertEqual(dict["choreId"] as? Int, 5)
        XCTAssertEqual(dict["completedAt"] as? String, "2026-07-02T09:15:00Z")
    }

    func testCreateLogRequestOmitsNewFieldsWhenNil() throws {
        let req = CreateLogRequest(
            choreId: 1,
            note: nil,
            indicators: nil,
            date: nil,
            hour: 14,
            completedAt: "2026-07-02T14:00:00Z",
            volumeML: nil,
            userId: nil,
            indicatorVolumes: nil,
            followUpMinutes: nil,
            followUpTime: nil
        )
        let data = try apiEncoder.encode(req)
        let dict = json(data)
        XCTAssertNil(dict["idempotencyKey"])
        XCTAssertNil(dict["durationSeconds"])
        XCTAssertNil(dict["subject"])
        XCTAssertNil(dict["rating"])
    }

    func testCreateLogRequestTitleWireFormat() throws {
        let req = CreateLogRequest(
            choreId: 1, note: nil, indicators: nil, date: nil, hour: 10,
            completedAt: "2026-07-03T10:00:00Z", volumeML: nil, userId: nil,
            indicatorVolumes: nil, followUpMinutes: nil, followUpTime: nil,
            title: "Great nap"
        )
        let data = try apiEncoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["title"] as? String, "Great nap")
    }

    func testUpdateLogRequestSubjectExplicitNullClearsTag() throws {
        // Deselecting the subject chip on edit sends an explicit JSON null
        // (clears the tag) — the PWA's updateLog behavior. An omitted subject
        // means "no change".
        let req = UpdateLogRequest(
            note: nil, indicators: nil, volumeML: nil, userId: nil,
            completedAt: nil, hour: nil, date: nil, indicatorVolumes: nil,
            subject: .some(nil)
        )
        let data = try apiEncoder.encode(req)
        let dict = json(data)
        XCTAssertTrue(dict["subject"] is NSNull, "explicit null expected, got \(String(describing: dict["subject"]))")

        let omitted = UpdateLogRequest(
            note: nil, indicators: nil, volumeML: nil, userId: nil,
            completedAt: nil, hour: nil, date: nil, indicatorVolumes: nil
        )
        let omittedDict = json(try apiEncoder.encode(omitted))
        XCTAssertNil(omittedDict["subject"])
    }

    func testUpdateLogRequestNewFieldsWireFormat() throws {
        let req = UpdateLogRequest(
            note: nil,
            indicators: nil,
            volumeML: nil,
            userId: nil,
            completedAt: nil,
            hour: nil,
            date: nil,
            indicatorVolumes: nil,
            rating: nil,
            durationSeconds: 900,
            subject: "Grace"
        )
        let data = try apiEncoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["durationSeconds"] as? Int, 900)
        XCTAssertEqual(dict["subject"] as? String, "Grace")
        XCTAssertNil(dict["rating"])
    }

    func testCreateChoreRequestMetricFieldsWireFormat() throws {
        let req = CreateChoreRequest(
            name: "Nap",
            icon: "😴",
            color: nil,
            category: nil,
            indicatorLabels: nil,
            indicatorDefaults: nil,
            followUpEnabled: nil,
            metricType: "duration",
            metricUnit: "",
            subjects: ["Ada", "Grace"]
        )
        let data = try apiEncoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["metricType"] as? String, "duration")
        XCTAssertEqual(dict["metricUnit"] as? String, "")
        XCTAssertEqual(dict["subjects"] as? [String], ["Ada", "Grace"])
    }

    func testCreateChoreRequestOmitsMetricFieldsWhenNil() throws {
        let req = CreateChoreRequest(name: "Simple", icon: nil, color: nil, category: nil, indicatorLabels: nil, indicatorDefaults: nil, followUpEnabled: nil)
        let data = try apiEncoder.encode(req)
        let dict = json(data)
        XCTAssertNil(dict["metricType"])
        XCTAssertNil(dict["metricUnit"])
        XCTAssertNil(dict["subjects"])
    }

    func testPatchUserPreferencesNewFieldsWireFormat() throws {
        var req = PatchUserPreferencesRequest()
        req.volumeUnit = "oz"
        req.statsWidgets = [
            StatsWidget(id: "abc-123", type: "total", choreIds: [5], metric: "amount",
                        agg: "sum", period: "week", grain: "daily", title: "Bottles this week")
        ]
        let data = try apiEncoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["volumeUnit"] as? String, "oz")
        let widgets = try XCTUnwrap(dict["statsWidgets"] as? [[String: Any]])
        XCTAssertEqual(widgets.count, 1)
        XCTAssertEqual(widgets[0]["id"] as? String, "abc-123")
        XCTAssertEqual(widgets[0]["choreIds"] as? [Int], [5])
        XCTAssertNil(dict["choreOrder"])
        XCTAssertNil(dict["statsSectionOrder"])
    }

    func testPatchUserPreferencesHideNotificationBadgeWireFormat() throws {
        // Only the badge field is set: it must encode under the server's
        // camelCase key and omit every untouched optional.
        var req = PatchUserPreferencesRequest()
        req.hideNotificationBadge = true
        let data = try apiEncoder.encode(req)
        let dict = json(data)
        XCTAssertEqual(dict["hideNotificationBadge"] as? Bool, true)
        XCTAssertNil(dict["volumeUnit"])
        XCTAssertNil(dict["choreOrder"])
        XCTAssertNil(dict["statsWidgets"])

        // And the false direction round-trips too.
        var off = PatchUserPreferencesRequest()
        off.hideNotificationBadge = false
        let offDict = json(try apiEncoder.encode(off))
        XCTAssertEqual(offDict["hideNotificationBadge"] as? Bool, false)
    }
}
