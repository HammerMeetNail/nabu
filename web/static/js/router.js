// The address bar follows the signed-in tab, so Back, Forward, reload and a
// shared link land where the user was. "/" stays Home's alias.
const TAB_PATHS = ["/today", "/activity", "/schedule", "/settings", "/stats"];
const STATS_CHORE = /^\/stats\/chore\/([1-9]\d{0,8})$/;

// tabForPath returns the tab a path shows, or null for any other page
// (sign-in, email links, invites).
export function tabForPath(path) {
  if (path === "/" || path === "/today") return "/today";
  if (STATS_CHORE.test(path)) return "/stats";
  return TAB_PATHS.includes(path) ? path : null;
}

// routeForPath maps a path to the route that renders it: a Stats deep link
// renders Stats, and every other path is its own route.
export function routeForPath(path) {
  return STATS_CHORE.test(path) ? "/stats" : path || "/";
}

// statsSectionForPath returns the Stats section a deep link opens at:
// "/stats/chore/12" opens activity 12's section ("chore:12").
export function statsSectionForPath(path) {
  const match = STATS_CHORE.exec(path);
  return match ? `chore:${match[1]}` : null;
}

// syncTabURL points the address bar at the tab a route shows. A user's
// navigation pushes a new entry. A move the app made on its own replaces the
// current entry and keeps its state, such as an open sheet's marker. A URL
// that already shows the tab is left alone, so "/" and a Stats deep link
// survive re-renders. Returns whether the URL changed.
export function syncTabURL(route, { push = false } = {}) {
  const tab = tabForPath(route);
  if (!tab || tabForPath(location.pathname) === tab) return false;
  if (push) history.pushState({}, "", tab);
  else history.replaceState(history.state, "", tab);
  return true;
}
