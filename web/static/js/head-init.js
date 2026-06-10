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
  var rafPending = false;

  function applyAppH(h) {
    document.documentElement.style.setProperty('--app-h', h + 'px');
  }

  function setAppH() {
    var h = window.innerHeight;
    if (h === lastH) return;
    lastH = h;
    // Debounce visual-viewport resize (fires rapidly during pinch-zoom and
    // keyboard transitions).  Skipping redundant frames prevents forced layout
    // recalcs that fight the compositor and cause the bottom tab bar to jump.
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
  requestAnimationFrame(function () {
    applyAppH(window.innerHeight);
  });
  setTimeout(function () {
    applyAppH(window.innerHeight);
  }, 150);
  window.addEventListener('resize', setAppH);
  if (window.visualViewport) {
    window.visualViewport.addEventListener('resize', setAppH);
  }
}());
