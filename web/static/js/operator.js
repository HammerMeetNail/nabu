import { bootstrapIdentity, contextIsCurrent, onExternalIdentityChange, storedIdentity } from "./browser-context.js";

const base = "/api/operator/v1";
const message = document.querySelector("#message");
const content = document.querySelector("main");
let usersCursor = "";
let householdsCursor = "";
let active = false;
let identity = null;
let revision = 0;
let overviewRequest = 0;
let activityRequest = 0;
let usersGeneration = 0;
let householdsGeneration = 0;
let keysRequest = 0;
let allRequest = 0;
let usersBusy = false;
let householdsBusy = false;

class ObsoleteRequest extends Error {}

function current(ticket) {
  return active && ticket === revision && identity?.status === "active" && contextIsCurrent(identity);
}

function clearSensitive(reason) {
  revision++;
  active = false;
  identity = null;
  overviewRequest++;
  activityRequest++;
  usersGeneration++;
  householdsGeneration++;
  keysRequest++;
  allRequest++;
  usersCursor = "";
  householdsCursor = "";
  usersBusy = false;
  householdsBusy = false;
  for (const selector of ["#overview", "#activity-chart", "#users-body", "#households-body", "#keys-body"]) {
    document.querySelector(selector).replaceChildren();
  }
  document.querySelector("#as-of").textContent = "";
  document.querySelector("#activity-chart").setAttribute("aria-label", "Daily registrations and log authors");
  document.querySelector("#overview").removeAttribute("aria-label");
  document.querySelector("#token").value = "";
  document.querySelector("#issued-key").hidden = true;
  document.querySelector("#key-form").reset();
  document.querySelector("#more-users").hidden = true;
  document.querySelector("#more-households").hidden = true;
  document.querySelector("#refresh").disabled = true;
  content.hidden = true;
  setMessage(reason);
}

function showError(err, ticket) {
  if (current(ticket) && !(err instanceof ObsoleteRequest)) setMessage(err.message);
}

async function ensureAuthority(ticket) {
  if (ticket !== revision || !active) throw new ObsoleteRequest();
  if (!contextIsCurrent(identity)) {
    clearSensitive("Account changed. Reload after signing in again.");
    throw new ObsoleteRequest();
  }
  let record;
  try { record = await storedIdentity(); }
  catch {
    if (ticket === revision) clearSensitive("Account status could not be confirmed. Reload after signing in again.");
    throw new ObsoleteRequest();
  }
  if (!current(ticket) || record?.status !== "active" || record.revision !== identity.revision) {
    if (ticket === revision) clearSensitive("Account status changed. Reload after signing in again.");
    throw new ObsoleteRequest();
  }
}

function csrfToken() {
  return document.cookie.match(/(?:^|;\s*)nabu_csrf=([^;]*)/)?.[1] || "";
}

async function request(path, options = {}) {
  const ticket = revision;
  await ensureAuthority(ticket);
  const headers = new Headers(options.headers || {});
  if (options.body) headers.set("Content-Type", "application/json");
  if (options.method && options.method !== "GET") headers.set("X-CSRF-Token", csrfToken());
  const response = await fetch(base + path, { ...options, headers, cache: "no-store", credentials: "same-origin" });
  await ensureAuthority(ticket);
  if (response.status === 401 || response.status === 403) {
    clearSensitive("Operator access changed. Return to Nabu and sign in again.");
    throw new ObsoleteRequest();
  }
  const data = response.headers.get("Content-Type")?.includes("application/json") ? await response.json() : null;
  await ensureAuthority(ticket);
  if (!response.ok) throw new Error(data?.error || `Request failed (${response.status})`);
  return data;
}

function date(value) {
  return value ? new Date(value).toISOString().slice(0, 10) : "—";
}

function setMessage(value) { message.textContent = value; }
function signalSettled(area) {
  window.dispatchEvent(new CustomEvent("operator-request-settled", { detail: area }));
}

function cell(row, value, className = "") {
  const td = document.createElement("td");
  td.textContent = value;
  if (className) td.className = className;
  row.append(td);
  return td;
}

async function loadOverview() {
  try {
    const ticket = revision, requestID = ++overviewRequest;
    let data;
    try { data = await request("/summary"); }
    catch (err) { if (!current(ticket) || requestID !== overviewRequest) return; throw err; }
    if (!current(ticket) || requestID !== overviewRequest) return;
    document.querySelector("#as-of").textContent = `As of ${new Date(data.asOf).toLocaleString()}`;
    const items = [
      [data.registeredUsers, "Registered users"],
      [data.verifiedUsers, "Verified users"],
      [data.households, "Households"],
      [data.authors7Days, "Log authors · 7 days"],
      [data.authors30Days, "Log authors · 30 days"],
      [data.activeHouseholds30Days, "Active households · 30 days"],
    ];
    const root = document.querySelector("#overview");
    root.replaceChildren();
    for (const [value, label] of items) {
      const item = document.createElement("div");
      item.className = "card";
      const number = document.createElement("strong");
      number.textContent = String(value);
      const text = document.createElement("span");
      text.textContent = label;
      item.append(number, text);
      root.append(item);
    }
    const completeness = data.logs30Days ? Math.round(data.logsWithActor30Days / data.logs30Days * 100) : 100;
    root.setAttribute("aria-label", `${completeness}% of logs in the last 30 days have creator identity`);
  } finally { signalSettled("overview"); }
}

