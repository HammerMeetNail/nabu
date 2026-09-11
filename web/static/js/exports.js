import { apiFetch } from "./api.js";
import { assertContext } from "./browser-context.js";
import { escapeHTML } from "./utils.js";

export function renderExports(state, canExportHousehold) {
  const range = state.exportRange || {};
  return `<div class="card mt-3" data-testid="export-controls">
    <h3>Export</h3>
    <p class="text-secondary">Download activity as CSV. Leave dates empty for all dates.</p>
    <div class="export-date-range">
      <label for="export-start">From<input id="export-start" type="date" data-action="export-range" data-field="start" value="${escapeHTML(range.start || '')}"${state.exportBusy ? ' disabled' : ''}></label>
      <label for="export-end">Through<input id="export-end" type="date" data-action="export-range" data-field="end" value="${escapeHTML(range.end || '')}"${state.exportBusy ? ' disabled' : ''}></label>
    </div>
    <p class="text-secondary">Each export can contain up to 10,000 records and 16 MB. Choose a smaller date range if needed.</p>
    <button class="btn btn-secondary btn-sm" data-action="export-csv" data-kind="logs"${state.exportBusy ? ' disabled' : ''}>Export logs as CSV</button>
    ${canExportHousehold ? `<div data-testid="household-export-section" class="mt-2">
      <button class="btn btn-secondary btn-sm" data-action="export-csv" data-kind="household"${state.exportBusy ? ' disabled' : ''}>Export household data as CSV</button>
      <p class="text-secondary mt-2">Includes household details, chores, schedules, participants, and activity and notes within the selected dates. Invite codes and account credentials are never included.</p>
    </div>` : ''}
    ${state.exportBusy ? '<p role="status">Preparing export…</p><button class="btn btn-secondary btn-sm" data-action="cancel-export">Cancel export</button>' : ''}
    ${state.exportError ? `<p role="alert" class="error-text" data-testid="export-error">${escapeHTML(state.exportError)}</p>` : ''}
    ${state.exportStatus ? `<p role="status">${escapeHTML(state.exportStatus)}</p>` : ''}
  </div>`;
}

export async function downloadCSV(kind, range, origin, signal) {
  if (range.start && range.end && range.start > range.end) throw new Error("Choose an end date on or after the start date.");
  const path = kind === "household" ? "/api/household/data" : "/api/logs/export";
  const query = new URLSearchParams({start: range.start || "0001-01-01", end: range.end || "9999-12-31"});
  const controller = new AbortController();
  const abort = () => controller.abort(signal.reason);
  if (signal.aborted) abort(); else signal.addEventListener("abort", abort, {once:true});
  const timer = setTimeout(() => controller.abort(new DOMException("Export timed out", "TimeoutError")), 22000);
  try {
    // Supplying this caller-owned signal also avoids sharing a consumable CSV
    // Response through the JSON GET deduplication path.
    const {response} = await apiFetch(`${path}?${query}`, {origin, signal:controller.signal});
    if (!response.headers.get("Content-Type")?.includes("text/csv")) throw new Error("The export response was incomplete. Please retry.");
    const blob = await response.blob();
    assertContext(origin);
    controller.signal.throwIfAborted();
    if (!blob.size || blob.size > 16 * 1024 * 1024) throw new Error("The export is too large. Choose a smaller date range.");
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = kind === "household" ? "nabu-household-data.csv" : "nabu-logs.csv";
    link.hidden = true;
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  } finally {
    clearTimeout(timer);
    signal.removeEventListener("abort", abort);
  }
}
