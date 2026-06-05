import SwiftUI

struct NotificationsView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var notifications: [AppNotification] = []

    var body: some View {
        NavigationStack {
            Group {
                if notifications.isEmpty {
                    VStack(spacing: 16) {
                        Text("🔔")
                            .font(.system(size: 48))
                        Text("No notifications")
                            .font(.title3)
                            .fontWeight(.semibold)
                        Text("You're all caught up!")
                            .foregroundColor(.secondary)
                    }
                    .frame(maxHeight: .infinity)
                } else {
                    List {
                        ForEach(notifications) { notif in
                            NotificationRow(notification: notif,
                                            onMarkRead: { Task { await markRead(notif) } },
                                            onDelete: { Task { await deleteNotification(notif) } })
                        }
                    }
                    .listStyle(.plain)
                }
            }
            .navigationTitle("Notifications")
            .toolbar {
                if !notifications.isEmpty {
                    ToolbarItem(placement: .navigationBarTrailing) {
                        Button("Mark All Read") {
                            Task { await markAllRead() }
                        }
                    }
                }
            }
        }
        .task {
            await loadNotifications()
        }
    }

    private func loadNotifications() async {
        do {
            let data: NotificationsResponse = try await environment.apiClient.get("/api/notifications")
            notifications = data.notifications
            state.notifications = data.notifications
            state.unreadNotifications = data.unreadCount
        } catch {}
    }

    private func markRead(_ notif: AppNotification) async {
        do {
            let _: StatusResponse = try await environment.apiClient.postEmpty("/api/notifications/\(notif.id)/read")
            if let idx = notifications.firstIndex(where: { $0.id == notif.id }) {
                notifications[idx] = AppNotification(
                    id: notif.id, userId: notif.userId, type: notif.type,
                    title: notif.title, body: notif.body,
                    isRead: true, createdAt: notif.createdAt
                )
            }
            state.unreadNotifications = max(0, state.unreadNotifications - 1)
        } catch {}
    }

    private func markAllRead() async {
        do {
            let _: StatusResponse = try await environment.apiClient.postEmpty("/api/notifications/read-all")
            notifications = notifications.map {
                AppNotification(id: $0.id, userId: $0.userId, type: $0.type,
                                title: $0.title, body: $0.body,
                                isRead: true, createdAt: $0.createdAt)
            }
            state.unreadNotifications = 0
        } catch {}
    }

    private func deleteNotification(_ notif: AppNotification) async {
        do {
            let _: StatusResponse = try await environment.apiClient.delete("/api/notifications/\(notif.id)")
            notifications.removeAll { $0.id == notif.id }
            if !notif.isRead {
                state.unreadNotifications = max(0, state.unreadNotifications - 1)
            }
        } catch {}
    }
}

struct NotificationRow: View {
    let notification: AppNotification
    let onMarkRead: () -> Void
    let onDelete: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Circle()
                .fill(notification.isRead ? Color.clear : Color.accentColor)
                .frame(width: 8, height: 8)

            VStack(alignment: .leading, spacing: 4) {
                Text(notification.title)
                    .font(.subheadline)
                    .fontWeight(notification.isRead ? .regular : .semibold)
                Text(notification.body)
                    .font(.caption)
                    .foregroundColor(.secondary)
                Text(notification.createdAt, style: .relative)
                    .font(.caption2)
                    .foregroundColor(.secondary)
            }

            Spacer()

            if !notification.isRead {
                Button {
                    onMarkRead()
                } label: {
                    Image(systemName: "envelope.open")
                        .font(.caption)
                }
                .buttonStyle(.plain)
            }
        }
        .swipeActions(edge: .trailing) {
            Button(role: .destructive) {
                onDelete()
            } label: {
                Label("Delete", systemImage: "trash")
            }
        }
    }
}
