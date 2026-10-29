// head-init.js — loaded render-blocking in <head>, runs before first paint.
//
// Sizes the app frame via --app-h, consumed by html/body { height: var(--app-h) }.
// The bottom tab bar is a static flex item at the end of the body column, so
// its position derives entirely from this value (see #bottom-tabs in app.css).
//
// The frame is the viewport's height (innerHeight). The app shell doesn't ask
// for a translucent status bar: from iOS 26 the system blurs about 40pt below
// one, over the top bar's logo and buttons. A Home Screen app therefore opens
// below an opaque status bar, and its viewport is right from first paint.
//
// iOS fixes the status bar style when the app is added, so apps added while
// the shell still asked for a translucent bar keep it. They run under the
// status bar, fullscreen (viewport-fit=cover) and portrait-locked, so their
// true height is the physical screen height. Their viewport geometry is stale
// at cold open: innerHeight, 100dvh and the safe-area insets all read as if
// the status bar were outside the page, until they settle. So such an app is
// only recognised once a measurement sees its top safe-area inset, and it
// remembers that: every later launch uses the screen height from first paint.
(function () {
  var docEl = document.documentElement;
  var standalone = navigator.standalone === true ||
    (window.matchMedia && window.matchMedia('(display-mode: standalone)').matches);
  var TRANSLUCENT_KEY = 'nabu-translucent-status-bar';
  var translucent = false;
  try { translucent = window.localStorage.getItem(TRANSLUCENT_KEY) === '1'; } catch (e) {}
  var lastH = 0;

  function zoomed() {
    return !!(window.visualViewport &&
      Math.abs(window.visualViewport.scale - 1) > 0.001);
  }

  // The top safe-area inset. Only a page under the status bar has one.
  function topInset() {
    var probe = document.createElement('div');
    probe.style.cssText = 'position:absolute;visibility:hidden;padding-top:env(safe-area-inset-top,0px)';
    docEl.appendChild(probe);
    var inset = parseFloat(window.getComputedStyle(probe).paddingTop) || 0;
    docEl.removeChild(probe);
    return inset;
  }

  function measure() {
    if (standalone && !translucent && topInset() > 0) {
      translucent = true;
      try { window.localStorage.setItem(TRANSLUCENT_KEY, '1'); } catch (e) {}
    }
    if (translucent && window.screen) {
      // Physical screen height is ground truth for a fullscreen app. iOS keeps
      // screen.width/height portrait-fixed while other platforms swap them,
      // so pick the orientation-appropriate dimension via min/max in case
      // the manifest's portrait lock is ever ignored (e.g. iPad).
      var landscape = window.matchMedia &&
        window.matchMedia('(orientation: landscape)').matches;
      return landscape
        ? Math.min(window.screen.width, window.screen.height)
        : Math.max(window.screen.width, window.screen.height);
    }
    return window.innerHeight;
  }

  function setAppH() {
    // Ignore measurements while pinch-zoomed: innerHeight shrinks to the
    // zoomed visual viewport, and writing that into --app-h collapses the
    // body and pulls the tab bar up into the content area. A resize event
    // fires again when scale returns to 1, re-measuring correctly.
    if (zoomed()) return;
    var h = measure();
    if (!h || h === lastH) return;
    lastH = h;
    docEl.style.setProperty('--app-h', h + 'px');
  }

  // Run synchronously before first paint.
  setAppH();
  // The browser-mode URL bar, and an older install's geometry, settle after
  // load; once the frame is right these re-runs change nothing.
  requestAnimationFrame(setAppH);
  setTimeout(setAppH, 150);
  setTimeout(setAppH, 1000);
  window.addEventListener('load', setAppH);
  window.addEventListener('pageshow', setAppH);
  window.addEventListener('resize', setAppH);
  window.addEventListener('orientationchange', function () {
    // Screen/viewport values lag the event on iOS; re-measure after a beat.
    setTimeout(setAppH, 50);
  });
  if (window.visualViewport) {
    window.visualViewport.addEventListener('resize', setAppH);
  }
}());
