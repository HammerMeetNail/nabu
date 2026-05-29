export function escapeHTML(str) {
  const div = document.createElement("div");
  div.textContent = str || "";
  return div.innerHTML;
}

export function localDateStr(d) {
  return d.getFullYear() + '-' +
    String(d.getMonth() + 1).padStart(2, '0') + '-' +
    String(d.getDate()).padStart(2, '0');
}

export function roundMinutesTo5(datetimeLocal) {
  if (!datetimeLocal) return datetimeLocal;
  const [datePart, timePart] = datetimeLocal.split('T');
  if (!timePart) return datetimeLocal;
  const [h, m] = timePart.split(':').map(Number);
  const rounded = Math.round(m / 5) * 5;
  const mm = String(rounded % 60).padStart(2, '0');
  const hh = String(h + Math.floor(rounded / 60)).padStart(2, '0');
  return `${datePart}T${hh}:${mm}`;
}
