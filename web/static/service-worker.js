const CACHE_NAME = "choresy-static-v1";
const OFFLINE_URL = "/static/offline.html";
const STATIC_ASSETS = [
  "/static/css/app.css",
  "/static/js/app.js",
  "/static/js/state.js",
  "/static/js/morph.js",
  "/static/js/api.js",
  "/static/manifest.webmanifest",
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

self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data?.json() || {};
  } catch {
    data = { title: "Choresy", body: event.data?.text() || "" };
  }
  const title = data.title || "Choresy";
  const body = data.body || "";
  const icon = "/static/icons/icon.svg";
  const badge = "/static/icons/icon.svg";
  event.waitUntil(
    self.registration.showNotification(title, { body, icon, badge, tag: "choresy" })
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clientList) => {
      for (const client of clientList) {
        if (client.url && client.focus) {
          return client.focus();
        }
      }
      if (self.clients.openWindow) {
        return self.clients.openWindow("/");
      }
    })
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
