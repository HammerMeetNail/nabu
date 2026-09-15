import { apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";
import { contextSnapshot, assertContext } from "./browser-context.js";

/**
 * Clear the PWA home screen icon badge.
 */
export async function clearAppBadge() {
  try {
    if (navigator.clearAppBadge) {
      await navigator.clearAppBadge();
    }
  } catch { /* not supported */ }
  try {
    const reg = await navigator.serviceWorker.getRegistration();
    if (reg && reg.active && navigator.serviceWorker.controller) {
      navigator.serviceWorker.controller.postMessage("clear-badge");
    }
  } catch { /* SW not available */ }
}

/**
 * Request notification permission. Must be called directly from a user-gesture
 * handler (click, submit) before any async/await operations.
 */
export function requestNotificationPermission() {
  if (typeof Notification === 'undefined') return;
  if (Notification.permission === 'default') {
    Notification.requestPermission().catch(() => {});
  }
}

/**
 * Attempt to register for Web Push notifications if the user granted permission.
 * This is called once after login / registration.
 */
export async function maybeSubscribePush() {
  const origin = contextSnapshot();
  if (!origin?.userId || origin.status !== "active") return;
  const vapidKey = document.querySelector('meta[name="vapid-public-key"]')?.content;
  if (!vapidKey || !navigator.serviceWorker || !window.PushManager) return;
  if (Notification.permission !== 'granted') return;

  try {
    // Register the service worker. Must happen before pushManager.subscribe.
    await navigator.serviceWorker.register("/service-worker.js");
  } catch {
    // Already registered or failed silently.
  }

  try {
    const reg = await navigator.serviceWorker.ready;
    const existing = await reg.pushManager.getSubscription();
    if (existing) {
      assertContext(origin);
      await sendSubscriptionToServer(existing, origin);
      return;
    }

    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidKey),
    });
    assertContext(origin);
    await sendSubscriptionToServer(sub, origin);
  } catch (e) {
    // Best-effort — push is optional. Log the reason for debugging.
    window.__pushError = "Push registration needs retry";
  }
}

async function sendSubscriptionToServer(sub, origin) {
    const { response } = await apiFetch("/api/push/subscribe", {
      method: "POST",
      origin,
      body: JSON.stringify({ subscription: sub.toJSON(), bindingId:origin.bindingId }),
    });
    if (!response.ok) throw new Error("Push registration needs retry");
}

function urlBase64ToUint8Array(base64String) {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/\-/g, "+").replace(/_/g, "/");
  const rawData = atob(base64);
  const outputArray = new Uint8Array(rawData.length);
  for (let i = 0; i < rawData.length; ++i) {
    outputArray[i] = rawData.charCodeAt(i);
  }
  return outputArray;
}

/**
 * Fetch the current user's notification preferences.
 * @returns {{ preferences: object, availableTypes: Array }}
 */
export async function loadNotificationPreferences() {
  return (await apiFetch("/api/notification-preferences")).data;
}

/**
 * Save notification preferences.
 * @param {{ pushEnabled?: boolean, emailEnabled?: boolean, enabledPushTypes?: string[], defaultReminderLeadMinutes?: number }} prefs
 * @returns {Promise<object>} The updated preferences
 */
export async function saveNotificationPreferences(prefs) {
  const { response, data } = await apiFetch("/api/notification-preferences", {
    method: "PATCH",
    body: JSON.stringify(prefs),
  });
  if (!response.ok) throw new Error("Failed to save notification preferences");
  return data;
}
export async function loadNotifications(cursor = null) {
  // Ordered pages must take a fresh snapshot after a mutation, even while
  // an older GET for the same cursor is still completing.
  return (await apiFetch("/api/notifications" + (cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""), {signal:AbortSignal.timeout(20000)})).data;
}

/**
 * Mark a single notification as read.
 * @param {number} id
 */
export async function markRead(id) {
  await apiFetch(`/api/notifications/${id}/read`, { method: "POST" });
}

/**
 * Mark all notifications for the current user as read.
 */
export async function markAllRead() {
  await apiFetch("/api/notifications/read-all", { method: "POST" });
}

