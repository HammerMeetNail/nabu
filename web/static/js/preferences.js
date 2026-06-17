import { apiFetch } from "./api.js";

/**
 * Load the current user's preferences from the server and store the
 * choreOrder, hiddenHomeChoreIDs, and timezone into the provided state object.
 *
 * @param {object} state - The global app state (mutated in place).
 */
export async function loadPreferences(state) {
  try {
    const { data } = await apiFetch("/api/preferences");
    state.choreOrder = data?.preferences?.choreOrder ?? [];
    state.hiddenHomeChoreIDs = data?.preferences?.hiddenHomeChoreIds ?? [];
    state.timezone = data?.preferences?.timezone ?? "";
    state.stats = state.stats || {};
    state.stats.sectionOrder = data?.preferences?.statsSectionOrder ?? [];
    state.stats.sectionHidden = data?.preferences?.statsSectionHidden ?? [];
  } catch {
    state.choreOrder = [];
    state.hiddenHomeChoreIDs = [];
    state.timezone = "";
    state.stats = state.stats || {};
    state.stats.sectionOrder = [];
    state.stats.sectionHidden = [];
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
    const { data } = await apiFetch("/api/preferences", {
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
 * Persist an updated hidden-home-chores list to the server and update state.
 * Chores in this list are not shown on the home grid for this user.
 *
 * @param {object} state       - The global app state (mutated in place).
 * @param {number[]} hiddenIds - Array of chore IDs to hide from the home grid.
 */
export async function saveHiddenHomeChores(state, hiddenIds) {
  state.hiddenHomeChoreIDs = hiddenIds;
  try {
    const { data } = await apiFetch("/api/preferences", {
      method: "PATCH",
      body: JSON.stringify({ hiddenHomeChoreIds: hiddenIds }),
    });
    state.hiddenHomeChoreIDs = data?.preferences?.hiddenHomeChoreIds ?? hiddenIds;
  } catch {
    // Keep the optimistic in-memory value even if the save fails.
  }
}

/**
 * Detect the browser's IANA timezone and sync it to the server if it differs
 * from the stored value.  Called once on page load after house data is ready.
 *
 * @param {object} state - The global app state (mutated in place).
 */
export async function syncTimezone(state) {
  try {
    const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
    const storedTz = state.timezone || "";
    if (browserTz !== storedTz) {
      state.timezone = browserTz;
      const { data } = await apiFetch("/api/preferences", {
        method: "PATCH",
        body: JSON.stringify({ timezone: browserTz }),
      });
      state.timezone = data?.preferences?.timezone ?? browserTz;
    }
  } catch {
    // Silently ignore; stats will fall back to UTC.
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

/**
 * Persist an updated stats section order to the server and update state.
 *
 * @param {object} state  - The global app state (mutated in place).
 * @param {string[]} order - Ordered array of section keys.
 */
export async function saveStatsSectionOrder(state, order) {
  state.stats.sectionOrder = order;
  try {
    const { data } = await apiFetch("/api/preferences", {
      method: "PATCH",
      body: JSON.stringify({ statsSectionOrder: order }),
    });
    state.stats.sectionOrder = data?.preferences?.statsSectionOrder ?? order;
  } catch {
    // Keep the optimistic in-memory value.
  }
}

/**
 * Persist an updated stats section hidden set to the server and update state.
 *
 * @param {object} state   - The global app state (mutated in place).
 * @param {string[]} hidden - Array of hidden section keys.
 */
export async function saveStatsSectionHidden(state, hidden) {
  state.stats.sectionHidden = hidden;
  try {
    const { data } = await apiFetch("/api/preferences", {
      method: "PATCH",
      body: JSON.stringify({ statsSectionHidden: hidden }),
    });
    state.stats.sectionHidden = data?.preferences?.statsSectionHidden ?? hidden;
  } catch {
    // Keep the optimistic in-memory value.
  }
}
