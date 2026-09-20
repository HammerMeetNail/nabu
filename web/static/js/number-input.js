import { escapeHTML } from './utils.js';
import { amountInputMax } from './metrics.js';

// Choices use displayed numbers, just like the adjacent input. Conversion to
// canonical units still happens once, in the existing log save handler.
function renderOptions(value, unit, kind) {
  const choices = [];
  const range = (start, end, step) => {
    for (let n = start; n <= end; n += step) choices.push(n);
  };
  if (kind === 'duration') {
    range(0, 45, 15);
    range(60, 3540, 60);
    range(3600, 86400, 3600);
  } else if (unit.toLowerCase() === 'oz') {
    range(0, 16, 0.5);
  } else if (unit.toLowerCase() === 'ml') {
    range(0, 300, 5);
    range(325, 1000, 25);
  } else {
    range(0, 20, 1);
    range(25, 100, 5);
    range(125, 1000, 25);
  }
  const max = kind === 'duration' ? 86400 : amountInputMax(unit);
  const selected = value === '' ? null : Number(value);
  if (selected !== null && Number.isFinite(selected) && selected >= 0 && selected <= max) choices.push(selected);
  const label = n => {
    if (kind !== 'duration') return `${n} ${unit}`.trim();
    if (n > 0 && n % 3600 === 0) return `${n / 3600} hr`;
    if (n > 0 && n % 60 === 0) return `${n / 60} min`;
    return `${n} sec`;
  };
  return `<option value="">No value</option>` + [...new Set(choices)].sort((a, b) => a - b)
    .map(n => `<option value="${n}"${n === selected ? ' selected' : ''}>${escapeHTML(label(n))}</option>`).join('');
}

export function renderNumberInput({id = '', className = '', label, pickerLabel, value = '', unit = '', kind = 'amount', indicator = null, disabled = false}) {
  const max = kind === 'duration' ? 86400 : amountInputMax(unit);
  const step = kind === 'amount' && unit.toLowerCase() === 'oz' ? 'any' : '1';
  return `<div class="number-input" data-number-kind="${escapeHTML(kind)}" data-number-unit="${escapeHTML(unit)}"${disabled ? ' hidden' : ''}>
    <input ${id ? `id="${escapeHTML(id)}"` : ''} class="text-input ${escapeHTML(className)}" type="number" inputmode="${step === 'any' ? 'decimal' : 'numeric'}" aria-label="${escapeHTML(label)}" min="0" max="${max}" step="${step}" value="${escapeHTML(String(value))}"${indicator === null ? '' : ` data-indicator="${escapeHTML(indicator)}"`}${disabled ? ' disabled' : ''}>
    <span class="number-input-choice">
      <span aria-hidden="true">⌄</span>
      <select data-action="pick-number" aria-label="${escapeHTML(pickerLabel)}" title="${escapeHTML(pickerLabel)}"${disabled ? ' disabled' : ''}>${renderOptions(String(value), unit, kind)}</select>
    </span>
  </div>`;
}

export function syncNumberInput(input, unit) {
  const field = input.closest('.number-input');
  if (!field) return;
  if (unit !== undefined) field.dataset.numberUnit = unit;
  field.querySelector('select').innerHTML = renderOptions(input.value, field.dataset.numberUnit, field.dataset.numberKind);
}

export function setNumberInputEnabled(input, enabled) {
  const field = input.closest('.number-input');
  field.hidden = !enabled;
  input.disabled = !enabled;
  field.querySelector('select').disabled = !enabled;
}
