You are debugging a real-device iPhone PWA layout bug for the Choresy repo at `/home/dave/git/choresy`.

Current production:
- URL: `https://choresy.yearofbingo.com`
- Live version: `v0.1.75`

Bug:
- iPhone home-screen PWA only
- On cold open, there is a visible gap below the bottom tabs
- If the user scrolls the page once, the tabs snap to the true bottom and stay correct
- If the user taps another tab (for example Calendar instead of Home), the tabs also snap to the bottom and stay correct
- Closing and reopening the PWA brings the bug back
- Playwright and desktop/mobile emulation do NOT reproduce it

Important findings so far:
- This is almost certainly iOS standalone WebKit behavior, not a normal CSS bug
- Multiple speculative fixes have already been tried and deployed
- Production already uses fixed bottom tabs with safe-area filler
- Recent attempts:
  - `v0.1.72`: fixed tabs + `#bottom-tabs::after` safe-area fill
  - `v0.1.73`: versioned CSS + manifest URLs to bust stale PWA assets
  - `v0.1.74`: iOS standalone reconcile on open
  - `v0.1.75`: retry reconcile after render
- None changed the real-device behavior

Most important clue:
- A manual page scroll or switching to another tab fixes it immediately
- That means some later viewport/layout/compositor reconciliation is happening after cold open but not during cold open

What I want you to do:
1. Do NOT start by making more code changes.
2. First help me connect to the real iPhone PWA and inspect it live, similar to `PUSH_DEBUG_PLAN.md`.
3. Guide me step by step to inspect the home-screen PWA target on the phone.
4. Collect concrete evidence for the cold-open state vs the post-tab-switch fixed state.
5. Focus specifically on:
   - `window.innerHeight`
   - `window.visualViewport?.height`
   - `window.outerHeight`
   - `window.screen.height`
   - `window.scrollY`
   - `document.documentElement.clientHeight`
   - `document.body.getBoundingClientRect()`
   - `document.querySelector('#bottom-tabs').getBoundingClientRect()`
   - computed styles for `#bottom-tabs`
   - whether any ancestor is scrolling
   - whether the Home route render differs from the Calendar route render
6. Compare measurements in 3 moments:
   - cold open on Home before touching anything
   - immediately after switching to another tab, when it snaps correct
   - after switching back to Home
7. If needed, help add TEMPORARY diagnostics to the app so we can inspect those values on-device.
8. Only after gathering evidence, recommend the smallest next code change.

Use these references:
- `PUSH_DEBUG_PLAN.md` for the connection/debugging style
- `AGENTS.md` section on iOS PWA debugging
- likely relevant files:
  - `web/static/js/app.js`
  - `web/static/css/app.css`
  - `web/templates/index.html`

Current hypothesis:
- The fixed bottom bar is being laid out against the wrong viewport/compositor state on cold open in iOS standalone
- A later event triggered by scroll or tab navigation causes WebKit to reconcile it
- We need to identify exactly what changes between those states before changing code again

Please start by:
1. telling me exactly how to connect to the PWA target on the iPhone
2. giving me the first console snippets to run
3. telling me what values to capture before and after a tab switch
