# Nabu Security Review Follow-Up

**Date:** 2026-06-01  
**Scope:** Re-review of the current codebase and live production deployment at `https://nabu-app.com` after the fixes described in `docs/security-review.md`  
**Audience:** Implementation handoff for a follow-up coding agent  
**Reviewer:** OpenCode automated security review

---

## Purpose

This document is a follow-up to `docs/security-review.md`.

The earlier review identified many serious issues. This re-review confirms that most of the critical and high-severity findings from that report have been fixed in the current codebase and are live in production. This document only covers the issues that still need work, plus a few hardening items that remain open.

This is written as an implementation guide for a less capable coding agent. It is intentionally concrete: each finding includes the affected files, what to change, and what tests must be added.

---

## Executive Summary

The app is in much better shape than the previous audit state.

Verified fixed:

- chore and log IDORs
- OIDC signature verification and nonce enforcement
- Go toolchain upgrade to `go1.25.10`
- registration enumeration response leak
- HSTS in production
- secure logout cookie clearing
- session idle timeout
- most prior frontend escaping gaps

The most important remaining issues are:

1. **High: stored HTML/CSS injection through `category` and indicator label fields** due to missing server-side validation and missing escaping in several render paths.
2. **Medium: cross-household invite deletion** because invite ownership is not checked before deletion.
3. **Medium: cross-household chore metadata leak in stats time-series** because the stats service fetches chore metadata by raw `chore_id` without household scoping.
4. **Low: session ID still returned in auth JSON responses** even though the session already lives in an `HttpOnly` cookie.

---

## Production Observations

The live site currently serves the expected hardening headers and asset behavior.

Verified on `https://nabu-app.com`:

- `Content-Security-Policy` present
- `Strict-Transport-Security` present
- `X-Frame-Options: DENY` present
- `X-Content-Type-Options: nosniff` present
- `Referrer-Policy: same-origin` present
- JS assets served with `Cache-Control: no-store`
- Cloudflare returns `cf-cache-status: BYPASS` for JS
- versioned JS imports active with `?v=0.1.193`

These checks mean the previous production header regressions are fixed.

---

## Findings

### Finding 1 — High — Stored HTML/CSS Injection via `chore.category`

**Status:** Open  
**Affected files:**

- `internal/handlers/chore.go`
- `internal/chore/service.go`
- `web/static/js/today.js`
- `web/static/js/stats.js`

#### Why this is a problem

`category` is accepted from the client, stored in the database, and rendered back into HTML in multiple places.

Current problems:

- `internal/handlers/chore.go:15-28` validates `name`, `icon`, and `color`, but not `category`.
- `web/static/js/today.js:120` renders `${chore.category}` without `escapeHTML()`.
- `web/static/js/stats.js:251` and `web/static/js/stats.js:735` render `${b.category}` without `escapeHTML()`.

Because the site has a CSP with `script-src 'self'`, this is not currently an easy inline-JavaScript execution path. But it is still a stored markup/style injection bug. It can break layout, inject attacker-controlled HTML, and becomes more dangerous if CSP changes later.

#### Required fix

Apply both server-side validation and frontend escaping.

#### Implementation steps

1. Extend chore input validation in `internal/handlers/chore.go` to validate `category`.
2. Enforce the same rule on both create and update.
3. Escape `category` anywhere it is interpolated into HTML.

#### Recommended validation rule

Use a conservative text rule. Do not allow arbitrary markup.

- required max length: 30 runes
- allow empty string only if the service normalizes it to `custom`
- reject control characters

Minimal acceptable rule for this codebase:

```go
if utf8.RuneCountInString(category) > 30 {
    return http.StatusBadRequest, "category must be 30 characters or fewer"
}
if strings.ContainsAny(category, "\x00\n\r\t") {
    return http.StatusBadRequest, "category contains invalid characters"
}
```

If you choose a stricter allowlist instead, keep it compatible with current predefined categories like `feeding`, `care`, `plants`, `cleaning`, and `custom`.

#### Frontend render sites to update

- `web/static/js/today.js:120`
- `web/static/js/stats.js:251`
- `web/static/js/stats.js:735`

Each of these should use `escapeHTML(...)`.

#### Required tests

Add or update tests for:

- handler rejects overlong or invalid `category`
- UI render escapes category text in Today view
- UI render escapes category text in Stats view

#### Acceptance criteria

- posting a chore with category `<style>body{display:none}</style>` is rejected server-side or rendered harmlessly everywhere
- no raw `category` interpolation remains in JS render templates

---

### Finding 2 — High — Stored HTML/SVG Injection via `indicatorLabels` and `indicatorDefaults`

