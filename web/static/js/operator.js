const base = "/api/operator/v1";
const message = document.querySelector("#message");
let usersCursor = "";
let householdsCursor = "";

function csrfToken() {
  return document.cookie.match(/(?:^|;\s*)nabu_csrf=([^;]*)/)?.[1] || "";
}

async function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (options.body) headers.set("Content-Type", "application/json");
  if (options.method && options.method !== "GET") headers.set("X-CSRF-Token", csrfToken());
  const response = await fetch(base + path, { ...options, headers, cache: "no-store", credentials: "same-origin" });
  const data = response.headers.get("Content-Type")?.includes("application/json") ? await response.json() : null;
  if (!response.ok) throw new Error(data?.error || `Request failed (${response.status})`);
  return data;
}

function date(value) {
  return value ? new Date(value).toISOString().slice(0, 10) : "—";
}

function setMessage(value) { message.textContent = value; }

function cell(row, value, className = "") {
  const td = document.createElement("td");
  td.textContent = value;
  if (className) td.className = className;
  row.append(td);
  return td;
}

async function loadOverview() {
  const data = await request("/summary");
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
}

async function loadActivity() {
  const days = document.querySelector("#activity-days").value;
  const data = await request(`/activity?days=${days}`);
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
}

async function loadUsers(reset = false) {
  if (reset) { usersCursor = ""; document.querySelector("#users-body").replaceChildren(); }
  const data = await request(`/users?limit=50${usersCursor ? `&after=${usersCursor}` : ""}`);
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
}

async function loadHouseholds(reset = false) {
  if (reset) { householdsCursor = ""; document.querySelector("#households-body").replaceChildren(); }
  const data = await request(`/households?limit=50${householdsCursor ? `&after=${householdsCursor}` : ""}`);
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
}

async function loadKeys() {
  const data = await request("/keys");
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
}

async function loadAll() {
  setMessage("Loading…");
  try {
    await Promise.all([loadOverview(), loadActivity(), loadUsers(true), loadHouseholds(true), loadKeys()]);
    setMessage("");
  } catch (err) { setMessage(err.message); }
}

document.querySelector("#refresh").addEventListener("click", loadAll);
document.querySelector("#activity-days").addEventListener("change", () => loadActivity().catch(err => setMessage(err.message)));
document.querySelector("#more-users").addEventListener("click", () => loadUsers().catch(err => setMessage(err.message)));
document.querySelector("#more-households").addEventListener("click", () => loadHouseholds().catch(err => setMessage(err.message)));
document.querySelector("#key-form").addEventListener("submit", async event => {
  event.preventDefault();
  const form = event.currentTarget;
  const button = form.querySelector('button[type="submit"]');
  button.disabled = true;
  try {
    const values = new FormData(form);
    const data = await request("/keys", { method: "POST", body: JSON.stringify({
      name: String(values.get("name") || ""), scope: String(values.get("scope") || "summary"), days: Number(values.get("days")),
    }) });
    const token = document.querySelector("#token");
    token.value = data.token;
    document.querySelector("#issued-key").hidden = false;
    form.reset();
    setMessage("Key created. Copy it now; it cannot be recovered.");
    await loadKeys();
  } catch (err) { setMessage(err.message); }
  finally { button.disabled = false; }
});
document.querySelector("#keys-body").addEventListener("click", async event => {
  const id = event.target.closest("button[data-key-id]")?.dataset.keyId;
  if (!id || !window.confirm("Revoke this API key now?")) return;
  try { await request(`/keys/${encodeURIComponent(id)}`, { method: "DELETE" }); await loadKeys(); setMessage("Key revoked."); }
  catch (err) { setMessage(err.message); }
});
document.querySelector("#copy-key").addEventListener("click", async () => {
  try { await navigator.clipboard.writeText(document.querySelector("#token").value); setMessage("Key copied."); }
  catch { setMessage("Copy failed. Select the key and copy it manually."); }
});
document.querySelector("#dismiss-key").addEventListener("click", () => {
  document.querySelector("#token").value = "";
  document.querySelector("#issued-key").hidden = true;
  setMessage("");
});

loadAll();
