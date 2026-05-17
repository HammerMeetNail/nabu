import { apiFetch } from "./api.js";

/**
 * Load the current user's preferences from the server and store the
 * choreOrder into the provided state object.
 *
 * @param {object} state - The global app state (mutated in place).
 */
export async function loadPreferences(state) {
  try {
    const data = await apiFetch("/api/preferences");
    state.choreOrder = data?.preferences?.choreOrder ?? [];
  } catch {
    state.choreOrder = [];
  }
}

/**
 * Persist an updated chore order to the server and update state.
 *
 * @param {object} state         - The global app state (mutated in place).
 * @param {number[]} choreOrder  - Ordered array of chore IDs.
 */
export async function saveChoreOrder(state, choreOrder) {
  state.choreOrder = choreOrder;
  try {
    const data = await apiFetch("/api/preferences", {
      method: "PATCH",
      body: JSON.stringify({ choreOrder }),
    });
    // Sync state with whatever the server echoes back.
    state.choreOrder = data?.preferences?.choreOrder ?? choreOrder;
  } catch {
    // Keep the optimistic in-memory order even if the save fails.
  }
}

/**
 * Returns a copy of `chores` sorted according to `state.choreOrder`.
 * Chores not present in choreOrder are appended in their original order.
 *
 * @param {object[]} chores      - Array of chore objects with an `id` field.
 * @param {number[]} choreOrder  - Ordered array of chore IDs.
 * @returns {object[]}
 */
export function sortChoresByOrder(chores, choreOrder) {
  if (!choreOrder || choreOrder.length === 0) return chores;
  const pos = new Map(choreOrder.map((id, i) => [id, i]));
  return [...chores].sort((a, b) => {
    const pa = pos.has(a.id) ? pos.get(a.id) : Infinity;
    const pb = pos.has(b.id) ? pos.get(b.id) : Infinity;
    return pa - pb;
  });
}
