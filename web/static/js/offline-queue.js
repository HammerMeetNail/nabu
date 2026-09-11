// A write-ahead journal: immutable payloads belong to the account and household
// that created them. A row is removed only after acceptance or deliberate discard.
import { assertContext, contextSnapshot, contextIsCurrent, sameOrigin } from "./browser-context.js";
import { withBrowserLock } from "./device-store.js";

const DB_NAME = "nabu-offline", STORE = "logQueue";
function changed() { globalThis.window?.dispatchEvent(new CustomEvent("nabu-journal-change")); }
function requireOrigin(origin) {
  if (origin?.status !== "active" || !origin.userId || !origin.householdId) throw new Error("Sign in to the original household to save this log.");
  assertContext(origin);
}
function owns(entry, origin) { return sameOrigin({ userId:entry.actorId, householdId:entry.householdId }, origin); }
function openDB() {
  return new Promise((resolve, reject) => {
    if (!globalThis.indexedDB) { reject(new Error("Saved-work storage is unavailable")); return; }
    const request = indexedDB.open(DB_NAME, 2);
    request.onupgradeneeded = () => {
      // Old rows have no trustworthy owner. Retain them without associating
      // them with whichever account next opens this browser.
      if (!request.result.objectStoreNames.contains(STORE)) request.result.createObjectStore(STORE, { keyPath:"idempotencyKey" });
    };
    request.onerror = () => reject(request.error);
    request.onblocked = () => reject(new Error("Saved-work storage is busy. Close older Nabu tabs and retry."));
    request.onsuccess = () => {
      request.result.onversionchange = () => request.result.close();
      resolve(request.result);
    };
  });
}
async function transaction(write, run) {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, write ? "readwrite" : "readonly");
    let result, error;
    const finish = value => { result = value; };
    const fail = err => { error = err; tx.abort(); };
    try { run(tx.objectStore(STORE), finish, fail); } catch (err) { fail(err); }
    tx.oncomplete = () => { db.close(); resolve(result); };
    tx.onabort = tx.onerror = () => { db.close(); reject(error || tx.error || new Error("Could not save work on this device")); };
  });
}
async function updateEntry(key, update) {
  const result = await transaction(true, (store, finish, fail) => {
    const request = store.get(key);
    request.onsuccess = () => {
      try {
        const next = update(request.result || null);
        if (next) store.put(next); else store.delete(key);
        finish(next);
      } catch (err) { fail(err); }
    };
  });
  changed();
  return result;
}
export async function enqueueLog(body, origin = contextSnapshot()) {
  requireOrigin(origin);
  if (!body?.idempotencyKey) throw new Error("Missing submission key");
  const frozen = JSON.parse(JSON.stringify(body));
  return updateEntry(body.idempotencyKey, previous => {
    if (previous) {
      if (!owns(previous, origin) || JSON.stringify(previous.body) !== JSON.stringify(frozen)) throw new Error("This saved submission has a different payload or owner.");
      return previous;
    }
    return { idempotencyKey:body.idempotencyKey, actorId:origin.userId, householdId:origin.householdId,
      body:frozen, createdAt:Date.now(), status:"submitting", error:"" };
  });
}
export async function queuedLogs(origin = contextSnapshot()) {
  requireOrigin(origin);
  const entries = await transaction(false, (store, finish) => {
    const request = store.getAll(); request.onsuccess = () => finish(request.result || []);
  });
  assertContext(origin);
  return entries.filter(entry => owns(entry, origin) && entry.body).sort((a,b) => b.createdAt-a.createdAt);
}
export async function queuedCount(origin = contextSnapshot()) { return (await queuedLogs(origin)).length; }

