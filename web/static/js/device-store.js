// Small durable records shared by tabs and the service worker. Private log
// payloads live in the separately scoped mutation journal.
export const DEVICE_DB = "nabu-device";
export const DEVICE_STORE = "state";

export function newKey() {
  if (globalThis.crypto?.randomUUID) return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return [...bytes].map(n => n.toString(16).padStart(2, "0")).join("");
}

export function openDeviceDB() {
  return new Promise((resolve, reject) => {
    if (!globalThis.indexedDB) { reject(new Error("Device storage is unavailable")); return; }
    const request = indexedDB.open(DEVICE_DB, 1);
    request.onupgradeneeded = () => request.result.createObjectStore(DEVICE_STORE);
    request.onerror = () => reject(request.error);
    request.onblocked = () => reject(new Error("Device storage is busy"));
    request.onsuccess = () => {
      request.result.onversionchange = () => request.result.close();
      resolve(request.result);
    };
  });
}

export async function deviceRecord(key, change) {
  const db = await openDeviceDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(DEVICE_STORE, change ? "readwrite" : "readonly");
    const store = tx.objectStore(DEVICE_STORE);
    const request = store.get(key);
    let value, error;
    request.onsuccess = () => {
      value = request.result || null;
      if (change) {
        try { value = change(value); store.put(value, key); }
        catch (err) { error = err; tx.abort(); }
      }
    };
    tx.oncomplete = () => { db.close(); resolve(value); };
    tx.onabort = tx.onerror = () => { db.close(); reject(error || tx.error || new Error("Could not save on this device")); };
  });
}

export async function withBrowserLock(name, run) {
  if (globalThis.navigator?.locks?.request) return navigator.locks.request(name, run);
  // A timed storage lease cannot fence a suspended tab when it resumes.
  // Identity changes and journal replay require a lock held for the entire
  // operation, including across browser suspension.
  throw new Error("Update your browser to sign in and sync saved work safely.");
}
