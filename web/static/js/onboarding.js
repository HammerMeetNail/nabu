// Setup for a signed-in user without a household (improvement plan, Phase 2):
// their name, the household's name, then who and what they look after, which
// picks the activity packs to seed. The household's invite link comes last.
//
// The draft lives in state.onboarding and every field renders from it: a
// re-render resets any input that isn't focused.

import { escapeHTML } from "./utils.js";
import { PROFILE_COLORS } from "./profile.js";

const STEPS = ["name", "household", "packs"];
export const MAX_PACK_NAMES = 6;

/** A new draft. The server names new accounts after their email, which is no
 *  name to prefill. */
export function createOnboarding(user) {
  const emailName = String(user?.email || "").split("@")[0];
  const name = user?.displayName && user.displayName !== emailName ? user.displayName : "";
  const current = String(user?.avatarColor || "").toUpperCase();
  const color = PROFILE_COLORS.some(([hex]) => hex === current) ? current : PROFILE_COLORS[0][0];
  return { step: "name", name, color, household: "", initials: "", picks: {}, error: "", saving: false };
}

/** "Dana O'Connor" → "The O'Connors' home"; "Dana" → "Dana's home". */
export function householdNameFor(name) {
  const words = String(name || "").trim().split(/\s+/).filter(Boolean);
  if (!words.length) return "";
  const last = words[words.length - 1];
  const suggestion = words.length === 1 ? `${last}'s home`
    : `The ${/(s|x|z|ch|sh)$/i.test(last) ? `${last}es` : `${last}s`}' home`;
  return [...suggestion].length <= 60 ? suggestion : "";
}

/** The packs to send to POST /api/chores/packs, in the catalog's order. Blank
 *  names are left out. */
export function onboardingPicks(draft, packs) {
  return (packs || []).filter(p => draft.picks[p.key]?.on).map(p => {
    const names = p.nameLabel ? (draft.picks[p.key].names || []).map(n => n.trim()).filter(Boolean) : [];
    return names.length ? { key: p.key, names } : { key: p.key };
  });
}

/** The first name given twice across the picks, ignoring case, or "". Names
 *  become activity names, so each must differ (the server checks too). */
export function duplicateName(picks) {
  const seen = new Set();
  for (const name of picks.flatMap(p => p.names || [])) {
    if (seen.has(name.toLowerCase())) return name;
    seen.add(name.toLowerCase());
  }
  return "";
}

function stepLabel(step) {
  return `<p class="onboarding-step">Step ${STEPS.indexOf(step) + 1} of ${STEPS.length}</p>`;
}

function errorBox(draft) {
  return `<div class="form-error${draft.error ? "" : " hidden"}" role="alert">${escapeHTML(draft.error)}</div>`;
}

function renderNameStep(draft) {
  return `${stepLabel("name")}
    <h2>Welcome to Nabu</h2>
    <form id="onboarding-name-form" data-action="onboarding-name" novalidate>
      <div class="form-group">
        <label class="form-label" for="onboarding-name">What should your household call you?</label>
        <input id="onboarding-name" data-onboarding-field="name" required maxlength="50" autocomplete="name" placeholder="Your name" value="${escapeHTML(draft.name)}">
      </div>
      <fieldset class="form-group profile-colors">
        <legend class="form-label">Your color</legend>
        ${PROFILE_COLORS.map(([hex, label]) => `<label class="profile-color" title="${label}">
          <input type="radio" name="onboarding-color" value="${hex}" data-onboarding-field="color"${hex === draft.color ? " checked" : ""}>
          <span class="profile-color-swatch" style="background:${hex}" aria-hidden="true"></span>
          <span class="sr-only">${label}</span>
        </label>`).join("")}
      </fieldset>
      ${errorBox(draft)}
      <button type="submit" class="btn btn-primary btn-block"${draft.saving ? " disabled" : ""}>${draft.saving ? "Saving…" : "Continue"}</button>
    </form>
    <button type="button" class="btn btn-ghost btn-block mt-2" data-action="onboarding-step" data-step="join">Joining someone? Enter an invite code</button>`;
}

function renderHouseholdStep(draft) {
  return `${stepLabel("household")}
    <h2>Name your household</h2>
    <form id="onboarding-household-form" data-action="onboarding-household" novalidate>
      <div class="form-group">
        <label class="form-label" for="onboarding-household">Household name</label>
        <input id="onboarding-household" data-onboarding-field="household" required maxlength="60" placeholder="Our home" value="${escapeHTML(draft.household)}">
      </div>
      <div class="form-group">
        <label class="form-label" for="onboarding-initials">Initials <span class="form-label-hint">(shown in the header)</span></label>
        <input id="onboarding-initials" data-onboarding-field="initials" maxlength="4" autocomplete="off" class="hh-initials-input" value="${escapeHTML(draft.initials)}">
      </div>
      ${errorBox(draft)}
      <div class="onboarding-actions">
        <button type="button" class="btn btn-secondary" data-action="onboarding-step" data-step="name">Back</button>
        <button type="submit" class="btn btn-primary">Continue</button>
      </div>
    </form>`;
}