**Status:** Open  
**Affected files:**

- `internal/handlers/chore.go`
- `internal/chore/service.go`
- `web/static/js/today.js`
- `web/static/js/stats.js`

#### Why this is a problem

The chore API currently accepts arbitrary `indicatorLabels` and `indicatorDefaults` arrays with no server-side validation.

Open render gaps:

- `web/static/js/today.js:206,294,304`
- `web/static/js/stats.js:454-460,505,600-605,648-660`

These values are eventually injected into HTML or SVG text without escaping. Even if script execution is constrained by CSP, this still allows stored markup injection and can become a stronger XSS vector later.

#### Required fix

Apply both server-side validation and output escaping.

#### Implementation steps

1. Validate `indicatorLabels` and `indicatorDefaults` in `internal/handlers/chore.go`.
2. Reject defaults that are not present in labels.
3. Escape indicator-derived text anywhere it is inserted into HTML or SVG.

#### Recommended validation rule

Use a simple, explicit rule:

- max 8 labels per chore
- each label max 30 runes
- reject control characters
- every default must exactly match one of the labels

Minimal acceptable validation sketch:

```go
if len(indicatorLabels) > 8 {
    return http.StatusBadRequest, "too many indicator labels"
}
for _, label := range indicatorLabels {
    if utf8.RuneCountInString(label) == 0 || utf8.RuneCountInString(label) > 30 {
        return http.StatusBadRequest, "indicator labels must be 1-30 characters"
    }
    if strings.ContainsAny(label, "\x00\n\r\t") {
        return http.StatusBadRequest, "indicator label contains invalid characters"
    }
}
labelSet := map[string]struct{}{}
for _, label := range indicatorLabels {
    labelSet[label] = struct{}{}
}
for _, label := range indicatorDefaults {
    if _, ok := labelSet[label]; !ok {
        return http.StatusBadRequest, "indicator defaults must be a subset of indicator labels"
    }
}
```

#### Frontend render sites to update

- in `today.js`, escape the indicator text before joining into `indicatorIconsStr`
- in `stats.js`, escape any indicator label content interpolated into SVG `<text>` and `aria-label` strings

If a value is intended to display only the emoji prefix, escape that derived prefix too. Do not rely on `split(' ')[0]` as a sanitizer.

#### Required tests

- handler rejects too many indicator labels
- handler rejects an indicator default that is not in the label list
- History view escapes malicious indicator label content
- Stats SVG/text output escapes malicious indicator label content

#### Acceptance criteria

- no malicious label text can inject markup into History or Stats
- defaults cannot reference labels that do not exist

---

### Finding 3 — Medium — Cross-Household Invite Deletion

**Status:** Open  
**Affected files:**

- `internal/handlers/household.go`
- `internal/household/service.go`
- `internal/household/postgres_store.go`
- `internal/household/memory_store.go`

#### Why this is a problem

`DeleteInvite` only checks that the actor is an owner of some household. It does not verify that the invite being deleted belongs to the actor's household.

Current flow:

- `internal/handlers/household.go:149` calls `service.DeleteInvite(...)`
- `internal/household/service.go:99-107` checks owner role only
- `internal/household/postgres_store.go:297-299` deletes by raw `id`

An owner from household A can delete invites from household B by guessing invite IDs.

#### Required fix

Enforce invite household ownership in the service layer and, if practical, in the store layer too.

#### Implementation steps

Preferred approach:

1. Add a store method to fetch an invite by ID.
2. In `DeleteInvite`, load the invite and compare `invite.HouseholdID` to the actor household ID.
3. Only delete if they match.
4. Optionally change the store delete method to accept both `inviteID` and `householdID` for defense in depth.

#### Suggested service logic

```go
actorHHID, role, err := s.store.GetMembership(ctx, userID)
if err != nil {
    return err
}
if role != RoleOwner {
    return ErrNotAuthorized
}
invite, err := s.store.GetInviteByID(ctx, inviteID)
if err != nil {
    return err
}
if invite.HouseholdID != actorHHID {
    return ErrNotAuthorized
}
return s.store.DeleteInvite(ctx, inviteID)
```

#### Required tests

- service test: owner cannot delete invite from another household
- handler test: delete returns forbidden when invite belongs to another household
- postgres store test if you add a household-scoped delete query

#### Acceptance criteria

- invite deletion only works for owners of the invite's own household

---

### Finding 4 — Medium — Cross-Household Chore Metadata Leak in Stats Time-Series

**Status:** Open  
**Affected files:**

- `internal/handlers/stats.go`
- `internal/stats/service.go`
- `internal/app/server.go`

#### Why this is a problem