async function loadActivity() {
  try {
    const ticket = revision, requestID = ++activityRequest;
    const days = document.querySelector("#activity-days").value;
    let data;
    try { data = await request(`/activity?days=${days}`); }
    catch (err) { if (!current(ticket) || requestID !== activityRequest || document.querySelector("#activity-days").value !== days) return; throw err; }
    if (!current(ticket) || requestID !== activityRequest || document.querySelector("#activity-days").value !== days) return;
    const chart = document.querySelector("#activity-chart");
    chart.replaceChildren();
    const max = Math.max(1, ...data.days.flatMap(day => [day.registrations, day.authors]));
    for (const day of data.days) {
      const group = document.createElement("div");
      group.className = "chart-day";
      group.title = `${day.date}: ${day.registrations} registrations, ${day.authors} log authors`;
      for (const [name, value] of [["registrations", day.registrations], ["authors", day.authors]]) {
        const bar = document.createElement("div");
        bar.className = `bar ${name}`;
        bar.style.height = `${Math.max(value ? 3 : 1, value / max * 100)}%`;
        group.append(bar);
      }
      chart.append(group);
    }
    chart.setAttribute("aria-label", `${days}-day chart of daily registrations and distinct log authors`);
  } finally { signalSettled("activity"); }
}

async function loadUsers(reset = false) {
  const ticket = revision;
  if (reset) {
    usersGeneration++;
    usersCursor = "";
    usersBusy = false;
    document.querySelector("#users-body").replaceChildren();
    document.querySelector("#more-users").hidden = true;
  }
  if (usersBusy) return;
  const generation = usersGeneration, cursor = usersCursor;
  usersBusy = true;
  document.querySelector("#more-users").disabled = true;
  try {
    const data = await request(`/users?limit=50${cursor ? `&after=${cursor}` : ""}`);
    if (!current(ticket) || generation !== usersGeneration) return;
    const body = document.querySelector("#users-body");
    for (const user of data.users) {
      const row = document.createElement("tr");
      cell(row, user.email);
      cell(row, date(user.registeredAt));
      cell(row, user.emailVerified ? "Yes" : "No");
      cell(row, `${user.authored7Days} / ${user.authored30Days}`, "number");
      cell(row, String(user.householdLogs30Days), "number");
      cell(row, date(user.lastAuthoredAt));
      cell(row, date(user.lastSessionSeenAt));
      body.append(row);
    }
    usersCursor = data.nextCursor || "";
    document.querySelector("#more-users").hidden = !usersCursor;
  } catch (err) {
    if (!current(ticket) || generation !== usersGeneration) return;
    throw err;
  } finally {
    if (current(ticket) && generation === usersGeneration) {
      usersBusy = false;
      document.querySelector("#more-users").disabled = false;
    }
    signalSettled("users");
  }
}

async function loadHouseholds(reset = false) {
  const ticket = revision;
  if (reset) {
    householdsGeneration++;
    householdsCursor = "";
    householdsBusy = false;
    document.querySelector("#households-body").replaceChildren();
    document.querySelector("#more-households").hidden = true;
  }
  if (householdsBusy) return;
  const generation = householdsGeneration, cursor = householdsCursor;
  householdsBusy = true;
  document.querySelector("#more-households").disabled = true;
  try {
    const data = await request(`/households?limit=50${cursor ? `&after=${cursor}` : ""}`);
    if (!current(ticket) || generation !== householdsGeneration) return;
    const body = document.querySelector("#households-body");
    for (const household of data.households) {
      const row = document.createElement("tr");
      cell(row, household.name);
      cell(row, date(household.createdAt));
      cell(row, String(household.members), "number");
      cell(row, String(household.logs30Days), "number");
      cell(row, date(household.lastLogAt));
      body.append(row);
    }
    householdsCursor = data.nextCursor || "";
    document.querySelector("#more-households").hidden = !householdsCursor;
  } catch (err) {
    if (!current(ticket) || generation !== householdsGeneration) return;
    throw err;
  } finally {
    if (current(ticket) && generation === householdsGeneration) {
      householdsBusy = false;
      document.querySelector("#more-households").disabled = false;
    }
    signalSettled("households");
  }
}

async function loadKeys() {
  try {
    const ticket = revision, requestID = ++keysRequest;
    let data;
    try { data = await request("/keys"); }
    catch (err) { if (!current(ticket) || requestID !== keysRequest) return; throw err; }
    if (!current(ticket) || requestID !== keysRequest) return;
    const body = document.querySelector("#keys-body");
    body.replaceChildren();
    for (const key of data.keys) {
      const row = document.createElement("tr");
      cell(row, key.name);
      cell(row, key.scope === "full" ? "Full" : "Summary");
      cell(row, date(key.expiresAt));
      cell(row, date(key.lastUsedAt));
      const status = key.revokedAt ? "Revoked" : key.invalidated ? "Invalidated" : new Date(key.expiresAt) <= new Date() ? "Expired" : "Active";
      cell(row, status);
      const action = document.createElement("td");
      if (status === "Active") {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "danger";
        button.textContent = "Revoke";
        button.dataset.keyId = key.id;
        action.append(button);
      }
      row.append(action);
      body.append(row);
    }
  } finally { signalSettled("keys"); }
}

