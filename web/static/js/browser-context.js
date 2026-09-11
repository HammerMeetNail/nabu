import { deviceRecord, newKey, withBrowserLock } from "./device-store.js";

const MIRROR = "nabu-browser-identity";
let current = null;
const listeners = new Set();

export class ContextChangedError extends Error {
  constructor() { super("Account or household changed. Your saved work remains with its original account."); this.name = "ContextChangedError"; }
}

function mirrored() { try { return JSON.parse(localStorage.getItem(MIRROR) || "null"); } catch { return null; } }
export function resultIsCurrent(result) { return !!result?.identity && contextIsCurrent(result.identity); }
export function contextSnapshot() { return current ? { ...current } : null; }
export function sameOrigin(a, b) { return !!a && !!b && a.userId === b.userId && a.householdId === b.householdId; }
export function contextIsCurrent(snapshot) {
  const mirror = mirrored();
  return snapshot?.revision === current?.revision && (!mirror || mirror.revision === snapshot?.revision);
}
export function assertContext(snapshot) { if (!contextIsCurrent(snapshot)) throw new ContextChangedError(); }
export function originHeaders(origin) {
  return origin?.userId ? { "X-Nabu-User-ID": String(origin.userId), "X-Nabu-Household-ID": String(origin.householdId || 0) } : {};
}
export function onExternalIdentityChange(listener) { listeners.add(listener); return () => listeners.delete(listener); }

async function publish(record, { required = false } = {}) {
  current = record;
  let durable = false;
  try { await deviceRecord("identity", () => record); durable = true; } catch (err) { if (required) throw err; }
  try { localStorage.setItem(MIRROR, JSON.stringify(record)); } catch { /* online use can still proceed */ }
  // Close previously displayed notifications as well as gating queued ones.
  navigator.serviceWorker?.controller?.postMessage({ type: "identity-changed" });
  return durable;
}

export async function storedIdentity() {
  try { return await deviceRecord("identity"); }
  catch (err) {
    // The mirror is advisory. A failed mirror write may leave it active after
    // a durable pending logout; an unreadable authority cannot reopen data.
    const mirror = mirrored();
    if (mirror?.status === "logout-pending") return mirror;
    throw err;
  }
}

async function readSession() {
  const response = await fetch("/api/me", { credentials: "same-origin", cache: "no-store", signal: AbortSignal.timeout(15000) });
  if (!response.ok) throw new Error("Could not check your session. Please retry.");
  return (await response.json()).user || null;
}

async function adopt(user, previous, rotate = false) {
  const userId = user?.id || 0, householdId = user?.householdId || 0, role = user?.role || "";
  const reuse = !rotate && (previous?.status === "active" || previous?.status === "signed-out") && previous.userId === userId && previous.householdId === householdId && previous.role === role;
  const record = { userId, householdId, role, status: user ? "active" : "signed-out", revision: reuse ? previous.revision : newKey(),
    bindingId: reuse ? previous.bindingId : newKey() };
  await publish(record);
  return user;
}

export async function bootstrapIdentity() {
  return withBrowserLock("nabu-identity", async () => {
    const previous = await storedIdentity();
    current = previous;
    if (previous?.status === "logout-pending") return { user: null, logoutPending: true, identity:contextSnapshot() };
    let user;
    try { user = await readSession(); }
    catch (err) { await publish({ ...previous, revision:newKey(), status:"unconfirmed" }); throw err; }
    await adopt(user, previous);
    return { user, logoutPending: false, identity:contextSnapshot() };
  });
}

