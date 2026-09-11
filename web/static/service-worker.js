// Bump on any release that changes shipped static assets so the new SW purges
// the previous cache on activate (the activate handler deletes caches whose key
// != CACHE_NAME). This prevents serving a stale mix of old runtime-cached,
// version-hashed JS modules alongside a newly-deployed app.
const CACHE_NAME = "nabu-static-v3";
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

self.addEventListener("pushsubscriptionchange", (event) => {
  const ts = Date.now();
  self.__diag = self.__diag || [];
  self.__diag.push({ type: "subscriptionchange", ts, old: !!event.oldSubscription, new: !!event.newSubscription });
});

function deviceState() {
  return new Promise((resolve, reject) => {
    const open = indexedDB.open("nabu-device", 1);
    open.onupgradeneeded = () => open.result.createObjectStore("state");
    open.onerror = () => reject(open.error);
    open.onblocked = () => reject(new Error("storage busy"));
    open.onsuccess = () => {
      const db = open.result;
      const tx = db.transaction("state", "readonly");
      const store = tx.objectStore("state");
      const req = store.get("identity");
      let result;
      req.onsuccess = () => { result=req.result; };
      tx.oncomplete = () => { db.close(); resolve(result); };
      tx.onabort = tx.onerror = () => { db.close(); reject(tx.error); };
    };
  });
}

async function withIdentityLock(run) {
  if (self.navigator?.locks?.request) return self.navigator.locks.request("nabu-identity", run);
  // Fail closed: an expiring lease cannot stop a suspended former owner.
}

async function permitsNotification(data) {
  try {
    const identity = await deviceState();
    if (!identity || identity.status !== "active" || !data.bindingId ||
        identity.bindingId !== data.bindingId || identity.userId !== data.userId ||
        !data.householdId || identity.householdId !== data.householdId) return false;
    const response = await fetch("/api/push/identity", {
      credentials:"include", cache:"no-store", signal:AbortSignal.timeout(8000),
      headers:{ "X-Nabu-Push-Binding":data.bindingId, "X-Nabu-User-ID":String(data.userId),
        "X-Nabu-Household-ID":String(data.householdId), "X-Nabu-Chore-ID":String(data.choreId || 0) },
    });
    return response.ok;
  } catch { return false; }
}

async function closeNotifications() {
  const identity = await deviceState().catch(() => null);
  const shown = await self.registration.getNotifications();
  for (const notification of shown) {
    if (identity?.status !== "active" || notification.data?.bindingId !== identity.bindingId) notification.close();
  }
  self.__badgeCount=0;
  await clearBadge();
}

self.addEventListener("push", event => {
  event.waitUntil(withIdentityLock(async () => {
    let data;
    try { data=event.data?.json(); } catch { return; }
    if (!data || !await permitsNotification(data)) return;
    const options = {
      body:data.body || "Tap to open", icon:"/static/icons/icon-192.png", tag:"nabu",
      data:{ userId:data.userId, householdId:data.householdId, bindingId:data.bindingId,
        choreId:data.choreId || null, type:data.type || null },
    };
    if (data.type === "schedule_reminder" && data.choreId) options.actions=[
      {action:"log-now",title:"✓ Log now"}, {action:"snooze",title:"⏰ Snooze 30m"},
    ];
    await self.registration.showNotification(data.title || "Nabu", options);
    self.lastPush={decrypted:true, time:Date.now(), hasData:true};
    self.__badgeCount=(self.__badgeCount || 0)+1;
    await setBadge(self.__badgeCount);
  }).catch(() => {}));
});

self.addEventListener("message", (event) => {
  if (event.data?.type === "identity-changed") {
    self.lastPush=null; self.__diag=[];
    event.waitUntil(withIdentityLock(closeNotifications).catch(() => {}));
  }
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
  const ua = self.navigator && self.navigator.userAgent ? self.navigator.userAgent : "";
  if (/HeadlessChrome|HeadlessShell/i.test(ua)) {
    return;
  }
  try {
    if ("setAppBadge" in self.navigator) {
      await self.navigator.setAppBadge(count);
    }
  } catch { /* not supported */ }
}

async function clearBadge() {
  const ua = self.navigator && self.navigator.userAgent ? self.navigator.userAgent : "";
  if (/HeadlessChrome|HeadlessShell/i.test(ua)) {
    return;
  }
  try {
    if ("clearAppBadge" in self.navigator) {
      await self.navigator.clearAppBadge();
    }
  } catch { /* not supported */ }
}

self.addEventListener("notificationclick", event => {
  event.notification.close();
  const data=event.notification.data || {};
  event.waitUntil(withIdentityLock(async () => {
    if (!await permitsNotification(data)) return;
    if (event.action === "snooze" && data.choreId) {
      await fetch("/api/reminders/snooze", {
        method:"POST", credentials:"include", signal:AbortSignal.timeout(8000),
        headers:{"Content-Type":"application/json", "X-Nabu-User-ID":String(data.userId), "X-Nabu-Household-ID":String(data.householdId)},
        body:JSON.stringify({choreId:data.choreId,minutes:30}),
      });
      return;
    }
    const wantsLog=event.action === "log-now" && data.choreId;
    const target=wantsLog ? `/?quicklog=chore:${data.choreId}&pushUser=${data.userId}&pushHousehold=${data.householdId}` : "/";
    await clearBadge();
    const clients=await self.clients.matchAll({type:"window",includeUncontrolled:true});
    for (const client of clients) if (client.focus) {
      if (wantsLog) client.postMessage({type:"quicklog",choreId:data.choreId,userId:data.userId,householdId:data.householdId});
      return client.focus();
    }
    return self.clients.openWindow?.(target);
  }).catch(() => {}));
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
