# APNs (Native iOS Push) — Implementation Plan

**Status: Not built.** This document replaces the previous, inaccurate "Phase 10
— Done" claim for native push. As of `main`, native iOS push is **non-functional
end to end** and the work below is required to make it real.

## Why this is a plan and not an implementation

A full APNs pipeline cannot be completed or verified without Apple developer
credentials that are not in the repository:

- An **APNs auth key** (`.p8`), its **Key ID**, and the **Apple Team ID**.
- The app's **bundle ID** registered for the Push Notifications capability.
- A real device (the iOS simulator cannot receive remote push) to test end to end.

Shipping a push backend that signs JWTs against these unknowns and talks to
Apple's servers, without the ability to run it once, would be untested code
masquerading as a feature — exactly the "Done but non-functional" state this
plan exists to correct. Implement the steps below once the credentials are
provisioned.

## Current state (audited on `main`)

| Layer | What exists | What is missing |
|-------|-------------|-----------------|
| iOS | `APNsRegisterRequest` / `APNsUnregisterRequest` structs in `API/RequestModels.swift` | They are referenced nowhere. No `UIApplication.registerForRemoteNotifications`, no `UNUserNotificationCenter` authorization request, no `application(_:didRegisterForRemoteNotificationsWithDeviceToken:)`, no call to a mobile register endpoint. |
| Backend | Web Push (VAPID) for the PWA: `/api/push/subscribe`, `/api/push/unsubscribe`, `internal/push/*` | No `/api/mobile/apns/*` routes; no device-token store; no APNs HTTP/2 client; the reminder/notification `PushSender` only fans out to Web Push subscriptions. |

## Work breakdown

### 1. iOS client

1. Add the **Push Notifications** capability and a background mode for remote
   notifications to the app target.
2. On launch / after login, request authorization via
   `UNUserNotificationCenter.current().requestAuthorization(options:)` and, if
   granted, call `UIApplication.shared.registerForRemoteNotifications()`.
3. Implement the app/scene delegate hooks:
   - `didRegisterForRemoteNotificationsWithDeviceToken` → hex-encode the token
     and `POST /api/mobile/apns/register` with the existing
     `APNsRegisterRequest` (token / environment / bundleId / deviceName). Send
     `environment` = `"sandbox"` for debug builds, `"production"` otherwise.
   - `didFailToRegisterForRemoteNotificationsWithError` → log and surface a
     non-fatal state.
   - On logout, `POST /api/mobile/apns/unregister` with `APNsUnregisterRequest`.
4. Handle foreground/background notification presentation
   (`UNUserNotificationCenterDelegate`).
5. Tests: a contract test that the register/unregister request bodies encode to
   the snake_case JSON the backend expects (mirrors `RequestEncodingTests`), and
   a unit test for the sandbox/production environment selection. Add an
   `APNsContractTests.swift` (the matrix currently references this name but the
   file does not exist).

### 2. Backend

1. **Migration**: a `mobile_device_tokens` table keyed by `(user_id, token)`
   with `environment`, `bundle_id`, `device_name`, `created_at`, `last_seen_at`.
   Unique on `token`; cascade-delete with the user.
2. **Store**: `internal/push` (or a new `internal/apns`) gains `RegisterDevice`,
   `UnregisterDevice`, and `DevicesForUser`, with memory + Postgres
   implementations to match the existing store pattern.
3. **Routes** (auth-required, CSRF-protected, behind the rate limiters):
   - `POST /api/mobile/apns/register`
   - `POST /api/mobile/apns/unregister`
   Wire them in `internal/app/server.go` next to the existing `/api/push/*`.
4. **APNs client**: an HTTP/2 sender that builds the `:path`
   `/3/device/{token}` request to `api.push.apple.com` (or
   `api.sandbox.push.apple.com` per the stored environment), authorized with a
   **JWT** signed by the ES256 `.p8` key (cached and refreshed < 1h), with the
   `apns-topic` = bundle ID. Handle `410 Unregistered`/`BadDeviceToken` by
   pruning the stored token.
5. **Fan-out**: extend the notification `PushSender` so a reminder/notification
   delivers to both Web Push subscriptions (existing) and APNs device tokens
   (new), per user.

### 3. Config & secrets

Add (and document in `.env.example`, and inject via the deploy job like the
VAPID keys):

```
APNS_AUTH_KEY_P8=        # contents of the .p8 (or a path/secret ref)
APNS_KEY_ID=
APNS_TEAM_ID=
APNS_BUNDLE_ID=com.nabu.app
APNS_ENVIRONMENT=production   # default target when not per-device
```

When these are unset, the APNs sender must no-op gracefully (as the VAPID
signer already does) so non-push deployments keep working.

### 4. Testing

- Unit-test the JWT construction and the environment→host selection.
- Integration-test register/unregister + token pruning against the memory store.
- Manual end-to-end on a **physical device** (simulator cannot receive push):
  register, trigger a schedule reminder, confirm delivery; then logout and
  confirm unregister.

### 5. Parity bookkeeping

When this lands, flip the **APNs (native iOS)** row in
`docs/plans/client-parity.md` from **Not built** to **Built** (then **Done**
once the contract tests run in the iOS CI lane), and update the Phase 10b entry.

## Recommendation

Until the above is implemented, keep APNs marked **Not built** in the parity
matrix and do not advertise native reminders as a shipped capability — the
reminder scheduler currently reaches iOS users only if they install the PWA and
enable Web Push.
