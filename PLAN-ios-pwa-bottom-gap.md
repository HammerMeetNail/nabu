# iOS PWA Bottom Gap Fix

## Symptom

On iPhone 17 Pro (iOS 26.5.1) in standalone/home-screen mode, `#bottom-tabs`
hovers slightly above the physical screen bottom on cold open, showing a gap.
Swiping down on the tab bar makes it snap to the bottom and stay there. Not
reproducible in Safari browser mode (the URL bar occupies that space, masking
the bug). This has never been correctly fixed — prior "fixed and verified"
claims in this file's history were wrong; do not trust them.

## Requirement

Tabs always locked to the physical screen bottom with a scrollable body above
them. The document itself must not scroll.

## Root cause

Two layers, and any fix must not depend on either:

1. **`position: fixed; bottom: 0`** on `#bottom-tabs` (reintroduced by commit
   `4906d02` for pinch-zoom stability). A fixed element is positioned against
   WebKit's **layout viewport**, not against `body`. On iOS standalone cold
   launch, the layout viewport's bottom edge sits above the physical bottom
   (safe-area/home-indicator band excluded) until the first scroll gesture
   forces WebKit to reconcile. `--app-h`/body height cannot influence a fixed
   element; the in-code comment claiming otherwise is wrong.

2. **JS viewport measurement is not trustworthy at cold open either.** The
   existing `head-init.js` sets `--app-h` from `window.innerHeight`. If
   WebKit's window geometry is stale at cold launch (reported short, no
   resize event until a gesture), the flex-layout variant of this fix
   produces the *same* gap — which is consistent with the user's report that
   the earlier flex-based fix (v0.1.87) never actually cured it on device.
   Both `100dvh` and `innerHeight` read the same stale geometry.

## Why ../yearofbingo doesn't show the bug (comparison, 2026-07-01)

Verified against `/Users/dave/git/yearofbingo/web`:

- **Nothing is pinned to the viewport bottom.** No bottom bar, no
  `bottom: 0` fixed UI. The stale layout-viewport bottom edge has no visible
  element attached to it, so there is nothing to hover or snap.
- **The document scrolls normally.** `body { min-height: 100vh }`, `.page`
  is a flex column with `min-height`, content flows past the fold. Nothing
  needs to know the exact viewport height (`min-height`, never `height`),
  and the first natural scroll reconciles WebKit anyway.
- **No `viewport-fit=cover`, no safe-area usage, no web-app manifest, no
  `apple-mobile-web-app` metas.** The page never opts into underlapping the
  home indicator, so there is no safe-area geometry to get wrong.
- The root background paints the entire canvas, so even a short viewport
  would show no visible band.

Nabu cannot copy this wholesale (the requirement is a locked bottom bar in a
fullscreen standalone app), but the transferable principle is: **nothing in
the layout may depend on WebKit's bottom viewport edge — neither via
`position: fixed; bottom: 0` nor via JS/CSS units that read the same stale
geometry.**

## Fix

### 1. Layout: static flex tab bar (app.css)

Keeps tabs locked at bottom with the body scrolling above, per requirement,
without touching the layout viewport's bottom edge:

- `#bottom-tabs`: remove `position: fixed; bottom: 0; left: 0; right: 0`;
  restore `flex-shrink: 0` as a static flex sibling below `.app-shell`.
  Comment must state why `fixed` is forbidden here.
- `.app-shell`: keep `flex: 1 1 0; min-height: 0; overflow-y: auto`; remove
  `padding-bottom: calc(64px + var(--safe-bottom) + 6px)` (base rule) and the
  `calc(44px + …)` variant in the landscape media query.
- `body`: stays `display: flex; flex-direction: column; height: var(--app-h)`.

### 2. Sizing: stop trusting viewport measurements in standalone (head-init.js)

Nabu is portrait-locked (`"orientation": "portrait"` in the manifest) and
fullscreen (`viewport-fit=cover` + `black-translucent`), so in standalone
mode the app's true height is exactly **`screen.height`** — a static device
property that does not depend on WebKit's cold-open viewport state at all.

