# Push Notification Debugging Playbook

This documents the diagnostic process for Web Push notification delivery issues.

## Architecture overview

```
Server (Go)                    Apple gateway                iPhone
┌───────────┐    POST          ┌─────────┐    APNs         ┌─────────┐
│ Encrypt   │ ──aes128gcm──→  │ web.push│ ──delivery──→  │ Web.app │
│ + VAPID   │   ←── 201 ✅    │ .apple  │                  │ (PWA)   │
│ JWT       │                  │  .com  │                  │  SW     │
└───────────┘                  └─────────┘                  └─────────┘
```

Key gotcha: **Apple returns 201 even when encryption is broken.** There is no error signal at the HTTP level for bad encryption. The only indication that a push failed is `self.lastPush` remaining `{}` in the service worker, or the diagnostic `__diag` array staying empty.

## Step 1: Verify the server pipeline

```bash
# SSH to production and check logs
ssh deploy@ssh.yearofbingo.com
podman logs choresy_app_1 --tail 20 | grep -E 'push|notif'

# Expected output for a successful send:
# notif: sending push to user N title="..."
# push: sent to user N endpoint https://web.push.apple.com/... status=201
```

- **status=201** — VAPID auth passed, Apple accepted the push. Does NOT mean the push was delivered to the device.
- **status=403 BadJwtToken** — VAPID JWT is malformed or keys don't match. Check `vapid.go`.
- **status=404 or 410** — Stale subscription. The server auto-deletes these.

## Step 2: Verify the service worker on iOS

### 2a. Connect Web Inspector to iPhone PWA

```bash
# Install and start the adapter
npm install -g remotedebug-ios-webkit-adapter
remotedebug_ios_webkit_adapter --port 9222

# List debuggable pages
curl http://localhost:9222/json
# Look for "Choresy" (main page) and "ServiceWorker" entries
```

The pages are accessible via WebSocket:
```
ws://localhost:9222/ios_<device-udid>/ws://127.0.0.1:<port>/devtools/page/<n>
```

### 2b. Check notification permission

Connect to the main page's WebSocket and send CDP commands:

```json
{"id":1, "method":"Runtime.enable"}
{"id":2, "method":"Runtime.evaluate", "params":{"expression":"Notification.permission", "returnByValue":true}}
```

Expected: `"granted"`

### 2c. Test notification display independently

```json
{"id":2, "method":"Runtime.evaluate", "params":{
  "expression": "navigator.serviceWorker.ready.then(r => r.showNotification('Test', {body:'hello'}))",
  "returnByValue":false,
  "awaitPromise":true
}}
```

If this shows a notification on the iPhone, the notification API works. The problem is in push delivery, not display.

**Note:** `awaitPromise` does not work properly with `remotedebug-ios-webkit-adapter`. Use `Runtime.awaitPromise` as a separate step:
1. `Runtime.evaluate` with `returnByValue:false` to get a promise `objectId`
2. `Runtime.awaitPromise` with `promiseObjectId` and `returnByValue:true`

### 2d. Check push delivery diagnostics

```json
// Evaluate async code to post a MessageChannel message to the SW
{"id":2, "method":"Runtime.evaluate", "params":{
  "expression": "(async () => { const sw = navigator.serviceWorker.controller; const r = await new Promise(res => { const c = new MessageChannel(); c.port1.onmessage = e => res(e.data); sw.postMessage('push-diag', [c.port2]); setTimeout(() => res({to:true}), 3000); }); return JSON.stringify(r); })()",
  "returnByValue":false
}}
```

Then await the promise with `Runtime.awaitPromise`.

Expected response with NO push events:
```json
{"lastPush":null,"diag":[],"registration":true}
```

Expected response WITH push events:
```json
{"lastPush":{"decrypted":true,"title":"🐱 Feed Cats","body":"user logged this","time":1716567665000,"hasData":true},"diag":[{"type":"push-received","ts":1716567665000,"decrypted":true,"hasData":true}],"registration":true}
```

- **`lastPush: null, diag: []`** — No push events have ever fired on this SW instance. Either the push never arrived at the device, or the SW was restarted after receiving it.
- **`diag` contains `push-decode-error`** — The push arrived but decryption failed. The HKDF chain in `encrypt.go` is wrong.
- **`diag` contains `subscriptionchange`** — The push subscription was invalidated by the browser. The user needs to re-subscribe.

