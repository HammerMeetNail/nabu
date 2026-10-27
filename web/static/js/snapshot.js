// The last confirmed Home data for this device's active identity, so a cold
// launch without a reachable server can open Home and queue logs in the
// journal. A snapshot is bound to the identity's user, household and binding,
// saved only from confirmed online state, and deleted whenever the identity
// stops being active (browser-context.js), so it never outlives a sign-out,
// an account or household switch, or an invalidated session.
import { DEVICE_STORE, deviceRecord, openDeviceDB } from "./device-store.js";

export const SNAPSHOT_KEY = "home-snapshot";
const VERSION = 1;
const FIELDS = ["user", "household", "members", "historicalMembers", "userHouseholds", "activeHouseholdId",
  "chores", "latestLogs", "choreOrder", "hiddenHomeChoreIDs", "timezone", "volumeUnit", "hideNotificationBadge"];

export function snapshotMatches(snapshot, identity) {
  return !!snapshot && !!identity && snapshot.version === VERSION && identity.status === "active" &&
    !!identity.userId && !!identity.householdId && !!identity.bindingId &&
    snapshot.userId === identity.userId && snapshot.householdId === identity.householdId &&
    snapshot.bindingId === identity.bindingId;
}

export function pickSnapshot(state, identity) {
  const data = {};
  for (const key of FIELDS) data[key] = state[key] ?? null;
  return { version:VERSION, userId:identity.userId, householdId:identity.householdId, bindingId:identity.bindingId,
    savedAt:Date.now(), data:JSON.parse(JSON.stringify(data)) };
}

// Only Home data the server confirmed for exactly this identity is saved.
export function canSnapshot(state, identity) {
  return !!identity && identity.status === "active" && !!state.user && state.user.id === identity.userId &&
    !!state.household && (identity.householdId === state.household.id) && !state.offline &&
    !state.sessionUnconfirmed && !state.logoutPending && !state.transitioning && !state.accessSuspended;
}

// The write re-reads the stored identity in the same transaction, so a save
// racing a sign-out or switch can never land after that change wiped it.
export async function saveSnapshot(state, identity) {
  if (!canSnapshot(state, identity)) return false;
  const snapshot = pickSnapshot(state, identity);
  const db = await openDeviceDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(DEVICE_STORE, "readwrite");
    const store = tx.objectStore(DEVICE_STORE);
    let saved = false;
    const request = store.get("identity");
    request.onsuccess = () => {
      if (snapshotMatches(snapshot, request.result)) { store.put(snapshot, SNAPSHOT_KEY); saved = true; }
    };
    tx.oncomplete = () => { db.close(); resolve(saved); };
    tx.onabort = tx.onerror = () => { db.close(); reject(tx.error || new Error("Could not save on this device")); };
  });
}

export async function loadSnapshot(identity) {
  const snapshot = await deviceRecord(SNAPSHOT_KEY);
  return snapshotMatches(snapshot, identity) ? snapshot : null;
}

export function applySnapshot(state, snapshot) {
  for (const key of FIELDS) if (key in snapshot.data) state[key] = snapshot.data[key];
  state.members ||= [];
  state.historicalMembers ||= [];
  state.userHouseholds ||= [];
  state.chores ||= [];
  state.latestLogs ||= {};
}

export async function clearSnapshot() {
  await deviceRecord(SNAPSHOT_KEY, () => null);
}
