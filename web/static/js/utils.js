// escapeHTML escapes a string for safe interpolation into HTML — including
// inside double/single-quoted attribute values. The previous implementation
// (textContent -> innerHTML) only escaped &, <, > (text-node escaping) and left
// quotes intact, so user content placed in an attribute (e.g. a chore subject
// in data-subject="...") could break out of the attribute and inject an event
// handler. Escaping the quotes too closes every attribute sink at the source
// and is harmless in text contexts (&quot;/&#39; render as "/'). The `|| ""`
// preserves the old falsy handling (0/false/null -> "").
export function escapeHTML(str) {
  return String(str || "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

export function localDateStr(d) {
  return d.getFullYear() + '-' +
    String(d.getMonth() + 1).padStart(2, '0') + '-' +
    String(d.getDate()).padStart(2, '0');
}

// ─── Volume units ────────────────────────────────────────────────────────────
// Volumes are stored canonically as milliliters (mL) everywhere in the DB and
// API. The user's `volumeUnit` preference ("ml" | "oz") only changes how they
// are displayed and how the log-sheet picker is labeled. US caregivers think
// in ounces, so oz support is a display/input convenience over the same data.

export const ML_PER_OZ = 29.5735;

export function mlToOz(ml) {
  return (ml || 0) / ML_PER_OZ;
}

export function ozToMl(oz) {
  return Math.round((oz || 0) * ML_PER_OZ);
}

function trimNum(n) {
  // "2.0" -> "2", "1.5" -> "1.5"
  return String(Number(n.toFixed(1))).replace(/\.0$/, "");
}

// formatVolume renders a canonical mL amount in the user's preferred unit.
export function formatVolume(ml, unit) {
  if (ml == null) return "";
  if (unit === "oz") {
    return `${trimNum(mlToOz(ml))} oz`;
  }
  return `${ml} mL`;
}

// volumeOptions returns the ordered list of { ml, label } choices for the
// log-sheet volume picker in the given unit. Option *values* are always
// canonical mL; only the labels differ by unit. `selectedML`, if it isn't
// already one of the presets, is appended so editing an existing log keeps
// its exact value selectable.
export function volumeOptions(unit, selectedML = null) {
  const opts = [];
  if (unit === "oz") {
    for (let oz = 0.5; oz <= 8.0001; oz += 0.5) {
      opts.push({ ml: ozToMl(oz), label: `${trimNum(oz)} oz` });
    }
  } else {
    for (let ml = 0; ml <= 200; ml += 5) {
      opts.push({ ml, label: `${ml} mL` });
    }
  }
  if (selectedML != null && !opts.some(o => o.ml === selectedML)) {
    opts.push({ ml: selectedML, label: formatVolume(selectedML, unit) });
    opts.sort((a, b) => a.ml - b.ml);
  }
  return opts;
}

// Arithmetic on a YYYY-MM-DD value, separate from a local/UTC instant.
export function shiftDateStr(isoDate, days) {
  const date = new Date(`${isoDate}T00:00:00Z`);
  if (Number.isNaN(date.getTime())) return "";
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}
