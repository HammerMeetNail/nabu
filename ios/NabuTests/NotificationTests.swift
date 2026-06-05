import XCTest
@testable import Nabu

final class NotificationTests: XCTestCase {

    func testNotificationIsRead() {
        let notif = AppNotification(id: 1, userId: 1, type: "chore_reminder",
                                    title: "Test", body: "Body",
                                    isRead: true, createdAt: Date())
        XCTAssertTrue(notif.isRead)
    }

    func testNotificationIsUnread() {
        let notif = AppNotification(id: 1, userId: 1, type: "chore_reminder",
                                    title: "Test", body: "Body",
                                    isRead: false, createdAt: Date())
        XCTAssertFalse(notif.isRead)
    }

    func testNotificationTypes() {
        let validTypes = ["chore_reminder", "household_joined", "member_removed",
                          "role_changed", "invite_created", "schedule_updated"]
        for type in validTypes {
            let notif = AppNotification(id: 1, userId: 1, type: type,
                                        title: "Test", body: "Body",
                                        isRead: false, createdAt: Date())
            XCTAssertEqual(notif.type, type)
        }
    }

    func testNotificationTypeInfo() {
        let info = NotificationTypeInfo(type: "chore_reminder", label: "Chore Reminders",
                                         description: "Get reminded about scheduled chores")
        XCTAssertEqual(info.type, "chore_reminder")
        XCTAssertEqual(info.id, "chore_reminder")
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
}
