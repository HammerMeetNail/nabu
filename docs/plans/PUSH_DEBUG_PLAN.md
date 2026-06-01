# Push Notification Debugging Plan

## Current Status

### What's deployed (v0.1.32 on production)
- In-app notifications: bell badge, panel, mark-all-read, dismiss ✓
- Push subscription creation + storage ✓
- Push delivery to Apple (HTTP 201 response) ✓
- Service worker with push handler + diagnostics ✓
- `self.lastPush` diagnostic object in service worker ✓

### What's NOT working
- iOS notification banner never appears on the device
- Push reaches Apple (status=201) but the device doesn't display it

### Push pipeline verification so far
```
chore logged → notification created → push encrypted → POST to Apple → 201 ✓
                                                                   ↓
                                                        device receives? (unknown)
                                                                   ↓
                                                        SW push event fires? (unknown)
                                                                   ↓
                                                        showNotification() works? (unknown)
```

### Server log evidence (production)
```
push: subscribe user 3 endpoint=https://web.push.apple.com/... ✓
push: subscribed user 3 ✓
notif: sending push to user 3 title="🛏️ Make Bed" ✓
push: sent to user 3 ... status=201 ✓
```

---

## Debugging Plan (iPhone + Mac Safari Web Inspector)

### Setup
1. Connect iPhone to Mac via USB cable
2. On iPhone: Settings → Safari → Advanced → Web Inspector = ON
3. On Mac: Safari → Settings → Advanced → "Show Develop menu in menu bar" = ON
4. On Mac: Safari → Develop → [iPhone name] → choose the Nabu PWA
   (The PWA will appear as a separate entry from regular Safari tabs)

### Step 1: Verify service worker is active and has push handler
In the Safari Web Inspector console for the PWA, check:

```javascript
// List all service workers
navigator.serviceWorker.getRegistrations().then(regs => {
  regs.forEach(r => console.log(r.active?.scriptURL, r.active?.state));
});

// Check the push handler exists in the SW
// (this requires inspecting the SW itself - see Step 3)
```

### Step 2: Verify push subscription exists
```javascript
navigator.serviceWorker.ready.then(reg => {
  return reg.pushManager.getSubscription();
}).then(sub => {
  console.log('Subscription:', sub ? sub.endpoint : 'none');
  console.log('Permission:', Notification.permission);
});
```

Expected: endpoint starting with `https://web.push.apple.com/` and permission = `granted`

### Step 3: Inspect the Service Worker directly
In Safari Develop menu, the PWA's service worker should appear as a separate
inspectable target. Select it to open a dedicated inspector.

In the SW console, check:
```javascript
// Check if the push event listener is registered
// (The push handler logs to self.lastPush)

// Check last push diagnostic
console.log(self.lastPush);
```

Expected: `{ decrypted: true, title: "🛏️ Make Bed", body: "...", time: ... }`
- If `decrypted: false` → encryption mismatch (payload decryption failed)
- If `self.lastPush` is undefined → the push event NEVER fired

### Step 4: Manually trigger a test push
From the Mac terminal (not in Safari console), use the test account to log a chore:

```bash
# On the production server:
podman-compose exec app wget -qO- --post-data='{"choreId":1,"note":"","indicators":[]}' \
  --header='Content-Type: application/json' \
  --header='X-CSRF-Token: ...' \
  http://localhost:8080/api/logs
```

Then immediately after:
1. Watch the SW console (from Step 3) for any errors
2. Check `self.lastPush` to see if the push event fired
3. Note any console errors

### Step 5: Diagnostic scenarios

#### A. If push event never fires (self.lastPush is undefined)
- Apple may be silently dropping the push
- Check: is the PWA completely closed? On iOS, push only shows when PWA is
  not in the app switcher
- Try: close PWA → wait 30 seconds → send push → wait 10 seconds → inspect SW

#### B. If push event fires but decrypted=false
- Encryption/decryption mismatch
- The `p256dh` and `auth` keys from subscription don't match server's encryption
- Try: capture the raw subscription data and compare with server-side keys
  ```javascript
  navigator.serviceWorker.ready.then(r => r.pushManager.getSubscription())
    .then(s => console.log(JSON.stringify(s.toJSON())));
  ```
  Compare endpoint/p256dh/auth with server DB:
  ```bash
  podman-compose exec app psql -c "SELECT * FROM push_subscriptions WHERE user_id=3"
  ```

#### C. If push event fires and decrypted=true but no notification
- `showNotification()` is being called but iOS isn't displaying it
- Check if any error is thrown in the SW console
- Try manually calling showNotification from SW console:
  ```javascript
  self.registration.showNotification("Test", {
    body: "Manual test",
    icon: "/static/icons/icon-192.png",
    requireInteraction: true
  });
  ```
- If THAT works, the issue is timing/scope in the push event handler

### Step 6: Verify notification settings on device
Even with all Settings → Notifications toggles on, also check:
- Settings → Screen Time → See All Activity → Notifications
- Settings → Focus → make sure no Focus mode is active
- "Scheduled Summary" could be grouping notifications

---

## Key Files Reference

| File | Purpose |
|------|---------|
| `web/static/service-worker.js` | Push event handler, self.lastPush diagnostic |
| `web/static/js/notifications.js` | maybeSubscribePush(), sendSubscriptionToServer() |
| `web/static/js/app.js` | requestNotificationPermission(), doLogin() |
| `web/templates/index.html` | VAPID meta tag |
| `internal/push/service.go` | Server-side push delivery |
| `internal/push/encrypt.go` | RFC 8291 payload encryption |
| `internal/push/vapid.go` | VAPID key signing, GenerateVAPIDKeys() |
| `internal/handlers/push.go` | Subscribe/unsubscribe HTTP handlers |
| `internal/handlers/notification.go` | Notification list/mark-read/delete |
| `internal/notification/service.go` | NotifyChoreLogged, push sender integration |
| `internal/notification/postgres_store.go` | Notification persistence |
| `migrations/012_push_subscriptions.sql` | push_subscriptions table |
| `cmd/vapid-keygen/main.go` | VAPID key generation tool |
| `compose.server.yaml` | Production env vars |
| `.github/workflows/ci.yaml` | CI deploy pipeline |

## Production Server Access

```bash
ssh ssh.yearofbingo.com   # requires cloudflared
cd /opt/nabu
podman-compose logs --tail=50 app 2>&1 | grep 'push:\|notif:'
podman-compose exec app env | grep VAPID
```
