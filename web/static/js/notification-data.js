import { loadNotifications, markRead, markAllRead, deleteNotification } from "./notifications.js";
import { captureScope } from "./request-scope.js";

// Refresh, append, and mutations share one sequence. A mutation invalidates
// pending reads before transport, so an older page cannot restore removed rows.
export async function loadNotificationPage(state, {append = false} = {}) {
  if (state.notificationMutating) return;
  if (append && (!state.notificationCursor || state.notificationLoading || state.notificationLoadingMore)) return;
  const cursor = append ? state.notificationCursor : null;
  const scope = captureScope(state, "notifications");
  state.notificationLoading = !append;
  state.notificationLoadingMore = append;
  state.notificationError = null;
  state.notificationErrorAction = null;
  try {
    const data = await loadNotifications(cursor);
    if (!scope.current()) return;
    if (!Array.isArray(data?.notifications)) throw new Error("Could not load notifications. Please retry.");
    const ids = new Set(state.notifications.map(n => n.id));
    state.notifications = append ? [...state.notifications, ...data.notifications.filter(n => !ids.has(n.id))] : data.notifications;
    state.notificationCursor = data.nextCursor || null;
    state.unreadNotifications = data.unreadCount || 0;
  } catch (err) {
    if (scope.current()) {
      state.notificationError = err.message || "Could not load notifications. Please retry.";
      state.notificationErrorAction = append ? "more" : "refresh";
    }
  } finally {
    if (scope.current()) { state.notificationLoading = false; state.notificationLoadingMore = false; }
  }
}

export async function mutateNotification(state, action, id) {
  if (state.notificationMutating) return;
  const scope = captureScope(state, "notifications");
  state.notificationMutating = true;
  state.notificationLoading = false;
  state.notificationLoadingMore = false;
  state.notificationError = null;
  state.notificationErrorAction = null;
  try {
    if (action === "all") await markAllRead();
    else if (action === "delete") await deleteNotification(id);
    else await markRead(id);
    if (!scope.current()) return;
    const wasUnread = state.notifications.some(n => n.id === id && !n.isRead);
    state.notifications = action === "delete" ? state.notifications.filter(n => n.id !== id)
      : state.notifications.map(n => action === "all" || n.id === id ? {...n, isRead:true} : n);
    state.unreadNotifications = action === "all" ? 0 : Math.max(0, state.unreadNotifications - (wasUnread ? 1 : 0));
  } catch (err) {
    if (scope.current()) state.notificationError = err.message || "Could not update the notification. Please retry.";
  } finally {
    if (scope.current()) state.notificationMutating = false;
  }
}
