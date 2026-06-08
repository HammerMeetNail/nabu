import Foundation

@MainActor
final class NotificationDataLoader {
    let api: APIClient
    let state: AppState

    init(api: APIClient, state: AppState) {
        self.api = api
        self.state = state
    }

    func loadNotifData() async {
        do {
            let data: NotificationsResponse = try await api.get("/api/notifications")
            state.notifications = data.notifications
            state.unreadNotifications = data.unreadCount
        } catch {
            // Silent failure
        }
    }

    func loadNotificationPreferences() async {
        do {
            let data: NotificationPrefsResponse = try await api.get("/api/notification-preferences")
            state.notificationPrefs = data.preferences
            state.availableNotificationTypes = data.availableTypes
        } catch {
            // Silent failure
        }
    }

    func saveNotificationPreferences(_ prefs: PatchNotificationPrefsRequest) async throws -> NotificationPrefsResponse {
        let data: NotificationPrefsResponse = try await api.patch("/api/notification-preferences", body: prefs)
        state.notificationPrefs = data.preferences
        return data
    }

    func loadChoreReminderPrefs() async {
        do {
            let data: ChoreReminderPrefsResponse = try await api.get("/api/chore-reminder-prefs")
            state.choreReminderPrefs = data.prefs
        } catch {}
    }

    func saveChoreReminderPref(choreId: Int, enabled: Bool, leadMinutes: Int) async throws -> ChoreReminderPref {
        struct Body: Codable {
            let enabled: Bool
            let leadMinutes: Int
        }
        let body = Body(enabled: enabled, leadMinutes: leadMinutes)
        let data: ChoreReminderPrefResponse = try await api.patch("/api/chore-reminder-prefs/\(choreId)", body: body)
        if let idx = state.choreReminderPrefs.firstIndex(where: { $0.choreId == choreId }) {
            state.choreReminderPrefs[idx] = data.pref
        } else {
            state.choreReminderPrefs.append(data.pref)
        }
        return data.pref
    }
}
