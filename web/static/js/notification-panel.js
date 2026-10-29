// The notification panel: opening and closing it, loading pages into it, and
// marking, dismissing or clearing notifications from it.
import { state, refreshTopBar } from "./app-core.js";
import { registerActions } from "./actions.js";
import { loadNotificationPage, mutateNotification } from "./notification-data.js";
import { renderNotificationPanel, clearAppBadge } from "./notifications.js";

// Opening or closing the panel bumps this, so a clear that settles after the
// reader moved on never closes a panel they have opened since.
let panelIntent = 0;

function renderNotifPanel() {
  const container = document.querySelector("#notif-panel-container");
  if (container && !container.hidden) container.innerHTML = renderNotificationPanel(state.notifications, state);
}

export async function loadNotifData(options) {
  if (!state.user) return;
  const pending = loadNotificationPage(state, options);
  renderNotifPanel();
  await pending;
  refreshTopBar();
  renderNotifPanel();
}

export async function updateNotification(action, id) {
  const intent = panelIntent;
  const pending = mutateNotification(state, action, id);
  renderNotifPanel();
  const updated = await pending;
  refreshTopBar();
  if (updated && action === "clear" && intent === panelIntent) closeNotifPanel();
  renderNotifPanel();
}

export function closeNotifPanel() {
  panelIntent++;
  const container = document.querySelector("#notif-panel-container");
  if (container && !container.hidden) {
    container.hidden = true;
    container.innerHTML = "";
  }
}

registerActions("click", {
  "open-notifications": e => {
    e.preventDefault();
    panelIntent++;
    clearAppBadge();
    const container = document.querySelector("#notif-panel-container");
    if (container) { container.hidden = false; renderNotifPanel(); }
    void loadNotifData();
  },
  "close-notifications": e => { e.preventDefault(); closeNotifPanel(); },
  "refresh-notifications": e => { e.preventDefault(); void loadNotifData(); },
  "more-notifications": e => { e.preventDefault(); void loadNotifData({ append: true }); },
  "mark-all-read": e => { e.preventDefault(); void updateNotification("all"); },
  "clear-all-notifications": e => { e.preventDefault(); void updateNotification("clear"); },
  "dismiss-notification": (e, el) => { e.preventDefault(); void updateNotification("delete", Number(el.dataset.notifId)); },
  "mark-notif-read": (e, el) => { e.preventDefault(); void updateNotification("read", Number(el.dataset.notifId)); },
});
