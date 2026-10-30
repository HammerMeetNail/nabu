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

let topBar = () => {};
// app.js registers its top-bar render.
export function setTopBar(update) { topBar = update; }
// Re-renders the top bar: the household, the profile and the notification badge.
export function refreshTopBar() { topBar(); }

let identityFlow = {};
// app.js registers how an answer that changes who is signed in becomes the
// app's identity: finish, recover, joinPendingInvite and changeHousehold below.
export function setIdentityFlow(flow) { identityFlow = flow; }
// Adopts a confirmed identity, loads its data and renders. False when a newer
// identity owns the screen.
export function finishIdentity(result, route, afterAdopt) { return identityFlow.finish(result, route, afterAdopt); }
// After an identity change fails, shows whichever identity the browser now holds.
export function recoverIdentity(error, route, afterAdopt) { return identityFlow.recover(error, route, afterAdopt); }
// Accepts an invite opened before signing in, if the user has no household.
export function joinPendingInvite() { return identityFlow.joinPendingInvite(); }
// Switches, joins or leaves a household: the answer becomes the identity.
export function changeHousehold(run, options) { return identityFlow.changeHousehold(run, options); }

let logFlow = {};
// app.js registers what follows a log written without the log sheet.
export function setLogFlow(flow) { logFlow = flow; }
// After a log the household display wrote: Home's tiles take its answer, and
// its toast offers Undo, as after the log sheet.
export function afterDirectLog(choreId, data, label) { return logFlow.afterDirectLog(choreId, data, label); }

// Capture ownership when a continuation is attached, before its request settles.
export function owned(callback) { return captureScope(state).guard(callback); }

export async function withCurrentContext(promise, scope) {
  const result = await promise;
  if (!scope.current()) throw new ContextChangedError();
  return result;
}
