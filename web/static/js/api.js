import { contextSnapshot, contextIsCurrent, assertContext, originHeaders, ContextChangedError } from "./browser-context.js";

const reads = new Map();
export function getCSRFToken() {
  const match = document.cookie.match(/(?:^|;\s*)nabu_csrf=([^;]*)/);
  return match ? match[1] : "";
}

async function request(path, options, origin) {
  const { origin:_origin, allowContextChange:_allow, ...requestOptions } = options;
  const headers = new Headers(options.headers || {});
  headers.set("Content-Type", "application/json");
  const token = getCSRFToken();
  if (token) headers.set("X-CSRF-Token", token);
  for (const [key,value] of Object.entries(originHeaders(origin))) headers.set(key,value);
  const response = await fetch(path, { ...requestOptions, headers, signal:options.signal || AbortSignal.timeout(20000) });
  if (!options.allowContextChange && origin?.status === "active" && contextIsCurrent(origin)
      && (response.status === 401 || (response.status === 409 && response.headers.get("X-Nabu-Context-Changed") === "true"))) {
    window.dispatchEvent(new CustomEvent("nabu-session-invalid", {detail:{origin}}));
  }
  let data = null;
  if (response.headers.get("Content-Type")?.includes("application/json")) {
    try { data = await response.json(); } catch { if (response.ok) throw new Error("The server response was incomplete. Please retry."); }
  }
  if (!response.ok) {
    const err = new Error(data?.error || `Could not load data (${response.status})`);
    err.status = response.status;
    throw err;
  }
  return {response,data};
}
export async function apiFetch(path, options = {}) {
  const origin = options.origin === undefined ? contextSnapshot() : options.origin;
  if (!options.allowContextChange) {
    assertContext(origin);
    if (origin?.userId && origin.status !== "active") throw new ContextChangedError();
  }
  const method = (options.method || "GET").toUpperCase();
  // Share only simultaneous identical reads. Data never survives an identity
  // revision; explicit cancellation remains owned by its individual caller.
  const key = method === "GET" && !options.signal ? JSON.stringify([origin?.revision,path,[...new Headers(options.headers || {}).entries()]]) : null;
  let pending = key && reads.get(key);
  if (!pending) {
    pending = request(path,options,origin);
    if (key) { pending = pending.finally(() => reads.delete(key)); reads.set(key,pending); }
  }
  const result = await pending;
  if (!options.allowContextChange) assertContext(origin);
  return {response:result.response,data:result.data === null ? null : structuredClone(result.data)};
}
export async function apiMe() { const {data} = await apiFetch("/api/me"); return data; }
