import Foundation

@MainActor
final class NotificationDataLoader {
    let api: APIClient
    let state: AppState

    init(api: APIClient, state: AppState) {
        self.api = api
        self.state = state
    }

    func loadNotifData(append: Bool = false) async {
        guard !state.notificationMutating else { return }
        if append && (state.notificationCursor == nil || state.notificationLoading || state.notificationLoadingMore) { return }
        let api = self.api.scoped()
        guard api.identity.isCurrent(api.requestContext) else { return }
        let owner = state.beginOperation("notifications")
        func owns() -> Bool { state.owns(owner) && api.identity.isCurrent(api.requestContext) }
        let cursor = append ? state.notificationCursor : nil
        state.notificationLoading = !append
        state.notificationLoadingMore = append
        state.notificationError = nil
        state.notificationErrorIsAppend = false
        defer { if owns() { state.notificationLoading = false; state.notificationLoadingMore = false } }
        do {
            let query = cursor.map { [URLQueryItem(name: "cursor", value: $0)] } ?? []
            let data: NotificationsResponse = try await api.get("/api/notifications", query: query)
            guard owns(), !Task.isCancelled else { return }
            let ids = Set(state.notifications.map(\.id))
            state.notifications = append ? state.notifications + data.notifications.filter { !ids.contains($0.id) } : data.notifications
            state.notificationCursor = data.nextCursor
            state.unreadNotifications = data.unreadCount
        } catch {
            guard owns(), !Task.isCancelled else { return }
            state.notificationError = error.localizedDescription
            state.notificationErrorIsAppend = append
        }
    }

    enum Action { case read(Int), delete(Int), all }
    func mutate(_ action: Action) async {
        guard !state.notificationMutating else { return }
        let api = self.api.scoped()
        guard api.identity.isCurrent(api.requestContext) else { return }
        let owner = state.beginOperation("notifications")
        func owns() -> Bool { state.owns(owner) && api.identity.isCurrent(api.requestContext) }
        state.notificationMutating = true
        state.notificationLoading = false
        state.notificationLoadingMore = false
        state.notificationError = nil
        state.notificationErrorIsAppend = false
        defer { if owns() { state.notificationMutating = false } }
        do {
            switch action {
            case .read(let id): let _: StatusResponse = try await api.postEmpty("/api/notifications/\(id)/read")
            case .delete(let id): let _: StatusResponse = try await api.delete("/api/notifications/\(id)")
            case .all: let _: StatusResponse = try await api.postEmpty("/api/notifications/read-all")
            }
            guard owns() else { return }
            let id: Int?
            switch action { case .read(let value), .delete(let value): id = value; case .all: id = nil }
            let wasUnread = state.notifications.contains { $0.id == id && !$0.isRead }
            switch action {
            case .delete(let value): state.notifications.removeAll { $0.id == value }
            case .read, .all:
                state.notifications = state.notifications.map { row in
                    guard id == nil || row.id == id else { return row }
                    return AppNotification(id: row.id, userId: row.userId, type: row.type, title: row.title, body: row.body, isRead: true, createdAt: row.createdAt)
                }
            }
            if id == nil { state.unreadNotifications = 0 }
            else if wasUnread { state.unreadNotifications = max(0, state.unreadNotifications - 1) }
        } catch {
            if owns() { state.notificationError = error.localizedDescription }
        }
    }

    func loadNotificationPreferences() async {
        let api = self.api.scoped()
        let owner = state.beginOperation("loadNotificationPreferences")
        do {
            let data: NotificationPrefsResponse = try await api.get("/api/notification-preferences")
            guard state.owns(owner) else { return }
            state.notificationPrefs = data.preferences
            state.availableNotificationTypes = data.availableTypes
        } catch {
            // Silent failure
        }
    }

    func saveNotificationPreferences(_ prefs: PatchNotificationPrefsRequest) async throws -> NotificationPrefsResponse {
        let api = self.api.scoped()
        let owner = state.beginOperation("saveNotificationPreferences")
        let data: NotificationPrefsResponse = try await api.patch("/api/notification-preferences", body: prefs)
        guard state.owns(owner) else { throw APIError.contextChanged }
        state.notificationPrefs = data.preferences
        return data
    }

    func loadChoreReminderPrefs() async {
        let api = self.api.scoped()
        let owner = state.beginOperation("loadChoreReminderPrefs")
        do {
            let data: ChoreReminderPrefsResponse = try await api.get("/api/chore-reminder-prefs")
            guard state.owns(owner) else { return }
            state.choreReminderPrefs = data.prefs
        } catch {}
    }

    func saveChoreReminderPref(choreId: Int, enabled: Bool, leadMinutes: Int) async throws -> ChoreReminderPref {
        let api = self.api.scoped()
        let owner = state.beginOperation("saveChoreReminderPref")
        struct Body: Codable {
            let enabled: Bool
            let leadMinutes: Int
        }
        let body = Body(enabled: enabled, leadMinutes: leadMinutes)
        let data: ChoreReminderPrefResponse = try await api.patch("/api/chore-reminder-prefs/\(choreId)", body: body)
        guard state.owns(owner) else { throw APIError.contextChanged }
        if let idx = state.choreReminderPrefs.firstIndex(where: { $0.choreId == choreId }) {
            state.choreReminderPrefs[idx] = data.pref
        } else {
            state.choreReminderPrefs.append(data.pref)
        }
        return data.pref
    }
}
