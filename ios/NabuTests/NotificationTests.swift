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
            enabledPushTypes: ["chore_logged"],
            defaultReminderLeadMinutes: nil
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
            enabledPushTypes: [],
            defaultReminderLeadMinutes: nil
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

@MainActor
final class NotificationOwnershipTests: XCTestCase {
    private func response(_ request: URLRequest, _ ids: [Int], cursor: String? = nil, status: Int = 200) throws -> (Data, URLResponse) {
        let rows = ids.map { AppNotification(id: $0, userId: 1, type: "chore_logged", title: "Synthetic \($0)", body: "", isRead: false, createdAt: Date()) }
        return (try apiEncoder.encode(NotificationsResponse(notifications: rows, unreadCount: rows.count, nextCursor: cursor)),
                HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!)
    }

    func testRefreshOrMutationInvalidatesPendingNotificationPage() async throws {
        for mutation in [false, true] {
            let state = AppState()
            let gate = NativeResponseGate()
            var api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
            var reads = 0
            api.mockAsyncHandler = { request in
                if request.httpMethod == "DELETE" {
                    return (Data(#"{"status":"ok"}"#.utf8), HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
                }
                if request.url!.query != nil { await gate.pause(); return try self.response(request, [60, 1], cursor: "obsolete") }
                reads += 1
                return try self.response(request, reads == 1 ? [60] : [70], cursor: reads == 1 ? "first" : "current")
            }
            let loader = NotificationDataLoader(api: api, state: state)
            await loader.loadNotifData()
            let old = Task { await loader.loadNotifData(append: true) }
            await gate.waitUntilPaused()
            if mutation { await loader.mutate(.delete(60)) } else { await loader.loadNotifData() }
            await gate.release()
            await old.value
            XCTAssertEqual(state.notifications.map(\.id), mutation ? [] : [70])
            XCTAssertEqual(state.notificationCursor, mutation ? "first" : "current")
            XCTAssertFalse(state.notificationLoadingMore)
            XCTAssertFalse(state.notificationMutating)
            XCTAssertNil(state.notificationError)
        }
    }

    func testFailedNotificationPageRetainsCursorAndCanRetry() async throws {
        let state = AppState()
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
        var fail = true
        api.mockAsyncHandler = { request in
            if request.url!.query == nil { return try self.response(request, [60], cursor: "older") }
            if fail { throw URLError(.notConnectedToInternet) }
            return try self.response(request, [1])
        }
        let loader = NotificationDataLoader(api: api, state: state)
        await loader.loadNotifData()
        await loader.loadNotifData(append: true)
        XCTAssertEqual(state.notifications.map(\.id), [60])
        XCTAssertEqual(state.notificationCursor, "older")
        XCTAssertNotNil(state.notificationError)
        fail = false
        await loader.loadNotifData(append: true)
        XCTAssertEqual(state.notifications.map(\.id), [60, 1])
        XCTAssertNil(state.notificationCursor)
        XCTAssertNil(state.notificationError)
    }

    func testPendingNotificationResponseCannotPublishAfterReset() async throws {
        let state = AppState()
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
        let gate = NativeResponseGate()
        api.mockAsyncHandler = { request in await gate.pause(); return try self.response(request, [99], cursor: "old") }
        let loader = NotificationDataLoader(api: api, state: state)
        let old = Task { await loader.loadNotifData() }
        await gate.waitUntilPaused()
        state.reset()
        await gate.release()
        await old.value
        XCTAssertTrue(state.notifications.isEmpty)
        XCTAssertNil(state.notificationCursor)
        XCTAssertFalse(state.notificationLoading)
    }
}