async function loadAll() {
  const ticket = revision, requestID = ++allRequest;
  setMessage("Loading…");
  try {
    await Promise.all([loadOverview(), loadActivity(), loadUsers(true), loadHouseholds(true), loadKeys()]);
    if (current(ticket) && requestID === allRequest) setMessage("");
  } catch (err) { if (requestID === allRequest) showError(err, ticket); }
  finally { signalSettled("all"); }
}

document.querySelector("#refresh").addEventListener("click", loadAll);
document.querySelector("#activity-days").addEventListener("change", () => {
  const ticket = revision;
  loadActivity().catch(err => showError(err, ticket));
});
document.querySelector("#more-users").addEventListener("click", () => {
  const ticket = revision;
  loadUsers().catch(err => showError(err, ticket));
});
document.querySelector("#more-households").addEventListener("click", () => {
  const ticket = revision;
  loadHouseholds().catch(err => showError(err, ticket));
});
document.querySelector("#key-form").addEventListener("submit", async event => {
  event.preventDefault();
  const ticket = revision;
  if (!current(ticket)) return;
  const form = event.currentTarget;
  const button = form.querySelector('button[type="submit"]');
  button.disabled = true;
  try {
    const values = new FormData(form);
    keysRequest++; // Any key mutation supersedes earlier list snapshots.
    const data = await request("/keys", { method: "POST", body: JSON.stringify({
      name: String(values.get("name") || ""), scope: String(values.get("scope") || "summary"), days: Number(values.get("days")),
    }) });
    if (!current(ticket)) return;
    const token = document.querySelector("#token");
    token.value = data.token;
    document.querySelector("#issued-key").hidden = false;
    const draft = new FormData(form);
    if (["name", "scope", "days"].every(name => draft.get(name) === values.get(name))) form.reset();
    setMessage("Key created. Copy it now; it cannot be recovered.");
    await loadKeys();
  } catch (err) { showError(err, ticket); }
  finally { if (current(ticket)) button.disabled = false; signalSettled("issue-key"); }
});
document.querySelector("#keys-body").addEventListener("click", async event => {
  const id = event.target.closest("button[data-key-id]")?.dataset.keyId;
  if (!id || !window.confirm("Revoke this API key now?")) return;
  const ticket = revision;
  try {
    keysRequest++;
    await request(`/keys/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (!current(ticket)) return;
    await loadKeys();
    if (current(ticket)) setMessage("Key revoked.");
  } catch (err) { showError(err, ticket); }
});
document.querySelector("#copy-key").addEventListener("click", async () => {
  const ticket = revision;
  if (!current(ticket)) return;
  try {
    await navigator.clipboard.writeText(document.querySelector("#token").value);
    if (current(ticket)) setMessage("Key copied.");
  } catch { if (current(ticket)) setMessage("Copy failed. Select the key and copy it manually."); }
});
document.querySelector("#dismiss-key").addEventListener("click", () => {
  document.querySelector("#token").value = "";
  document.querySelector("#issued-key").hidden = true;
  setMessage("");
});

onExternalIdentityChange(record => {
  const reason = record?.status === "logout-pending" ? "Sign-out is unfinished. Return to Nabu to retry." :
    "Account changed. Reload this page after signing in again.";
  clearSensitive(reason);
});

async function verifyAuthority() {
  if (!active) return;
  const ticket = revision;
  try {
    const record = await storedIdentity();
    if (ticket === revision && (!record || record.status !== "active" || record.revision !== identity?.revision || !contextIsCurrent(identity))) {
      clearSensitive("Account status could not be confirmed. Reload after signing in again.");
    }
  } catch {
    if (ticket === revision) clearSensitive("Account status could not be confirmed. Reload after signing in again.");
  }
}

window.addEventListener("focus", () => { void verifyAuthority(); });
document.addEventListener("visibilitychange", () => { if (!document.hidden) void verifyAuthority(); });
window.addEventListener("pagehide", () => clearSensitive("Reload after signing in again."));

async function start() {
  const ticket = revision;
  try {
    const result = await bootstrapIdentity();
    if (ticket !== revision) return;
    if (result.logoutPending) {
      clearSensitive("Sign-out is unfinished. Return to Nabu to retry.");
      return;
    }
    if (!result.user || !result.identity || result.identity.status !== "active" || !contextIsCurrent(result.identity)) {
      clearSensitive("Sign in to view the operator dashboard.");
      return;
    }
    identity = result.identity;
    active = true;
    content.hidden = false;
    document.querySelector("#refresh").disabled = false;
    await loadAll();
  } catch {
    if (ticket === revision) clearSensitive("Account status could not be confirmed. Return to Nabu to retry.");
  }
}

void start();
