// head-init.js — loaded render-blocking in <head>, runs before first paint.
//
// Sizes the app frame via --app-h, consumed by html/body { height: var(--app-h) }.
// The bottom tab bar is a static flex item at the end of the body column, so
// its position derives entirely from this value (see #bottom-tabs in app.css).
//
// In iOS standalone (home-screen) mode, WebKit's viewport geometry is stale
// at cold open: 100dvh, window.innerHeight, and the layout viewport's bottom
// edge all exclude the bottom safe-area until the first scroll gesture
// reconciles them. No viewport measurement can be trusted at first paint.
// The app is fullscreen (viewport-fit=cover, black-translucent status bar)
// and portrait-locked, so the true app height is exactly the physical screen
// height — a static device property immune to that bug.
//
// In browser mode the URL bar occupies part of the screen, so innerHeight
// (not screen height) is correct there.
(function () {
  var docEl = document.documentElement;
  var standalone = navigator.standalone === true ||
    (window.matchMedia && window.matchMedia('(display-mode: standalone)').matches);
  var lastH = 0;

  function zoomed() {
    return !!(window.visualViewport &&
      Math.abs(window.visualViewport.scale - 1) > 0.001);
  }

  function measure() {
    if (standalone && window.screen) {
      // Fullscreen app: physical screen height is ground truth. iOS keeps
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
  // Browser-mode URL bar settles after load; in standalone measure() is
  // static so these re-runs are no-ops.
  requestAnimationFrame(setAppH);
  setTimeout(setAppH, 150);
  window.addEventListener('resize', setAppH);
  window.addEventListener('orientationchange', function () {
    // Screen/viewport values lag the event on iOS; re-measure after a beat.
    setTimeout(setAppH, 50);
  });
  if (window.visualViewport) {
    window.visualViewport.addEventListener('resize', setAppH);
  }

  // Diagnostic overlay: open the app with ?vpdebug=1 to see live viewport
  // metrics, including a snapshot captured at script parse (cold open).
  // Temporary tooling for the iOS bottom-gap bug; remove once confirmed.
  if (/[?&]vpdebug=1/.test(window.location.search)) {
    var t0 = {
      innerH: window.innerHeight,
      screenH: window.screen ? window.screen.height : 0,
      vvH: window.visualViewport ? window.visualViewport.height : 0,
      appH: docEl.style.getPropertyValue('--app-h')
    };
    document.addEventListener('DOMContentLoaded', function () {
      var el = document.createElement('div');
      el.style.cssText = 'position:fixed;top:env(safe-area-inset-top,0px);' +
        'left:0;right:0;z-index:9999;background:rgba(0,0,0,0.75);color:#0f0;' +
        'font:11px/1.4 ui-monospace,monospace;padding:4px 6px;' +
        'pointer-events:none;white-space:pre;';
      document.body.appendChild(el);
      var probe = document.createElement('div');
      probe.style.cssText = 'position:absolute;visibility:hidden;pointer-events:none;';
      document.body.appendChild(probe);
      function probeH(value) {
        probe.style.height = value;
        return probe.offsetHeight;
      }
      function render() {
        var vv = window.visualViewport;
        el.textContent =
          'standalone=' + standalone +
          ' scale=' + (vv ? vv.scale.toFixed(2) : '-') + '\n' +
          't0: innerH=' + t0.innerH + ' screenH=' + t0.screenH +
          ' vvH=' + Math.round(t0.vvH) + ' appH=' + t0.appH + '\n' +
          'now: innerH=' + window.innerHeight +
          ' screen=' + window.screen.width + 'x' + window.screen.height +
          ' clientH=' + docEl.clientHeight + '\n' +
          'vvH=' + (vv ? Math.round(vv.height) : '-') +
          ' vvTop=' + (vv ? Math.round(vv.offsetTop) : '-') +
          ' appH=' + docEl.style.getPropertyValue('--app-h') + '\n' +
          'dvh=' + probeH('100dvh') + ' lvh=' + probeH('100lvh') +
          ' vh=' + probeH('100vh') +
          ' safeB=' + probeH('env(safe-area-inset-bottom,0px)');
      }
      render();
      setInterval(render, 500);
    });
  }
}());
