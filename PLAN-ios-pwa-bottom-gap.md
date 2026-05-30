# iOS PWA Bottom Gap Fix

## Issue

On iPhone 17 Pro (iOS 26.5), the Choresy PWA shows a blank gap at the bottom of the screen on cold open in standalone mode. The bottom tab bar sits above the physical screen bottom instead of aligning with it.

**Symptoms:**
- Gap appears only on cold app open
- Navigating to another tab or scrolling down on the home tab causes the tabs to snap to the correct position
- Issue is PWA-only; does not reproduce in Safari browser

## Root Cause

Two compounding issues on iOS PWA cold open in standalone mode:

1. **`position: fixed; bottom: 0`** on `#bottom-tabs` — WebKit's layout viewport excludes `env(safe-area-inset-bottom)` until the first scroll/resize event, even with `viewport-fit=cover`. The fixed tab bar renders above the home indicator area, leaving a visible gap.

2. **`100dvh`** on `.app-shell` — same problem: `dvh` excludes the safe-area inset at cold open, so the shell is shorter than the visual viewport.

When the user scrolls or navigates tabs, WebKit fires a viewport resize and the values correct themselves.

## Fix

Three-part approach:

### 1. CSS: Flex layout instead of fixed positioning

Replace the `position: fixed` tab bar with a flex-column layout where the tab bar is a natural static sibling of the scrollable content area. This eliminates the WebKit layout-vs-visual-viewport reconciliation bug entirely.

**Changes to `app.css`:**
- `html` and `body` get `height: var(--app-h, 100dvh)` (JS-set custom property with dvh fallback)
- `body` gets `display: flex; flex-direction: column`
- `.app-shell` changes from `min-height: 100dvh` to `flex: 1 1 0; min-height: 0; overflow-y: auto` (fills remaining space, scrolls internally)
- `#bottom-tabs` removes `position: fixed; bottom: 0; left: 50%; transform` — becomes a plain flex item with `flex-shrink: 0`
- `#bottom-tabs` height adds extra 6px padding to clear the iPhone home indicator
- `#app` removes the `padding-bottom: calc(80px + var(--safe-bottom))` compensation (no longer needed)

### 2. JS: Render-blocking viewport height script

Create `web/static/js/head-init.js` — loaded synchronously in `<head>` before first paint. Sets `--app-h` to `window.innerHeight` (which always returns the full visual viewport height, including safe area).

Includes robust timing to handle iOS PWA cold-open instability:
- Immediate set on script parse
- `requestAnimationFrame` re-measure after first paint
- `setTimeout(setAppH, 150)` for post-stabilization
- `window.resize` listener for orientation changes
- `window.visualViewport.resize` listener (fires more reliably than window resize on iOS PWA)

### 3. HTML: Load head-init.js

Add `<script src="/static/js/head-init.js?v={{.Version}}"></script>` to `<head>` in `index.html`, before the CSS link so it runs before first paint.

## Progress

- [x] Plan documented
- [x] Create `head-init.js` with viewport height timing fixes
- [x] Update `index.html` to load `head-init.js`
- [x] Update `app.css` — flex layout, remove fixed positioning
- [x] Run tests (`make test`, `make lint`)
- [x] Rebase onto main (merged rename-cat-chores + feed-baby-volume)
- [x] Commit and push
- [x] Tag v0.1.87 and deploy
- [x] Monitor CI and verify production
- [x] Merge back into main and clean up worktree

**Deployed:** v0.1.87 — CI passed (all 11 jobs), production verified.
