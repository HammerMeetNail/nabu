import { formatVolume } from './utils.js';

export function isVolumeMetric(chore) {
  return !!chore.hasVolumeML && ['', 'ml', 'oz'].includes((chore.metricUnit || '').trim().toLowerCase());
}
export function formatAmount(amount, chore, volumeUnit = 'ml') {
  return isVolumeMetric(chore) ? formatVolume(amount,volumeUnit) : `${amount}${chore.metricUnit ? ` ${chore.metricUnit}` : ''}`;
}

export const commonAmountUnits = ['mL', 'oz', 'mg', 'g', 'kg', 'lb', 'tsp', 'tbsp', 'drops', 'tablets', 'units', 'min'];
export function amountUnitOptions(unit) { return [...new Set([...commonAmountUnits, unit].filter(Boolean))]; }
export function entryMetric(chore, log) { return log?.metricUnit ? {...chore, metricUnit:log.metricUnit} : chore; }
export function entryVolumeUnit(log, fallback = 'ml') {
  return log?.metricUnit && ['ml','oz'].includes(log.metricUnit.toLowerCase()) ? log.metricUnit.toLowerCase() : fallback;
}
export function amountPresets(chore, selected = null) {
  const values = [...Array(201).keys(), 250, 500, 750, 1000];
  if (selected != null && Number.isInteger(selected)) values.push(selected);
  return [...new Set(values)].sort((a,b)=>a-b);
}
