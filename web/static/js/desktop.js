// The wide layout (tablets in landscape, desktops). The stylesheet's media
// query of the same value lays it out: the tab bar becomes a sidebar and
// sheets open as dialogs. This module covers what only the wide layout
// renders, and the keyboard shortcuts.
export const WIDE_LAYOUT = "(min-width: 900px) and (min-height: 521px)";

let wideQuery = null;

function query() {
  if (typeof window === "undefined" || !window.matchMedia) return null;
  wideQuery ||= window.matchMedia(WIDE_LAYOUT);
  return wideQuery;
}

// wideLayout reports whether the window shows the wide layout now.
export function wideLayout() {
  return !!query()?.matches;
}

// onWideLayoutChange calls back when the window enters or leaves it.
export function onWideLayoutChange(callback) {
  query()?.addEventListener?.("change", event => callback(event.matches));
}

// placeHomeTab lists Home first in the sidebar, where the phone's tab bar
// keeps it in the middle. Moving the link instead of reordering it with CSS
// keeps the keyboard's focus order the same as the order on screen.
export function placeHomeTab(nav, wide) {
  const home = nav?.querySelector('.tab-item[data-nav="today"]');
  if (!home) return;
  const first = nav.querySelector(".tab-item");
  if (wide) {
    if (first !== home) first.before(home);
  } else {
    nav.querySelector('.tab-item[data-nav="schedule"]')?.before(home);
  }
}

// keyboardShortcut maps a key press to a shortcut: 1–9 log the Home tile in
// that place, u undoes the last log and / searches Activity. A press in a
// text field, or with Control, Command or Option, is not a shortcut.
export function keyboardShortcut(event) {
  if (event.defaultPrevented || event.repeat || event.isComposing) return null;
  if (event.ctrlKey || event.metaKey || event.altKey) return null;
  if (event.target?.closest?.('input, textarea, select, [contenteditable]:not([contenteditable="false"])')) return null;
  if (/^[1-9]$/.test(event.key)) return { action: "log", index: Number(event.key) - 1 };
  if (event.key === "u" || event.key === "U") return { action: "undo" };
  if (event.key === "/") return { action: "search" };
  return null;
}
