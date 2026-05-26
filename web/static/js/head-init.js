// head-init.js — loaded render-blocking in <head>, runs before first paint.
//
// On iOS PWA standalone mode with viewport-fit=cover, position:fixed elements
// are composited on a GPU layer using a pre-settled compositor viewport that
// does not match the CSS layout viewport at cold open.  This produces a visible
// gap below the tab bar until the first user scroll or navigation.
//
// The fix: remove position:fixed from #bottom-tabs entirely and make it a
// natural flex sibling of .app-shell inside body { display:flex; flex-direction:column }.
// body height must be pinned to window.innerHeight (not 100dvh) because on
// iOS PWA cold open, 100dvh can exclude the safe-area inset even with
// viewport-fit=cover, whereas window.innerHeight always returns the full
// visual-viewport height.
//
// This script runs synchronously before the first paint and writes --app-h so
// the body height CSS rule has the correct value from frame 0.
(function () {
  function setAppH() {
    document.documentElement.style.setProperty('--app-h', window.innerHeight + 'px');
  }
  setAppH();
  window.addEventListener('resize', setAppH);
}());