## Step 3: Check for `pushsubscriptionchange` events

The SW (v0.1.37+) records subscription invalidation events in `self.__diag`. If the subscription was changed or revoked by the browser, this event fires. Check `diag` for `type:"subscriptionchange"` entries.

## Step 4: Compare with a known-good implementation

If the server returns 201 but `self.lastPush` stays `null`:

```bash
# Use web-push npm library to send a test push with the same keys
npm install web-push
```

```js
import webpush from 'web-push';

webpush.setVapidDetails(
  'mailto:admin@yearofbingo.com',
  'BKRVx3HSWeKcIo9s7DdM-XjneLqhSBFAKxKRtyN9OoTz3pY-JXD-yFamVfMISE44UzELH5DePnd7iWEjNxCfzWc',
  'Jshc1aguHuf75oaO86Pf1C45jK5bbK7qzTLB7eYvlBw'
);

await webpush.sendNotification(subscription, JSON.stringify({title:"Test",body:"Hello"}));
```

If `web-push` delivers but our Go server doesn't → the Go encryption is broken.

## Step 5: Verify the HKDF encryption chain

The correct HKDF chain for RFC 8291 aes128gcm (matching `http_ece` npm library):

```
1. prk1 = HKDF-Extract(salt=auth_secret, IKM=DH_shared_secret)
2. secret = HKDF-Expand(prk1, "WebPush: info\0" || client_pub_key || ephemeral_pub_key, 32)
3. prk2 = HKDF-Extract(salt=random_salt, IKM=secret)
4. CEK = HKDF-Expand(prk2, "Content-Encoding: aes128gcm\0", 16)
5. nonce = HKDF-Expand(prk2, "Content-Encoding: nonce\0", 12)
```

Key points:
- The `random_salt` is the 16-byte salt included in the encrypted message header
- Steps 4 and 5 use **plain strings** (no context) as the info parameter
- The info in step 2 uses **raw** public keys (not length-prefixed)
- `Go's hkdf.Extract(hash, secret, salt)` maps to `HKDF-Extract(salt, IKM=secret)`

See `internal/push/encrypt.go:52-67` for the current implementation.

### To verify the Go encryption matches the JS library:

Write a test that encrypts with both Go and `http_ece` using the same salt, ephemeral key, and inputs, then compare the CEK and nonce values.

## VAPID JWT format

The JWT header must contain ONLY:
```json
{"typ":"JWT","alg":"ES256"}
```

Do NOT include `kty` or `crv` fields — these are JWK fields, not JWT header fields.

### Generating new VAPID keys

```bash
go run ./cmd/vapid-keygen/
```

This prints base64url-encoded P-256 key pair values ready for CI secrets:
```
VAPID_PRIVATE_KEY=<32-byte-d-value>
VAPID_PUBLIC_KEY=<65-byte-uncompressed-point>
VAPID_SUBJECT=mailto:your-email@example.com
```

After rotating keys, update the `vapid-public-key` meta tag in `web/templates/index.html` to match `VAPID_PUBLIC_KEY`. All existing push subscriptions become invalid after key rotation — users must re-subscribe.

The private key is the raw P-256 `d` value (32 bytes, base64url). The public key is the uncompressed point `0x04 || x || y` (65 bytes, base64url). Both are stored without padding.

## Service worker cache

The `/service-worker.js` endpoint must return `Cache-Control: no-store` with `cf-cache-status: BYPASS`. Without this, Cloudflare caches the SW for 4 hours, preventing updates from reaching devices.

Verify:
```bash
curl -sI https://nabu-app.com/service-worker.js | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: BYPASS
```

See `internal/app/server.go` for the SW handler.

## Quick API test

```bash
# Log a chore to trigger push notifications
curl -X POST https://nabu-app.com/api/logs \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: <csrf_token>" \
  -b "choresy_session=<session>; choresy_csrf=<csrf>" \
  -d '{"choreId": 1, "hour": '"$(date +%H)"'}'
```

Use `choreId` (camelCase), not `chore_id`.
