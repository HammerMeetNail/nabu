// The household display: a shared tablet (in the kitchen, say) shows Home's
// tiles large, under a row of the household's members. Someone taps who they
// are, then what they did, and it's logged at once for them, with Undo. An
// owner or admin turns it on in Settings with a PIN that only this device
// keeps; the PIN is what leaves it. While it's on there are no tabs, no
// Settings and no switching accounts.
import { registerActions } from "./actions.js";
import { state, rerender, owned, afterDirectLog } from "./app-core.js";
import { formatTimeAgo } from "./home.js";
import { sortChoresByOrder } from "./preferences.js";
import { logChore, todayISO } from "./today.js";
import { showToast } from "./toast.js";
import { escapeHTML } from "./utils.js";

// Who did it resets after two quiet minutes, so the next person can't log as
// the last one by mistake.
export const PICK_IDLE_MS = 2 * 60 * 1000;
// Wrong PINs before leaving pauses, and for how long.
export const MAX_PIN_TRIES = 5;
export const PIN_PAUSE_MS = 30 * 1000;
const PIN_PATTERN = /^\d{4,8}$/;

// One display setting per account and household on this device.
export function displayKey(userId, householdId) {
  return `nabu-display:${userId}:${householdId}`;
}

function key(s) {
  return s?.user && s?.household ? displayKey(s.user.id, s.household.id) : null;
}

function storedSetting(s) {
  const k = key(s);
  if (!k) return null;
  try {
    const value = JSON.parse(localStorage.getItem(k) || "null");
    return value && typeof value.salt === "string" && typeof value.hash === "string" ? value : null;
  } catch {
    return null;
  }
}

// Whether this device shows the household display for the signed-in account.
export function displayActive(s = state) {
  return !!storedSetting(s);
}

// Whether the account may turn a device into a display: owners and admins.
export function canManageDisplay(s = state) {
  const role = s.members?.find(m => m.userId === s.user?.id)?.role || s.user?.role;
  return !!s.household && (role === "owner" || role === "admin");
}

function hex(bytes) {
  return [...new Uint8Array(bytes)].map(b => b.toString(16).padStart(2, "0")).join("");
}

// A salted SHA-256 of the PIN: what this device keeps instead of the PIN.
export async function hashPIN(pin, salt) {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(`${salt}:${pin}`));
  return hex(digest);
}

export function validPIN(pin) {
  return PIN_PATTERN.test(pin);
}

// One try at the PIN that leaves the display: whether it may leave, and what
// to say when it may not. `attempts` is kept with the setting, so neither
// closing the dialog nor reloading resets it: after MAX_PIN_TRIES wrong ones,
// even the right PIN waits out the pause.
export function tryLeave(attempts, correct, now = Date.now()) {
  if ((attempts.pausedUntil || 0) > now) {
    return { left: false, error: `Too many tries. Wait ${Math.ceil((attempts.pausedUntil - now) / 1000)} seconds.` };
  }
  if (correct) return { left: true, error: null };
  attempts.tries = (attempts.tries || 0) + 1;
  if (attempts.tries < MAX_PIN_TRIES) return { left: false, error: "That PIN isn't right." };
  attempts.tries = 0;
  attempts.pausedUntil = now + PIN_PAUSE_MS;
  return { left: false, error: `Too many tries. Wait ${PIN_PAUSE_MS / 1000} seconds.` };
}

function freshDisplay() {
  return { pickedUserId: null, pickedAt: 0, needsPick: false, choosing: null, leaving: null };
}

function ui() {
  state.display ||= freshDisplay();
  return state.display;
}

// Members who can be picked: everyone whose access isn't suspended.
function members(s) {
  return (s.members || []).filter(m => !m.suspended);
}

function memberName(m) {
  return m.displayName || m.email || "Someone";
}

function picked(s, now = Date.now()) {
  const d = s.display;
  if (!d?.pickedUserId || now - d.pickedAt > PICK_IDLE_MS) return null;
  return members(s).find(m => m.userId === d.pickedUserId) || null;
}

// The display's tiles: Home's, in Home's order, without hidden ones or
// Admins-only activities, since anyone in the room can see the display.
export function displayChores(s = state) {
  const hidden = new Set(s.hiddenHomeChoreIDs || []);
  return sortChoresByOrder(s.chores || [], s.choreOrder || [])
    .filter(c => !hidden.has(c.id) && c.visibility !== "admins");
}

