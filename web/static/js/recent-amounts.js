import { apiFetch } from './api.js';
import { captureScope } from './request-scope.js';

export async function loadRecentAmounts(state, choreId, unit = '') {
  const scope = captureScope(state, `recent-amounts:${choreId}`);
  try {
    const {data} = await apiFetch(`/api/logs/recent-amounts?choreId=${choreId}&unit=${encodeURIComponent(unit)}`);
    if (!scope.current() || !Array.isArray(data.amounts)) return;
    state.recentAmounts ||= {};
    state.recentAmountUnits ||= {};
    state.recentAmountUnits[choreId] = unit;
    state.recentAmounts[choreId] = data.amounts.filter(v => Number.isInteger(v) && v > 0).slice(0,3);
  } catch { /* Keep the last confirmed values through a transient error. */ }
}
