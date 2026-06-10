// head-init.js — loaded render-blocking in <head>, runs before first paint.
//
// On iOS PWA standalone mode with viewport-fit=cover, the CSS 100dvh unit
// excludes env(safe-area-inset-bottom) at cold open even though the visual
// viewport covers the full screen.  This leaves a gap between #bottom-tabs
// and the physical screen bottom until the first scroll event.
//
// window.innerHeight always returns the full visual-viewport height.  By
// writing it into --app-h before the first paint, body { height: var(--app-h) }
// uses the correct value from frame 0, eliminating the gap.
(function () {
  var lastH = 0;
  var lastScale = 1;
  var rafPending = false;

  function applyAppH(h) {
    document.documentElement.style.setProperty('--app-h', h + 'px');
  }

  function setAppH() {
    var h = window.innerHeight;
    if (h === lastH) return;
    lastH = h;
    if (rafPending) return;
    rafPending = true;
    requestAnimationFrame(function () {
      rafPending = false;
      applyAppH(lastH);
    });
  }

  // Run synchronously before first paint to avoid the cold-open gap.
  applyAppH(window.innerHeight);
  lastH = window.innerHeight;
  if (window.visualViewport) {
    lastScale = window.visualViewport.scale || 1;
  }
  requestAnimationFrame(function () {
    applyAppH(window.innerHeight);
  });
  setTimeout(function () {
    applyAppH(window.innerHeight);
  }, 150);
  window.addEventListener('resize', setAppH);
  if (window.visualViewport) {
    window.visualViewport.addEventListener('resize', function () {
      var s = window.visualViewport.scale || 1;
      // Ignore resize events triggered by pinch-zoom scale changes;
      // these fire rapidly during zoom and are not genuine layout resizes.
      if (s !== lastScale) {
        lastScale = s;
        return;
      }
      setAppH();
    });
  }
}());
