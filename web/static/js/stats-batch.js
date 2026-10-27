// Stats reads that start together travel together. The Stats page starts every
// section's read in one synchronous pass; instead of one request per chart
// they leave as /api/stats/dashboard requests in two lanes, cheap sections
// and the heavier per-chore charts, so the sections still render first. Each
// caller gets its own data or error, exactly as from the endpoint. A lone
// read, such as a period switch, goes straight to its endpoint, and a read
// that starts after an await never joins an earlier, slower request.
import { apiFetch } from "./api.js";
import { contextSnapshot } from "./browser-context.js";

const PREFIX = "/api/stats/";
// The server answers at most 64 reads per request (maxDashboardReads).
const MAX_READS = 64;
const CHART_READ = /^\/api\/stats\/chores\/\d+\/(time-series|summary)(\?|$)/;
const queues = new Map();
// The last results per dashboard URL, kept in memory for the identity that read
// them, so reopening Stats revalidates (304) instead of downloading again.
const kept = new Map();
const KEEP = 4;

export function statsGet(path) {
  const origin = contextSnapshot();
  const key = `${origin?.revision || ""}|${CHART_READ.test(path) ? "charts" : "sections"}`;
  let queue = queues.get(key);
  if (!queue) {
    queue = {origin, waiters:new Map()};
    queues.set(key, queue);
    queueMicrotask(() => { queues.delete(key); void flush(queue); });
  }
  return new Promise((resolve, reject) => {
    if (!queue.waiters.has(path)) queue.waiters.set(path, []);
    queue.waiters.get(path).push({resolve, reject});
  });
}

async function flush({origin, waiters}) {
  // Every caller gets its own copy, as apiFetch gives each caller.
  const settle = (path, err, data) => {
    for (const waiter of waiters.get(path)) err ? waiter.reject(err) : waiter.resolve(structuredClone(data));
  };
  const paths = [...waiters.keys()];
  if (paths.length === 1) {
    try { settle(paths[0], null, (await apiFetch(paths[0], {origin})).data); }
    catch (err) { settle(paths[0], err); }
    return;
  }
  const chunks = [];
  for (let i = 0; i < paths.length; i += MAX_READS) chunks.push(paths.slice(i, i + MAX_READS));
  await Promise.all(chunks.map(async chunk => {
    let results;
    try { results = await dashboard(chunk, origin); }
    catch (err) { for (const path of chunk) settle(path, err); return; }
    chunk.forEach((path, i) => {
      const result = results[i];
      if (result.status >= 200 && result.status < 300) settle(path, null, result.data);
      else settle(path, readError(result));
    });
  }));
}

// The same error apiFetch raises for that status from the endpoint itself.
function readError(result) {
  const err = new Error(result.data?.error || `Could not load data (${result.status})`);
  err.status = result.status;
  err.data = result.data ?? null;
  return err;
}

async function dashboard(paths, origin) {
  const params = new URLSearchParams();
  for (const path of paths) params.append("read", path.slice(PREFIX.length));
  const url = `${PREFIX}dashboard?${params}`;
  const revision = origin?.revision || "";
  const previous = kept.get(url);
  kept.delete(url);
  const usable = previous?.revision === revision ? previous : null;
  let results;
  try {
    const {response, data} = await apiFetch(url, {origin, headers:usable ? {"If-None-Match":usable.etag} : {}});
    results = data?.results;
    if (!Array.isArray(results) || results.length !== paths.length) throw new Error("The server response was incomplete. Please retry.");
    // Only a fully successful page is kept: a retry after a failed read
    // always asks for fresh results.
    const etag = response.headers.get("ETag");
    if (etag && results.every(result => result.status >= 200 && result.status < 300)) remember(url, {revision, etag, results});
  } catch (err) {
    if (err.status !== 304 || !usable) throw err;
    results = usable.results;
    remember(url, usable);
  }
  return results;
}

function remember(url, entry) {
  kept.set(url, entry);
  while (kept.size > KEEP) kept.delete(kept.keys().next().value);
}

// Tests start from an empty batch and no kept responses.
export function resetStatsBatch() {
  queues.clear();
  kept.clear();
}
