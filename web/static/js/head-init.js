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
// Apps added while the shell still asked for a translucent bar run under it,
// fullscreen (viewport-fit=cover) and portrait-locked, so their true height is
// the physical screen height. But iOS doesn't always open them that way: the
// same app can open under a translucent status bar one time and below an
// opaque one the next, where the screen height leaves the tab bar below the
// screen by the status bar's height (62pt on an iPhone 17 Pro). So each
// launch decides for itself.
//
// Nothing tells the two apart at first paint: innerHeight, 100dvh and every
// safe-area inset read as they do below an opaque bar until iOS delivers the
// insets, a few frames later. So the first paint uses the last launch's
// answer, and the insets decide as soon as they arrive: a top inset means the
// page is under a translucent status bar, a bottom inset alone that it's below
// an opaque one. The answer is remembered for the next launch. A second of
// frames with no inset at all (an opaque bar on an iPhone without a home
// indicator) means opaque too.
(function () {
  var docEl = document.documentElement;
  var standalone = navigator.standalone === true ||
    (window.matchMedia && window.matchMedia('(display-mode: standalone)').matches);
  var TRANSLUCENT_KEY = 'nabu-translucent-status-bar';
  // Whether the page runs under a translucent status bar: the last launch's
  // answer until this launch's insets arrive.
  var translucent = false;
  try { translucent = window.localStorage.getItem(TRANSLUCENT_KEY) === '1'; } catch (e) {}
  var insetsSeen = false;
  var lastH = 0;

  function zoomed() {
    return !!(window.visualViewport &&
      Math.abs(window.visualViewport.scale - 1) > 0.001);
  }

  // The top and bottom safe-area insets. Both read 0 until iOS delivers them.
  function insets() {
    var probe = document.createElement('div');
    probe.style.cssText = 'position:absolute;visibility:hidden;' +
      'padding-top:env(safe-area-inset-top,0px);padding-bottom:env(safe-area-inset-bottom,0px)';
    docEl.appendChild(probe);
    var style = window.getComputedStyle(probe);
    var top = parseFloat(style.paddingTop) || 0;
    var bottom = parseFloat(style.paddingBottom) || 0;
    docEl.removeChild(probe);
    return { top: top, bottom: bottom };
  }

  function remember(value) {
    if (value === translucent) return;
    translucent = value;
    try {
      if (value) window.localStorage.setItem(TRANSLUCENT_KEY, '1');
      else window.localStorage.removeItem(TRANSLUCENT_KEY);
    } catch (e) {}
  }

  function measure() {
    if (standalone) {
      var inset = insets();
      if (inset.top > 0 || inset.bottom > 0) {
        insetsSeen = true;
        remember(inset.top > 0);
      }
    }
    if (standalone && translucent && window.screen) {
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

  // In a Home Screen app, re-measure every frame for a second after anything
  // that can move the insets: iOS delivers them on a frame of their own, with
  // or without a resize event. If no inset has arrived by the end of that
  // second, the app is below an opaque status bar.
  var watching = false, restart = false, deadline = 0;
  function watch() {
    if (!standalone) return;
    restart = true;
    if (watching) return;
    watching = true;
    requestAnimationFrame(tick);
  }
  function tick(time) {
    if (restart) {
      deadline = time + 1000;
      restart = false;
    }
    setAppH();
    if (time < deadline) {
      requestAnimationFrame(tick);
      return;
    }
    watching = false;
    if (!insetsSeen) {
      remember(false);
      setAppH();
    }
  }
  function remeasure() {
    setAppH();
    watch();
  }

  // Run synchronously before first paint.
  setAppH();
  watch();
  // The browser-mode URL bar settles after load; once the frame is right
  // these re-runs change nothing.
  requestAnimationFrame(setAppH);
  setTimeout(setAppH, 150);
  setTimeout(setAppH, 1000);
  window.addEventListener('load', setAppH);
  window.addEventListener('pageshow', remeasure);
  window.addEventListener('resize', remeasure);
  window.addEventListener('orientationchange', function () {
    // Screen/viewport values lag the event on iOS; re-measure after a beat.
    setTimeout(remeasure, 50);
  });
  if (window.visualViewport) {
    window.visualViewport.addEventListener('resize', remeasure);
  }
  // Coming back from the background, iOS can have changed the status bar.
  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState === 'visible') remeasure();
  });
}());
