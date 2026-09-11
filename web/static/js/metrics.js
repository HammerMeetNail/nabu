import { formatVolume } from './utils.js';

export function isVolumeMetric(chore) {
  return !!chore.hasVolumeML && ['', 'ml', 'oz'].includes((chore.metricUnit || '').trim().toLowerCase());
}
export function formatAmount(amount, chore, volumeUnit = 'ml') {
  return isVolumeMetric(chore) ? formatVolume(amount,volumeUnit) : `${amount}${chore.metricUnit ? ` ${chore.metricUnit}` : ''}`;
}