`GetChoreTimeSeries` fetches chore metadata by raw `choreID` before filtering logs by household:

- `internal/stats/service.go:517` calls `s.choreStore.GetChore(ctx, choreID)`

This means an authenticated user can probe chore IDs and learn another household's chore name/icon/category even though the log history remains scoped.

#### Required fix

The time-series endpoint must only operate on chores belonging to the current household.

#### Implementation steps

Use one of these approaches:

1. Preferred: extend the stats chore-store interface with a household-scoped getter, such as `GetChoreForHousehold(ctx, householdID, choreID)`.
2. Acceptable minimal fix: fetch the chore and compare `HouseholdID` before returning metadata.

Because `stats.ChoreInfo` currently omits `HouseholdID`, the adapter may need to return a richer type or expose a new method.

#### Suggested behavior

- if the chore does not belong to the current household, return not found
- the handler should map this to `404`

#### Required tests

- stats service test for cross-household chore ID rejection
- handler test for `GET /api/stats/chores/{id}/time-series` returning `404` when the chore is outside the household

#### Acceptance criteria

- probing a foreign `chore_id` reveals nothing about another household's chore metadata

---

### Finding 5 — Low — Session ID Still Returned in JSON Responses

**Status:** Open  
**Affected file:** `internal/handlers/auth.go`

#### Why this is a problem

`authResponse()` still returns:

```go
map[string]any{
    "user": user,
    "session": session.ID,
}
```

The session is already issued as an `HttpOnly` cookie, which is the correct transport. Returning it in JSON adds avoidable exposure surface in logs, browser network tools, and any code that inspects response bodies.

#### Required fix

Remove the `session` field unless there is a documented, necessary client dependency.

#### Implementation steps

1. Confirm the frontend does not use `response.session`.
2. Remove the `session` field from `authResponse()`.
3. Update tests that assert the full auth response shape.

#### Required tests

- handler auth tests updated to assert user payload only

#### Acceptance criteria

- no auth endpoint returns raw session IDs in JSON

---

## Remaining Hardening Items

These are lower priority than the findings above, but still worth tracking.

### Hardening A — Rate Limiter Still IP-Only and Per-Path

**Files:** `internal/middleware/ratelimit.go`

Current state is better than before, but still has these limitations:

- keyed by `IP|path`
- fixed-window algorithm
- in-memory only
- no per-account throttling

This is acceptable to defer if the team needs to focus on the higher-value fixes first.

### Hardening B — CSRF Token Still Not Bound to Session

**Files:** `internal/middleware/csrf.go`

The constant-time comparison fix is in place. The remaining improvement is binding the CSRF token to server-side session state instead of using only a double-submit cookie.

---

## Implementation Order

Apply fixes in this order:

1. `category` validation + escaping
2. `indicatorLabels` / `indicatorDefaults` validation + escaping
3. cross-household invite deletion fix
4. stats time-series chore ownership fix
5. remove session ID from auth JSON

---

## Required Test Plan

At minimum, the follow-up agent should add or update:

### Go tests

- `internal/handlers/chore_test.go`
  - reject malicious or overlong `category`
  - reject too many indicator labels
  - reject indicator defaults outside label set
- `internal/household/service_test.go`
  - cross-household invite deletion fails
- `internal/handlers/stats_test.go` or `internal/stats/service_test.go`
  - cross-household time-series request fails
- auth handler tests if session field is removed

### JS/unit rendering tests

- Today view escapes `category`
- Today history escapes indicator-derived text
- Stats view escapes `category`
- Stats SVG/text output escapes indicator-derived text

### E2E tests

Because the repo policy requires E2E coverage for bug fixes, add Playwright tests for the user-visible issues:

- create a chore with a malicious category and confirm it renders harmlessly
- create a chore with malicious indicator labels, log it, and confirm History/Stats render harmlessly

The invite deletion and stats metadata leak can be covered with API-driven Playwright tests if that is the easiest path.

---

## Notes For The Implementing Agent

- Prefer small fixes over large refactors.
- Do not rely on CSP as the primary fix. Escape output and validate input anyway.
- Do not add duplicate escaping helpers; use `escapeHTML()` from `web/static/js/utils.js`.
- For authorization fixes, enforce ownership in services even if the handler already checked it.
- If you add a new store method for invite lookup or household-scoped chore lookup, update both memory and Postgres stores.
- After changing JS under `web/static/`, rebuild with `make local-fresh` before running E2E.

---

## Done Definition

This follow-up is complete when:

- all five findings above are fixed
- new tests cover each regression path
- `make test` passes
- `make e2e` passes
- a manual production verification after deploy still shows CSP, HSTS, `no-store`, and versioned imports
