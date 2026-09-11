import { apiFetch } from "./api.js";
import { captureScope } from "./request-scope.js";

const mapping = {
  choreOrder:"choreOrder", hiddenHomeChoreIds:"hiddenHomeChoreIDs", timezone:"timezone", volumeUnit:"volumeUnit", hideNotificationBadge:"hideNotificationBadge",
  statsSectionOrder:"sectionOrder", statsSectionHidden:"sectionHidden", statsWidgets:"widgets",
};
const defaults = { choreOrder:[], hiddenHomeChoreIds:[], timezone:"", volumeUnit:"ml", hideNotificationBadge:false, statsSectionOrder:[], statsSectionHidden:[], statsWidgets:[] };
function target(state,key) { return key.startsWith("stats") ? (state.stats ||= {}) : state; }
function values(state) { return Object.keys(mapping).map(key => target(state,key)[mapping[key]]); }
export async function loadPreferences(state) {
  const scope = captureScope(state,"preferences-read",() => values(state));
  try {
    const {data} = await apiFetch("/api/preferences");
    if (!scope.current() || !data?.preferences) return;
    for (const [key,field] of Object.entries(mapping)) target(state,key)[field] = data.preferences[key] ?? structuredClone(defaults[key]);
  } catch { /* A failed or obsolete read must preserve the last good settings. */ }
}
async function savePreference(state,key,value) {
  const scope = captureScope(state,`preference:${key}`);
  const object = target(state,key), field = mapping[key], previous = object[field];
  object[field] = value;
  try {
    const {response,data} = await apiFetch("/api/preferences", {method:"PATCH",body:JSON.stringify({[key]:value})});
    if (!response.ok) throw new Error("Could not save preference");
    if (!scope.current()) return null;
    object[field] = data?.preferences?.[key] ?? value;
    return object[field];
  } catch {
    if (scope.current()) object[field] = previous;
    return null;
  }
}
export const saveStatsWidgets = (state,widgets) => savePreference(state,"statsWidgets",widgets);
export const saveVolumeUnit = (state,unit) => savePreference(state,"volumeUnit",unit);
export const saveHideNotificationBadge = (state,hide) => savePreference(state,"hideNotificationBadge",hide);
export const saveChoreOrder = (state,order) => savePreference(state,"choreOrder",order);
export const saveHiddenHomeChores = (state,ids) => savePreference(state,"hiddenHomeChoreIds",ids);
export const saveStatsSectionOrder = (state,order) => savePreference(state,"statsSectionOrder",order);
export const saveStatsSectionHidden = (state,hidden) => savePreference(state,"statsSectionHidden",hidden);
export async function syncTimezone(state) {
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  if (state.timezone !== zone) await savePreference(state,"timezone",zone);
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
