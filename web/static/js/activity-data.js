import { loadHistory, loadMoreHistory } from "./today.js";
import { captureScope } from "./request-scope.js";
import { localDateStr } from "./utils.js";

function query(state) { return [(state.historySearch || "").trim(), [...(state.historyChoreFilter || [])].sort()]; }
export function setActivityChoreFilter(state, filter) {
  const reload = state.historyLoading && !state.historyBefore;
  state.historyChoreFilter = filter;
  captureScope(state, "activity");
  // Chore selection filters the loaded rows locally. Keep the page boundary,
  // but release loading state owned by any now-obsolete request.
  state.historyQuery = JSON.stringify(query(state));
  state.historyLoading = false;
  state._historyLoadingMore = false;
  state.historyError = null;
  return reload;
}
export function prepareActivity(state) {
  const key = JSON.stringify(query(state));
  if (state.historyQuery === key) return false;
  captureScope(state, "activity"); // invalidate an in-flight older query now, before debounce
  state.historyQuery = key;
  state.historyLogs = [];
  state.historyBefore = null;
  state.historyHasMore = false;
  state.historyError = null;
  state.historyLoading = true;
  state._historyLoadingMore = false;
  return true;
}
export async function loadActivity(state, { append = false, preservePages = false } = {}) {
  if (prepareActivity(state)) append = false;
  const search = (state.historySearch || "").trim();
  if (append && (search || !state.historyBefore || state._historyLoadingMore)) return;
  const before = state.historyBefore, previous = state.historyLogs || [], hadMore = state.historyHasMore;
  const scope = captureScope(state, "activity", () => query(state));
  state.historyError = null;
  state.historyLoading = !append;
  state._historyLoadingMore = append;
  try {
    const data = append ? await loadMoreHistory(before) : await loadHistory(search);
    if (!scope.current()) return;
    if (!data || !Array.isArray(data.logs)) throw new Error("Could not read Activity. Please retry.");
    let logs = data.logs, nextBefore = data.start || null, hasMore = !!data.hasMore;
    if (append) {
      const ids = new Set(previous.map(log => log.id));
      logs = [...previous, ...logs.filter(log => !ids.has(log.id))];
    } else if (preservePages && !search && before && nextBefore && before < nextBefore) {
      const ids = new Set(logs.map(log => log.id));
      logs = [...logs, ...previous.filter(log => {
        const date = log.logDate?.slice(0,10) || (log.completedAt ? localDateStr(new Date(log.completedAt)) : "");
        return date && date < nextBefore && !ids.has(log.id);
      })];
      nextBefore = before; hasMore = hadMore;
    }
    state.historyLogs = logs;
    state.historyBefore = nextBefore;
    state.historyHasMore = hasMore;
  } catch (err) {
    if (scope.current()) state.historyError = err.message || "Could not load Activity. Please retry.";
  } finally {
    if (scope.current()) { state.historyLoading = false; state._historyLoadingMore = false; }
  }
}
