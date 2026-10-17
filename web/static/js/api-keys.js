import { apiFetch } from "./api.js";
import { escapeHTML } from "./utils.js";

export async function loadApiKeys() {
  const { data } = await apiFetch("/api/api-keys");
  return Array.isArray(data?.keys) ? data.keys : [];
}

export async function createApiKey(name) {
  const { data } = await apiFetch("/api/api-keys", { method: "POST", body: JSON.stringify({ name }) });
  return data;
}

export async function revokeApiKey(id) {
  await apiFetch(`/api/api-keys/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export function apiKeyStatus(key) {
  if (key.revokedAt) return "Revoked";
  if (key.invalidated) return "Off (password changed)";
  return "Active";
}

function shortDate(value) {
  if (!value) return "Never";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

// Settings card for personal API keys. The clear token is only in state
// between creation and "Done"; the server never returns it again.
export function renderApiKeys(state, origin = "") {
  const keys = state.apiKeys || [];
  const issued = state.apiKeyIssued;
  const busy = !!state.apiKeyBusy;
  const issuedBlock = issued ? `
    <div class="api-key-issued mt-2" data-testid="api-key-issued">
      <p><strong>Copy this key now.</strong> You won't be able to see it again.</p>
      <input id="api-key-token" type="text" readonly value="${escapeHTML(issued)}" aria-label="New API key" autocomplete="off" spellcheck="false">
      <div class="mt-2">
        <button type="button" class="btn btn-primary btn-sm" data-action="copy-api-key">Copy</button>
        <button type="button" class="btn btn-ghost btn-sm" data-action="dismiss-api-key">Done</button>
      </div>
    </div>` : "";
  const rows = keys.map(key => {
    const status = apiKeyStatus(key);
    return `<li class="api-key-row" data-testid="api-key-row">
      <div class="api-key-meta">
        <span class="api-key-name">${escapeHTML(key.name)}</span>
        <span class="text-secondary">Created ${escapeHTML(shortDate(key.createdAt))} · Last used ${escapeHTML(shortDate(key.lastUsedAt))} · ${escapeHTML(status)}</span>
      </div>
      ${status === "Active" ? `<button type="button" class="btn btn-danger btn-sm" data-action="revoke-api-key" data-key-id="${escapeHTML(key.id)}" data-key-name="${escapeHTML(key.name)}"${busy ? " disabled" : ""}>Revoke</button>` : ""}
    </li>`;
  }).join("");
  return `<div class="card mt-3" data-testid="api-keys">
    <h3>API Keys</h3>
    <p class="text-secondary">Let a script add, view, edit, and delete activity logs as you, with the same permissions you have in the app. Keys can't change settings or chores. Changing your password turns off every key.</p>
    ${issuedBlock}
    <form id="create-api-key-form" data-action="create-api-key" class="mt-2">
      <div class="form-group">
        <label class="form-label" for="api-key-name">Key name</label>
        <input id="api-key-name" type="text" name="name" maxlength="60" required placeholder="e.g. Backfill script" autocomplete="off"${busy ? " disabled" : ""}>
      </div>
      <button type="submit" class="btn btn-secondary btn-sm"${busy ? " disabled" : ""}>Create key</button>
    </form>
    ${state.apiKeyError ? `<p role="alert" class="form-error" data-testid="api-key-error">${escapeHTML(state.apiKeyError)}</p>` : ""}
    ${keys.length ? `<ul class="api-key-list mt-2">${rows}</ul>` : ""}
    <p class="text-secondary mt-2">Send the key as <code>Authorization: Bearer &lt;key&gt;</code>, for example <code>curl -H "Authorization: Bearer &lt;key&gt;" ${escapeHTML(origin)}/api/chores</code>.</p>
  </div>`;
}
