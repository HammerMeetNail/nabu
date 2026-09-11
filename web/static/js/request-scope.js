import { contextSnapshot, contextIsCurrent } from "./browser-context.js";
const sequences = new WeakMap();

// A response may publish only into the state generation and query that started
// it. A newer request for the same resource supersedes success AND failure.
export function captureScope(state, resource = null, query = null) {
  const generation = state.contextGeneration, origin = contextSnapshot();
  const queryKey = query ? JSON.stringify(query()) : null;
  let sequence = 0, counters = sequences.get(state);
  if (!counters) { counters = new Map(); sequences.set(state, counters); }
  if (resource) { sequence = (counters.get(resource) || 0) + 1; counters.set(resource, sequence); }
  const owns = () => state.contextGeneration === generation && contextIsCurrent(origin) &&
    (!resource || counters.get(resource) === sequence);
  const current = () => owns() && (!query || JSON.stringify(query()) === queryKey);
  return { origin, current, owns, guard: fn => (...args) => current() ? fn(...args) : undefined };
}