// Resume and invalid-session recovery share the identity lock with login,
// switching and logout. A stale 401 must never replace a newer signed-in user.
export async function checkIdentity(expected = contextSnapshot(), { control = {invalidated:false} } = {}) {
  return withBrowserLock("nabu-identity", async () => {
    let previous;
    try { previous = await storedIdentity(); }
    catch (err) {
      // Do not overwrite an authority we could not read. Fence this page until
      // storage recovers; the durable record may contain unfinished logout.
      current = { ...current, revision:newKey(), status:"unconfirmed" };
      err.identity = contextSnapshot();
      throw err;
    }
    if (expected?.revision !== previous?.revision) throw new ContextChangedError();
    if (previous?.status === "logout-pending") return { user:null, logoutPending:true, identity:contextSnapshot() };
    let invalidation;
    control.invalidate = () => {
      // Called synchronously by a current-origin 401, including while the
      // foreground GET is suspended. Fence all old requests immediately.
      invalidation ||= publish({ ...previous, revision:newKey(), status:"unconfirmed" });
    };
    if (control.invalidated) control.invalidate();
    try {
      for (;;) {
        const invalidated = control.invalidated;
        const user = await readSession();
        // A 401 that arrived during this GET requires a later canonical read.
        if (invalidated !== control.invalidated) continue;
        if (invalidation) await invalidation;
        await adopt(user, previous, invalidated);
        if (invalidated !== control.invalidated) continue;
        return { user, logoutPending:false, identity:contextSnapshot() };
      }
    } catch (err) {
      await publish({ ...previous, revision:newKey(), status:"unconfirmed" });
      err.identity = contextSnapshot();
      throw err;
    } finally { control.invalidate = null; }
  });
}

// All in-page authentication and household transitions share this lock. The
// changing revision invalidates reads before any cookie or active-household
// mutation, and the next identity is derived from the confirmed server session.
export async function changeIdentity(run) {
  const expected = contextSnapshot();
  if (expected?.status === "changing") throw new Error("An account or household change is already in progress.");
  return withBrowserLock("nabu-identity", async () => {
    const previous = await storedIdentity();
    if (expected?.revision !== previous?.revision) throw new ContextChangedError();
    if (previous?.status === "logout-pending") throw new Error("Finish signing out before signing in again.");
    await publish({ ...previous, revision: newKey(), status: "changing" });
    try {
      const result = await run(previous);
      const user = await readSession();
      await adopt(user, previous, true);
      return { ...result, user, identity:contextSnapshot() };
    } catch (err) {
      // A failed switch/auth request may have committed. Keep data hidden until
      // a fresh session check resolves the actual server state.
      try { err.user = await readSession(); await adopt(err.user, previous, true); err.confirmed = true; }
      catch { await publish({ ...previous, revision:newKey(), status:"unconfirmed" }); }
      err.identity = contextSnapshot();
      throw err;
    }
  });
}

export async function logoutIdentity(run, onPending) {
  const expected = contextSnapshot();
  return withBrowserLock("nabu-identity", async () => {
    let previous;
    try { previous = await storedIdentity() || current; }
    catch (err) {
      current = { ...expected, replacesRevision:expected?.replacesRevision ?? expected?.revision, revision:newKey(), status:"logout-pending" };
      onPending?.({durable:false});
      err.message = "Could not check saved sign-out state. Keep this page open and retry.";
      throw err;
    }
    const memoryRetry = expected?.status === "logout-pending" && expected.replacesRevision === previous?.revision;
    if (expected?.revision !== previous?.revision && !memoryRetry) throw new ContextChangedError();
    const pending = { ...previous, replacesRevision:previous?.revision, revision:newKey(), bindingId:newKey(), status:"logout-pending" };
    // Without a durable gate we cannot promise offline sign-out privacy.
    onPending?.({durable:false});
    try { await publish(pending, { required:true }); }
    catch (err) { err.message = "Could not save unfinished sign-out on this device. Keep this page open and retry."; throw err; }
    onPending?.({durable:true});
    const result = await run(previous);
    if (!result.ok) throw new Error(result.error || "Sign-out is unfinished. Please retry.");
    await adopt(null, pending, true);
    return {user:null, identity:contextSnapshot()};
  });
}

export async function clearBrowserIdentity() {
  return withBrowserLock("nabu-identity", () => publish({ userId:0, householdId:0, revision:newKey(), bindingId:newKey(), status:"signed-out" }, { required:true }));
}

if (typeof window !== "undefined") window.addEventListener("storage", event => {
  if (event.key !== MIRROR) return;
  const next = mirrored();
  if (next?.revision === current?.revision) return;
  current = next;
  for (const listener of listeners) listener(next);
});