export function renderDisplayView(s = state) {
  const d = s.display || freshDisplay();
  const who = picked(s);
  const latestLogs = s.latestLogs || {};
  const people = members(s).map(m => {
    const name = memberName(m);
    const on = who?.userId === m.userId;
    return `<button type="button" class="display-person${on ? " display-person--picked" : ""}" data-key="display-person-${m.userId}"
      data-action="display-pick" data-user-id="${m.userId}" aria-pressed="${on}">
      <span class="avatar-circle display-avatar" style="background:${escapeHTML(m.avatarColor || "#19323C")}" aria-hidden="true">${escapeHTML(name.charAt(0).toUpperCase())}</span>
      <span class="display-person-name">${escapeHTML(name)}</span>
    </button>`;
  }).join("");
  const tiles = displayChores(s).map(chore => {
    const latest = latestLogs[chore.id];
    const timeAgo = latest ? formatTimeAgo(latest.completedAt) : "";
    return `<button type="button" class="home-chore-card display-tile" data-key="display-chore-${chore.id}"
      data-home-chore-id="${chore.id}" data-action="display-log" data-chore-id="${chore.id}" style="--chore-color:${escapeHTML(chore.color)}">
      <span class="home-card-icon">${escapeHTML(chore.icon)}</span>
      <span class="home-card-name">${escapeHTML(chore.name)}</span>
      ${timeAgo ? `<span class="home-card-time">${escapeHTML(timeAgo)}</span>` : `<span class="home-card-time home-card-time--never">never</span>`}
    </button>`;
  }).join("");
  const prompt = who
    ? `Logging for ${escapeHTML(memberName(who))}`
    : d.needsPick ? "Tap who you are first" : "Tap who you are, then what you did";
  return `<div class="display-view" data-key="display">
    <div class="display-bar">
      <div class="display-people" role="group" aria-label="Who did it">${people}</div>
      <button type="button" class="display-leave" data-action="display-leave-open" aria-label="Leave the household display">🔒</button>
    </div>
    <p class="display-prompt${!who && d.needsPick ? " display-prompt--needed" : ""}" role="status" data-testid="display-prompt">${prompt}</p>
    <div class="home-grid display-grid">${tiles}</div>
    <div class="display-overlay" data-key="display-overlay"${d.choosing || d.leaving ? "" : " hidden"}>${renderOverlay(s, d)}</div>
  </div>`;
}

function renderOverlay(s, d) {
  if (d.choosing) {
    const chore = (s.chores || []).find(c => c.id === d.choosing);
    if (!chore) return "";
    const options = (chore.subjects || []).map(subject => `<button type="button" class="btn btn-primary btn-block" data-action="display-log-subject"
      data-chore-id="${chore.id}" data-subject="${escapeHTML(subject)}">${escapeHTML(subject)}</button>`).join("");
    return `<div class="display-dialog" role="dialog" aria-modal="true" aria-labelledby="display-choose-title">
      <h2 id="display-choose-title">${escapeHTML(chore.icon)} ${escapeHTML(chore.name)}: which one?</h2>
      <div class="display-choices">${options}</div>
      <button type="button" class="btn btn-secondary btn-block mt-2" data-action="display-cancel">Cancel</button>
    </div>`;
  }
  if (d.leaving) {
    return `<form class="display-dialog" role="dialog" aria-modal="true" aria-labelledby="display-leave-title" data-action="display-leave" novalidate>
      <h2 id="display-leave-title">Leave the household display</h2>
      <div class="form-group mt-2">
        <label class="form-label" for="display-leave-pin">PIN</label>
        <input id="display-leave-pin" type="password" inputmode="numeric" autocomplete="off" maxlength="8">
        <p class="form-error" role="alert" data-testid="display-leave-error">${escapeHTML(d.leaving.error || "")}</p>
      </div>
      <div class="display-dialog-actions">
        <button type="button" class="btn btn-secondary" data-action="display-cancel">Cancel</button>
        <button type="submit" class="btn btn-primary">Leave</button>
      </div>
    </form>`;
  }
  return "";
}

// Settings' card, for owners and admins.
export function renderDisplaySettings(s = state) {
  if (!canManageDisplay(s)) return "";
  const setup = s.displaySetup;
  const form = setup ? `<form class="display-setup mt-2" data-action="display-turn-on" novalidate>
      <div class="form-group">
        <label class="form-label" for="display-pin">Choose a PIN, 4 to 8 digits</label>
        <input id="display-pin" type="password" inputmode="numeric" autocomplete="new-password" maxlength="8">
      </div>
      <div class="form-group">
        <label class="form-label" for="display-pin-again">Enter it again</label>
        <input id="display-pin-again" type="password" inputmode="numeric" autocomplete="new-password" maxlength="8">
        <p class="form-error" role="alert" data-testid="display-setup-error">${escapeHTML(setup.error || "")}</p>
      </div>
      <div class="display-dialog-actions">
        <button type="button" class="btn btn-secondary btn-sm" data-action="display-setup-cancel">Cancel</button>
        <button type="submit" class="btn btn-primary btn-sm">Turn on</button>
      </div>
    </form>`
    : `<button type="button" class="btn btn-secondary btn-sm mt-2" data-action="display-setup-open">Use this device as a household display</button>`;
  return `<div class="card mt-3" data-key="household-display">
    <h3>Household display</h3>
    <p class="text-secondary">For a shared tablet: big tiles and everyone in the household to pick from, and one tap logs. Admins-only activities stay off it. Only this device keeps the PIN, and only the PIN leaves it.</p>
    ${form}
  </div>`;
}

