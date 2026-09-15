import SwiftUI

struct NotificationsView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.dismiss) private var dismiss
    @State private var loader: NotificationDataLoader?
    @State private var presentationID: UUID?

    var body: some View {
        NavigationStack {
            List {
                if let error = state.notificationError {
                    Text(error).foregroundStyle(.red).accessibilityIdentifier("notification-error")
                }
                if state.notificationLoading { ProgressView("Loading notifications…") }
                if state.notifications.isEmpty && !state.notificationLoading {
                    ContentUnavailableView("No notifications", systemImage: "bell")
                }
                ForEach(state.notifications) { notification in
                    NotificationRow(notification: notification,
                                    onMarkRead: { Task { await loader?.mutate(.read(notification.id)) } },
                                    onDelete: { Task { await loader?.mutate(.delete(notification.id)) } })
                    .disabled(state.notificationMutating)
                }
                if state.notificationCursor != nil {
                    Button(state.notificationLoadingMore ? "Loading…" : state.notificationErrorIsAppend ? "Retry loading older notifications" : "Load older notifications") {
                        Task { await loader?.loadNotifData(append: true) }
                    }
                    .disabled(state.notificationLoading || state.notificationLoadingMore || state.notificationMutating)
                    .accessibilityIdentifier("notifications-load-more")
                }
                Text("Notifications stay here until you delete them.").font(.footnote).foregroundStyle(.secondary)
            }
            .listStyle(.plain)
            .refreshable { await loader?.loadNotifData() }
            .navigationTitle("Notifications")
            .safeAreaInset(edge: .top, spacing: 0) {
                let layout = dynamicTypeSize.isAccessibilitySize
                    ? AnyLayout(VStackLayout(spacing: 8))
                    : AnyLayout(HStackLayout(spacing: 8))
                layout {
                    bulkAction("Mark all read", symbol: "checkmark", identifier: "notifications-mark-all-read",
                               disabled: state.unreadNotifications == 0, action: .all)
                    bulkAction("Clear all", symbol: "trash", identifier: "notifications-clear-all",
                               disabled: state.notifications.isEmpty && state.notificationCursor == nil && state.unreadNotifications == 0,
                               action: .clearAll)
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 12)
                .background(.bar, ignoresSafeAreaEdges: [])
            }
            .toolbar {
                ToolbarItemGroup(placement: .navigationBarTrailing) {
                    Button("Refresh") { Task { await loader?.loadNotifData() } }
                        .disabled(state.notificationLoading || state.notificationMutating)
                        .accessibilityLabel("Refresh notifications")
                }
            }
        }
        .onAppear { presentationID = UUID() }
        .task {
            state.notificationPanelOpen = true
            // Freeze this screen's API identity; shared AppState operations also
            // fence reads from the background loader and any older screen.
            let current = NotificationDataLoader(api: environment.apiClient.scoped(), state: state)
            loader = current
            await current.loadNotifData()
        }
        .onDisappear {
            presentationID = nil
            state.notificationPanelOpen = false
        }
    }

    private func bulkAction(_ title: String, symbol: String, identifier: String,
                            disabled: Bool, action: NotificationDataLoader.Action) -> some View {
        Button {
            let presentation = presentationID
            Task {
                let updated = await loader?.mutate(action) ?? false
                guard updated, case .clearAll = action,
                      let presentation, presentationID == presentation,
                      !Task.isCancelled else { return }
                dismiss()
            }
        } label: {
            Label(title, systemImage: symbol)
                .font(.subheadline.weight(.semibold))
                .frame(maxWidth: .infinity, minHeight: 48)
                .contentShape(Rectangle())
        }
        .buttonStyle(.bordered)
        .disabled(disabled || state.notificationMutating)
        .accessibilityIdentifier(identifier)
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
                .accessibilityLabel("Mark notification read")
                .accessibilityIdentifier("notification-mark-read-\(notification.id)")
            }
        }
        .swipeActions(edge: .trailing) {
            Button(role: .destructive) {
                onDelete()
            } label: {
                Label("Delete", systemImage: "trash")
            }
        }
        .swipeActions(edge: .leading) {
            if !notification.isRead {
                Button {
                    onMarkRead()
                } label: {
                    Label("Mark read", systemImage: "envelope.open")
                }
                .tint(.blue)
            }
        }
    }
}
