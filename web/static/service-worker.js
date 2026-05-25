const CACHE_NAME = "choresy-static-v2";
const OFFLINE_URL = "/static/offline.html";
const STATIC_ASSETS = [
  "/static/css/app.css",
  "/static/css/app.css?v=latest",
  "/static/js/app.js",
  "/static/js/state.js",
  "/static/js/morph.js",
  "/static/js/api.js",
  "/static/manifest.webmanifest",
  "/static/manifest.webmanifest?v=latest",
  "/static/icons/icon.svg",
  OFFLINE_URL,
];

self.addEventListener("install", (event) => {
  event.waitUntil((async () => {
    const cache = await caches.open(CACHE_NAME);
    await cache.addAll(STATIC_ASSETS);
    await self.skipWaiting();
  })());
});

self.addEventListener("activate", (event) => {
  event.waitUntil((async () => {
    const keys = await caches.keys();
    await Promise.all(keys.filter((key) => key !== CACHE_NAME).map((key) => caches.delete(key)));
    await self.clients.claim();
  })());
});

self.addEventListener("pushsubscriptionchange", (event) => {
  const ts = Date.now();
  self.__diag = self.__diag || [];
  self.__diag.push({ type: "subscriptionchange", ts, old: !!event.oldSubscription, new: !!event.newSubscription });
});

self.addEventListener("push", (event) => {
  const ts = Date.now();
  let data = {};
  let decrypted = false;
  let hasData = !!event.data;
  try {
    if (event.data) {
      data = event.data.json();
      decrypted = true;
    }
  } catch (e) {
    self.__diag = self.__diag || [];
    self.__diag.push({ type: "push-decode-error", ts, msg: e.message });
  }
  const title = data.title || "Choresy";
  const body = data.body || "";
  const icon = "/static/icons/icon-192.png";
  self.lastPush = { decrypted, title, body, time: ts, hasData };
  self.__diag = self.__diag || [];
  self.__diag.push({ type: "push-received", ts, decrypted, hasData });
  self.__badgeCount = (self.__badgeCount || 0) + 1;
  event.waitUntil(
    Promise.all([
      self.registration.showNotification(title, {
        body: body || "(tap to open)",
        icon,
        tag: "choresy",
        requireInteraction: true,
        vibrate: [200, 100, 200],
      }),
      setBadge(self.__badgeCount),
    ])
  );
});

self.addEventListener("message", (event) => {
  if (event.data === "last-push") {
    event.ports[0].postMessage(self.lastPush || {});
  }
  if (event.data === "push-diag") {
    event.ports[0].postMessage({
      lastPush: self.lastPush || null,
      diag: self.__diag || [],
      registration: !!self.registration,
    });
  }
  if (event.data === "clear-badge") {
    self.__badgeCount = 0;
    event.waitUntil(clearBadge());
  }
});

async function setBadge(count) {
  try {
    if ("setAppBadge" in self.navigator) {
      await self.navigator.setAppBadge(count);
    }
  } catch { /* not supported */ }
}

async function clearBadge() {
  try {
    if ("clearAppBadge" in self.navigator) {
      await self.navigator.clearAppBadge();
    }
  } catch { /* not supported */ }
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(
    Promise.all([
      clearBadge(),
      self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clientList) => {
        for (const client of clientList) {
          if (client.url && client.focus) {
            return client.focus();
          }
        }
        if (self.clients.openWindow) {
          return self.clients.openWindow("/");
        }
      }),
    ])
  );
});

self.addEventListener("fetch", (event) => {
  if (event.request.method !== "GET") {
    return;
  }

  const requestURL = new URL(event.request.url);
  if (requestURL.origin !== self.location.origin) {
    return;
  }

  if (event.request.mode === "navigate") {
    event.respondWith((async () => {
      try {
        return await fetch(event.request);
      } catch {
        const cache = await caches.open(CACHE_NAME);
        return await cache.match(OFFLINE_URL) || Response.error();
      }
    })());
    return;
  }

  if (!requestURL.pathname.startsWith("/static/")) {
    return;
  }

  event.respondWith((async () => {
    const cache = await caches.open(CACHE_NAME);
    const cached = await cache.match(event.request);
    if (cached) {
      void fetch(event.request).then((response) => {
        if (response && response.ok) {
          void cache.put(event.request, response.clone());
        }
      }).catch(() => {});
      return cached;
    }
    const response = await fetch(event.request);
    if (response && response.ok) {
      await cache.put(event.request, response.clone());
    }
    return response;
  })());
});