let pickTimer = null;

function pick(userId) {
  const d = ui();
  d.pickedUserId = userId;
  d.pickedAt = Date.now();
  d.needsPick = false;
  clearTimeout(pickTimer);
  const at = d.pickedAt;
  pickTimer = setTimeout(owned(() => {
    if (state.display?.pickedAt === at) { state.display.pickedUserId = null; rerender(); }
  }), PICK_IDLE_MS);
}

function logFor(choreId, subject = null) {
  const d = ui();
  const who = picked(state);
  const chore = (state.chores || []).find(c => c.id === choreId);
  if (!chore) return;
  if (!who) { d.needsPick = true; d.choosing = null; rerender(); return; }
  if (subject === null && chore.subjects?.length) { d.choosing = choreId; rerender(); return; }
  d.choosing = null;
  pick(who.userId);
  rerender();
  const now = new Date();
  const label = `${chore.icon} ${chore.name}${subject ? ` (${subject})` : ""} for ${memberName(who)}`;
  logChore(choreId, "", todayISO(), chore.indicatorDefaults || [], now.getHours(), now.toISOString(), null, who.userId,
    {}, 0, null, null, null, null, subject)
    .then(owned(data => afterDirectLog(choreId, data, label)))
    .catch(owned(err => showToast(err.message || "Failed to log chore", "error")));
}

registerActions("click", {
  "display-pick": (e, el) => {
    e.preventDefault();
    pick(parseInt(el.dataset.userId, 10));
    rerender();
  },
  "display-log": (e, el) => {
    e.preventDefault();
    logFor(parseInt(el.dataset.choreId, 10));
  },
  "display-log-subject": (e, el) => {
    e.preventDefault();
    logFor(parseInt(el.dataset.choreId, 10), el.dataset.subject);
  },
  "display-leave-open": (e) => {
    e.preventDefault();
    ui().leaving = { error: null };
    rerender();
    document.querySelector("#display-leave-pin")?.focus();
  },
  "display-cancel": (e) => {
    e.preventDefault();
    const d = ui();
    d.choosing = null;
    d.leaving = null;
    rerender();
  },
  "display-setup-open": (e) => {
    e.preventDefault();
    state.displaySetup = { error: null };
    rerender();
    document.querySelector("#display-pin")?.focus();
  },
  "display-setup-cancel": (e) => {
    e.preventDefault();
    state.displaySetup = null;
    rerender();
  },
});

registerActions("submit", {
  "display-turn-on": async (e) => {
    e.preventDefault();
    const pin = document.querySelector("#display-pin")?.value || "";
    const again = document.querySelector("#display-pin-again")?.value || "";
    const k = key(state);
    const fail = (error) => { state.displaySetup = { error }; rerender(); };
    if (!k || !canManageDisplay(state)) return fail("Only owners and admins can turn on the household display.");
    if (!validPIN(pin)) return fail("Use 4 to 8 digits.");
    if (pin !== again) return fail("The PINs don't match.");
    if (!globalThis.crypto?.subtle) return fail("Turning on the display needs a secure (HTTPS) connection.");
    const salt = hex(crypto.getRandomValues(new Uint8Array(16)));
    const hash = await hashPIN(pin, salt);
    if (key(state) !== k) return;
    try {
      localStorage.setItem(k, JSON.stringify({ salt, hash }));
    } catch {
      return fail("This browser couldn't save the setting. Check that it allows site data.");
    }
    state.displaySetup = null;
    state.display = freshDisplay();
    rerender();
  },
  "display-leave": async (e) => {
    e.preventDefault();
    const d = ui();
    const setting = storedSetting(state);
    if (!d.leaving || !setting) return;
    const pin = document.querySelector("#display-leave-pin")?.value || "";
    const k = key(state);
    const correct = validPIN(pin) && await hashPIN(pin, setting.salt) === setting.hash;
    if (key(state) !== k || state.display !== d || !d.leaving) return;
    const attempts = { tries: setting.tries || 0, pausedUntil: setting.pausedUntil || 0 };
    const { left, error } = tryLeave(attempts, correct);
    if (left) {
      try { localStorage.removeItem(k); } catch { /* A setting that stays keeps the display on. */ }
      state.display = freshDisplay();
      rerender();
      return;
    }
    try { localStorage.setItem(k, JSON.stringify({ ...setting, ...attempts })); } catch { /* The pause then lasts until a reload. */ }
    d.leaving.error = error;
    rerender();
    document.querySelector("#display-leave-pin")?.focus();
  },
});