function renderPackOption(pack, pick) {
  const on = !!pick?.on;
  const names = pick?.names?.length ? pick.names : [""];
  const key = escapeHTML(pack.key), label = escapeHTML(pack.nameLabel || "");
  const nameFields = on && pack.nameLabel ? `<div class="pack-names">
      ${names.map((name, i) => `<input id="pack-${key}-name-${i}" class="pack-name-input" data-onboarding-pack-name="${key}" data-index="${i}"
        maxlength="30" autocomplete="off" aria-label="${label}${names.length > 1 ? ` ${i + 1}` : ""}" placeholder="${label} (optional)" value="${escapeHTML(name)}">`).join("")}
      ${names.length < MAX_PACK_NAMES ? `<button type="button" class="btn btn-ghost btn-sm" data-action="onboarding-add-name" data-pack="${key}">Add another</button>` : ""}
    </div>` : "";
  return `<div class="pack-option${on ? " pack-option--on" : ""}">
    <label class="pack-toggle">
      <input type="checkbox" data-onboarding-pack="${key}"${on ? " checked" : ""}>
      <span class="pack-icon" aria-hidden="true">${escapeHTML(pack.icon)}</span>
      <span class="pack-text"><span class="pack-title">${escapeHTML(pack.title)}</span><span class="pack-summary">${escapeHTML(pack.summary)}</span></span>
    </label>
    ${nameFields}
  </div>`;
}

/** The pack picker, shared by setup and Manage → Add activities. */
export function renderPackOptions(packs, picks) {
  return `<div class="pack-options">${packs.map(p => renderPackOption(p, picks[p.key])).join("")}</div>`;
}

function renderPackBody(state, picks) {
  if (state.packs) return renderPackOptions(state.packs, picks);
  return state.packsError
    ? `<p class="text-secondary">Couldn't load the activities.</p><button type="button" class="btn btn-secondary" data-action="onboarding-load-packs">Retry</button>`
    : `<p class="text-secondary" role="status">Loading…</p>`;
}

/** Manage → Add activities: the same picker for a household that exists. */
export function renderPacksSheet(state) {
  const draft = state.activeSheetData;
  return `<div class="bottom-sheet packs-sheet">
    <div class="sheet-handle"></div>
    <div class="sheet-title">Add activities</div>
    <p class="text-secondary">Add a pack, or name a new baby or pet to give them their own. Activities you have already are kept as they are.</p>
    <form id="packs-sheet-form" data-action="packs-add">
      ${renderPackBody(state, draft.picks)}
      ${errorBox(draft)}
      <button type="submit" class="btn btn-primary btn-block mt-2"${draft.saving || !state.packs ? " disabled" : ""}>${draft.saving ? "Adding…" : "Add"}</button>
      <button type="button" class="btn btn-secondary btn-block mt-2" data-action="close-sheet">Cancel</button>
    </form>
  </div>`;
}

function renderPacksStep(state, draft) {
  const packs = state.packs;
  const body = renderPackBody(state, draft.picks);
  return `${stepLabel("packs")}
    <h2>Who and what do you look after?</h2>
    <p class="text-secondary">Pick everything that applies. You can add more later in Manage.</p>
    <form id="onboarding-packs-form" data-action="onboarding-create">
      ${body}
      ${errorBox(draft)}
      <div class="onboarding-actions">
        ${draft.householdReady ? "" : `<button type="button" class="btn btn-secondary" data-action="onboarding-step" data-step="household">Back</button>`}
        <button type="submit" class="btn btn-primary"${draft.saving || !packs ? " disabled" : ""}>${draft.saving ? "Setting up…" : draft.householdReady ? "Add activities" : "Create household"}</button>
      </div>
    </form>`;
}

function renderInviteStep(state) {
  const code = state.household?.inviteCode || "";
  const link = `${window.location.origin}/join?code=${encodeURIComponent(code)}`;
  const canShare = typeof navigator !== "undefined" && typeof navigator.share === "function";
  return `<h2>Invite your household</h2>
    <p>Nabu works best when everyone logs. Send this link to the people you share the care with.</p>
    <div class="invite-link-row"><code class="invite-link-url">${escapeHTML(link)}</code></div>
    <div class="onboarding-actions">
      ${canShare ? `<button type="button" class="btn btn-primary" data-action="onboarding-share" data-code="${escapeHTML(code)}">Share link</button>` : ""}
      <button type="button" class="btn btn-secondary" data-action="copy-invite-link" data-code="${escapeHTML(code)}">Copy link</button>
    </div>
    <button type="button" class="btn btn-ghost btn-block mt-2" data-action="onboarding-done">Go to Home</button>`;
}

function renderJoinStep() {
  return `<h2>Join a household</h2>
    <p class="text-secondary">Enter the code from your invite, or open the invite link on this device.</p>
    <form id="join-household-form" data-action="join-household">
      <div class="form-group">
        <label class="form-label" for="invite-code">Invite code</label>
        <input id="invite-code" type="text" name="inviteCode" required placeholder="ABCDEF" autocomplete="off">
      </div>
      <button type="submit" class="btn btn-primary btn-block">Join household</button>
    </form>
    <button type="button" class="btn btn-ghost btn-block mt-2" data-action="onboarding-step" data-step="name">Set up a new household instead</button>`;
}

export function renderOnboarding(state) {
  const draft = state.onboarding;
  let body;
  switch (draft.step) {
    case "household": body = renderHouseholdStep(draft); break;
    case "packs": body = renderPacksStep(state, draft); break;
    case "invite": body = renderInviteStep(state); break;
    case "join": body = renderJoinStep(); break;
    default: body = renderNameStep(draft);
  }
  return `<div class="card mt-3 onboarding" data-onboarding-step="${escapeHTML(draft.step)}">${body}</div>`;
}
