import { formatVolume, mlToOz, ozToMl } from './utils.js';

export function isVolumeMetric(chore) {
  return !!chore.hasVolumeML && ['', 'ml', 'oz'].includes((chore.metricUnit || '').trim().toLowerCase());
}
export function formatAmount(amount, chore, volumeUnit = 'ml') {
  return isVolumeMetric(chore) ? formatVolume(amount,volumeUnit) : `${amount}${chore.metricUnit ? ` ${chore.metricUnit}` : ''}`;
}

export const commonAmountUnits = ['mcg', 'mg', 'g', 'mL', 'L', 'drops', 'tablets', 'capsules', 'puffs', 'units', 'oz', 'kg', 'lb', 'tsp', 'tbsp', 'min'];
export function amountUnitOptions(unit) { return [...new Set([...commonAmountUnits, unit].filter(Boolean))]; }
export function entryMetric(chore, log) { return log?.metricUnit ? {...chore, metricUnit:log.metricUnit} : chore; }
export function entryVolumeUnit(log, fallback = 'ml') {
  return log?.metricUnit && ['ml','oz'].includes(log.metricUnit.toLowerCase()) ? log.metricUnit.toLowerCase() : fallback;
}
// The legacy API stores oz in canonical mL; all other amounts use their unit.
export function amountInputValue(value, unit) {
  if (value == null) return '';
  return unit?.toLowerCase() === 'oz' ? String(Math.round(mlToOz(value) * 1000) / 1000) : String(value);
}
export function storedAmount(value, unit) {
  if (value === '' || value == null) return null;
  return unit?.toLowerCase() === 'oz' ? ozToMl(Number(value)) : Number(value);
}

export function amountInputMax(unit) { return Number(amountInputValue(100000, unit)); }
