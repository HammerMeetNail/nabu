import { apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";

/**
 * Attempt to register for Web Push notifications if the user grants permission.
 * This is called once after login / registration.
 */
export async function maybeSubscribePush() {
  const vapidKey = window.VAPID_PUBLIC_KEY;
  if (!vapidKey || !navigator.serviceWorker || !window.PushManager) return;

  try {
    // Register the service worker. Must happen before pushManager.subscribe.
    await navigator.serviceWorker.register("/service-worker.js");
  } catch {
    // Already registered or failed silently.
  }

  try {
    const permission = await Notification.requestPermission();
    if (permission !== "granted") return;

    const reg = await navigator.serviceWorker.ready;
    const existing = await reg.pushManager.getSubscription();
    if (existing) {
      await sendSubscriptionToServer(existing);
      return;
    }

    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidKey),
    });
    await sendSubscriptionToServer(sub);
  } catch {
    // Best-effort — push is optional.
  }
}

async function sendSubscriptionToServer(sub) {
  await apiFetch("/api/push/subscribe", {
    method: "POST",
    body: JSON.stringify({ subscription: sub.toJSON() }),
  });
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
 * Fetch the current user's notifications + unread count from the server.
 * @returns {{ notifications: Array, unreadCount: number }}
 */
export async function loadNotifications() {
  const res = await fetch("/api/notifications", { credentials: "same-origin" });
  if (!res.ok) return { notifications: [], unreadCount: 0 };
  return res.json();
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

/**
 * Render the notification panel HTML.
 * @param {Array} notifications
 * @returns {string} HTML string
 */
export function renderNotificationPanel(notifications) {
  const items = notifications.length
    ? notifications
        .map(
          (n) => `
    <li class="notif-item${n.isRead ? " notif-item--read" : ""}" data-notif-id="${n.id}">
      <div class="notif-content">
        <span class="notif-title">${escapeHTML(n.title)}</span>
        <span class="notif-body">${escapeHTML(n.body)}</span>
      </div>
      <button type="button" class="notif-dismiss icon-button" data-action="dismiss-notification" data-notif-id="${n.id}" aria-label="Dismiss">
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
  <div class="notif-panel" id="notif-panel">
    <div class="notif-panel-header">
      <span class="notif-panel-title">Notifications</span>
      ${
        notifications.some((n) => !n.isRead)
          ? `<button type="button" class="notif-mark-all-read text-button" data-action="mark-all-read">Mark all read</button>`
          : ""
      }
      <button type="button" class="notif-close icon-button" data-action="close-notifications" aria-label="Close">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>
    </div>
    <ul class="notif-list">
      ${items}
    </ul>
  </div>`;
}
