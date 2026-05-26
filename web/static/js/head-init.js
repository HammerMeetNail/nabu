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
  function setAppH() {
    document.documentElement.style.setProperty('--app-h', window.innerHeight + 'px');
  }
  setAppH();
  window.addEventListener('resize', setAppH);
}());
