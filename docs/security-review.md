# Choresy Security Review

**Date:** 2026-05-30  
**Scope:** Full codebase, dependency audit, live pentest of production (https://choresy.yearofbingo.com)  
**Reviewer:** OpenCode automated security review  
**Go version in use:** go1.25 (go.mod)  
**Current app version:** v0.1.163  

---

## Executive Summary

The codebase has a solid foundation with many security controls correctly in place: parameterized SQL queries throughout, bcrypt password hashing, `crypto/rand` for all token generation, a Content Security Policy, sensible cookie settings, and CSRF protection. However, several serious vulnerabilities require immediate attention before this app can be considered production-safe.

The most urgent issues are:

1. **Critical IDOR on chore and log endpoints** — any authenticated user can read, modify, or delete another household's chores and logs by guessing sequential integer IDs.
2. **Critical: OIDC JWT signature is never verified** — a forged Google ID token would be accepted as a valid authentication.
3. **Critical: 27 Go standard library CVEs** — the app is built with Go 1.25.0 against a stdlib that has multiple known vulnerabilities including XSS in `html/template`. The fix is a single `go toolchain` upgrade.
4. **High: Stored XSS via `chore.icon`** — user-supplied icon values are rendered into the DOM without escaping in nine locations across the JS frontend.
5. **High: Account enumeration** — the registration endpoint leaks whether an email is already registered.

---

## Severity Legend

| Severity | Meaning |
|----------|---------|
| **CRITICAL** | Exploitable by any authenticated attacker, directly compromises confidentiality/integrity of other users' data |
| **HIGH** | Significant vulnerability requiring attack conditions that are realistic |
| **MEDIUM** | Notable weakness; exploitable under specific circumstances |
| **LOW** | Defense-in-depth gap or hardening opportunity |
| **INFO** | Best-practice deviation; no direct exploit path |

---

## Finding Index

| # | Severity | Title |
|---|----------|-------|
| 1 | CRITICAL | IDOR: Chore Read/Update/Delete/Restore With No Household Check |
| 2 | CRITICAL | IDOR: Log Update With No Household Check |
| 3 | CRITICAL | OIDC ID Token Signature Never Verified |
| 4 | CRITICAL | Go Stdlib CVEs (27 vulnerabilities, fix: upgrade toolchain) |
| 5 | HIGH | Stored XSS via `chore.icon` Field |
| 6 | HIGH | Account Enumeration via Registration 409 Response |
| 7 | MEDIUM | bcrypt 72-Byte Password Truncation — No Max Length Enforced |
| 8 | MEDIUM | Rate Limiting: Too Permissive, IP-Only, Fixed Window, No Account Lock |
| 9 | MEDIUM | Non-Constant-Time Comparison of CSRF Token and OIDC State |
| 10 | MEDIUM | 30-Day Session Lifetime with No Idle or Absolute Timeout |
| 11 | MEDIUM | Cross-Household Member Removal via Mismatched Active Household |
| 12 | MEDIUM | Missing `Strict-Transport-Security` (HSTS) Header |
| 13 | MEDIUM | No Server-Side Validation on Chore/Log/Household Text Fields |
| 14 | LOW | `chore.color` Injected Into `style=` Without Server-Side Validation |
| 15 | LOW | CSRF Token Double-Submit: No Server-Side Binding, No Rotation |
| 16 | LOW | Logout Clear-Cookie Missing `Secure` Flag |
| 17 | LOW | OIDC Nonce Check Silently Bypassed When Claim Is Empty |
| 18 | LOW | VAPID JWT Uses DER-Encoded Signature Instead of Raw r\|\|s |
| 19 | LOW | `RequireHousehold` Middleware Defined but Never Used |
| 20 | INFO | `escapeHTML` Duplicated Across Three JS Files |
| 21 | INFO | Session ID Returned in JSON Response Body |
| 22 | INFO | bcrypt Cost 12 — Low End of 2026 Recommendations |
| 23 | INFO | No `Retry-After` Header on Rate-Limit 429 Responses |

---

## Critical Findings

### Finding 1 — IDOR: Chore Read/Update/Delete/Restore With No Household Check

**Severity:** CRITICAL  
**Files:** `internal/handlers/chore.go:66-126, 151-165`, `internal/chore/service.go:48-117`, `internal/chore/postgres_store.go:27-85`

#### Description

Four endpoints accept a chore ID from the URL path but never verify that the chore belongs to the authenticated user's household:

| Endpoint | Handler | Store Query |
|----------|---------|-------------|
| `GET /api/chores/{id}` | `chore.go:66` | `WHERE id = $1` |
| `PATCH /api/chores/{id}` | `chore.go:82` | `WHERE id = $7` |
| `DELETE /api/chores/{id}` | `chore.go:112` | `WHERE id = $1` |
| `POST /api/chores/{id}/restore-default` | `chore.go:151` | fetches by id, no household filter |

The `PATCH` handler is especially notable — at `chore.go:83` the user object is deliberately discarded with `_, _ = middleware.CurrentUser(r.Context())` and the user's household is never consulted.

Chore IDs are sequential `int64` values. Any authenticated user (including an invited household member) can:
- Read any chore in any household
- Rename, recolor, or recategorize any chore in any household
- Delete any chore in any household (except system-predefined ones)
- Restore a default chore in any household

The `GET /api/chores` list endpoint correctly scopes by `user.HouseholdID` (`WHERE household_id = $1`), making the inconsistency clear.

#### Remediation

Add a household ownership check immediately after fetching the chore in each handler. The cleanest approach adds a `householdID` parameter to the service methods and a `household_id = $N` filter to the store queries:

```go
// handlers/chore.go — Get, Update, Delete, RestoreDefault
user, ok := middleware.CurrentUser(r.Context())
if !ok || user.HouseholdID == nil {
    writeError(w, http.StatusUnauthorized, "unauthorized")
    return
}

c, err := h.service.GetChore(r.Context(), id)
if err != nil || c.HouseholdID != *user.HouseholdID {
    writeError(w, http.StatusNotFound, "chore not found")
    return
}
```

Also add `AND household_id = $2` to the `UpdateChore` and `DeleteChore` SQL queries as defense in depth.

---

### Finding 2 — IDOR: Log Update With No Household Check

**Severity:** CRITICAL  
**Files:** `internal/handlers/log.go:145-224`, `internal/log/service.go:54-77`, `internal/log/postgres_store.go:72-82`

#### Description

`PATCH /api/logs/{id}` accepts a log ID from the URL path and allows modification of note, indicators, attributed user, timestamp, and slot hour — without ever verifying the log belongs to the authenticated user's household.

The handler (`log.go:147`) discards the user with `_ = user` except for a narrow check of whether a new `userId` target is a household member. It never fetches the existing log to compare its `household_id`.

The `DELETE /api/logs/{id}` path correctly checks ownership (`log/service.go:84-86`):
```go
if log.HouseholdID != householdID {
    return errors.New("can only undo logs in your own household")
}
```
The update path has no equivalent guard, creating an asymmetry.

Log IDs are sequential `int64` values. Any authenticated user can silently overwrite log entries belonging to any other household.

#### Remediation

Mirror the pattern from `UndoLog`. Pass `*user.HouseholdID` into `service.UpdateLog` and verify ownership:

```go
// log/service.go — UpdateLog
existing, err := s.store.GetLog(ctx, logID)
if err != nil {
    return Log{}, err
}
if existing.HouseholdID != householdID {
    return Log{}, errors.New("log does not belong to your household")
}
```

Also add `AND household_id = $N` to the `UPDATE chore_logs ... WHERE id = $8` query.

---

### Finding 3 — OIDC ID Token Signature Never Verified

**Severity:** CRITICAL  
**Files:** `internal/auth/oidc.go:112-147`

#### Description

The `verifyToken` function parses the Google ID token by:
1. Splitting on `.`
2. Base64-decoding the payload (part 1)
3. JSON-unmarshaling the claims
4. Checking `iss` and optionally `nonce`

**The JWT signature is never verified.** The code never:
- Fetches Google's JWKS (`https://www.googleapis.com/oauth2/v3/certs`)
- Validates the RS256 signature against the public key
- Checks the `aud` claim against the configured `ClientID`
- Validates the `alg` header (an `alg: none` token would be accepted)

```go
// oidc.go:112 — verifyToken
func (p *OIDCProvider) verifyToken(idToken, expectedNonce string) (OIDCIdentity, error) {
    parts := strings.Split(idToken, ".")
    // ... base64 decode parts[1] only ...
    // checks iss, optionally nonce
    // never touches parts[2] (the signature)
}
```

An attacker who can intercept or inject a crafted `id_token` anywhere in the OAuth code-exchange flow can authenticate as **any Google email address** — including `admin@yourdomain.com` if a user with that email already exists. While the `id_token` normally originates from Google's token endpoint (requiring the `client_secret`), absence of signature verification means a compromised reverse proxy, a token issued for a different OAuth app, or a future token endpoint vulnerability would all be catastrophically exploitable.

Additionally at `oidc.go:138`, the nonce check has a bypass:
```go
if claims.Nonce != "" && claims.Nonce != expectedNonce {
```
If the `nonce` claim is absent (empty string), the check is skipped entirely.

#### Remediation

Use a proper OIDC library. The standard approach for Go:

```go
// go get github.com/coreos/go-oidc/v3
import "github.com/coreos/go-oidc/v3/oidc"

provider, _ := oidc.NewProvider(ctx, "https://accounts.google.com")
verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
idToken, err := verifier.Verify(ctx, rawIDToken)
```

This handles JWKS fetching, key rotation, signature verification, `aud`/`iss`/`exp` checks, and nonce validation in one call. At minimum, add `aud` claim verification and reject tokens with missing nonce.

---

### Finding 4 — Go Stdlib CVEs (27 Active Vulnerabilities)

**Severity:** CRITICAL (several are XSS; DoS; TLS weaknesses)  
**Tool:** `govulncheck ./...`  
**Affected version:** `go1.25` (as specified in `go.mod`)  
**Fix version:** `go1.25.10` (fixes all 27 listed vulnerabilities)

#### Description

`govulncheck` found **27 vulnerabilities in the Go standard library** that are reachable from the application's call graph. The highest-severity items:

| CVE ID | Package | Description | Fixed In |
|--------|---------|-------------|----------|
| GO-2026-4982 | `html/template` | Bypass of meta content URL escaping causes XSS | go1.25.10 |
| GO-2026-4980 | `html/template` | Escaper bypass leads to XSS | go1.25.10 |
| GO-2026-4865 | `html/template` | JsBraceDepth context tracking XSS | go1.25.9 |
| GO-2026-4603 | `html/template` | URLs in meta content attribute not escaped | go1.25.8 |
| GO-2026-4870 | `crypto/tls` | Unauthenticated TLS 1.3 KeyUpdate DoS | go1.25.9 |
| GO-2026-4340 | `crypto/tls` | Handshake messages at incorrect encryption level | go1.25.6 |
| GO-2026-4337 | `crypto/tls` | Unexpected session resumption | go1.25.7 |
| GO-2026-4918 | `net/http` | HTTP/2 infinite loop on bad SETTINGS_MAX_FRAME_SIZE | go1.25.10 |
| GO-2025-4012 | `net/http` | Cookie parsing memory exhaustion | go1.25.2 |
| GO-2026-4341 | `net/url` | Memory exhaustion in query param parsing | go1.25.6 |
| GO-2026-4977/4986 | `net/mail` | Quadratic complexity in `ParseAddress` (used for email validation) | go1.25.10 |
| GO-2025-4006 | `net/mail` | Excessive CPU consumption in `ParseAddress` | go1.25.2 |

The four `html/template` XSS vulnerabilities are directly exercised via `app.renderIndex` in `internal/app/server.go:393`. Although the template currently only renders server-controlled values, the vulnerability exists in the library itself and could be triggered by future template changes.

The `net/mail` vulnerabilities (`ParseAddress`) are directly called from `internal/auth/service.go:470` during email validation — a public unauthenticated endpoint. A malicious actor could send a crafted email string that causes quadratic CPU consumption, effectively a DoS.

#### Remediation

Update the Go toolchain in `go.mod`:

```
go 1.25.10
```

Then run:
```bash
go mod tidy
make test
```

This is a drop-in compatible patch release. No API changes.

---

## High Findings

### Finding 5 — Stored XSS via `chore.icon` Field

**Severity:** HIGH  
**Files:** `web/static/js/home.js:93,104`, `web/static/js/calendar.js:182,346`, `web/static/js/schedule.js:254,311`, `web/static/js/today.js:118`, `web/static/js/chores.js:63`, `web/static/js/schedule-tab.js:106`

#### Description

The `chore.icon` field is user-supplied (via `POST /api/chores` and `PATCH /api/chores/{id}`), persisted to the database, and served back via the API. The server performs no validation on its content. The JavaScript client renders it into the DOM at nine locations using template literals assigned to `innerHTML` — without calling `escapeHTML()`:

```js
// home.js:93 — direct injection into innerHTML
`<span class="home-card-icon">${chore.icon}</span>`

// schedule.js:311 — icon mixed with escaped name
`<h2 class="sheet-title">${chore.icon} ${escapeHTML(chore.name)}</h2>`
```

A household member who can create or edit a chore can set the icon to:
```
"><img src=x onerror="fetch('/api/auth/logout',{method:'POST'})">
```
or any other XSS payload. Every other household member viewing the home, calendar, schedule, or chores tab would have this payload execute in their browser.

**Mitigation factor:** The Content Security Policy (`script-src 'self'`) blocks inline `<script>` tags and event handlers are **not** blocked by the current CSP (`style-src 'unsafe-inline'` creates a limited bypass path via CSS injection). `<img onerror>` event handlers are **not** blocked by CSP by default.

#### Remediation

**Option A (preferred):** Apply `escapeHTML()` at every render site:
```js
`<span class="home-card-icon">${escapeHTML(chore.icon)}</span>`
```

**Option B (defense-in-depth):** Add server-side validation to reject non-emoji characters. The icon field should only contain 1–4 emoji codepoints:
```go
// In handlers/chore.go Create/Update
if !isValidIcon(req.Icon) {
    writeError(w, http.StatusBadRequest, "icon must be 1-4 emoji characters")
    return
}
```

Both options should be implemented. Option A is the immediate fix; Option B prevents malicious data from entering the database in the first place.

---

### Finding 6 — Account Enumeration via Registration 409 Response

**Severity:** HIGH  
**File:** `internal/handlers/auth.go:34-35`

#### Description

`POST /api/auth/register` returns `HTTP 409 Conflict` with the message `"email already registered"` when the submitted email is already in the database:

```go
case auth.ErrDuplicateEmail:
    writeError(w, http.StatusConflict, "email already registered")
```

All other registration failures (validation errors, server errors) return different status codes. An attacker can test whether a given email has an account by observing the HTTP status code. With the rate limit of 20 req/min/IP, this allows harvesting ~1,200 email addresses per hour per IP address.

#### Remediation

Return `HTTP 200 OK` with a generic response for all registration outcomes when the email address in question should be kept confidential:

```go
// Return 200 for duplicate email — send a "you already have an account" email
// instead of leaking the information in the API response.
case auth.ErrDuplicateEmail:
    // Optionally: send email to the address saying "someone tried to register"
    writeJSON(w, http.StatusOK, map[string]string{"status": "check your email"})
    return
```

This is also the recommended UX pattern: tell the user "if this email is new, you'll receive a verification email" without revealing whether the account already exists.

---

## Medium Findings

### Finding 7 — bcrypt 72-Byte Password Truncation, No Maximum Length

**Severity:** MEDIUM  
**File:** `internal/auth/password.go:7`, `internal/auth/service.go:80`

#### Description

bcrypt silently truncates any password longer than 72 bytes. The code enforces a minimum length of 8 characters but no maximum:

```go
if len(password) < 8 {
    return ErrWeakPassword
}
// No upper bound check
```

A user who sets a password of 80+ characters would authenticate successfully using only the first 72 bytes, allowing any suffix to be appended or truncated without affecting authentication. An attacker who obtains the first 72 characters of a long password (e.g., from a breach of another service) could authenticate as that user.

#### Remediation

Add a maximum password byte length before hashing. 100 bytes is a safe and generous upper bound:

```go
if len(password) > 100 {
    return ErrPasswordTooLong
}
```

---

### Finding 8 — Rate Limiting: Too Permissive, IP-Only, Fixed Window, No Account Lockout

**Severity:** MEDIUM  
**Files:** `internal/config/config.go:45`, `internal/middleware/ratelimit.go`

#### Description

The current rate limiter has several weaknesses:

1. **20 requests/minute** per IP per path is generous for an auth endpoint. Credential stuffing attacks are effective at much lower rates.
2. **Per-path buckets** (`key = ip + "|" + path`). An attacker targeting a specific user gets 20 attempts per minute on each of `/api/auth/login`, `/api/auth/magic-link/request`, and `/api/auth/password/forgot` — effectively 60 guesses per minute against one account.
3. **No per-account rate limiting.** If a user's email is known, an attacker can make 20 login attempts per minute against that account forever without triggering any account-level protection.
4. **Fixed window algorithm.** Burst at window boundaries: 20 requests at second 59, 20 more at second 61 = 40 in 2 seconds.
5. **In-process memory only.** In a horizontally-scaled deployment each instance has its own counter, multiplying the effective limit by the number of instances.
6. **No `Retry-After` header** in 429 responses.

#### Remediation

- Lower `RATE_LIMIT_AUTH_MAX` default from 20 to 5.
- Add a per-account lockout: after N consecutive failures for a specific email (regardless of IP), delay responses or require CAPTCHA. Store this in the database (or cache) so it applies across instances.
- Switch to a sliding-window or token-bucket algorithm.
- Add `Retry-After` header to 429 responses.
- Consider adding a CAPTCHA for login after 3 failures.

---

### Finding 9 — Non-Constant-Time Comparison of CSRF Token and OIDC State

**Severity:** MEDIUM  
**Files:** `internal/middleware/csrf.go:27`, `internal/handlers/auth.go:255`, `internal/auth/oidc.go:138`

#### Description

Go's `!=` string comparison short-circuits on the first differing byte, creating a timing side-channel that can leak information about the expected secret value one byte at a time:

```go
// csrf.go:27
r.Header.Get("X-CSRF-Token") != token

// handlers/auth.go:255
state != expectedState

// oidc.go:138
claims.Nonce != expectedNonce
```

Exploiting this over the internet is difficult (high network jitter masks timing differences), but it violates a security best practice and is trivially fixable.

#### Remediation

Use `crypto/subtle.ConstantTimeCompare` for all secret value comparisons:

```go
import "crypto/subtle"

// CSRF check
headerToken := r.Header.Get("X-CSRF-Token")
if subtle.ConstantTimeCompare([]byte(headerToken), []byte(token)) != 1 {
    http.Error(w, "csrf token invalid", http.StatusForbidden)
    return
}
```

---

### Finding 10 — 30-Day Session Lifetime with No Idle or Absolute Timeout

**Severity:** MEDIUM  
**File:** `internal/handlers/auth.go:295`

#### Description

Sessions are issued with a 30-day `MaxAge` and there is no server-side idle timeout or absolute expiry. A stolen session cookie (via XSS, device theft, network sniffing on a non-Secure connection, or a browser extension compromise) remains valid for up to 30 days.

```go
// handlers/auth.go:295
MaxAge: 30 * 24 * 60 * 60, // 30 days
```

There is no mechanism for a session to expire due to inactivity, no absolute maximum lifetime regardless of activity, and no server-side session invalidation triggered by inactivity.

#### Remediation

- Add a `last_seen_at` column to the `sessions` table.
- The session middleware should update `last_seen_at` on each authenticated request.
- Sessions idle for more than 24 hours (or a configurable period) should be rejected and deleted.
- Consider reducing the absolute session TTL to 7 days.

---

### Finding 11 — Cross-Household Member Removal via Mismatched Active Household

**Severity:** MEDIUM  
**Files:** `internal/handlers/household.go:253-303`, `internal/household/service.go:200-248`

#### Description

`PATCH /api/household/members/{id}` and `DELETE /api/household/members/{id}` take a target **user ID** from the URL path. The service fetches the target user's active household and operates on it:

```go
// service.go (UpdateMemberRole, RemoveMember)
targetMembership, err := s.GetMembership(ctx, targetUserID)
// target's active household, not verified against actor's household
```

If user A (owner of household 1) calls `DELETE /api/household/members/42` and user 42's active household is household 2, the service will remove user 42 from household 2 — a household user A does not own. The actor's ownership is only verified against the actor's own active household, never cross-referenced against the target's household.

**Realistic attack scenario:** An attacker who is an owner of any household can remove users from other households if those users share the same user IDs, as long as the attacker knows the target's user ID.

#### Remediation

Add an explicit cross-household guard in `UpdateMemberRole` and `RemoveMember`:

```go
if targetMembership.HouseholdID != actorMembership.HouseholdID {
    return ErrNotAuthorized
}
```

---

### Finding 12 — Missing `Strict-Transport-Security` (HSTS) Header

**Severity:** MEDIUM  
**File:** `internal/middleware/security.go`

#### Description

The application does not send a `Strict-Transport-Security` header. Without HSTS, browsers are not instructed to always use HTTPS for future connections. This allows:
- SSL stripping attacks (e.g., via a malicious network)
- Mixed-content issues if HTTP is ever accessible

Production uses Cloudflare (confirmed via `cf-ray` and `server: cloudflare` response headers), which may inject HSTS independently, but relying on the CDN for this critical header is not a reliable control — it can be bypassed by direct server access or Cloudflare misconfiguration.

#### Remediation

Add HSTS to `SecurityHeaders()`:

```go
// Only send HSTS if the request came in over HTTPS
if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
    w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
}
```

---

### Finding 13 — No Server-Side Validation on Chore, Log, and Household Text Fields

**Severity:** MEDIUM  
**Files:** `internal/handlers/chore.go:43-57`, `internal/handlers/log.go:62-143`, `internal/handlers/household.go:54-77`

#### Description

The following user-supplied fields have client-side `maxlength` attributes but **no server-side length or content validation**:

| Field | Client Limit | Server Check | Risk |
|-------|-------------|--------------|------|
| `chore.name` | maxlength=60 | None | DB column overflow, oversized payloads |
| `chore.icon` | maxlength=4 | None | XSS (see Finding 5), arbitrary content |
| `chore.color` | Swatch picker | None | CSS injection (see Finding 14) |
| `chore.category` | Swatch picker | None | Arbitrary string |
| `chore.indicatorLabels` | maxlength=30 | None | Array length unbounded |
| `log.note` | None | None | Unbounded text |
| `household.name` | maxlength=40 | None | DB column overflow |
| `household.initials` | maxlength=4 | None | Arbitrary content |

An attacker bypassing the browser UI (e.g., via `curl`) can submit kilobyte-length strings for any of these fields. The 1 MB body size limit (`handlers/json.go:19`) provides a floor, but a 999 KB chore name is still absurd.

#### Remediation

Add server-side validation in the handler before calling the service:

```go
if len(req.Name) == 0 || len(req.Name) > 60 {
    writeError(w, http.StatusBadRequest, "name must be 1-60 characters")
    return
}
if len(req.Icon) > 16 { // generous emoji allowance
    writeError(w, http.StatusBadRequest, "icon too long")
    return
}
```

---

## Low Findings

### Finding 14 — `chore.color` Injected Into `style=` Without Server-Side Validation

**Severity:** LOW  
**Files:** `web/static/js/calendar.js:173`, `web/static/js/household.js:155` (and others)

#### Description

The `chore.color` field is rendered directly into inline `style` attributes:
```js
`style="background-color: ${chore.color}"`
```

There is no server-side validation that the value is a valid CSS hex color. A value like `red; background-image: url(//attacker.com/pixel.gif)` would inject additional CSS properties. The current CSP restricts `img-src 'self' data:`, which limits cross-origin image loads, but CSS-based data exfiltration (e.g., via font loading or attribute selectors) is possible in some configurations.

#### Remediation

Validate `color` server-side against a regex: `^#[0-9A-Fa-f]{6}$`

---

### Finding 15 — CSRF Double-Submit Cookie: No Server-Side Binding, No Token Rotation

**Severity:** LOW  
**File:** `internal/middleware/csrf.go`

#### Description

The CSRF implementation uses the double-submit cookie pattern without server-side binding. The server trusts that the `X-CSRF-Token` header value matches the `choresy_csrf` cookie value, but never ties the CSRF token to a specific session. Weaknesses:

- If a subdomain cookie injection were possible (it is not currently, since the site has no subdomains), an attacker could set a known CSRF token and forge requests.
- The CSRF token never rotates within a session, so a leaked token (e.g., via a future XSS) remains valid until the session ends.

This is a known weakness of the double-submit pattern, not unique to this implementation. The risk is low given no subdomain exposure, but a synchronized token pattern (server stores CSRF token in session) would eliminate this category.

---

### Finding 16 — Logout Clear-Cookie Missing `Secure` Flag

**Severity:** LOW  
**File:** `internal/handlers/auth.go:299-307`

#### Description

The logout handler clears the session cookie by sending a cookie with `MaxAge: -1`. However, the clear-cookie omits the `Secure` flag:

```go
// handlers/auth.go:299
http.SetCookie(w, &http.Cookie{
    Name:     h.cookieName,
    Value:    "",
    Path:     "/",
    HttpOnly: true,
    MaxAge:   -1,
    // Secure: missing!
})
```

Modern browsers will not overwrite a `Secure` cookie with a non-Secure cookie in a response arriving over HTTPS (RFC 6265bis). This means the logout may silently fail to clear the session cookie in some browsers.

#### Remediation

Add `Secure: h.secure` to the clear-cookie in `clearSessionCookie`:

```go
http.SetCookie(w, &http.Cookie{
    Name:     h.cookieName,
    Value:    "",
    Path:     "/",
    HttpOnly: true,
    SameSite: http.SameSiteLaxMode,
    Secure:   h.secure,
    MaxAge:   -1,
})
```

---

### Finding 17 — OIDC Nonce Check Silently Bypassed When Claim Is Empty

**Severity:** LOW  
**File:** `internal/auth/oidc.go:138`

#### Description

```go
if claims.Nonce != "" && claims.Nonce != expectedNonce {
    return OIDCIdentity{}, fmt.Errorf("nonce mismatch")
}
```

If the `nonce` claim is absent from the ID token (an empty string after JSON unmarshaling), the check is completely skipped. An attacker replaying a token that lacks a nonce claim would bypass this protection.

#### Remediation

Reject tokens where the nonce claim is empty:

```go
if claims.Nonce == "" || claims.Nonce != expectedNonce {
    return OIDCIdentity{}, fmt.Errorf("nonce missing or mismatch")
}
```

---

### Finding 18 — VAPID JWT Uses DER-Encoded Signature Instead of Raw r‖s

**Severity:** LOW  
**File:** `internal/push/vapid.go:139-172`

#### Description

RFC 7518 §3.4 specifies that JWT ES256 signatures must be the raw concatenation of `r || s`, each zero-padded to 32 bytes (64 bytes total). The `encodeECDSASignature` function instead produces an ASN.1 DER-encoded ECDSA signature (approximately 70-72 bytes with DER framing bytes):

```go
func encodeECDSASignature(r, s *big.Int) []byte {
    // ...produces DER SEQUENCE { INTEGER r, INTEGER s }
    return append(derHeader(0x30, len(seq)), seq...)
}
```

Push services that strictly validate VAPID JWT signatures per RFC 7518 would reject these tokens. The fact that push notifications work in practice suggests the push services being used (Chrome/Firefox/Apple) may be lenient about this, but it is non-conformant and could cause silent failures with stricter endpoints in the future.

#### Remediation

Replace `encodeECDSASignature` with raw encoding:

```go
func encodeECDSASignature(r, s *big.Int) []byte {
    rb := make([]byte, 32)
    sb := make([]byte, 32)
    r.FillBytes(rb)
    s.FillBytes(sb)
    return append(rb, sb...)
}
```

---

### Finding 19 — `RequireHousehold` Middleware Defined but Never Used

**Severity:** LOW  
**File:** `internal/middleware/auth.go:65-78`

#### Description

A `RequireHousehold` middleware wrapper is defined and would enforce that the current user has an active household before reaching a handler. However, it is never applied to any route. All endpoints that need household access perform a manual inline check (`if user.HouseholdID == nil`). This inconsistency makes it harder to audit coverage and risks future endpoints being added without the check.

#### Remediation

Apply `RequireHousehold` in `server.go` to all household-scoped route groups, or delete the middleware if the inline pattern is preferred.

---

## Informational Findings

### Finding 20 — `escapeHTML` Duplicated Across Three JS Files

**File:** `web/static/js/utils.js:1`, `web/static/js/auth.js:6`, `web/static/js/household.js:219`

All three files contain an identical implementation of `escapeHTML`. The copies in `auth.js` and `household.js` should be removed in favor of importing from `utils.js`. Risk: a future fix to the escape function applied to `utils.js` would not propagate to the copies.

---

### Finding 21 — Session ID Returned in JSON Response Body

**File:** `internal/handlers/auth.go:280-285`

After login, register, password change, reset, and magic link consumption, the raw session ID is included in the JSON response body (`"session": session.ID`). The session is also set as an `HttpOnly` cookie. Returning the raw session ID in the response body increases its exposure surface (browser developer tools, logs, JSON parsers). The SPA currently needs this value — if it doesn't, remove it from the response.

---

### Finding 22 — bcrypt Cost 12 — Low End of 2026 Recommendations

**File:** `internal/auth/password.go:5`

bcrypt cost 12 is the current minimum recommended by OWASP (2025). Hardware improvements have made cost 12 faster to brute-force than when it was originally recommended. Consider increasing to cost 13 or 14 for new password hashes (existing hashes will continue to work; the cost only affects new hashes and re-hashes on login).

---

### Finding 23 — No `Retry-After` Header on Rate-Limit 429 Responses

**File:** `internal/middleware/ratelimit.go:89`

The 429 response does not include a `Retry-After` header. This makes it harder for legitimate clients to back off appropriately, and is required by RFC 6585 §4.

---

## SQL Injection: No Vulnerabilities Found

A complete audit of all SQL queries in the following packages found **zero SQL injection vulnerabilities**:

- `internal/auth/postgres_store.go`
- `internal/household/postgres_store.go`
- `internal/chore/postgres_store.go`
- `internal/log/postgres_store.go`
- `internal/schedule/postgres_store.go`
- `internal/notification/postgres_store.go`
- `internal/push/store.go`
- `internal/userprefs/postgres_store.go`

Every user-supplied value uses parameterized placeholders (`$1`, `$2`, etc.). All `ORDER BY` and `LIMIT`/`OFFSET` clauses use hardcoded column names or parameterized values. The one case of string concatenation in `schedule/postgres_store.go` (`SELECT ` + `scheduleColumns`) uses a compile-time constant column list, not user input.

---

## Production Configuration Observations (Pentest)

The following was observed from the live site `https://choresy.yearofbingo.com`:

| Check | Result |
|-------|--------|
| TLS version | TLS 1.3 (TLS_AES_256_GCM_SHA384) — **good** |
| Certificate | Valid, ECDSA, issued via Cloudflare |
| HSTS header | **Missing** — see Finding 12 |
| CSP | Present and well-configured |
| `X-Frame-Options: DENY` | Present |
| `X-Content-Type-Options: nosniff` | Present |
| `Referrer-Policy: same-origin` | Present |
| `Permissions-Policy` | Present |
| `Cache-Control: no-store` on JS | Present (Cloudflare BYPASS confirmed) |
| Versioned JS imports | Present (`?v=0.1.163`) |
| Server header | `cloudflare` (no app version disclosed) |
| CSRF cookie on initial load | Set correctly (Secure, SameSite=Lax, no HttpOnly — correct for double-submit) |
| Unauthenticated API access | Returns 401 as expected |
| `/api/me` unauthenticated | Returns 200 with `user: null` — by design |

---

## Dependency Audit

### Go Dependencies

| Package | Version | Status |
|---------|---------|--------|
| `github.com/jackc/pgx/v5` | v5.9.0 | Latest — no known CVEs |
| `golang.org/x/crypto` | v0.50.0 | Latest — no known CVEs |
| `golang.org/x/sync` | v0.20.0 | Latest — no known CVEs |
| `golang.org/x/text` | v0.36.0 | Latest — no known CVEs |
| **Go stdlib** | **go1.25.0** | **27 CVEs — upgrade to go1.25.10** |

### JavaScript Dependencies (devDependencies only)

| Package | Version | Status |
|---------|---------|--------|
| `@playwright/test` | ^1.60.0 | Dev only — no production impact |
| `jsdom` | ^25.0.0 | Dev only — no production impact |

The frontend uses **no production JavaScript dependencies** — no npm packages are bundled into the application. All frontend code is hand-written ES modules. This is a significant positive: the entire JS supply chain risk is eliminated.

---

## Remediation Roadmap

### Immediate Priority (Before Next Deploy)

| # | Finding | Action |
|---|---------|--------|
| 1 | **Finding 4** — Go stdlib CVEs | Change `go 1.25.0` to `go 1.25.10` in `go.mod`, run `go mod tidy` |
| 2 | **Finding 1** — Chore IDOR | Add household ownership check to `Get`, `Update`, `Delete`, `RestoreDefault` handlers |
| 3 | **Finding 2** — Log IDOR | Pass `householdID` into `service.UpdateLog`, verify before update |
| 4 | **Finding 5** — Stored XSS | Apply `escapeHTML()` to `chore.icon` at all 9 render sites |

### Short-Term (Within 2 Weeks)

| # | Finding | Action |
|---|---------|--------|
| 5 | **Finding 3** — OIDC JWT | Integrate `go-oidc/v3` library, replace `verifyToken` |
| 6 | **Finding 6** — Account Enumeration | Return HTTP 200 for duplicate email registrations |
| 7 | **Finding 11** — Cross-Household Member Removal | Add `actorHouseholdID == targetHouseholdID` guard in service |
| 8 | **Finding 12** — Missing HSTS | Add `Strict-Transport-Security` header in `security.go` |
| 9 | **Finding 13** — No Server-Side Validation | Add length/content validation to chore, log, household fields |
| 10 | **Finding 16** — Logout Cookie | Add `Secure: h.secure` to the clear-cookie call |
| 11 | **Finding 17** — OIDC Nonce Bypass | Reject tokens where nonce claim is absent |

### Medium-Term (Within 1 Month)

| # | Finding | Action |
|---|---------|--------|
| 12 | **Finding 7** — bcrypt Truncation | Add `len(password) > 100` check in `validatePassword` |
| 13 | **Finding 8** — Rate Limiting | Lower default to 5/min, add per-account lockout |
| 14 | **Finding 9** — Timing Attack | Replace `!=` with `subtle.ConstantTimeCompare` for secrets |
| 15 | **Finding 10** — Session Timeout | Add `last_seen_at`, implement 24h idle timeout |
| 16 | **Finding 14** — Color CSS Injection | Add regex validation `^#[0-9A-Fa-f]{6}$` server-side |
| 17 | **Finding 18** — VAPID DER Signature | Replace DER encoding with raw r‖s (64 bytes) |
| 18 | **Finding 20** — Duplicate escapeHTML | Remove copies from `auth.js` and `household.js`, import from `utils.js` |

### Backlog

| # | Finding | Action |
|---|---------|--------|
| 19 | **Finding 15** — CSRF Binding | Consider upgrading to synchronized token pattern |
| 20 | **Finding 19** — Unused Middleware | Apply `RequireHousehold` to route groups or remove |
| 21 | **Finding 22** — bcrypt Cost | Increase cost to 13 for new hashes |
| 22 | **Finding 23** — Retry-After | Add `Retry-After` to 429 responses |

---

## Summary Risk Matrix

| Finding | Severity | Exploitable Now? |
|---------|----------|-----------------|
| IDOR: Chore endpoints | CRITICAL | Yes — any authenticated user |
| IDOR: Log update | CRITICAL | Yes — any authenticated user |
| OIDC JWT not verified | CRITICAL | Yes — requires MITM or special conditions |
| Go stdlib CVEs (27) | CRITICAL | Several exploitable via public endpoints |
| Stored XSS via chore.icon | HIGH | Yes — any household member |
| Account enumeration | HIGH | Yes — no auth required |
| bcrypt 72-byte truncation | MEDIUM | Requires knowing 72 chars of password |
| Rate limiting weaknesses | MEDIUM | Yes — credential stuffing |
| Timing attack on CSRF | MEDIUM | Difficult over internet |
| Session 30-day lifetime | MEDIUM | Requires session theft first |
| Cross-household removal | MEDIUM | Requires being owner + knowing target ID |
| Missing HSTS | MEDIUM | Requires network-level attack |
| No server-side field validation | MEDIUM | Yes — any authenticated user |
| CSS color injection | LOW | Yes — any authenticated user |
| CSRF no server-side binding | LOW | Requires subdomain cookie injection |
| Logout clear-cookie | LOW | May silently fail in some browsers |
| OIDC nonce bypass | LOW | Only affects OIDC flow |
| VAPID DER encoding | LOW | May cause push failures |