- Detect standalone: `navigator.standalone === true ||
  matchMedia('(display-mode: standalone)').matches`.
- Standalone: `--app-h = screen.height + 'px'` (recompute on
  orientationchange as a safety net, using the portrait/landscape-appropriate
  dimension in case orientation lock is ever ignored, e.g. iPad).
- Browser mode: keep `window.innerHeight` (URL bar means innerHeight is
  correct and `screen.height` would be too tall).
- Zoom guard: `setAppH()` early-returns whenever
  `visualViewport.scale !== 1` (epsilon), covering the currently-unguarded
  `window.resize` path too; re-measure once when scale returns to 1. This
  preserves the pinch-zoom fix that motivated `4906d02` — the jump was
  `--app-h` shrinking to the zoomed `innerHeight`, not anything that needed
  `position: fixed`.

### 3. Instrumentation before/with the fix (temporary)

Multiple blind fixes have failed; capture ground truth from the device this
time. Add a debug overlay gated behind `?vpdebug=1` (small fixed-top box,
plain text) showing, live: `innerHeight`, `screen.height`,
`visualViewport.height` + `offsetTop`, `documentElement.clientHeight`,
computed `--app-h`, and probes for `100dvh`/`100lvh`/
`env(safe-area-inset-bottom)`. Cold-open the PWA with it once before and once
after the fix and screenshot. If the fix somehow still fails, this tells us
exactly which metric is stale instead of guessing. Remove after confirmation.

### 4. Optional follow-up: overlays anchored to body, not the viewport

FABs/toasts use `position: fixed; bottom: calc(64px + …)` and anchor to the
same unreliable layout viewport. Since `body` is height-locked and
non-scrolling after this fix, switching them to `position: absolute` (with
`body { position: relative }`) gives identical visuals with correct geometry
sourced from `--app-h`. They only appear post-interaction (viewport already
reconciled), so this is cleanup, not part of the bug fix.

## Verification

- `make test` and `make lint`.
- On-device (home-screen install; re-adding the icon not required):
  1. Kill the app from the app switcher, relaunch → tab bar flush with the
     screen bottom at first paint, no swipe needed. Repeat several times
     (cold-open behavior is timing-sensitive).
  2. Pinch-zoom in/out on a content page → tab bar must not jump into the
     content area (regression check for `4906d02`).
  3. Rotate to landscape and back → compact bar stays flush.
  4. Safari browser mode sanity check.
  5. Open with `?vpdebug=1` and screenshot the metrics for the record.

## Fallback (if the static bar still gaps and debug data shows why)

If `screen.height` is somehow wrong on some device, the debug overlay will
show which metric is trustworthy at cold open; switch `--app-h`'s source to
that. Do **not** return to `position: fixed; bottom: 0` under any analysis —
that anchor is the one geometry confirmed stale.

## Progress

- [x] Diagnosis + yearofbingo comparison documented
- [x] CSS: static flex tab bar, padding compensation removed
- [x] head-init.js: screen.height in standalone, hardened zoom guard
- [x] Debug overlay (`?vpdebug=1`, in head-init.js)
- [x] Tests + lint (Go unit, 39 JS unit, 272/275 e2e — 3 failures are
      Mailpit-dependent email specs, environmental; `three-fixes.spec.js`
      updated to assert the static layout instead of position:fixed)
- [ ] On-device cold-open + pinch-zoom verification (screenshots)
- [ ] Remove debug overlay, deploy

## History

- **v0.1.87** shipped a flex-layout fix driven by `--app-h =
  window.innerHeight` and recorded itself as "production verified". Per the
  user, the bug was never actually fixed on device — consistent with
  `innerHeight` being stale at standalone cold open (root cause #2).
- **Commit `4906d02`** (2026-06-09) reintroduced `position: fixed; bottom: 0`
  on `#bottom-tabs` for pinch-zoom stability (root cause #1). The zoom guard
  added in the same commit is kept; the positioning change is reverted.
