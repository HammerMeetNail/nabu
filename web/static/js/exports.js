import { registerActions } from "./actions.js";
import { apiFetch } from "./api.js";
import { state, rerender } from "./app-core.js";
import { assertContext } from "./browser-context.js";
import { captureScope } from "./request-scope.js";
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

// "Download my data", in Account and in the deletion panel: everything Nabu
// keeps about the signed-in user as JSON, or the logs they recorded as CSV.
export function renderMyDataDownload(state, {jsonId, csvId = ""}) {
  const disabled = state.myDataBusy ? " disabled" : "";
  return `<div class="mt-2" data-testid="my-data-download">
    <button type="button" class="btn btn-secondary btn-sm" data-action="download-my-data" data-format="json" data-testid="${escapeHTML(jsonId)}"${disabled}>Download my data</button>
    ${csvId ? `<button type="button" class="btn btn-ghost btn-sm" data-action="download-my-data" data-format="csv" data-testid="${escapeHTML(csvId)}"${disabled}>Logs as CSV</button>` : ""}
    <p class="text-secondary mt-1">Everything Nabu keeps about you, as a JSON file${csvId ? ", or every log you recorded, as CSV" : ""}.</p>
    ${state.myDataBusy ? '<p role="status">Preparing your data…</p><button type="button" class="btn btn-secondary btn-sm" data-action="cancel-my-data">Cancel</button>' : ""}
    ${state.myDataError ? `<p role="alert" class="error-text" data-testid="my-data-error">${escapeHTML(state.myDataError)}</p>` : ""}
  </div>`;
}

export async function downloadCSV(kind, range, origin, signal) {
  if (range.start && range.end && range.start > range.end) throw new Error("Choose an end date on or after the start date.");
  const path = kind === "household" ? "/api/household/data" : "/api/logs/export";
  const query = new URLSearchParams({start: range.start || "0001-01-01", end: range.end || "9999-12-31"});
  await saveExport(`${path}?${query}`, "text/csv", kind === "household" ? "nabu-household-data.csv" : "nabu-logs.csv",
    "The export is too large. Choose a smaller date range.", origin, signal);
}

const myDataFiles = {
  json: ["/api/me/export", "application/json", "nabu-my-data.json"],
  csv: ["/api/me/logs/export", "text/csv", "nabu-my-logs.csv"],
};

export async function downloadMyData(format, origin, signal) {
  const [path, type, filename] = myDataFiles[format];
  await saveExport(path, type, filename, "Your data is too large for one download. Contact support for a copy.", origin, signal);
}

// Saves a complete export of type as filename, or throws a message to show.
// A canceled, late or partial response never becomes a file.
async function saveExport(path, type, filename, tooLarge, origin, signal) {
  const controller = new AbortController();
  const abort = () => controller.abort(signal.reason);
  if (signal.aborted) abort(); else signal.addEventListener("abort", abort, {once:true});
  const timer = setTimeout(() => controller.abort(new DOMException("Export timed out", "TimeoutError")), 22000);
  try {
    // A raw read with its own signal leaves the body unread and unshared.
    const {response} = await apiFetch(path, {origin, signal:controller.signal, raw:true});
    if (!response.headers.get("Content-Type")?.includes(type)) throw new Error("The export response was incomplete. Please retry.");
    const blob = await response.blob();
    assertContext(origin);
    controller.signal.throwIfAborted();
    if (!blob.size || blob.size > 16 * 1024 * 1024) throw new Error(tooLarge);
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
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

let activeExport = null;
let activeMyData = null;

// Stops the exports in progress. Their rows belong to the identity that
// started them, so a change of identity stops them.
export function cancelExport() {
  activeExport?.controller.abort();
  activeExport = null;
  activeMyData?.controller.abort();
  activeMyData = null;
}

registerActions("click", {
  "export-csv"(_event, el) {
    if (state.exportBusy) return;
    const scope = captureScope(state, "export");
    const ticket = {scope, controller:new AbortController()};
    activeExport = ticket;
    state.exportBusy = true;
    state.exportError = null;
    state.exportStatus = null;
    const range = {...(state.exportRange || {})};
    rerender();
    downloadCSV(el.dataset.kind, range, scope.origin, ticket.controller.signal)
      .then(() => { if (scope.current()) state.exportStatus = "Export ready."; })
      .catch(err => {
        if (scope.current() && !ticket.controller.signal.aborted) state.exportError = err.name === "TimeoutError"
          ? "Export took too long. Choose a smaller date range and retry." : err.message;
      })
      .finally(() => {
        if (activeExport === ticket) activeExport = null;
        if (scope.current()) { state.exportBusy = false; rerender(); }
      });
  },
  "download-my-data"(_event, el) {
    if (state.myDataBusy) return;
    const scope = captureScope(state, "my-data");
    const ticket = {scope, controller:new AbortController()};
    activeMyData = ticket;
    state.myDataBusy = true;
    state.myDataError = null;
    rerender();
    downloadMyData(el.dataset.format, scope.origin, ticket.controller.signal)
      .catch(err => {
        if (scope.current() && !ticket.controller.signal.aborted) state.myDataError = err.name === "TimeoutError"
          ? "Preparing your data took too long. Please retry." : err.message;
      })
      .finally(() => {
        if (activeMyData === ticket) activeMyData = null;
        if (scope.current()) { state.myDataBusy = false; rerender(); }
      });
  },
  "cancel-my-data"() {
    activeMyData?.controller.abort();
    activeMyData = null;
    captureScope(state, "my-data");
    state.myDataBusy = false;
    state.myDataError = null;
    rerender();
  },
  "cancel-export"() {
    activeExport?.controller.abort();
    activeExport = null;
    captureScope(state, "export");
    state.exportBusy = false;
    state.exportError = null;
    state.exportStatus = "Export canceled.";
    rerender();
  },
});

registerActions("change", {
  "export-range"(_event, el) {
    state.exportRange = {...(state.exportRange || {}), [el.dataset.field]:el.value};
    state.exportError = null;
    state.exportStatus = null;
  },
});
