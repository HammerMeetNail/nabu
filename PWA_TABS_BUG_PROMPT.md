Fix a CSS layout bug on an iOS PWA (standalone mode, `viewport-fit=cover`, `apple-mobile-web-app-status-bar-style: black-translucent`).

**The bug**: When the PWA first opens, `#bottom-tabs` has a visible gap between itself and the bottom of the screen. If the user scrolls the page, the tabs snap down to the correct position (`bottom: 0`) and stay there. This happens on every cold open of the PWA.

**What's been tried and DIDN'T work:**

1. `position: fixed; bottom: 0;` — the original approach. Has the gap on initial render. Scroll fixes it. (WebKit bug: fixed elements don't respect `bottom: 0` until a scroll event reconciles layout viewport vs visual viewport.)

2. Adding `void tabs.offsetHeight` after removing the `hidden` attribute — no effect.

3. Setting `height: 100%` on `html` and `body`, `overflow: hidden` on `body`, making `.app-shell` scrollable — didn't fix the gap. Introduced unwanted side effects (scrolling broken, top nav hides content).

4. Using `height: 100dvh` directly on `.app-shell` instead of inheriting `100%` — didn't fix it. `100%` heights on iOS PWA resolve to the safe viewport (without safe areas), even with `viewport-fit=cover`. `100dvh` was tried to get the full-screen height but the gap persisted.

5. `position: sticky; bottom: 0;` (outside `.app-shell`, body as scroll container) — same gap-on-initial-load bug as fixed. Scroll fixes it.

6. `position: sticky; bottom: 0;` (inside `.app-shell`, `.app-shell` as scroll container with `overflow-y: auto`) — gap returned, scrolling didn't fix it this time.

7. Adding extra 6px `padding-bottom` / `height` to clear the iPhone home indicator — addressed a secondary overlap issue but not the initial gap.

8. `min-height: calc(100dvh - 64px - var(--safe-bottom) - 6px)` on `.app-shell` so total page = viewport height, preventing unwanted body scroll — addressed a secondary scrolling issue but not the initial gap.

9. `transform: translateZ(0)` / `-webkit-transform: translateZ(0)` on `#bottom-tabs` (GPU compositing layer hack) — didn't fix the initial gap.

10. `requestAnimationFrame(() => window.scrollBy(0, 0))` after unhiding tabs — didn't fix it.

**Current state of the code**: Tabs are outside `.app-shell` (direct children of `body`), using `position: sticky; bottom: 0; margin: 0 auto;`. Body has no `overflow-y: hidden` (natural document scroll). `.app-shell` has `min-height: calc(100dvh - 64px - var(--safe-bottom) - 6px)`. Tabs have `height: calc(64px + var(--safe-bottom) + 6px)` and `padding-bottom: calc(var(--safe-bottom) + 6px)`. The `transform: translateZ(0)` and `rAF` scroll hack are still in place.

**Key files**: `web/static/css/app.css` (lines 31-47 for html/body, 79-87 for .app-shell, 164-177 for #bottom-tabs), `web/templates/index.html` (tabs at line 34, outside .app-shell div), `web/static/js/app.js` (updateTopBar at line 462, unhides tabs). E2E tests: `tests/e2e/nav-tabs-position.spec.js`, `tests/e2e/three-fixes.spec.js`.
