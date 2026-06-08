import XCTest
@testable import Nabu

final class NotificationTests: XCTestCase {

    func testNotificationIsRead() {
        let notif = AppNotification(id: 1, userId: 1, type: "chore_logged",
                                    title: "Test", body: "Body",
                                    isRead: true, createdAt: Date())
        XCTAssertTrue(notif.isRead)
    }

    func testNotificationIsUnread() {
        let notif = AppNotification(id: 1, userId: 1, type: "chore_logged",
                                    title: "Test", body: "Body",
                                    isRead: false, createdAt: Date())
        XCTAssertFalse(notif.isRead)
    }

    func testNotificationTypes() {
        let validTypes = ["chore_logged", "household_joined"]
        for type in validTypes {
            let notif = AppNotification(id: 1, userId: 1, type: type,
                                        title: "Test", body: "Body",
                                        isRead: false, createdAt: Date())
            XCTAssertEqual(notif.type, type)
        }
    }

    func testNotificationTypeInfo() {
        let info = NotificationTypeInfo(type: "chore_logged", label: "Chore Logged",
                                         description: "When someone else in your household logs a chore.")
        XCTAssertEqual(info.type, "chore_logged")
        XCTAssertEqual(info.id, "chore_logged")
    }

    func testUnreadCount() {
        let notifications = [
            AppNotification(id: 1, userId: 1, type: "test", title: "1", body: "b",
                            isRead: false, createdAt: Date()),
            AppNotification(id: 2, userId: 1, type: "test", title: "2", body: "b",
                            isRead: true, createdAt: Date()),
            AppNotification(id: 3, userId: 1, type: "test", title: "3", body: "b",
                            isRead: false, createdAt: Date()),
        ]
        let unread = notifications.filter { !$0.isRead }.count
        XCTAssertEqual(unread, 2)
    }

    // MARK: - Preferences

    func testReminderPreferenceDefaults() {
        let prefs = ReminderPreference(
            userId: 1, pushEnabled: true, emailEnabled: false,
            quietHoursStart: "", quietHoursEnd: "", timezone: "UTC",
            enabledPushTypes: [], defaultReminderLeadMinutes: 10
        )
        XCTAssertTrue(prefs.pushEnabled)
        XCTAssertFalse(prefs.emailEnabled)
        XCTAssertTrue(prefs.enabledPushTypes.isEmpty)
    }

    func testReminderPreferencePushDisabled() {
        let prefs = ReminderPreference(
            userId: 1, pushEnabled: false, emailEnabled: false,
            quietHoursStart: "", quietHoursEnd: "", timezone: "UTC",
            enabledPushTypes: [], defaultReminderLeadMinutes: 10
        )
        XCTAssertFalse(prefs.pushEnabled)
        XCTAssertTrue(prefs.enabledPushTypes.isEmpty)
    }

    func testReminderPreferenceSpecificTypes() {
        let prefs = ReminderPreference(
            userId: 1, pushEnabled: true, emailEnabled: false,
            quietHoursStart: "", quietHoursEnd: "", timezone: "UTC",
            enabledPushTypes: ["chore_logged"], defaultReminderLeadMinutes: 10
        )
        XCTAssertTrue(prefs.pushEnabled)
        XCTAssertEqual(prefs.enabledPushTypes, ["chore_logged"])
    }

    func testPatchNotificationPrefsRequestEncoding() throws {
        let req = PatchNotificationPrefsRequest(
            pushEnabled: true,
            emailEnabled: nil,
            enabledPushTypes: ["chore_logged"]
        )
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .useDefaultKeys
        let data = try encoder.encode(req)
        let dict = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        XCTAssertEqual(dict?["pushEnabled"] as? Bool, true)
        XCTAssertNil(dict?["emailEnabled"])
        XCTAssertEqual(dict?["enabledPushTypes"] as? [String], ["chore_logged"])
    }

    func testPatchNotificationPrefsDisableAll() throws {
        let req = PatchNotificationPrefsRequest(
            pushEnabled: false,
            emailEnabled: nil,
            enabledPushTypes: []
        )
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .useDefaultKeys
        let data = try encoder.encode(req)
        let dict = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        XCTAssertEqual(dict?["pushEnabled"] as? Bool, false)
        let types = dict?["enabledPushTypes"] as? [String] ?? []
        XCTAssertTrue(types.isEmpty)
    }

    func testNotificationPrefsResponseDecoding() throws {
        let json = """
        {
          "preferences": {
            "userId": 1,
            "pushEnabled": true,
            "emailEnabled": false,
            "quietHoursStart": "",
            "quietHoursEnd": "",
            "timezone": "UTC",
            "enabledPushTypes": ["chore_logged", "household_joined"],
            "defaultReminderLeadMinutes": 10
          },
          "availableTypes": [
            {"type": "chore_logged", "label": "Chore Logged", "description": "When someone else in your household logs a chore."},
            {"type": "household_joined", "label": "Household Joined", "description": "When someone joins your household."}
          ]
        }
        """.data(using: .utf8)!
        let decoder = JSONDecoder()
        let response = try decoder.decode(NotificationPrefsResponse.self, from: json)
        XCTAssertTrue(response.preferences.pushEnabled)
        XCTAssertEqual(response.preferences.enabledPushTypes, ["chore_logged", "household_joined"])
        XCTAssertEqual(response.availableTypes.count, 2)
        XCTAssertEqual(response.availableTypes[0].type, "chore_logged")
        XCTAssertEqual(response.availableTypes[1].type, "household_joined")
    }

    func testNotificationPrefsResponseEmptyEnabledTypesMeansAll() throws {
        let json = """
        {
          "preferences": {
            "userId": 1,
            "pushEnabled": true,
            "emailEnabled": false,
            "quietHoursStart": "",
            "quietHoursEnd": "",
            "timezone": "UTC",
            "enabledPushTypes": [],
            "defaultReminderLeadMinutes": 10
          },
          "availableTypes": [
            {"type": "chore_logged", "label": "Chore Logged", "description": "When someone else in your household logs a chore."},
            {"type": "household_joined", "label": "Household Joined", "description": "When someone joins your household."}
          ]
        }
        """.data(using: .utf8)!
        let decoder = JSONDecoder()
        let response = try decoder.decode(NotificationPrefsResponse.self, from: json)
        XCTAssertTrue(response.preferences.pushEnabled)
        XCTAssertTrue(response.preferences.enabledPushTypes.isEmpty)
        XCTAssertEqual(response.availableTypes.count, 2)
    }

    @MainActor
    func testStateNotificationPrefsDefaultNil() {
        let state = AppState()
        XCTAssertNil(state.notificationPrefs)
        XCTAssertTrue(state.availableNotificationTypes.isEmpty)
    }

    @MainActor
    func testStateNotificationPrefsReset() {
        let state = AppState()
        state.notificationPrefs = ReminderPreference(
            userId: 1, pushEnabled: true, emailEnabled: false,
            quietHoursStart: "", quietHoursEnd: "", timezone: "UTC",
            enabledPushTypes: ["chore_logged"], defaultReminderLeadMinutes: 10
        )
        state.availableNotificationTypes = [
            NotificationTypeInfo(type: "chore_logged", label: "Chore Logged", description: "desc")
        ]
        state.reset()
        XCTAssertNil(state.notificationPrefs)
        XCTAssertTrue(state.availableNotificationTypes.isEmpty)
    }
}