/**
 * Delete a single notification.
 * @param {number} id
 */
export async function deleteNotification(id) {
  await apiFetch(`/api/notifications/${id}`, { method: "DELETE" });
}

/** Remove the current user's entire notification history. */
export async function clearAllNotifications() {
  await apiFetch("/api/notifications", { method: "DELETE" });
}

export async function loadChoreReminderPrefs() {
  const { response, data } = await apiFetch("/api/chore-reminder-prefs");
  if (!response.ok) return [];
  return data.prefs || [];
}

export async function saveChoreReminderPref(choreId, pref) {
  const { response, data } = await apiFetch(`/api/chore-reminder-prefs/${choreId}`, {
    method: "PATCH",
    body: JSON.stringify(pref),
  });
  if (!response.ok) throw new Error("Failed to save chore reminder pref");
  return data.pref;
}

/**
 * Render the notification panel HTML.
 * @param {Array} notifications
 * @returns {string} HTML string
 */
export function renderNotificationPanel(notifications, state = {}) {
  const unread = state.unreadNotifications ?? notifications.filter((n) => !n.isRead).length;
  const busy = state.notificationLoading || state.notificationLoadingMore || state.notificationMutating;
  const disabled = state.notificationMutating ? ' disabled' : '';
  const items = notifications.length
    ? notifications
        .map(
          (n) => `
    <li class="notif-item${n.isRead ? ' notif-read' : ''}" data-notif-id="${n.id}">
      <button type="button" class="notif-content" data-action="mark-notif-read" data-notif-id="${n.id}" aria-label="${n.isRead ? 'Read notification' : 'Mark notification read'}"${disabled}>
        <span class="notif-title">${escapeHTML(n.title)}</span>
        <span class="notif-body">${escapeHTML(n.body)}</span>
      </button>
      <button type="button" class="notif-dismiss icon-button" data-action="dismiss-notification" data-notif-id="${n.id}" aria-label="Dismiss"${disabled}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>
    </li>`
        )
        .join("")
    : `<li class="notif-empty">No notifications</li>`;

  return `
  <div class="notif-backdrop" data-action="close-notifications"></div>
  <div class="notif-panel" id="notif-panel">
    <div class="notif-panel-handle" aria-hidden="true"></div>
    <div class="notif-panel-header">
      <span class="notif-panel-title">Notifications</span>
      <button type="button" class="notif-close icon-button" data-action="close-notifications" aria-label="Close">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>
    </div>
    <div class="notif-panel-actions" role="group" aria-label="Notification actions">
      <button type="button" class="btn btn-secondary notif-bulk-action" data-action="mark-all-read"${state.notificationMutating || unread === 0 ? ' disabled' : ''}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m4 12 5 5L20 6"/></svg>
        <span>Mark all read</span>
      </button>
      <button type="button" class="btn btn-secondary notif-bulk-action" data-action="clear-all-notifications"${state.notificationMutating || (!notifications.length && !state.notificationCursor && unread === 0) ? ' disabled' : ''}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M3 6h18M9 6V4h6v2M5 6l1 14h12l1-14M10 10v6M14 10v6"/></svg>
        <span>Clear all</span>
      </button>
    </div>
    <button type="button" class="text-button" data-action="refresh-notifications"${busy ? ' disabled' : ''}>Refresh notifications</button>
    ${state.notificationError ? `<p role="alert" class="error-text" data-testid="notification-error">${escapeHTML(state.notificationError)}</p>` : ''}
    ${state.notificationLoading ? '<p role="status">Loading notifications…</p>' : ''}
    <ul class="notif-list">${items}</ul>
    ${state.notificationCursor ? `<button type="button" class="btn btn-secondary btn-sm" data-action="more-notifications"${busy ? ' disabled' : ''}>${state.notificationLoadingMore ? 'Loading…' : state.notificationErrorAction === 'more' ? 'Retry loading older notifications' : 'Load older notifications'}</button>` : ''}
    <p class="text-secondary notif-retention">Notifications stay here until you delete them.</p>
  </div>`;
}