export class SaveError extends Error {
  constructor(message, status = 0, durable = false) {
    super(message); this.name = "SaveError"; this.status = status; this.durable = durable;
  }
}
function errorText(status) {
  if (status === 401) return "Sign in again to sync this saved log.";
  if (status === 403 || status === 409) return "Could not sync in this session. Your log is saved here; check access and retry.";
  if (status === 429) return "Too many requests. Your log is saved here; retry shortly.";
  return "The server has not confirmed this log. Your draft is saved here for retry.";
}
async function recordFailure(entry, status, message) {
  await updateEntry(entry.idempotencyKey, previous => previous && ({ ...previous, status:status ? "failed" : "queued", httpStatus:status, error:message })).catch(() => {});
}
async function sendEntry(entry, apiFetch, origin, durable) {
  requireOrigin(origin);
  let result;
  try {
    result = await apiFetch("/api/logs", { method:"POST", body:JSON.stringify(entry.body), origin });
  } catch (err) {
    if (!contextIsCurrent(origin)) throw err;
    if (err.status) {
      const message = durable ? errorText(err.status) : "The server has not saved this log. Keep this draft open and retry.";
      if (durable) await recordFailure(entry,err.status,message);
      throw new SaveError(message,err.status,durable);
    }
    await recordFailure(entry, 0, "Waiting for a connection or server confirmation.");
    if (durable) return { log:null, queued:true };
    throw new SaveError("Could not reach the server or save on this device. Keep this draft open and retry.");
  }
  assertContext(origin);
  if (!result.response?.ok || !result.data?.log) {
    const status = result.response?.status || 500;
    const message = durable ? errorText(status) : "The server has not saved this log. Keep this draft open and retry.";
    if (durable) await recordFailure(entry, status, message);
    throw new SaveError(message, status, durable);
  }
  // A failed cleanup is safe to retry: the same key and frozen body replay the
  // accepted log, including resumable server side effects.
  if (durable) await updateEntry(entry.idempotencyKey, () => null).catch(() => {});
  return result.data;
}
export async function submitLog(body, apiFetch, origin = contextSnapshot()) {
  requireOrigin(origin);
  return withBrowserLock(`nabu-log:${body.idempotencyKey}`, async () => {
    requireOrigin(origin);
    let entry = { idempotencyKey:body.idempotencyKey, actorId:origin.userId, householdId:origin.householdId, body };
    let durable = false;
    try { entry = await enqueueLog(body, origin); durable = true; }
    catch (err) { if (!contextIsCurrent(origin) || /different payload or owner/.test(err.message)) throw err; }
    return sendEntry(entry, apiFetch, origin, durable);
  });
}
export async function discardQueuedLog(key, origin = contextSnapshot()) {
  requireOrigin(origin);
  return withBrowserLock(`nabu-log:${key}`, () => {
    requireOrigin(origin);
    return updateEntry(key, previous => {
      if (previous && !owns(previous, origin)) throw new Error("This saved log belongs to another account or household.");
      return null;
    });
  });
}
export async function replayQueue(apiFetch, { origin = contextSnapshot(), retryKey = null } = {}) {
  requireOrigin(origin);
  return withBrowserLock("nabu-log-replay", async () => {
    const entries = await queuedLogs(origin), syncedKeys = [];
    for (const entry of entries) {
      requireOrigin(origin);
      if (retryKey && entry.idempotencyKey !== retryKey) continue;
      // Rejected requests remain visible for an explicit retry. In particular,
      // do not repeatedly send a forbidden chore or a malformed draft.
      if (!retryKey && entry.status === "failed") continue;
      try {
        const data = await withBrowserLock(`nabu-log:${entry.idempotencyKey}`, async () => {
          requireOrigin(origin);
          const current = await transaction(false, (store, finish) => {
            const request = store.get(entry.idempotencyKey); request.onsuccess = () => finish(request.result);
          });
          // Another tab may have synced or discarded this row while the pass
          // was sending an earlier entry. Replay never recreates absent work.
          if (!current || !owns(current, origin)) return null;
          return sendEntry(current, apiFetch, origin, true);
        });
        if (data?.queued) break;
        if (data?.log) syncedKeys.push(entry.idempotencyKey);
      } catch (err) {
        if (!contextIsCurrent(origin)) throw err;
        if (err.status === 401 || err.status === 429) break;
      }
    }
    return { syncedKeys };
  });
}
