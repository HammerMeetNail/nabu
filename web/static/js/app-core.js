// The app's shared core: its one state object, the render hook and the guards
// that keep a late answer from changing a later identity's state. Feature
// modules import these rather than app.js, which wires them up.
import { ContextChangedError } from "./browser-context.js";
import { captureScope } from "./request-scope.js";

// app.js creates it at startup (initState); nothing replaces it after.
export let state;
export function initState(next) { state = next; }

let renderer = () => {};
// app.js registers its whole-app render.
export function setRenderer(render) { renderer = render; }
// Renders the app from state.
export function rerender() {
  const root = document.querySelector("#app");
  if (root) renderer(root);
}

// Capture ownership when a continuation is attached, before its request settles.
export function owned(callback) { return captureScope(state).guard(callback); }

export async function withCurrentContext(promise, scope) {
  const result = await promise;
  if (!scope.current()) throw new ContextChangedError();
  return result;
}
