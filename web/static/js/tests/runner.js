import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import { readFileSync } from "node:fs";

const dom = new JSDOM("<!doctype html><html><body></body></html>", {
  url: "http://localhost:8080",
});

globalThis.document = dom.window.document;
globalThis.Node = dom.window.Node;
Object.defineProperty(globalThis, "navigator", {
  value: { onLine: true },
  writable: true,
  configurable: true,
});
globalThis.fetch = async () => ({
  ok: true,
  json: async () => ({}),
  headers: new Map(),
});

describe("State", () => {
  it("creates state", async () => {
    const { createAppState } = await import("../state.js");
    const state = createAppState();
    assert.equal(state.user, null);
    assert.equal(state.networkOnline, true);
    assert.deepEqual(state.chores, []);
    assert.deepEqual(state.schedules, []);
    assert.equal(state.calendarView, "day");
    assert.equal(state.calendarDate, null);
  });

  it("resets authed state", async () => {
    const { createAppState, resetAuthedState } = await import("../state.js");
    const state = createAppState();
    state.user = { email: "test@example.com" };
    state.schedules = [{ id: 1 }];
    state.calendarView = "week";
    resetAuthedState(state);
    assert.equal(state.user, null);
    assert.deepEqual(state.schedules, []);
    assert.equal(state.calendarView, "day");
    assert.equal(state.calendarDate, null);
  });
});

describe("DOM Morphing", () => {
  it("morphInnerHTML updates root element", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const root = dom.window.document.createElement("div");
    root.innerHTML = "<p>Hello</p>";
    morphInnerHTML(root, "<p>World</p>");
    assert.equal(root.textContent, "World");
  });

  it("morphInnerHTML preserves attributes", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const root = dom.window.document.createElement("div");
    root.innerHTML = '<p class="old">Text</p>';
    morphInnerHTML(root, '<p class="new">Changed</p>');
    assert.equal(root.textContent, "Changed");
  });

  it("keeps a <details> someone opened open, and opens one the markup opens", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const root = dom.window.document.createElement("div");
    const markup = '<details class="member-row"><summary>Sam</summary><button>Remove</button></details><details class="card" open><summary>Feed</summary></details><details class="spare"><summary>Ada</summary></details>';
    root.innerHTML = markup;
    root.querySelector(".member-row").open = true;
    morphInnerHTML(root, markup);
    assert.deepEqual([...root.querySelectorAll("details")].map(d => d.open), [true, true, false]);
    // Other attributes still follow the markup.
    morphInnerHTML(root, markup.replace('class="member-row"', 'class="member-row member-row--self"'));
    assert.equal(root.querySelector("details").className, "member-row member-row--self");
    assert.equal(root.querySelector("details").open, true);
  });

  // Each markup below morphs into each other one; the DOM must then be
  // exactly what the markup parses to.
  const shapes = [
    '<div><ul><li data-key="a">A</li><li data-key="b">B</li><li data-key="c">C</li></ul></div>',
    '<div><ul><li data-key="c">C</li><li data-key="a">A!</li></ul></div>',
    '<div><ul><li data-key="z">Z</li><li data-key="a">A</li><li data-key="b">B</li><li data-key="c">C</li></ul></div>',
    '<div>\n  <p class="status">Loading</p>\n  <div class="search"><input class="q"></div>\n  <ul><li data-key="b"><span>B</span></li></ul>\n</div>',
    '<div><div class="search"><input class="q"></div><p class="empty">Nothing yet</p>text<!-- note --></div>',
    '<div><section class="offer">Offer</section><div class="grid"><button class="tile">1</button></div></div>',
    '<div><div class="grid"><button class="tile">1</button><button class="tile">2</button></div></div>',
    '<div><details class="pending" open><summary>1</summary></details><div class="grid"><button class="tile">1</button></div></div>',
    '<div><ul><li data-key="a"><b>A</b></li><li id="x">X</li><li>plain</li><li data-key="b">B</li></ul> tail </div>',
    '<div><ul><span data-key="a">A as span</span><li>plain</li><li id="x">X</li></ul></div>',
    '<div></div>',
  ];

  it("lands on exactly the new markup from any old one", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const parsed = html => { const t = document.createElement("template"); t.innerHTML = html; return t.innerHTML; };
    for (const from of shapes) {
      for (const to of shapes) {
        const root = document.createElement("div");
        root.innerHTML = from;
        morphInnerHTML(root, to);
        assert.equal(root.innerHTML, parsed(to), `${from}\n→ ${to}`);
      }
    }
  });

  it("keeps a keyed node, and its focus, as nodes are added, removed and reordered around it", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const root = document.createElement("div");
    document.body.appendChild(root);
    try {
      const list = keys => `<div><ul>${keys.map(k => `<li data-key="${k}">${k === "b" ? '<input class="q">' : k}</li>`).join("")}</ul></div>`;
      root.innerHTML = list(["a", "b", "c"]);
      const b = root.querySelector('[data-key="b"]');
      const input = b.querySelector("input");
      input.focus();
      input.value = "typed";
      // ["b", "a"] moves the focused row itself ahead of the other.
      for (const keys of [["z", "a", "b", "c"], ["b", "c"], ["c", "b", "a"], ["a", "b"], ["b", "a"], ["b"]]) {
        morphInnerHTML(root, list(keys));
        assert.deepEqual([...root.querySelectorAll("li")].map(li => li.dataset.key), keys);
        assert.equal(root.querySelector('[data-key="b"]'), b, keys.join());
        assert.equal(document.activeElement, input, keys.join());
        assert.equal(input.value, "typed");
      }
    } finally { root.remove(); }
  });

  it("keeps an open <details> with its own row when a row above it goes", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const rows = ids => `<div>${ids.map(id => `<details class="member-row" data-key="member-${id}"><summary>${id}</summary></details>`).join("")}</div>`;
    const root = document.createElement("div");
    root.innerHTML = rows([1, 2, 3]);
    root.querySelector('[data-key="member-3"]').open = true;
    morphInnerHTML(root, rows([1, 3]));
    assert.deepEqual([...root.querySelectorAll("details")].map(d => [d.dataset.key, d.open]),
      [["member-1", false], ["member-3", true]]);
  });

  it("keeps the nodes after an element that was added, taken away, or differently indented", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const root = document.createElement("div");
    document.body.appendChild(root);
    try {
      const steps = [
        '<div><div class="grid"><button class="tile">1</button></div></div>',
        // A notice arrives above the grid, then goes.
        '<div><details class="pending"><summary>1 saved</summary></details><div class="grid"><button class="tile">1</button></div></div>',
        '<div><div class="grid"><button class="tile">1</button></div></div>',
        // An offer of the same tag as the grid, matched by its class.
        '<div><div class="offer">Offer</div><div class="grid"><button class="tile">1</button></div></div>',
        // The same layout, indented.
        '<div>\n  <div class="offer">Offer</div>\n  <div class="grid">\n    <button class="tile">1</button>\n  </div>\n</div>',
        '<div><div class="grid"><button class="tile">1 just now</button></div></div>',
      ];
      root.innerHTML = steps[0];
      const tile = root.querySelector(".tile");
      tile.focus();
      for (const step of steps.slice(1)) {
        morphInnerHTML(root, step);
        assert.equal(root.querySelector(".tile"), tile, step);
        assert.equal(document.activeElement, tile, step);
      }
      assert.equal(tile.textContent, "1 just now");
    } finally { root.remove(); }
  });

  it("matches an id as a key, so a field's typed text never moves to another field", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const root = document.createElement("div");
    document.body.appendChild(root);
    try {
      root.innerHTML = '<form><input id="first"><input id="second"></form>';
      const first = root.querySelector("#first");
      const second = root.querySelector("#second");
      first.focus();
      first.value = "for the first";
      second.value = "for the second";
      morphInnerHTML(root, '<form><input id="second" value="from markup"></form>');
      assert.equal(root.querySelector("input"), second);
      assert.equal(second.value, "from markup", "an unfocused field follows the markup");
      assert.equal(first.isConnected, false);
    } finally { root.remove(); }
  });

  it("replaces a keyed node whose tag changed", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const root = document.createElement("div");
    root.innerHTML = '<div><div data-key="chore-1" class="home-card-wrapper"><button>1</button></div></div>';
    morphInnerHTML(root, '<div><button data-key="chore-1" class="home-chore-card">1</button></div>');
    const card = root.querySelector('[data-key="chore-1"]');
    assert.equal(card.tagName, "BUTTON");
    assert.equal(card.className, "home-chore-card");
  });
});

describe("API", () => {
  it("apiFetch returns data", async () => {
    globalThis.fetch = async () => ({
      ok: true,
      json: async () => ({ status: "ok" }),
      headers: {
        get: (key) => key === "Content-Type" ? "application/json" : null,
      },
    });
    const { apiFetch } = await import("../api.js");
    const { data } = await apiFetch("/health");
    assert.deepEqual(data, { status: "ok" });
  });

  it("a raw read leaves a JSON download unread and never shares it", async () => {
    const previousFetch = globalThis.fetch;
    let parsed = 0;
    const respond = (ok, body) => ({
      ok, status: ok ? 200 : 429,
      json: async () => { parsed++; return body; },
      headers: { get: (key) => key === "Content-Type" ? "application/json; charset=utf-8" : null },
    });
    try {
      const { apiFetch } = await import("../api.js");
      globalThis.fetch = async () => respond(true, { account: {} });
      const [first, second] = await Promise.all([apiFetch("/api/me/export", { raw: true }), apiFetch("/api/me/export", { raw: true })]);
      assert.equal(parsed, 0);
      assert.equal(first.data, null);
      assert.notEqual(first.response, second.response);

      globalThis.fetch = async () => respond(false, { error: "an export is already running; try again shortly" });
      await assert.rejects(apiFetch("/api/me/export", { raw: true }), /already running/);
    } finally { globalThis.fetch = previousFetch; }
  });
});

describe("Download my data", () => {
  it("disables both downloads while one runs and escapes its error", async () => {
    const { renderMyDataDownload } = await import("../exports.js");
    const root = document.createElement("div");
    root.innerHTML = renderMyDataDownload({ myDataBusy: true, myDataError: '<img src=x onerror="alert(1)">' },
      { jsonId: "download-my-data", csvId: "download-my-logs" });
    const buttons = [...root.querySelectorAll('[data-action="download-my-data"]')];
    assert.deepEqual(buttons.map(b => [b.dataset.format, b.disabled]), [["json", true], ["csv", true]]);
    assert.ok(root.querySelector('[data-action="cancel-my-data"]'));
    assert.equal(root.querySelector("img"), null);
    assert.equal(root.querySelector('[data-testid="my-data-error"]').textContent, '<img src=x onerror="alert(1)">');

    root.innerHTML = renderMyDataDownload({}, { jsonId: "account-download-my-data" });
    assert.equal(root.querySelectorAll("button").length, 1);
    assert.equal(root.querySelector("button").disabled, false);
  });
});

describe("Schedule request IDs", () => {
  it("rejects malformed IDs before any authenticated request", async () => {
    const { updateSchedule, deleteSchedule } = await import("../schedule.js");
    const previousFetch = globalThis.fetch;
    let requests = 0;
    globalThis.fetch = async () => { requests++; throw new Error("Unexpected fetch"); };
    try {
      for (const id of ["../notification-preferences", "%2e%2e/chores", "1/../../chores",
        "1?other=1", "1#fragment", "1\\..", "", " 1", "1e2", "0x10", "1.5",
        null, undefined, true, {}, [1], 0, -1, 1.5, NaN, Infinity, Number.MAX_SAFE_INTEGER + 1]) {
        await assert.rejects(updateSchedule(id, { timePeriod: "anytime" }), /Invalid schedule ID/);
        await assert.rejects(deleteSchedule(id), /Invalid schedule ID/);
      }
      assert.equal(requests, 0);
    } finally { globalThis.fetch = previousFetch; }
  });

  it("uses numeric path segments for valid numeric and decimal-string IDs", async () => {
    const { updateSchedule, deleteSchedule } = await import("../schedule.js");
    const previousFetch = globalThis.fetch;
    const requests = [];
    globalThis.fetch = async (path, options) => {
      requests.push([path, options.method]);
      return { ok: true, headers: new Map() };
    };
    try {
      await updateSchedule(42, { timePeriod: "anytime" });
      await deleteSchedule("0042");
      assert.deepEqual(requests, [["/api/schedules/42", "PATCH"], ["/api/schedules/42", "DELETE"]]);
    } finally { globalThis.fetch = previousFetch; }
  });
});

describe("Auth Views", () => {
  it("renders login view", async () => {
    const { renderLoginView } = await import("../auth.js");
    const html = renderLoginView();
    assert.ok(html.includes("Sign In"));
    assert.ok(html.includes("Nabu"));
    assert.ok(html.includes("Create Account"));
    const root = document.createElement("div");
    root.innerHTML = html;
    assert.equal(root.querySelector(".auth-logo").getAttribute("src"), "/static/icons/icon.svg");
    assert.equal(root.querySelector(".auth-tagline").textContent, "One shared log for your home: feeds, meds, walks and chores.");
    assert.equal(root.querySelector("#login-password").getAttribute("autocomplete"), "current-password");
    const toggle = root.querySelector('[data-action="toggle-password"]');
    assert.deepEqual([toggle.dataset.target, toggle.getAttribute("aria-pressed"), toggle.getAttribute("aria-label")], ["login-password", "false", "Show password"]);
  });

  it("renders register view with one password, its rule and Show/Hide", async () => {
    const { renderRegisterView } = await import("../auth.js");
    const html = renderRegisterView();
    assert.ok(html.includes("Create Account"));
    const root = document.createElement("div");
    root.innerHTML = html;
    assert.equal(root.querySelector("#reg-confirm"), null);
    assert.ok(!html.includes("Confirm Password"));
    const password = root.querySelector("#reg-password");
    assert.deepEqual([password.type, password.minLength, password.maxLength, password.getAttribute("autocomplete")], ["password", 8, 72, "new-password"]);
    assert.equal(root.querySelector(`#${password.getAttribute("aria-describedby")}`).textContent, "At least 8 characters.");
    assert.equal(root.querySelector('[data-action="toggle-password"]').dataset.target, "reg-password");
    assert.ok(root.querySelector(".auth-logo") && root.querySelector(".auth-tagline"));
  });

  it("hides third-party sign-in buttons when not configured", async () => {
    const { renderLoginView, renderRegisterView } = await import("../auth.js");
    for (const html of [renderLoginView(false, false), renderRegisterView(false, false)]) {
      assert.ok(!html.includes("google-signin"));
      assert.ok(!html.includes("apple-signin"));
    }
  });

  it("renders Apple above Google on login and register when both enabled", async () => {
    const { renderLoginView, renderRegisterView } = await import("../auth.js");
    for (const html of [renderLoginView(true, true), renderRegisterView(true, true)]) {
      assert.ok(html.includes("Continue with Apple"));
      assert.ok(html.includes("Continue with Google"));
      assert.ok(
        html.indexOf("apple-signin") < html.indexOf("google-signin"),
        "Apple button must render above Google (guideline 4.8 prominence)",
      );
    }
  });

  it("renders Apple button alone when only Apple is enabled", async () => {
    const { renderLoginView } = await import("../auth.js");
    const html = renderLoginView(false, true);
    assert.ok(html.includes("apple-signin"));
    assert.ok(!html.includes("google-signin"));
  });

  it("renders magic link request view", async () => {
    const { renderMagicLinkRequestView } = await import("../auth.js");
    const html = renderMagicLinkRequestView();
    assert.ok(html.includes("Magic Link"));
    assert.ok(html.includes("magic-link-request"));
  });

  it("renders forgot password view", async () => {
    const { renderForgotPasswordView } = await import("../auth.js");
    const html = renderForgotPasswordView();
    assert.ok(html.includes("Forgot Password"));
  });

  it("renders reset password view", async () => {
    const { renderResetPasswordView } = await import("../auth.js");
    const html = renderResetPasswordView("test-token");
    assert.ok(html.includes("Reset Password"));
    assert.ok(html.includes("test-token"));
  });

  it("renders verify email view", async () => {
    const { renderVerifyEmailView } = await import("../auth.js");
    assert.ok(!renderVerifyEmailView("pending").includes("Email Verified"));
    assert.ok(!renderVerifyEmailView("error").includes("Email Verified"));
    const html = renderVerifyEmailView("success");
    assert.ok(html.includes("Email Verified"));
  });
});

// ─── Schedule helpers ─────────────────────────────────────────────────────────

describe("Schedule: recurrenceSummary", () => {
  it("returns not-scheduled for null", async () => {
    const { recurrenceSummary } = await import("../schedule.js");
    assert.equal(recurrenceSummary(null), "Not scheduled");
  });

  it("daily returns every day label", async () => {
    const { recurrenceSummary } = await import("../schedule.js");
    const s = recurrenceSummary({ frequencyType: "daily" });
    assert.ok(s.includes("Every day"));
  });

  it("weekly with specific days", async () => {
    const { recurrenceSummary } = await import("../schedule.js");
    const s = recurrenceSummary({ frequencyType: "weekly", daysOfWeek: [1, 3, 5], timePeriod: "anytime" });
    assert.ok(s.includes("Mon"));
    assert.ok(s.includes("Wed"));
    assert.ok(s.includes("Fri"));
  });

  it("every_n_days", async () => {
    const { recurrenceSummary } = await import("../schedule.js");
    const s = recurrenceSummary({ frequencyType: "every_n_days", intervalDays: 3, timePeriod: "anytime" });
    assert.ok(s.includes("3 days"));
  });

  it("monthly_by_date", async () => {
    const { recurrenceSummary } = await import("../schedule.js");
    const s = recurrenceSummary({ frequencyType: "monthly_by_date", dayOfMonth: 15, timePeriod: "anytime" });
    assert.ok(s.includes("15th"));
  });

  it("shows specific time instead of period", async () => {
    const { recurrenceSummary } = await import("../schedule.js");
    const s = recurrenceSummary({ frequencyType: "daily", timePeriod: "morning", specificTime: "08:00" });
    assert.ok(s.includes("8:00 AM"));
    // period label should NOT appear when specificTime is set
    assert.ok(!s.includes("Morning"));
  });
});

describe("Schedule: renderPickChoreSheet", () => {
  it("lists available chores", async () => {
    const { renderPickChoreSheet } = await import("../schedule.js");
    const chores = [
      { id: 1, icon: "🐱", name: "Feed cats", category: "Pets" },
      { id: 2, icon: "🌿", name: "Water plants", category: "Garden" },
    ];
    const html = renderPickChoreSheet(chores, { date: "2026-04-28", hour: 8 }, []);
    assert.ok(html.includes("Feed cats"));
    assert.ok(html.includes("Water plants"));
    assert.ok(html.includes("schedule-chore-here"));
  });

  it("excludes already-scheduled chores", async () => {
    // Behaviour change: all chores are always shown so they can be added
    // multiple times (e.g. feed cat morning AND evening).
    const { renderPickChoreSheet } = await import("../schedule.js");
    const chores = [
      { id: 1, icon: "🐱", name: "Feed cats", category: "Pets" },
      { id: 2, icon: "🌿", name: "Water plants", category: "Garden" },
    ];
    const existing = [{ choreId: 1 }];
    const html = renderPickChoreSheet(chores, { date: "2026-04-28", hour: 8 }, existing);
    // Both chores must still be present — scheduling one does not remove it
    assert.ok(html.includes("Feed cats"));
    assert.ok(html.includes("Water plants"));
  });

  it("shows empty message when all scheduled", async () => {
    // Behaviour change: the sheet now always shows all chores (repeatable).
    // The "empty" state only appears when the household has zero chores at all.
    const { renderPickChoreSheet } = await import("../schedule.js");
    // With an empty chores array the empty message should appear
    const html = renderPickChoreSheet([], { date: "2026-04-28", hour: 8 }, [{ choreId: 1 }]);
    assert.ok(html.includes("sheet-empty"));
  });
});

// ─── Calendar helpers ─────────────────────────────────────────────────────────

describe("Calendar: shiftISO", () => {
  it("shifts forward by days", async () => {
    const { shiftISO } = await import("../calendar.js");
    assert.equal(shiftISO("2026-04-28", 1),  "2026-04-29");
    assert.equal(shiftISO("2026-04-28", 7),  "2026-05-05");
    assert.equal(shiftISO("2026-04-28", -1), "2026-04-27");
  });

  it("handles month boundaries", async () => {
    const { shiftISO } = await import("../calendar.js");
    assert.equal(shiftISO("2026-01-31", 1),  "2026-02-01");
    assert.equal(shiftISO("2026-03-01", -1), "2026-02-28");
  });
});

describe("Calendar: isActiveForDayJS", () => {
  it("daily schedule is always active", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = { isActive: true, frequencyType: "daily" };
    assert.equal(isActiveForDayJS(sch, "2026-04-28"), true);
    assert.equal(isActiveForDayJS(sch, "2026-01-01"), true);
  });

  it("inactive schedule returns false", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = { isActive: false, frequencyType: "daily" };
    assert.equal(isActiveForDayJS(sch, "2026-04-28"), false);
  });

  it("weekly schedule matches correct days", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    // 2026-04-27 = Monday (day 1), 2026-04-28 = Tuesday (day 2)
    const sch = { isActive: true, frequencyType: "weekly", daysOfWeek: [1, 3] };
    assert.equal(isActiveForDayJS(sch, "2026-04-27"), true);  // Monday
    assert.equal(isActiveForDayJS(sch, "2026-04-28"), false); // Tuesday
    assert.equal(isActiveForDayJS(sch, "2026-04-29"), true);  // Wednesday
  });

  it("every_n_days schedule", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = {
      isActive: true,
      frequencyType: "every_n_days",
      intervalDays: 3,
      createdAt: new Date("2026-04-01T00:00:00").toISOString(),
    };
    assert.equal(isActiveForDayJS(sch, "2026-04-01"), true);
    assert.equal(isActiveForDayJS(sch, "2026-04-02"), false);
    assert.equal(isActiveForDayJS(sch, "2026-04-04"), true);
  });

  it("monthly_by_date schedule", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = { isActive: true, frequencyType: "monthly_by_date", dayOfMonth: 15 };
    assert.equal(isActiveForDayJS(sch, "2026-04-15"), true);
    assert.equal(isActiveForDayJS(sch, "2026-04-16"), false);
    assert.equal(isActiveForDayJS(sch, "2026-05-15"), true);
  });

  it("monthly_by_weekday — 2nd Monday", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = {
      isActive: true,
      frequencyType: "monthly_by_weekday",
      monthWeekday: { week: 2, day: 1 }, // 2nd Monday
    };
    // April 2026: Mondays are 6,13,20,27. 2nd Monday = Apr 13.
    assert.equal(isActiveForDayJS(sch, "2026-04-13"), true);
    assert.equal(isActiveForDayJS(sch, "2026-04-06"), false);
    assert.equal(isActiveForDayJS(sch, "2026-04-20"), false);
  });

  it("yearly schedule", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = { isActive: true, frequencyType: "yearly", dayOfMonth: 28, monthOfYear: 4 };
    assert.equal(isActiveForDayJS(sch, "2026-04-28"), true);
    assert.equal(isActiveForDayJS(sch, "2026-04-29"), false);
    assert.equal(isActiveForDayJS(sch, "2027-04-28"), true);
  });

  it("respects recurrenceEnd", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = { isActive: true, frequencyType: "daily", recurrenceEnd: "2026-04-30" };
    assert.equal(isActiveForDayJS(sch, "2026-04-28"), true);
    assert.equal(isActiveForDayJS(sch, "2026-05-01"), false);
  });

  it("once schedule is active only on its startDate", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = { isActive: true, frequencyType: "once", startDate: "2026-04-30" };
    assert.equal(isActiveForDayJS(sch, "2026-04-30"), true);   // matches exactly
    assert.equal(isActiveForDayJS(sch, "2026-04-29"), false);  // day before
    assert.equal(isActiveForDayJS(sch, "2026-05-01"), false);  // day after
  });

  it("once schedule with no startDate returns false", async () => {
    const { isActiveForDayJS } = await import("../calendar.js");
    const sch = { isActive: true, frequencyType: "once" };
    assert.equal(isActiveForDayJS(sch, "2026-04-30"), false);
  });
});

describe("Calendar: renderDayView", () => {
  it("renders hourly grid", async () => {
    const { renderDayView } = await import("../calendar.js");
    const state = {
      calendarDate: "2026-04-28",
      chores: [{ id: 1, icon: "🐱", name: "Feed cats", color: "#aabbcc", category: "Pets" }],
      schedules: [{ id: 1, choreId: 1, timePeriod: "anytime", specificTime: "08:00", isActive: true, frequencyType: "daily" }],
      todayLogs: [],
    };
    const html = renderDayView(state);
    assert.ok(html.includes("day-hour-row"));
    assert.ok(html.includes("Feed cats"));
    assert.ok(html.includes("data-view=\"day\""));
  });

  it("marks completed chore as done", async () => {
    const { renderDayView } = await import("../calendar.js");
    const state = {
      calendarDate: "2026-04-28",
      chores: [{ id: 1, icon: "🐱", name: "Feed cats", color: "#aabbcc", category: "Pets" }],
      schedules: [{ id: 1, choreId: 1, timePeriod: "anytime", specificTime: "08:00", isActive: true, frequencyType: "daily" }],
      todayLogs: [{ id: 99, choreId: 1, completedAt: "2026-04-28T09:00:00Z" }],
    };
    const html = renderDayView(state);
    assert.ok(html.includes("chore-card--done"));
    assert.ok(html.includes("view-log"));
  });

  it("unscheduled chores are not shown in the day view", async () => {
    const { renderDayView } = await import("../calendar.js");
    const state = {
      calendarDate: "2026-04-28",
      chores: [{ id: 1, icon: "🐱", name: "Feed cats", color: "#aabbcc", category: "Pets" }],
      schedules: [],
      todayLogs: [],
    };
    const html = renderDayView(state);
    // Without a schedule or slot log, unscheduled chores are not rendered
    assert.ok(!html.includes("day-anytime-section"));
    assert.ok(!html.includes("Feed cats"));
  });

  it("uses compact chip cards inside hour rows", async () => {
    const { renderDayView } = await import("../calendar.js");
    const state = {
      calendarDate: "2026-04-28",
      chores: [{ id: 1, icon: "🐱", name: "Feed cats", color: "#aabbcc", category: "Pets" }],
      schedules: [{ id: 1, choreId: 1, timePeriod: "anytime", specificTime: "08:00", isActive: true, frequencyType: "daily" }],
      todayLogs: [],
    };
    const html = renderDayView(state);
    // Hour-row card should be compact
    assert.ok(html.includes("chore-card--compact"));
    // No anytime section in the day view
    assert.ok(!html.includes("day-anytime-section"));
  });

  it("two chores at the same hour both render as compact chips", async () => {
    const { renderDayView } = await import("../calendar.js");
    const state = {
      calendarDate: "2026-04-28",
      chores: [
        { id: 1, icon: "🐱", name: "Feed cats",  color: "#aabbcc", category: "Pets" },
        { id: 2, icon: "🐶", name: "Walk dog",   color: "#ccaabb", category: "Pets" },
      ],
      schedules: [
        { id: 1, choreId: 1, timePeriod: "anytime", specificTime: "08:00", isActive: true, frequencyType: "daily" },
        { id: 2, choreId: 2, timePeriod: "anytime", specificTime: "08:00", isActive: true, frequencyType: "daily" },
      ],
      todayLogs: [],
    };
    const html = renderDayView(state);
    // Both chore names appear
    assert.ok(html.includes("Feed cats"));
    assert.ok(html.includes("Walk dog"));
    // Two compact cards rendered
    const matches = html.match(/chore-card--compact/g);
    assert.equal(matches?.length, 2);
  });
});

// ─── Service Worker update toast ──────────────────────────────────────────────

describe("Service Worker: update toast", () => {
  it("shows toast on controllerchange when previously controlled", async () => {
    // Simulate the controllerchange listener pattern from init().
    // Set up navigator.serviceWorker with a controller already active,
    // fire controllerchange, and verify the toast DOM is created.
    const ctors = [];
    const container = dom.window.document.createElement("div");
    container.id = "toast-container";
    dom.window.document.body.appendChild(container);

    globalThis.navigator.serviceWorker = {
      controller: { state: "activated" },
      addEventListener: (type, fn) => { ctors.push(fn); },
      register: async () => ({ update: async () => {} }),
    };

    let hadController = !!navigator.serviceWorker.controller;
    let swRefreshing = false;
    navigator.serviceWorker.addEventListener("controllerchange", () => {
      if (swRefreshing) return;
      if (!hadController) { hadController = true; return; }
      const toast = dom.window.document.createElement("div");
      toast.className = "toast toast-info sw-update-toast";
      toast.textContent = "App updated";
      container.appendChild(toast);
    });

    // First fire: page was already controlled → toast shows immediately
    ctors[0]();
    assert.equal(container.children.length, 1);
    assert.equal(container.children[0].textContent, "App updated");

    // Clean up and simulate another controllerchange (subsequent deploy)
    container.innerHTML = "";
    ctors[0]();
    assert.equal(container.children.length, 1);
  });

  it("does not show toast on first-ever controller activation", async () => {
    const ctors = [];
    const container = dom.window.document.createElement("div");
    container.id = "toast-container";
    dom.window.document.body.appendChild(container);

    globalThis.navigator.serviceWorker = {
      controller: null, // no controller yet (fresh load)
      addEventListener: (type, fn) => { ctors.push(fn); },
      register: async () => ({ update: async () => {} }),
    };

    let hadController = !!navigator.serviceWorker.controller;
    let swRefreshing = false;
    navigator.serviceWorker.addEventListener("controllerchange", () => {
      if (swRefreshing) return;
      if (!hadController) { hadController = true; return; }
      const toast = dom.window.document.createElement("div");
      toast.textContent = "App updated";
      container.appendChild(toast);
    });

    // First fire: no controller at init → skip and set hadController
    ctors[0]();
    assert.equal(container.children.length, 0);

    // Second fire: now hadController is true → should show toast
    ctors[0]();
    assert.equal(container.children.length, 1);
  });
});

describe("Stats: colorForIndicator", () => {
  it("keeps the four predefined labels at their historical colors", async () => {
    const { colorForIndicator } = await import("../stats.js");
    assert.equal(colorForIndicator("🍼 formula"), "#EC4899");
    assert.equal(colorForIndicator("🤱 breast"), "#F59E0B");
    assert.equal(colorForIndicator("💩 poo"), "#8B4513");
    assert.equal(colorForIndicator("💛 pee"), "#FACC15");
  });

  it("assigns a stable color from the palette for custom labels", async () => {
    const { colorForIndicator } = await import("../stats.js");
    const a = colorForIndicator("🌙 night");
    const b = colorForIndicator("🌙 night");
    assert.equal(a, b); // stable across calls
    assert.match(a, /^#[0-9A-Fa-f]{6}$/);
  });

  it("does not collapse two distinct custom labels to the same gray", async () => {
    const { colorForIndicator } = await import("../stats.js");
    // Regression: previously both fell through to a single "#6B7280" gray.
    const c1 = colorForIndicator("💊 vitamin");
    const c2 = colorForIndicator("🌡️ temp");
    assert.notEqual(c1, c2);
  });

  it("never returns undefined for null/empty labels", async () => {
    const { colorForIndicator } = await import("../stats.js");
    assert.match(colorForIndicator(null), /^#[0-9A-Fa-f]{6}$/);
    assert.match(colorForIndicator(""), /^#[0-9A-Fa-f]{6}$/);
  });
});

describe("Stats: generalized per-chore sections (Phase 3)", () => {
  it("choreHasAnalytics: true for metric or indicators, false for plain/baby", async () => {
    const { choreHasAnalytics } = await import("../stats.js");
    assert.equal(choreHasAnalytics({ id: 1, name: "Naps", metricType: "duration" }), true);
    assert.equal(choreHasAnalytics({ id: 2, name: "Meds", metricType: "none", indicatorLabels: ["am"] }), true);
    assert.equal(choreHasAnalytics({ id: 3, name: "Vacuum", metricType: "none", indicatorLabels: [] }), false);
    // Baby chores are covered by the dedicated baby section.
    assert.equal(choreHasAnalytics({ id: 4, name: "Feed Baby", metricType: "amount" }), false);
    assert.equal(choreHasAnalytics({ id: 5, name: "Change Baby", indicatorLabels: ["poo"] }), false);
  });

  it("eligibleChoreSectionKeys maps eligible chores to chore:<id> keys", async () => {
    const { eligibleChoreSectionKeys } = await import("../stats.js");
    const keys = eligibleChoreSectionKeys([
      { id: 7, name: "Naps", metricType: "duration" },
      { id: 8, name: "Vacuum", metricType: "none", indicatorLabels: [] },
      { id: 9, name: "Meds", indicatorLabels: ["am", "pm"] },
    ]);
    assert.deepEqual(keys, ["chore:7", "chore:9"]);
  });

  it("resolveStatsLayout keeps valid dynamic keys and drops stale ones", async () => {
    const { resolveStatsLayout } = await import("../stats.js");
    const order = resolveStatsLayout(
      ["overview", "chore:7", "chore:999"], // 999 no longer eligible
      [],
      ["chore:7", "chore:9"],
    );
    assert.ok(order.includes("chore:7"));
    assert.ok(!order.includes("chore:999")); // dropped: not a valid dynamic key
    assert.ok(order.includes("chore:9"));     // auto-appended eligible key
    assert.ok(order.includes("overview"));
  });

  it("resolveStatsLayout excludes hidden dynamic keys", async () => {
    const { resolveStatsLayout } = await import("../stats.js");
    const order = resolveStatsLayout([], ["chore:7"], ["chore:7", "chore:9"]);
    assert.ok(!order.includes("chore:7"));
    assert.ok(order.includes("chore:9"));
  });

  it("renderChoreAnalyticsSection renders a card with chore name and chart", async () => {
    const { renderChoreAnalyticsSection } = await import("../stats.js");
    const chore = { id: 7, name: "Naps", icon: "😴", metricType: "duration" };
    const ts = { byMember: [{ userId: 1, count: 3 }], periods: [
      { start: "2026-07-01", end: "2026-07-02", count: 1, totalDuration: 1800 },
    ] };
    const html = renderChoreAnalyticsSection(chore, ts, [{ userId: 1, displayName: "A", avatarColor: "#000" }]);
    assert.ok(html.includes("Naps"));
    assert.ok(html.includes("baby-chart"));
    assert.ok(html.includes("min")); // duration axis label
  });

  it("widget/chore key type guards", async () => {
    const { isChoreSectionKey, isWidgetSectionKey } = await import("../stats.js");
    assert.equal(isChoreSectionKey("chore:12"), true);
    assert.equal(isChoreSectionKey("chore:x"), false);
    assert.equal(isWidgetSectionKey("widget:abc-123_XY"), true);
    assert.equal(isWidgetSectionKey("widget:bad key"), false);
  });
});

describe("Offline pending log badge (Phase 2.1)", () => {
  it("renders a pending row with a badge and no view-log action", async () => {
    const { renderHistoryView } = await import("../today.js");
    const state = {
      chores: [{ id: 1, name: "Feed", icon: "🍼", color: "#000" }],
      members: [{ userId: 1, displayName: "Ann" }],
      historyLogs: [],
      pendingLogs: [{
        id: "pending-1", choreId: 1, userId: 1, note: "", indicators: [],
        completedAt: "2026-07-02T10:00:00Z", _pending: true,
      }],
    };
    const html = renderHistoryView(state);
    assert.ok(html.includes("hist-pending"));
    assert.ok(html.includes("hist-row--pending"));
    assert.ok(html.includes("Feed"));
    // A pending row must not be a tappable view-log target.
    assert.ok(!html.includes('data-action="view-log"\n          data-chore-id="1"\n          data-log-id="pending-1"'));
  });

  it("excludes pending rows while searching", async () => {
    const { renderHistoryView } = await import("../today.js");
    const state = {
      chores: [{ id: 1, name: "Feed", icon: "🍼", color: "#000" }],
      members: [],
      historySearch: "foo",
      historyLogs: [],
      pendingLogs: [{ id: "p", choreId: 1, completedAt: "2026-07-02T10:00:00Z", _pending: true }],
    };
    const html = renderHistoryView(state);
    assert.ok(!html.includes("hist-pending"));
  });
});

describe("Historical household members", () => {
  it("renders retained activity with a removed member's name", async () => {
    const { renderHistoryView } = await import("../today.js");
    const state = {
      chores: [{ id: 1, name: "Feed", icon: "🍼", color: "#000" }],
      members: [],
      historicalMembers: [{ userId: 17, displayName: "Former Member", avatarColor: "#123456" }],
      historyLogs: [{ id: 1, choreId: 1, userId: 17, note: "", indicators: [], completedAt: "2026-07-02T10:00:00Z" }],
    };
    const html = renderHistoryView(state);
    assert.ok(html.includes("Former Member"));
    assert.ok(!html.includes("Someone"));
  });
});

describe("Subject tagging (Phase 5.5)", () => {
  it("chore sheet renders a subjects field with existing tags", async () => {
    const { renderChoreSheet } = await import("../chores.js");
    const html = renderChoreSheet({ id: 1, name: "Feed", icon: "🍼", color: "#000", subjects: ["👶 Alice", "👶 Bob"] });
    assert.ok(html.includes("Subjects"));
    assert.ok(html.includes("add-subject-label"));
    assert.ok(html.includes("👶 Alice"));
    assert.ok(html.includes("👶 Bob"));
  });

  it("log sheet renders subject chips only when the chore has subjects", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const withSubjects = renderLogSheet(
      { id: 1, icon: "🍼", name: "Feed", color: "#000", subjects: ["Alice", "Bob"] },
      null, "2026-07-02", [], 1, null, { volumeUnit: "ml" });
    assert.ok(withSubjects.includes("pick-subject"));
    assert.ok(withSubjects.includes("Alice"));

    const without = renderLogSheet(
      { id: 2, icon: "🧹", name: "Vacuum", color: "#000", subjects: [] },
      null, "2026-07-02", [], 1, null, { volumeUnit: "ml" });
    assert.ok(!without.includes("pick-subject"));
  });

  it("log sheet preselects the existing log's subject", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(
      { id: 1, icon: "🍼", name: "Feed", color: "#000", subjects: ["Alice", "Bob"] },
      { id: 9, subject: "Bob", indicators: [] }, "2026-07-02", [], 1, null, { volumeUnit: "ml" });
    // The Bob chip should be pressed/on.
    assert.match(html, /data-subject="Bob"[^>]*aria-pressed="true"|aria-pressed="true"[^>]*data-subject="Bob"/);
  });

  it("new log sheet preselects the cached (latest log's) subject", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(
      { id: 1, icon: "🍼", name: "Feed", color: "#000", subjects: ["Alice", "Bob"] },
      null, "2026-07-02", [], 1, null, { volumeUnit: "ml", cachedSubject: "Bob" });
    // Bob (the previous log's subject) is on; Alice is not.
    assert.match(html, /data-subject="Bob"[^>]*aria-pressed="true"|aria-pressed="true"[^>]*data-subject="Bob"/);
    assert.match(html, /data-subject="Alice"[^>]*aria-pressed="false"|aria-pressed="false"[^>]*data-subject="Alice"/);
  });

  it("new log sheet preselects nothing without a cached subject", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(
      { id: 1, icon: "🍼", name: "Feed", color: "#000", subjects: ["Alice", "Bob"] },
      null, "2026-07-02", [], 1, null, { volumeUnit: "ml" });
    // Subject chips render, but none is preselected.
    assert.ok(html.includes('data-subject="Alice"'));
    assert.ok(html.includes('data-subject="Bob"'));
    assert.ok(!/data-subject="Alice"[^>]*aria-pressed="true"|aria-pressed="true"[^>]*data-subject="Alice"/.test(html), "Alice must not be preselected");
    assert.ok(!/data-subject="Bob"[^>]*aria-pressed="true"|aria-pressed="true"[^>]*data-subject="Bob"/.test(html), "Bob must not be preselected");
  });

  it("editing prefers the log's own subject over the cached subject", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(
      { id: 1, icon: "🍼", name: "Feed", color: "#000", subjects: ["Alice", "Bob"] },
      { id: 9, subject: "Alice", indicators: [] }, "2026-07-02", [], 1, null,
      { volumeUnit: "ml", cachedSubject: "Bob" });
    assert.match(html, /data-subject="Alice"[^>]*aria-pressed="true"|aria-pressed="true"[^>]*data-subject="Alice"/);
    assert.match(html, /data-subject="Bob"[^>]*aria-pressed="false"|aria-pressed="false"[^>]*data-subject="Bob"/);
  });

  it("log sheet caps title and note inputs to the server limits", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(
      { id: 1, icon: "🍼", name: "Feed", color: "#000", hasRating: true },
      null, "2026-07-02", [], 1, null, { volumeUnit: "ml" });
    assert.match(html, /id="log-title"[^>]*maxlength="120"/);
    assert.match(html, /id="log-note"[^>]*maxlength="2000"/);
  });
});

describe("Duration timer (Phase 5.2)", () => {
  it("formatElapsed renders m:ss and h:mm:ss", async () => {
    const { formatElapsed } = await import("../timer.js");
    assert.equal(formatElapsed(0), "0:00");
    assert.equal(formatElapsed(65), "1:05");
    assert.equal(formatElapsed(3725), "1:02:05");
  });

  it("elapsedSeconds computes whole seconds since start", async () => {
    const { elapsedSeconds } = await import("../timer.js");
    const t = { choreId: 1, startedAt: 10_000 };
    assert.equal(elapsedSeconds(t, 10_000), 0);
    assert.equal(elapsedSeconds(t, 95_500), 85);
    assert.equal(elapsedSeconds(null), 0);
  });

  it("save/load round-trips a timer via localStorage", async () => {
    const store = {};
    globalThis.localStorage = {
      getItem: (k) => (k in store ? store[k] : null),
      setItem: (k, v) => { store[k] = String(v); },
      removeItem: (k) => { delete store[k]; },
    };
    const { saveTimer, loadTimer } = await import("../timer.js");
    const origin = {userId:1, householdId:2};
    saveTimer({ choreId: 7, choreName: "Nap", choreIcon: "😴", startedAt: 123 }, origin);
    assert.equal(loadTimer({userId:1,householdId:3}), null);
    assert.equal(loadTimer({userId:2,householdId:2}), null);
    const t = loadTimer(origin);
    assert.equal(t.choreId, 7);
    assert.equal(t.choreName, "Nap");
    saveTimer(null, origin);
    assert.equal(loadTimer(origin), null);
    delete globalThis.localStorage;
  });

  it("loadTimer rejects a corrupt value", async () => {
    const store = { "nabu_timer:1:2": "{not json", nabu_active_timer: JSON.stringify({choreId:4,startedAt:10}) };
    globalThis.localStorage = {
      getItem: (k) => (k in store ? store[k] : null),
      setItem: () => {}, removeItem: () => {},
    };
    const { loadTimer } = await import("../timer.js");
    assert.equal(loadTimer({userId:1,householdId:2}), null);
    assert.ok(store.nabu_active_timer, "unscoped legacy timer retained without adopting it");
    delete globalThis.localStorage;
  });
});

describe("Log sheet: recent-value chips (Phase 5.3)", () => {
  const chore = { id: 1, icon: "🍼", name: "Feed", color: "#000", hasVolumeML: true, indicatorLabels: ["🍼 formula"] };

  it("renders recent-value chips when provided", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(chore, null, "2026-07-02", [], 1, null, {
      volumeUnit: "ml", recentVolumes: [60, 90, 120],
    });
    assert.ok(html.includes("set-recent-volume"));
    assert.ok(html.includes("60 mL"));
    assert.ok(html.includes("90 mL"));
  });

  it("omits the recent row when there are no recent volumes", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(chore, null, "2026-07-02", [], 1, null, {
      volumeUnit: "ml", recentVolumes: [],
    });
    assert.ok(!html.includes("set-recent-volume"));
  });

  it("shows a plain volume input for amount chores without indicators", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const amountChore = { id: 2, icon: "💧", name: "Water", color: "#000", hasVolumeML: true, indicatorLabels: [] };
    const html = renderLogSheet(amountChore, null, "2026-07-02", [], 1, null, { volumeUnit: "ml" });
    assert.ok(html.includes("log-volume"));
  });
});

describe("Log sheet: prefill from latest log (type + volume)", () => {
  // jsdom 30's selector engine never matches an attribute value holding a
  // character outside the Basic Multilingual Plane, such as these emoji, so
  // find the field by comparing the attribute itself. Browsers match either way.
  const volumeField = (html, label) => [...JSDOM.fragment(html).querySelectorAll("input[data-indicator]")]
    .find(input => input.getAttribute("data-indicator") === label);
  const feedChore = {
    id: 1, icon: "🍼", name: "Feed Baby", color: "#000", hasVolumeML: true,
    indicatorLabels: ["🍼 formula", "🤱 breast"],
    indicatorDefaults: ["🍼 formula"],
  };

  it("echoes the previous log's type selection when cachedIndicators is provided", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(feedChore, null, "2026-07-02", [], 1, null, {
      volumeUnit: "ml",
      cachedIndicators: ["🤱 breast"],
      cachedIndicatorVolumes: { "🤱 breast": 95 },
    });
    // Breast chip on with its volume; formula off.
    assert.match(html, /data-label="🤱 breast"[^>]*aria-pressed="true"|aria-pressed="true"[^>]*data-label="🤱 breast"/);
    assert.match(html, /data-label="🍼 formula"[^>]*aria-pressed="false"|aria-pressed="false"[^>]*data-label="🍼 formula"/);
    const field = volumeField(html, "🤱 breast");
    assert.equal(field.value, '95', "breast volume preselected");
  });

  it("does not prefill a volume for a type absent from the previous selection", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(feedChore, null, "2026-07-02", [], 1, null, {
      volumeUnit: "ml",
      cachedIndicators: ["🍼 formula"],
      // Polluted cache: a stale breast volume the user never selected.
      cachedIndicatorVolumes: { "🍼 formula": 150, "🤱 breast": 150 },
    });
    const field = volumeField(html, "🤱 breast");
    assert.equal(field.value, '', "breast stale volume must not be preselected");
    // Breast chip must be off.
    assert.match(html, /data-label="🤱 breast"[^>]*aria-pressed="false"|aria-pressed="false"[^>]*data-label="🤱 breast"/);
  });

  it("falls back to chore defaults when there is no previous log", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(feedChore, null, "2026-07-02", [], 1, null, {
      volumeUnit: "ml",
      cachedIndicators: null,
      cachedIndicatorVolumes: null,
    });
    assert.match(html, /data-label="🍼 formula"[^>]*aria-pressed="true"|aria-pressed="true"[^>]*data-label="🍼 formula"/);
    assert.match(html, /data-label="🤱 breast"[^>]*aria-pressed="false"|aria-pressed="false"[^>]*data-label="🤱 breast"/);
    const field = volumeField(html, "🍼 formula");
    assert.equal(field.value, '', "no volume when nothing cached");
  });

  it("never echoes the previous selection for plain chip chores (e.g. Laundry)", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const laundryChore = {
      id: 2, icon: "👕", name: "Laundry", color: "#000", hasVolumeML: false,
      indicatorLabels: ["🧺 washed", "👖 folded"],
      indicatorDefaults: ["🧺 washed"],
    };
    const html = renderLogSheet(laundryChore, null, "2026-07-02", [], 1, null, {
      volumeUnit: "ml",
      // A prior log selected the non-default chip; plain chip chores must
      // NOT echo it — the sheet always starts from the chore's defaults.
      cachedIndicators: ["👖 folded"],
      cachedIndicatorVolumes: null,
    });
    assert.match(html, /data-label="🧺 washed"[^>]*aria-pressed="true"|aria-pressed="true"[^>]*data-label="🧺 washed"/);
    assert.match(html, /data-label="👖 folded"[^>]*aria-pressed="false"|aria-pressed="false"[^>]*data-label="👖 folded"/);
    // And no per-indicator volume selects render for non-volume chores.
    assert.ok(!html.includes("indicator-volume-select"), "no volume pickers for chip-only chore");
  });

  it("keeps every indicator off when cachedIndicators is an empty array", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(feedChore, null, "2026-07-02", [], 1, null, {
      volumeUnit: "ml",
      // The user deselected every indicator in the open sheet: the
      // in-progress selection is an empty array (not null) and must win
      // over the latest log's echo, including its stale per-indicator
      // volumes.
      cachedIndicators: [],
      cachedIndicatorVolumes: { "🍼 formula": 50, "🤱 breast": 95 },
    });
    assert.match(html, /data-label="🍼 formula"[^>]*aria-pressed="false"|aria-pressed="false"[^>]*data-label="🍼 formula"/);
    assert.match(html, /data-label="🤱 breast"[^>]*aria-pressed="false"|aria-pressed="false"[^>]*data-label="🤱 breast"/);
    const formula = volumeField(html, "🍼 formula");
    assert.equal(formula.value, '', "stale formula volume must not be preselected");
    const breast = volumeField(html, "🤱 breast");
    assert.equal(breast.value, '', "stale breast volume must not be preselected");
  });
});

describe("Stats: user-defined widgets (Phase 4)", () => {
  const baseState = () => ({
    chores: [{ id: 12, name: "Feed", icon: "🍼", color: "#000", metricType: "amount", metricUnit: "mL" }],
    members: [{ userId: 1, displayName: "Ann", avatarColor: "#000" }],
    latestLogs: {},
    stats: { widgetData: {} },
  });

  it("renders a total widget from the period-scoped summary", async () => {
    const { renderWidgetSection } = await import("../stats.js");
    const state = baseState();
    const w = { id: "abc", type: "total", metric: "amount", period: "week", choreIds: [12], title: "Bottles" };
    // The server bounds the total to the period; the client just reads it.
    state.stats.widgetData["abc"] = [{ chore: state.chores[0], summary: { totalML: 150, metricUnit: "mL" } }];
    const html = renderWidgetSection(w, state);
    assert.ok(html.includes("Bottles"));
    assert.ok(html.includes("150"));
    assert.ok(html.includes("mL"));
  });

  it("member-split reads the period-scoped summary byMember", async () => {
    const { renderWidgetSection } = await import("../stats.js");
    const state = baseState();
    state.members = [{ userId: 1, displayName: "Ann", avatarColor: "#000" }, { userId: 2, displayName: "Bo", avatarColor: "#111" }];
    const w = { id: "ms", type: "member-split", metric: "count", period: "week", choreIds: [12], title: "Split" };
    state.stats.widgetData["ms"] = [{ chore: state.chores[0], summary: { byMember: [{ userId: 1, count: 3 }, { userId: 2, count: 1 }] } }];
    const html = renderWidgetSection(w, state);
    assert.ok(html.includes("Ann"));
    assert.ok(html.includes("Bo"));
  });

  it("escapes a malicious widget title so it renders inert", async () => {
    const { renderWidgetSection } = await import("../stats.js");
    const state = baseState();
    const w = { id: "x1", type: "total", metric: "count", period: "week", choreIds: [], title: `<img src=x onerror="alert(1)">` };
    const html = renderWidgetSection(w, state);
    // The raw tag must not appear; it must be HTML-escaped so it renders inert.
    assert.ok(!html.includes("<img src=x"));
    assert.ok(html.includes("&lt;img"));
  });

  it("wizard has no period dropdown; widget card shows a day/week/month toggle", async () => {
    const { renderWidgetWizard, renderWidgetSection } = await import("../stats.js");
    const wiz = renderWidgetWizard({ chores: [] }, {});
    assert.ok(!wiz.includes('id="widget-period"'));

    const state = {
      chores: [{ id: 1, name: "C", icon: "x", color: "#000" }],
      members: [], latestLogs: {},
      stats: { widgetData: { w: [{ summary: { count: 1 } }] } },
    };
    const total = renderWidgetSection({ id: "w", type: "total", metric: "count", period: "week", choreIds: [1], title: "T" }, state);
    assert.ok(total.includes('data-action="widget-period"'));
    assert.ok(total.includes('data-period="day"'));
    assert.ok(total.includes('data-period="month"'));
    assert.ok(!total.includes('data-period="all"'));
    // The active button reflects the widget's stored period.
    assert.match(total, /period-toggle--active[^>]*data-period="week"|data-period="week"[^>]*period-toggle--active/);
    // last-done widgets have no period.
    const ld = renderWidgetSection({ id: "ld", type: "last-done", choreIds: [1], title: "L" }, { ...state, stats: { widgetData: { ld: [] } } });
    assert.ok(!ld.includes('data-action="widget-period"'));
  });

  it("per-chore analytics card shows a day/week/month toggle reflecting the active period", async () => {
    const { renderChoreAnalyticsSection, choreAnalyticsGrain } = await import("../stats.js");
    const chore = { id: 7, name: "Laundry", icon: "🧺", metricType: "none" };
    const ts = { periods: [], byMember: [] };
    const html = renderChoreAnalyticsSection(chore, ts, [], "week");
    assert.ok(html.includes('data-action="chore-analytics-period"'));
    assert.ok(html.includes('data-chore-id="7"'));
    assert.ok(html.includes('data-period="day"'));
    assert.ok(html.includes('data-period="month"'));
    // The active button reflects the passed period; default is "day".
    assert.match(html, /period-toggle--active[^>]*data-period="week"|data-period="week"[^>]*period-toggle--active/);
    const dflt = renderChoreAnalyticsSection(chore, ts, [], undefined);
    assert.match(dflt, /period-toggle--active[^>]*data-period="day"|data-period="day"[^>]*period-toggle--active/);
    // Period maps to the endpoint's bucket grain.
    assert.equal(choreAnalyticsGrain("day"), "daily");
    assert.equal(choreAnalyticsGrain("week"), "weekly");
    assert.equal(choreAnalyticsGrain("month"), "monthly");
  });

  it("widget wizard lists chores and presentation options", async () => {
    const { renderWidgetWizard } = await import("../stats.js");
    const state = baseState();
    const html = renderWidgetWizard(state, { type: "total", metric: "count", period: "week" });
    assert.ok(html.includes("Add widget"));
    assert.ok(html.includes("Feed"));
    assert.ok(html.includes("widget-save"));
    assert.ok(html.includes("Big number"));
  });

  it("widgetGrain derives from period (month/all -> monthly)", async () => {
    const { widgetGrain } = await import("../stats.js");
    assert.equal(widgetGrain({}), "daily");
    assert.equal(widgetGrain({ period: "day" }), "daily");
    assert.equal(widgetGrain({ period: "week" }), "daily");
    assert.equal(widgetGrain({ period: "month" }), "monthly");
    assert.equal(widgetGrain({ period: "all" }), "monthly");
  });

});

describe("Utils: escapeHTML (attribute-safe)", () => {
  it("escapes angle brackets, ampersand, and quotes", async () => {
    const { escapeHTML } = await import("../utils.js");
    assert.equal(escapeHTML(`<b>&"'`), "&lt;b&gt;&amp;&quot;&#39;");
  });

  it("a double quote cannot break out of an attribute value", async () => {
    const { escapeHTML } = await import("../utils.js");
    const evil = `x" onmouseover="alert(1)`;
    const attr = `data-subject="${escapeHTML(evil)}"`;
    // The injected quote is neutralized, so no second attribute can appear.
    assert.ok(!attr.includes(`data-subject="x" onmouseover`));
    assert.ok(attr.includes("&quot;"));
  });

  it("preserves falsy handling (0/false/null -> empty)", async () => {
    const { escapeHTML } = await import("../utils.js");
    assert.equal(escapeHTML(0), "");
    assert.equal(escapeHTML(false), "");
    assert.equal(escapeHTML(null), "");
    assert.equal(escapeHTML("0"), "0");
    assert.equal(escapeHTML(123), "123");
  });
});

describe("Subject tagging: attribute XSS is inert", () => {
  it("a subject containing a quote does not break out of the chip attribute", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const chore = { id: 1, icon: "🍼", name: "Feed", color: "#000", subjects: [`x" onmouseover="alert(1)`] };
    const html = renderLogSheet(chore, null, "2026-07-02", [], 1, null, { volumeUnit: "ml" });
    // The raw handler injection must not appear as real markup.
    assert.ok(!html.includes(`" onmouseover="alert(1)"`));
    assert.ok(html.includes("&quot;"));
  });
});

describe("Utils: volume units", () => {
  it("formatVolume renders mL and oz", async () => {
    const { formatVolume } = await import("../utils.js");
    assert.equal(formatVolume(60, "ml"), "60 mL");
    assert.equal(formatVolume(null, "ml"), "");
    // 60 mL ≈ 2.03 oz → rounded to 1dp
    assert.equal(formatVolume(60, "oz"), "2 oz");
    assert.equal(formatVolume(30, "oz"), "1 oz");
    assert.equal(formatVolume(89, "oz"), "3 oz");
  });

  it("ozToMl / mlToOz round-trip within a rounding step", async () => {
    const { ozToMl, mlToOz } = await import("../utils.js");
    assert.equal(ozToMl(2), 59); // 2 * 29.5735 ≈ 59.15
    assert.ok(Math.abs(mlToOz(59) - 2) < 0.05);
  });

  it("volumeOptions returns mL-valued options with unit-specific labels", async () => {
    const { volumeOptions } = await import("../utils.js");
    const ml = volumeOptions("ml");
    assert.equal(ml[0].ml, 0);
    assert.equal(ml[1].label, "5 mL");
    assert.equal(ml[ml.length - 1].ml, 200);

    const oz = volumeOptions("oz");
    assert.equal(oz[0].label, "0.5 oz");
    // value is always canonical mL
    assert.equal(oz[0].ml, ozToMlLocal(0.5));
    function ozToMlLocal(o) { return Math.round(o * 29.5735); }
  });

  it("volumeOptions injects a selected mL not already present, keeping sort", async () => {
    const { volumeOptions } = await import("../utils.js");
    const oz = volumeOptions("oz", 137); // arbitrary mL not an oz preset
    assert.ok(oz.some(o => o.ml === 137));
    // still sorted ascending by ml
    for (let i = 1; i < oz.length; i++) {
      assert.ok(oz[i].ml >= oz[i - 1].ml);
    }
  });
});

describe("Hide notification badge preference", () => {
  it("loadPreferences reads hideNotificationBadge from the server", async () => {
    globalThis.fetch = async () => ({
      ok: true,
      json: async () => ({
        preferences: { hideNotificationBadge: true, volumeUnit: "ml" },
      }),
      headers: {
        get: (key) => key === "Content-Type" ? "application/json" : null,
      },
    });
    const { loadPreferences } = await import("../preferences.js");
    const { createAppState } = await import("../state.js");
    const state = createAppState();
    await loadPreferences(state);
    assert.equal(state.hideNotificationBadge, true);
  });

  it("loadPreferences defaults hideNotificationBadge to false when absent", async () => {
    globalThis.fetch = async () => ({
      ok: true,
      json: async () => ({ preferences: {} }),
      headers: {
        get: (key) => key === "Content-Type" ? "application/json" : null,
      },
    });
    const { loadPreferences } = await import("../preferences.js");
    const { createAppState } = await import("../state.js");
    const state = createAppState();
    await loadPreferences(state);
    assert.equal(state.hideNotificationBadge, false);
  });

  it("saveHideNotificationBadge PATCHes and adopts the server echo", async () => {
    let capturedBody = null;
    globalThis.fetch = async (_url, opts) => {
      capturedBody = opts?.body ?? null;
      return {
        ok: true,
        json: async () => ({
          preferences: { hideNotificationBadge: true, choreOrder: [7] },
        }),
        headers: {
          get: (key) => key === "Content-Type" ? "application/json" : null,
        },
      };
    };
    const { saveHideNotificationBadge } = await import("../preferences.js");
    const { createAppState } = await import("../state.js");
    const state = createAppState();
    state.choreOrder = [7];
    await saveHideNotificationBadge(state, true);
    assert.equal(state.hideNotificationBadge, true);
    assert.deepEqual(JSON.parse(capturedBody), { hideNotificationBadge: true });
    // Other prefs survive the round trip.
    assert.deepEqual(state.choreOrder, [7]);
  });

  it("saveHideNotificationBadge rolls back on failure", async () => {
    globalThis.fetch = async () => ({
      ok: false,
      status: 500,
      json: async () => ({ error: "boom" }),
      headers: {
        get: (key) => key === "Content-Type" ? "application/json" : null,
      },
    });
    const { saveHideNotificationBadge } = await import("../preferences.js");
    const { createAppState } = await import("../state.js");
    const state = createAppState();
    state.hideNotificationBadge = false;
    await saveHideNotificationBadge(state, true);
    assert.equal(state.hideNotificationBadge, false);
  });
});

describe("Calendar dates remain separate from instants", () => {
  it("advances inclusive ranges across month, year and leap-day boundaries", async () => {
    const { shiftDateStr } = await import("../utils.js");
    assert.equal(shiftDateStr("2026-09-10", 1), "2026-09-11");
    assert.equal(shiftDateStr("2026-12-31", 1), "2027-01-01");
    assert.equal(shiftDateStr("2028-02-28", 1), "2028-02-29");
    assert.equal(shiftDateStr("2026-03-09", -2), "2026-03-07");
  });
});

describe("Browser ownership lock", () => {
  it("refuses identity changes when a browser cannot hold an origin lock", async () => {
    const { withBrowserLock } = await import("../device-store.js");
    let ran = false;
    await assert.rejects(withBrowserLock("nabu-identity", () => { ran = true; }), /Update your browser/);
    assert.equal(ran, false);
  });
});

// Answers /api/stats/dashboard the way the server does: every read goes to
// `respond` as its own endpoint request and keeps its own status.
function statsServer(respond) {
  return async (path, options) => {
    const url = new URL(path, 'http://localhost:8080');
    if (url.pathname !== '/api/stats/dashboard') return respond(path, options);
    const results = await Promise.all(url.searchParams.getAll('read').map(async read => {
      const response = await respond(`/api/stats/${read}`, options);
      return {status:response.status, data:await response.json()};
    }));
    return new Response(JSON.stringify({results}), {headers:{'Content-Type':'application/json'}});
  };
}
const nextTask = () => new Promise(resolve => setTimeout(resolve, 0));

describe('Scoped chart loading and metric rendering', () => {
  it('rejects a late chart response after the period changes', async () => {
    const {createAppState} = await import('../state.js');
    const {loadStatsResource} = await import('../stats-data.js');
    const state = createAppState();
    let oldResolve;
    const old = new Promise(resolve => {oldResolve = resolve;});
    globalThis.fetch = async path => {
      if (path.endsWith('period=week')) await old;
      return new Response(JSON.stringify({breakdown:[{category:path.endsWith('period=week') ? 'old' : 'new'}]}), {headers:{'Content-Type':'application/json'}});
    };
    const pending = loadStatsResource(state,'categories');
    // Reads that start together share a request; a period switch comes later.
    await nextTask();
    try {
      state.stats.categoriesPeriod = 'month';
      await loadStatsResource(state,'categories');
      assert.equal(state.stats.categoriesBreakdown[0].category,'new');
    } finally {oldResolve(); await pending;}
    assert.equal(state.stats.categoriesBreakdown[0].category,'new');
    assert.equal(state.stats.loading.categories,false);
  });
  it('does not publish a deleted widget or a pre-reset request', async () => {
    const {createAppState,resetAuthedState} = await import('../state.js');
    const {loadStatsResource} = await import('../stats-data.js');
    for (const reset of [false,true]) {
      const state = createAppState();
      state.stats.widgets=[{id:'one',type:'total',choreIds:[1],period:'week'}];
      state.chores=[{id:1}];
      let release; const gate=new Promise(resolve=>{release=resolve;});
      globalThis.fetch=async()=>{await gate;return new Response(JSON.stringify({summary:{count:9}}),{headers:{'Content-Type':'application/json'}});};
      const pending=loadStatsResource(state,'widget','one');
      if(reset) resetAuthedState(state);else state.stats.widgets=[];
      release();await pending;
      assert.equal(state.stats.widgetData?.one,undefined);
    }
  });
  it('skips hidden sections and shares overlapping identical reads', async () => {
    const {createAppState} = await import('../state.js');
    const {STATS_SECTIONS} = await import('../stats.js');
    const {loadStatsPage,loadStatsResource} = await import('../stats-data.js');
    const state=createAppState();state.stats.sectionHidden=[...STATS_SECTIONS];
    const paths=[];
    globalThis.fetch=async path=>{paths.push(path);return new Response(JSON.stringify({overview:{}}),{headers:{'Content-Type':'application/json'}});};
    await Promise.all([loadStatsPage(state),loadStatsResource(state,'overview')]);
    assert.deepEqual(paths,['/api/stats/overview']);
  });
  it('renders generic units as text, labels datetime and escapes the concrete baby heading sink', async () => {
    const {renderLogSheet} = await import('../schedule.js');
    const {renderBabyCareSection} = await import('../stats.js');
    const chore={id:1,name:'Flour',icon:'📝',hasVolumeML:true,metricType:'amount',metricUnit:'g',indicatorLabels:[]};
    const html=renderLogSheet(chore,null,'2026-09-10',[],null,null,{showWhen:true,recentVolumes:[37]});
    assert.ok(html.includes('for="log-volume" class="field-label">Amount</label>'));assert.ok(html.includes('37 g'));assert.ok(!html.includes('37 mL'));
    assert.ok(html.includes('for="log-when"'));
    const rendered=renderBabyCareSection({stats:{babyTimeSeries:{feedBaby:{choreIcon:'<img src=x onerror=alert(1)>',choreName:'Feed Baby',periods:[]}}}});
    assert.ok(!rendered.includes('<img'));assert.ok(rendered.includes('&lt;img'));
  });
});

describe('Chart loading cleanup',()=>{
  it('preserves chart control identity as loading, errors and empty sections change',async()=>{
    const {renderStatsPage}=await import('../stats.js');
    const {morphInnerHTML}=await import('../morph.js');
    const {createAppState}=await import('../state.js');
    const state=createAppState();
    state.stats.loading={overview:true};
    state.stats.errors={activity:'Retry'};
    // A custom layout puts a previously empty recap ahead of Categories.
    state.stats.sectionOrder=['recap','categories','chores'];
    const root=document.createElement('div');document.body.appendChild(root);
    try {
      morphInnerHTML(root,renderStatsPage(state));
      const selector='[data-action="stats-period"][data-section="categories"][data-period="month"]';
      const button=root.querySelector(selector);button.focus();
      state.stats.overview={recap:{totalChores:5}};
      state.stats.loading.overview=false;state.stats.errors={};
      morphInnerHTML(root,renderStatsPage(state));
      assert.equal(root.querySelector(selector),button);
      assert.equal(document.activeElement,button);
      assert.equal(button.dataset.section,'categories');
    } finally {root.remove();}
  });
  it('publishes page progress while a chart is pending and suppresses superseded page callbacks',async()=>{
    const {createAppState}=await import('../state.js');
    const {loadStatsPage}=await import('../stats-data.js');
    const state=createAppState();
    state.chores=[{id:7,name:'Water Plants',metricType:'amount',indicatorLabels:[]}];
    state.stats.sectionHidden=['activity','busy-hours','chores','categories','top-chores','leaderboard','baby'];
    let release, ready;
    const gate=new Promise(resolve=>{release=resolve;});
    const progressed=new Promise(resolve=>{ready=resolve;});
    // Per-chore charts travel apart from the sections, so the overview
    // renders while a slower chart is still loading.
    globalThis.fetch=statsServer(async path=>{
      if(path.includes('/chores/7/time-series?period=daily')) await gate;
      return new Response(JSON.stringify(path.includes('/overview') ? {overview:{recap:{totalChores:42}}} : {timeSeries:{periods:[]}}),{headers:{'Content-Type':'application/json'}});
    });
    let updates=0;
    const pending=loadStatsPage(state,()=>{
      updates++;
      if(state.stats.overview) ready();
    });
    try {
      await progressed;
      assert.equal(state.stats.loading['chore:7'],true);
      assert.equal(state.stats.overview.recap.totalChores,42);
      state.stats.choreAnalyticsPeriod={7:'week'};
      await loadStatsPage(state);
      const before=updates;
      release();await pending;
      assert.equal(updates,before);
      assert.equal(state.stats.loading['chore:7'],false);
    } finally {release();await pending;}
  });
  it('loads every server-supported widget and shares identical child reads',async()=>{
    const {createAppState}=await import('../state.js');
    const {loadStatsWidgets}=await import('../stats-data.js');
    const state=createAppState();state.chores=[{id:42}];
    state.stats.widgets=Array.from({length:20},(_,index)=>({id:`w${index}`,type:'total',choreIds:[42],period:'week'}));
    let calls=0;
    globalThis.fetch=async()=>{calls++;return new Response(JSON.stringify({summary:{count:7}}),{headers:{'Content-Type':'application/json'}});};
    await loadStatsWidgets(state);
    assert.equal(Object.keys(state.stats.widgetData).length,20);
    assert.ok(state.stats.widgets.every(widget=>state.stats.widgetData[widget.id]?.[0]?.summary.count===7));
    assert.equal(calls,1);
  });
  it('finishes superseded keyed loads and hidden or deleted initial widgets',async()=>{
    const {createAppState}=await import('../state.js');
    const {loadStatsResource,loadStatsPage}=await import('../stats-data.js');
    const state=createAppState();
    let release;const gate=new Promise(resolve=>{release=resolve;});
    globalThis.fetch=async path=>{
      if(path.includes('period=week')) await gate;
      return new Response(JSON.stringify({leaderboard:[],summary:{count:1},overview:{}}),{headers:{'Content-Type':'application/json'}});
    };
    const first=loadStatsResource(state,'leaderboard');
    await nextTask();
    state.stats.leaderboardPeriod='day';await loadStatsResource(state,'leaderboard');
    release();await first;
    assert.equal(state.stats.loading['leaderboard:week'],false);
    assert.equal(state.stats.loading['leaderboard:day'],false);
    let releaseWidget;const widgetGate=new Promise(resolve=>{releaseWidget=resolve;});
    globalThis.fetch=async path=>{
      if(path.includes('/summary')) await widgetGate;
      return new Response(JSON.stringify({summary:{count:1},overview:{}}),{headers:{'Content-Type':'application/json'}});
    };
    state.stats.widgets=[{id:'new',type:'total',choreIds:[1],period:'week'}];
    const widget=loadStatsResource(state,'widget','new');
    state.stats.widgets=[];
    state.stats.sectionHidden=['activity','busy-hours','chores','categories','top-chores','leaderboard','baby'];
    await loadStatsPage(state);
    assert.equal(state.stats.loading['widget:new'],false);
    releaseWidget();await widget;
    assert.equal(state.stats.widgetData.new,undefined);
    assert.ok(Object.values(state.stats.loading).every(value=>!value));
  });
});

describe('Stats read batching',()=>{
  const json=(body,init={})=>new Response(JSON.stringify(body),{...init,headers:{'Content-Type':'application/json',...(init.headers||{})}});
  it('sends reads that start together as one request per lane and answers each caller',async()=>{
    const {statsGet,resetStatsBatch}=await import('../stats-batch.js');
    resetStatsBatch();
    const requests=[];
    globalThis.fetch=statsServer(async(path,options)=>json({path}));
    const recorded=globalThis.fetch;
    globalThis.fetch=async(path,options)=>{requests.push(path);return recorded(path,options);};
    const reads=['/api/stats/overview','/api/stats/leaderboard?period=week','/api/stats/overview',
      '/api/stats/chores/1/time-series?period=daily','/api/stats/chores/2/summary?period=week'];
    const results=await Promise.all(reads.map(path=>statsGet(path)));
    assert.deepEqual(results.map(result=>result.path),reads);
    assert.notEqual(results[0],results[2],'each caller gets its own copy');
    assert.deepEqual(requests.map(path=>new URL(path,'http://localhost').searchParams.getAll('read')),[
      ['overview','leaderboard?period=week'],
      ['chores/1/time-series?period=daily','chores/2/summary?period=week'],
    ]);
    requests.length=0;
    assert.deepEqual(await statsGet('/api/stats/heatmap'),{path:'/api/stats/heatmap'});
    assert.deepEqual(requests,['/api/stats/heatmap'],'a lone read uses its endpoint');
  });
  it('fails only the read the server refused, with its status and message',async()=>{
    const {statsGet,resetStatsBatch}=await import('../stats-batch.js');
    resetStatsBatch();
    globalThis.fetch=statsServer(async path=>path.includes('/chores/9/')
      ? json({error:'chore not found'},{status:404}) : json({ok:path}));
    const [missing,overview]=await Promise.allSettled([
      statsGet('/api/stats/chores/9/summary?period=week'),statsGet('/api/stats/chores/1/summary?period=week')]);
    assert.equal(missing.status,'rejected');
    assert.equal(missing.reason.status,404);
    assert.equal(missing.reason.message,'chore not found');
    assert.deepEqual(overview.value,{ok:'/api/stats/chores/1/summary?period=week'});
  });
  it('revalidates a repeated page with its ETag and splits more than 64 reads',async()=>{
    const {statsGet,resetStatsBatch}=await import('../stats-batch.js');
    resetStatsBatch();
    const seen=[];let version='v1';
    globalThis.fetch=async(path,options)=>{
      const reads=new URL(path,'http://localhost').searchParams.getAll('read');
      const tag=options.headers.get('If-None-Match');
      seen.push({count:reads.length,tag});
      if(tag===`"${version}"`) return new Response(null,{status:304,headers:{ETag:tag}});
      return json({results:reads.map(read=>({status:200,data:{read,version}}))},{headers:{ETag:`"${version}"`}});
    };
    const page=()=>Promise.all(['/api/stats/overview','/api/stats/heatmap'].map(path=>statsGet(path)));
    assert.equal((await page())[0].version,'v1');
    assert.deepEqual((await page()).map(result=>result.read),['overview','heatmap'],'304 reuses the kept results');
    version='v2';
    assert.equal((await page())[1].version,'v2');
    assert.deepEqual(seen,[{count:2,tag:null},{count:2,tag:'"v1"'},{count:2,tag:'"v1"'}]);

    seen.length=0;
    const charts=Array.from({length:70},(_,i)=>`/api/stats/chores/${i+1}/time-series?period=daily`);
    const results=await Promise.all(charts.map(path=>statsGet(path)));
    assert.deepEqual(results.map(result=>result.read),charts.map(path=>path.slice('/api/stats/'.length)));
    assert.deepEqual(seen.map(request=>request.count),[64,6]);

    // A page with a failed read is not kept, so its retry asks for fresh results.
    resetStatsBatch();seen.length=0;
    globalThis.fetch=async(path,options)=>{
      seen.push(options.headers.get('If-None-Match'));
      return json({results:[{status:500,data:{error:'failed'}},{status:200,data:{}}]},{headers:{ETag:'"v3"'}});
    };
    await Promise.allSettled([statsGet('/api/stats/overview'),statsGet('/api/stats/heatmap')]);
    await Promise.allSettled([statsGet('/api/stats/overview'),statsGet('/api/stats/heatmap')]);
    assert.deepEqual(seen,[null,null]);
  });
});

describe('Activity request completion ownership',()=>{
  it('keeps the older-page boundary when a local chore filter cancels an append',async()=>{
    const {createAppState}=await import('../state.js');
    const {loadActivity,setActivityChoreFilter}=await import('../activity-data.js');
    const state=createAppState();
    let release,entered=false,hold=true;
    const gate=new Promise(resolve=>{release=resolve;});
    const paths=[];
    globalThis.fetch=async path=>{
      paths.push(path);
      if(path.includes('before=') && hold) {entered=true;await gate;}
      const older=path.includes('before=');
      return new Response(JSON.stringify({logs:[{id:older?2:1,choreId:older?2:1}],start:older?'2026-08-25':'2026-09-01',hasMore:!older}),{headers:{'Content-Type':'application/json'}});
    };
    await loadActivity(state);
    const old=loadActivity(state,{append:true});assert.equal(entered,true);
    try {
      assert.equal(setActivityChoreFilter(state,[2]),false);
      assert.equal(state._historyLoadingMore,false);
      assert.equal(state.historyBefore,'2026-09-01');
    } finally {release();await old;hold=false;}
    assert.deepEqual(state.historyLogs.map(log=>log.id),[1]);
    await loadActivity(state,{append:true});
    assert.deepEqual(state.historyLogs.map(log=>log.id),[1,2]);
    assert.equal(paths.at(-1),'/api/logs/history?before=2026-09-01');
    assert.equal(state.historyHasMore,false);
  });
  it('awaits and rejects old searches and old identity generations',async()=>{
    const {createAppState,resetAuthedState}=await import('../state.js');
    const {loadActivity}=await import('../activity-data.js');
    for(const changeIdentity of [false,true]) {
      const state=createAppState();state.historySearch='first';
      let release,entered=false;const gate=new Promise(resolve=>{release=resolve;});
      globalThis.fetch=async path=>{
        if(path.includes('q=first')) {entered=true;await gate;}
        return new Response(JSON.stringify({logs:[{id:path.includes('q=first')?1:2,note:path}],hasMore:false}),{headers:{'Content-Type':'application/json'}});
      };
      const old=loadActivity(state);assert.equal(entered,true);
      try {
        if(changeIdentity) resetAuthedState(state);
        state.historySearch='second';await loadActivity(state);
        assert.deepEqual(state.historyLogs.map(log=>log.id),[2]);
      } finally {release();await old;}
      assert.deepEqual(state.historyLogs.map(log=>log.id),[2]);
      assert.equal(state.historyLoading,false);assert.equal(state.historyError,null);
    }
  });
  it('cannot append an old page after a filter replaces its query',async()=>{
    const {createAppState}=await import('../state.js');const {loadActivity}=await import('../activity-data.js');
    const state=createAppState();
    let release,entered=false;const gate=new Promise(resolve=>{release=resolve;});
    globalThis.fetch=async path=>{
      if(path.includes('before=')) {entered=true;await gate;}
      return new Response(JSON.stringify({logs:[{id:path.includes('before=')?99:path.includes('q=new')?2:1}],start:'2026-09-01',hasMore:true}),{headers:{'Content-Type':'application/json'}});
    };
    await loadActivity(state);const old=loadActivity(state,{append:true});assert.equal(entered,true);
    try {state.historySearch='new';await loadActivity(state);} finally {release();await old;}
    assert.deepEqual(state.historyLogs.map(log=>log.id),[2]);assert.equal(state._historyLoadingMore,false);
  });
});


describe("Empty Activity pagination", () => {
  it("keeps older-page access when no logs are loaded yet", async () => {
    const {renderHistoryView} = await import("../today.js");
    const root = document.createElement("div");
    root.innerHTML = renderHistoryView({historyLogs: [], historyHasMore: true});
    assert.ok(root.querySelector('[data-action="load-more-history"]'));
    assert.match(root.textContent, /look further back/);
    assert.doesNotMatch(root.textContent, /No completed chores yet/);
    root.innerHTML = renderHistoryView({historyLogs: [], historyHasMore: false});
    assert.equal(root.querySelector('[data-action="load-more-history"]'), null);
  });
});

describe('Notification feed ownership and recovery',()=>{
  it('rejects old page results after refresh, mutation and identity reset',async()=>{
    const {createAppState,resetAuthedState}=await import('../state.js');
    const {loadNotificationPage,mutateNotification}=await import('../notification-data.js');
    for(const action of ['refresh','delete','clear','reset']) {
      const state=createAppState();
      let release;const gate=new Promise(resolve=>{release=resolve;});let reads=0,entered=false;
      globalThis.fetch=async(path,options)=>{
        if(options.method==='DELETE') return new Response('{"status":"ok"}',{headers:{'Content-Type':'application/json'}});
        if(path.includes('cursor=')){entered=true;await gate;return new Response(JSON.stringify({notifications:[{id:60},{id:1}],nextCursor:'obsolete',unreadCount:3}),{headers:{'Content-Type':'application/json'}});}
        return new Response(JSON.stringify({notifications:[{id:++reads===1?60:70,isRead:false}],nextCursor:reads===1?'first':'current',unreadCount:1}),{headers:{'Content-Type':'application/json'}});
      };
      await loadNotificationPage(state);const old=loadNotificationPage(state,{append:true});assert.equal(entered,true);
      try {
        if(action==='delete' || action==='clear') await mutateNotification(state,action,60);
        else if(action==='reset') resetAuthedState(state);
        else await loadNotificationPage(state);
      } finally {release();await old;}
      assert.deepEqual(state.notifications.map(n=>n.id),action==='refresh'?[70]:[]);
      assert.equal(state.notificationCursor,action==='refresh'?'current':action==='delete'?'first':null);
      assert.equal(state.notificationLoadingMore,false);assert.equal(state.notificationError,null);
    }
  });
  it('does not reuse an old first-page transport after a confirmed deletion',async()=>{
    const {createAppState}=await import('../state.js');
    const {loadNotificationPage,mutateNotification}=await import('../notification-data.js');
    const state=createAppState();state.notifications=[{id:60,isRead:false}];
    let release;const gate=new Promise(resolve=>{release=resolve;});let calls=0;
    globalThis.fetch=async(_path,options)=>{
      if(options.method==='DELETE') return new Response('{"status":"ok"}',{headers:{'Content-Type':'application/json'}});
      if(++calls===1){await gate;return new Response('{"notifications":[{"id":60}],"unreadCount":1}',{headers:{'Content-Type':'application/json'}});}
      return new Response('{"notifications":[],"unreadCount":0}',{headers:{'Content-Type':'application/json'}});
    };
    const old=loadNotificationPage(state);
    try {await mutateNotification(state,'delete',60);await loadNotificationPage(state);assert.equal(calls,2);}
    finally {release();await old;}
    assert.deepEqual(state.notifications,[]);
  });
  it('clear all keeps data on failure, blocks duplicate actions, and resets the whole feed on retry', async () => {
    const {createAppState} = await import('../state.js');
    const {mutateNotification, loadNotificationPage} = await import('../notification-data.js');
    const {renderNotificationPanel} = await import('../notifications.js');
    const state = createAppState();
    const rows = [{id: 1, isRead: true}, {id: 2, isRead: false}];
    Object.assign(state, {notifications: rows, unreadNotifications: 25, notificationCursor: 'older'});
    globalThis.fetch = async () => new Response('{"error":"Clear failed"}', {status: 500, headers: {'Content-Type': 'application/json'}});
    assert.equal(await mutateNotification(state, 'clear'), false);
    assert.equal(state.notifications, rows);
    assert.equal(state.notificationCursor, 'older');
    assert.equal(state.unreadNotifications, 25);
    assert.equal(state.notificationError, 'Clear failed');
    assert.equal(state.notificationMutating, false);
    let release, calls = 0;
    const gate = new Promise(resolve => { release = resolve; });
    globalThis.fetch = async (path, options) => {
      calls++;
      assert.equal(path, '/api/notifications');
      assert.equal(options.method, 'DELETE');
      await gate;
      return new Response('{"status":"deleted"}', {headers: {'Content-Type': 'application/json'}});
    };
    const pending = mutateNotification(state, 'clear');
    try {
      assert.equal(calls, 1);
      const root = document.createElement('div');
      root.innerHTML = renderNotificationPanel(rows, state);
      assert.equal(root.querySelector('[data-action="mark-all-read"]').disabled, true);
      assert.equal(root.querySelector('[data-action="clear-all-notifications"]').disabled, true);
      assert.equal(await mutateNotification(state, 'clear'), false);
      assert.equal(await mutateNotification(state, 'all'), false);
      await loadNotificationPage(state);
      assert.equal(calls, 1);
    } finally { release(); assert.equal(await pending, true); }
    assert.deepEqual(state.notifications, []);
    assert.equal(state.notificationCursor, null);
    assert.equal(state.unreadNotifications, 0);
    assert.equal(state.notificationError, null);
    assert.equal(state.notificationMutating, false);
  });
  it('late clear success or failure cannot replace state after an account reset', async () => {
    const {createAppState, resetAuthedState} = await import('../state.js');
    const {mutateNotification} = await import('../notification-data.js');
    for (const status of [200, 500]) {
      const state = createAppState();
      state.notifications = [{id: 1, isRead: false}];
      let release, entered = false;
      const gate = new Promise(resolve => { release = resolve; });
      globalThis.fetch = async () => { entered = true; await gate; return new Response(status === 200 ? '{"status":"deleted"}' : '{"error":"Old error"}', {status, headers: {'Content-Type': 'application/json'}}); };
      const pending = mutateNotification(state, 'clear');
      try {
        assert.equal(entered, true);
        resetAuthedState(state);
        Object.assign(state, {notifications: [{id: 99}], notificationCursor: 'new', unreadNotifications: 7});
      } finally { release(); assert.equal(await pending, false); }
      assert.deepEqual(state.notifications, [{id: 99}]);
      assert.equal(state.notificationCursor, 'new');
      assert.equal(state.unreadNotifications, 7);
      assert.equal(state.notificationError, null);
      assert.equal(state.notificationMutating, false);
    }
  });
  it('preserves rows and cursor on errors; read pages still offer older access and escape metadata',async()=>{
    const {createAppState}=await import('../state.js');
    const {loadNotificationPage,mutateNotification}=await import('../notification-data.js');
    const {renderNotificationPanel}=await import('../notifications.js');
    const state=createAppState();state.notifications=[{id:60,title:'<img src=x onerror=alert(1)>',body:'<script>bad</script>',isRead:true}];state.notificationCursor='older';
    globalThis.fetch=async()=>new Response('{"error":"Temporary failure"}',{status:500,headers:{'Content-Type':'application/json'}});
    await loadNotificationPage(state,{append:true});
    assert.equal(state.notifications.length,1);assert.equal(state.notificationCursor,'older');assert.equal(state.notificationError,'Temporary failure');
    await mutateNotification(state,'delete',60);assert.equal(state.notifications.length,1);
    const root=document.createElement('div');root.innerHTML=renderNotificationPanel(state.notifications,state);
    assert.ok(root.querySelector('[data-action="more-notifications"]'));assert.equal(root.querySelector('img,script'),null);
    globalThis.fetch=async()=>new Response('{"notifications":[{"id":1,"isRead":false}],"unreadCount":1}',{headers:{'Content-Type':'application/json'}});
    await loadNotificationPage(state,{append:true});
    assert.deepEqual(state.notifications.map(n=>n.id),[60,1]);assert.equal(state.notificationCursor,null);assert.equal(state.notificationError,null);
  });
});

describe("Activity amount units", () => {
  it("uses each chore's unit for plain and per-indicator amounts, with escaping", async () => {
    const { renderHistoryView } = await import("../today.js");
    for (const [unit, preference, expected] of [['mg', 'ml', '25 mg'], ['g', 'oz', '25 g'], ['mL', 'ml', '25 mL'], ['mL', 'oz', '0.8 oz'], ['<mg>', 'ml', '25 &lt;mg&gt;']]) {
      for (const perIndicator of [true, false]) {
        const html = renderHistoryView({
          volumeUnit: preference,
          chores: [{ id: 1, name: 'Meds', icon: '💊', color: '#112233', hasVolumeML: true, metricType: 'amount', metricUnit: unit }],
          historyLogs: [{ id: 1, choreId: 1, completedAt: '2026-07-02T10:00:00Z', volumeML: 25,
            indicatorVolumes: perIndicator ? { Morning: 25 } : {} }],
        });
        assert.ok(html.includes(expected), `${unit}/${preference}/${perIndicator}: ${expected}`);
        assert.ok(!html.includes('<mg>'));
      }
    }
  });

  it("retains an edited indicator amount and visibly labels its custom unit", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet({ id: 1, name: 'Meds', icon: '💊', hasVolumeML: true,
      metricUnit: 'mg', indicatorLabels: ['Morning'] },
      { id: 2, indicators: ['Morning'], indicatorVolumes: { Morning: 25 } }, '2026-07-02', [], 1);
    assert.ok(html.includes('value="25"'));
    assert.ok(html.includes('<option value="mg" selected>mg</option>'));
    assert.ok(html.includes('type="number"'));
    assert.ok(!html.includes('Other amount…'));
  });
});

describe('Independent amount entry', () => {
  it('keeps units separate and preserves canonical oz compatibility', async () => {
    const {commonAmountUnits, amountInputValue, storedAmount, amountInputMax} = await import('../metrics.js');
    for (const unit of ['mcg','mg','g','mL','L','drops','tablets','capsules','puffs','units']) {
      assert.ok(commonAmountUnits.includes(unit));
      assert.equal(amountInputValue(200,unit),'200');
      assert.equal(storedAmount('200',unit),200);
    }
    assert.equal(storedAmount('4','oz'),118);
    assert.equal(storedAmount(amountInputValue(321,'oz'),'oz'),321);
    assert.equal(storedAmount(amountInputMax('oz'),'oz'),100000);
    assert.ok(Number(amountInputValue(100000,'oz')) <= amountInputMax('oz'));
    assert.equal(storedAmount('', 'mg'),null);
  });
});

describe('Activity number selectors', () => {
  it('offers displayed-unit values while retaining exact amounts outside the presets', async () => {
    const {renderLogSheet} = await import('../schedule.js');
    const {amountInputValue, storedAmount} = await import('../metrics.js');
    for (const [unit, stored] of [['mL', 12345], ['oz', 37], ['mg', 0]]) {
      const root = JSDOM.fragment(renderLogSheet({id: 1, name: 'Amount', hasVolumeML: true, metricUnit: unit},
        {volumeML: stored, metricUnit: unit}, '', [], 1));
      const input = root.querySelector('#log-volume');
      const picker = root.querySelector('[data-action="pick-number"]');
      assert.equal(input.value, amountInputValue(stored, unit));
      assert.equal(picker.value, input.value);
      assert.equal(storedAmount(picker.value, unit), stored);
      assert.ok(picker.querySelector('option[value=""]'), 'blank remains distinct from zero');
      assert.ok(picker.querySelector('option[value="0"]'));
      if (unit === 'oz') assert.equal(picker.querySelector('option[value="2.5"]').textContent, '2.5 oz');
    }
  });

  it('escapes indicator names and custom units in picker labels and choices', async () => {
    const {renderLogSheet} = await import('../schedule.js');
    const label = '<img src=x onerror="alert(1)">', unit = '<svg onload="alert(2)">';
    const root = JSDOM.fragment(renderLogSheet({id: 1, name: 'Custom', hasVolumeML: true,
      metricUnit: unit, indicatorLabels: [label], indicatorDefaults: [label]}, null, '', [], 1));
    assert.equal(root.querySelectorAll('img, [onload], [onerror]').length, 0);
    const picker = root.querySelector('[data-action="pick-number"]');
    assert.equal(picker.getAttribute('aria-label'), `Choose ${label} amount`);
    assert.equal(picker.querySelector('option[value="2"]').textContent, `2 ${unit}`);
  });

  it('keeps exact duration seconds and offers a bounded set of readable choices', async () => {
    const {renderLogSheet} = await import('../schedule.js');
    const root = JSDOM.fragment(renderLogSheet({id: 1, name: 'Walk', metricType: 'duration'},
      {durationSeconds: 301}, '', [], 1));
    const picker = root.querySelector('[data-action="pick-number"]');
    assert.equal(picker.value, '301');
    assert.equal(picker.querySelector('option[value="300"]').textContent, '5 min');
    assert.equal(picker.querySelector('option[value="86400"]').textContent, '24 hr');
    assert.ok([...picker.options].every(option => Number(option.value) <= 86400));
  });
});

describe('Member access disable/enable', () => {
  // renderHouseholdView builds invite URLs from window.location.
  globalThis.window = dom.window;
  const household = {id: 1, name: 'Sitter House', initials: 'SH', inviteCode: 'CODE123'};
  const owner = {id: 10, role: 'owner', email: 'owner@test.com'};
  const admin = {id: 11, role: 'admin', email: 'admin@test.com'};
  const ownerMember = {userId: 10, email: 'owner@test.com', displayName: 'Owner', avatarColor: '#111', emailVerified: true, role: 'owner', suspended: false};
  const adminMember = {userId: 11, email: 'admin@test.com', displayName: 'Admin', avatarColor: '#222', emailVerified: true, role: 'admin', suspended: false};
  const sitter = {userId: 12, email: 'sitter@test.com', displayName: 'Sitter', avatarColor: '#111', emailVerified: true, role: 'member', suspended: false};

  it('shows Disable access and the Disabled badge per member suspension state', async () => {
    const {renderHouseholdView} = await import('../household.js');
    const disabled = {...sitter, suspended: true};

    const root = JSDOM.fragment(renderHouseholdView(household, [ownerMember, {...sitter}], [], owner));
    const row = root.querySelector(`.member-row[data-user-id="${sitter.userId}"]`);
    assert.ok(row.querySelector('[data-action="toggle-member-access"][data-suspended="0"]'), 'owner gets a Disable button');
    assert.equal(row.querySelectorAll('.member-disabled-badge').length, 0);

    const root2 = JSDOM.fragment(renderHouseholdView(household, [ownerMember, disabled], [], owner));
    const row2 = root2.querySelector(`.member-row[data-user-id="${sitter.userId}"]`);
    assert.equal(row2.querySelector('.member-disabled-badge').textContent, 'Disabled');
    assert.equal(row2.querySelector('[data-action="toggle-member-access"]').textContent.trim(), 'Enable access');
    assert.equal(row2.querySelector('[data-action="toggle-member-access"]').dataset.suspended, '1');
  });

  it('grants admins Disable on members but never on owners or themselves', async () => {
    const {renderHouseholdView} = await import('../household.js');
    const root = JSDOM.fragment(renderHouseholdView(household, [ownerMember, adminMember, {...sitter}], [], admin));
    assert.ok(root.querySelector(`.member-row[data-user-id="${sitter.userId}"] [data-action="toggle-member-access"]`), 'admin can disable a member');
    assert.equal(root.querySelector(`.member-row[data-user-id="${ownerMember.userId}"] [data-action="toggle-member-access"]`), null, 'admin cannot touch owners');
    assert.equal(root.querySelector(`.member-row[data-user-id="${adminMember.userId}"] [data-action="toggle-member-access"]`), null, 'no self-disable');
    assert.equal(root.querySelector(`.member-row[data-user-id="${ownerMember.userId}"] [data-action="remove-member"]`), null, 'admin sees no role controls');
  });

  it('never offers disable on owners or self, and escapes hostile display names', async () => {
    const {renderHouseholdView} = await import('../household.js');
    const hostile = {...sitter, displayName: '<img src=x onerror="alert(1)">'};
    const root = JSDOM.fragment(renderHouseholdView(household, [ownerMember, hostile], [], owner));
    assert.equal(root.querySelectorAll('img').length, 0);
    assert.equal(root.querySelector(`.member-row[data-user-id="${ownerMember.userId}"] [data-action="toggle-member-access"]`), null, 'owner cannot disable self');
  });

  it('loadHousehold surfaces access_suspended responses as {suspended}', async () => {
    const realFetch = globalThis.fetch;
    globalThis.fetch = async () => ({
      ok: false, status: 403,
      headers: new Map([['Content-Type', 'application/json']]),
      json: async () => ({error: 'Your access to Sitter House is temporarily disabled.', status: 'access_suspended', household: 'Sitter House'}),
    });
    try {
      const {loadHousehold} = await import('../household.js');
      const data = await loadHousehold();
      assert.equal(data.suspended.status, 'access_suspended');
      assert.equal(data.suspended.household, 'Sitter House');
    } finally {
      globalThis.fetch = realFetch;
    }
  });

  it('replaces the setup empty state with the access-disabled card', async () => {
    const {renderAccessSuspendedCard, accessSuspendedMessage} = await import('../utils.js');
    const suspended = {household: 'Sitter House', error: 'Your access to Sitter House is temporarily disabled.'};

    assert.equal(accessSuspendedMessage({}), '');
    assert.equal(accessSuspendedMessage({accessSuspended: {household: 'Sitter House'}}),
      'Your access to Sitter House is temporarily disabled. Contact an administrator to re-enable access.');

    const root = JSDOM.fragment(renderAccessSuspendedCard({accessSuspended: suspended}));
    assert.equal(root.querySelector('h3').textContent, 'Access disabled');
    assert.match(root.querySelector('p').textContent, /temporarily disabled/);

    const hostile = JSDOM.fragment(renderAccessSuspendedCard({
      accessSuspended: {household: 'X', error: '<img src=x onerror="alert(1)">'},
    }));
    assert.equal(hostile.querySelectorAll('img').length, 0, 'the server message is escaped');
  });

  it('a disabled member sees the disabled card instead of setup prompts', async () => {
    const {renderHomeView} = await import('../home.js');
    const {renderChoresView} = await import('../chores.js');
    const state = {
      user: {id: 12, email: 'sitter@test.com'},
      household: null,
      chores: [],
      choreOrder: [],
      hiddenHomeChoreIDs: [],
      latestLogs: {},
      accessSuspended: {household: 'Sitter House', error: 'Your access to Sitter House is temporarily disabled. Contact an administrator to re-enable access.'},
    };

    for (const html of [renderHomeView(state), renderChoresView(state)]) {
      assert.match(html, /Access disabled/, 'shows the disabled heading');
      assert.doesNotMatch(html, /Set Up Household/, 'never invites a disabled member to set up');
      assert.doesNotMatch(html, /No household set up/, 'never claims they have no household');
    }

    // Without the suspension flag the normal empty states remain.
    const fresh = {...state, accessSuspended: null};
    assert.match(renderHomeView(fresh), /Set Up Household/);
    assert.match(renderChoresView(fresh), /No household set up/);
  });

  it('a disabled member sees the disabled card instead of empty stats', async () => {
    const {renderStatsPage} = await import('../stats.js');
    const state = {
      user: {id: 12, email: 'sitter@test.com'},
      household: null,
      chores: [],
      members: [],
      historicalMembers: [],
      volumeUnit: 'ml',
      accessSuspended: {household: 'Sitter House', error: 'Your access to Sitter House is temporarily disabled. Contact an administrator to re-enable access.'},
    };

    const html = renderStatsPage(state);
    assert.match(html, /Access disabled/, 'stats explains the real reason');
    assert.match(html, /temporarily disabled/);
    assert.doesNotMatch(html, /No chores yet|No data yet/, 'a disabled member is never told the household is empty');

    const normal = renderStatsPage({...state, accessSuspended: null, stats: {}});
    assert.doesNotMatch(normal, /Access disabled/, 'an active member keeps the normal stats page');
  });
});

describe("API keys settings card", () => {
  it("escapes key names and only offers revoke for active keys", async () => {
    const { renderApiKeys } = await import("../api-keys.js");
    const html = renderApiKeys({ apiKeys: [
      { id: "aaaaaaaaaaaaaaaa", name: '<img src=x onerror=alert(1)>', createdAt: "2026-10-16T00:00:00Z", lastUsedAt: null, revokedAt: null, invalidated: false },
      { id: "bbbbbbbbbbbbbbbb", name: "old", createdAt: "2026-10-01T00:00:00Z", lastUsedAt: null, revokedAt: "2026-10-02T00:00:00Z", invalidated: false },
      { id: "cccccccccccccccc", name: "stale", createdAt: "2026-10-01T00:00:00Z", lastUsedAt: null, revokedAt: null, invalidated: true },
    ] }, "https://nabu.example");
    const root = dom.window.document.createElement("div");
    root.innerHTML = html;
    assert.equal(root.querySelector("img"), null);
    assert.equal(root.querySelectorAll('[data-testid="api-key-row"]').length, 3);
    const revoke = root.querySelectorAll('[data-action="revoke-api-key"]');
    assert.equal(revoke.length, 1);
    assert.equal(revoke[0].dataset.keyId, "aaaaaaaaaaaaaaaa");
    assert.match(root.textContent, /Revoked/);
    assert.match(root.textContent, /password changed/);
    assert.equal(root.querySelector('[data-testid="api-key-issued"]'), null);
  });

  it("shows a newly issued token once", async () => {
    const { renderApiKeys } = await import("../api-keys.js");
    const root = dom.window.document.createElement("div");
    root.innerHTML = renderApiKeys({ apiKeys: [], apiKeyIssued: 'nabu_pat_x."y' });
    assert.equal(root.querySelector("#api-key-token").value, 'nabu_pat_x."y');
  });

  it("keeps the typed key name in its markup, escaped", async () => {
    const { renderApiKeys } = await import("../api-keys.js");
    const root = dom.window.document.createElement("div");
    root.innerHTML = renderApiKeys({ apiKeys: [], apiKeyName: 'Nightly "sync" <b>x</b>' });
    assert.equal(root.querySelector("#api-key-name").value, 'Nightly "sync" <b>x</b>');
    assert.equal(root.querySelector("b"), null);
    root.innerHTML = renderApiKeys({ apiKeys: [] });
    assert.equal(root.querySelector("#api-key-name").value, "");
  });

  it("keeps its form's nodes when a card appears above it", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const { renderApiKeys } = await import("../api-keys.js");
    // Settings cards that render once their data loads (notification
    // settings) land above this one.
    const settings = (above) => `<div class="settings-view"><div class="card mt-3" data-key="exports"></div>${above}${renderApiKeys({ apiKeys: [] })}<div class="card mt-3" data-key="account"></div></div>`;
    const root = document.createElement("div");
    document.body.appendChild(root);
    morphInnerHTML(root, settings(""));
    const input = root.querySelector("#api-key-name");
    input.focus();
    input.value = "Nightly sync";
    morphInnerHTML(root, settings('<div class="card mt-3"><h3>Notifications</h3></div>'));
    assert.equal(root.querySelector("#api-key-name"), input, "the field being typed in is the same node");
    assert.equal(input.value, "Nightly sync");
    root.remove();
  });
});

describe("Chore icon and member avatar sinks escape untrusted values", () => {
  const evilIcon = `<style>*{display:none}</style>`;
  const evilName = `<img src=x onerror=alert(1)>`;
  const evilColor = `red;}</style><img src=x onerror=alert(1)>`;
  const assertInert = (html) => {
    assert.ok(!html.includes("<style>"), html);
    assert.ok(!html.includes("<img"), html);
  };

  it("history row escapes the chore icon", async () => {
    const { renderHistoryView } = await import("../today.js");
    const html = renderHistoryView({
      chores: [{ id: 1, name: "Feed", icon: evilIcon, color: "#000000" }],
      members: [{ userId: 1, displayName: "Ann" }],
      historyLogs: [{ id: 1, choreId: 1, userId: 1, note: "", indicators: [], completedAt: "2026-07-02T10:00:00Z" }],
    });
    assertInert(html);
    assert.ok(html.includes("&lt;style&gt;"));
  });

  it("top chores list escapes the chore icon", async () => {
    const { renderTopChoresList } = await import("../stats.js");
    const html = renderTopChoresList([{ choreId: 1, choreName: "Feed", choreIcon: evilIcon, count: 2 }], "week");
    assertInert(html);
    assert.ok(html.includes("&lt;style&gt;"));
  });

  it("chore stats list escapes the chore icon", async () => {
    const { renderChoreStatsList } = await import("../stats.js");
    const html = renderChoreStatsList([{ choreId: 1, choreName: "Feed", choreIcon: evilIcon, totalInRange: 3 }], {}, "week");
    assertInert(html);
    assert.ok(html.includes("&lt;style&gt;"));
  });

  it("leaderboard and member lists escape the initial and avatar color", async () => {
    const { renderLeaderboardList, renderMemberList } = await import("../stats.js");
    const memberMap = { 1: { userId: 1, displayName: evilName, avatarColor: evilColor } };
    assertInert(renderLeaderboardList([{ userId: 1, count: 2 }], memberMap, "week"));
    assertInert(renderMemberList([{ userId: 1, count: 2 }], memberMap));
  });

  it("household member rows escape the initial and avatar color", async () => {
    const { renderHouseholdView } = await import("../household.js");
    const html = renderHouseholdView(
      { id: 1, name: "Home", initials: "H", inviteCode: "abc" },
      [{ userId: 2, displayName: evilName, email: "x@example.com", avatarColor: evilColor, role: "member" }],
      [],
      { id: 1, role: "owner" },
    );
    assertInert(html);
  });
});

describe("Offline Home snapshot", () => {
  const identity = { status:"active", userId:1, householdId:2, bindingId:"b1", revision:"r1" };
  const state = { user:{id:1}, household:{id:2, name:"Home"}, members:[{userId:1}], chores:[{id:3}],
    latestLogs:{3:{id:9}}, choreOrder:[3], hiddenHomeChoreIDs:[], volumeUnit:"oz",
    notifications:[{id:1}], historyLogs:[{id:5}] };

  it("saves only Home fields, bound to exactly this identity", async () => {
    const { pickSnapshot, snapshotMatches } = await import("../snapshot.js");
    const snapshot = pickSnapshot(state, identity);
    assert.deepEqual(snapshot.data.chores, [{id:3}]);
    assert.equal(snapshot.data.volumeUnit, "oz");
    assert.ok(!("notifications" in snapshot.data));
    assert.ok(!("historyLogs" in snapshot.data));
    assert.ok(snapshotMatches(snapshot, identity));
    for (const other of [{...identity, status:"unconfirmed"}, {...identity, status:"logout-pending"},
      {...identity, status:"signed-out"}, {...identity, bindingId:"b2"}, {...identity, userId:7},
      {...identity, householdId:8}, null]) {
      assert.ok(!snapshotMatches(snapshot, other), JSON.stringify(other));
    }
  });

  it("never saves offline, unconfirmed or mismatched state", async () => {
    const { canSnapshot } = await import("../snapshot.js");
    assert.ok(canSnapshot(state, identity));
    for (const extra of [{offline:true}, {sessionUnconfirmed:true}, {logoutPending:true}, {transitioning:true},
      {accessSuspended:{}}, {household:{id:99}}, {user:{id:5}}, {household:null}]) {
      assert.ok(!canSnapshot({...state, ...extra}, identity), JSON.stringify(extra));
    }
    assert.ok(!canSnapshot(state, {...identity, status:"changing"}));
  });

  it("restores Home fields with safe empty defaults", async () => {
    const { applySnapshot, pickSnapshot } = await import("../snapshot.js");
    const restored = {};
    applySnapshot(restored, pickSnapshot({ user:{id:1}, household:{id:2} }, identity));
    assert.equal(restored.household.id, 2);
    assert.deepEqual(restored.chores, []);
    assert.deepEqual(restored.latestLogs, {});
    assert.deepEqual(restored.members, []);
  });
});

describe("Log sheet: last dose prefill", () => {
  const meds = { id: 5, icon: "💊", name: "Cat Meds", color: "#000", hasVolumeML: true,
    metricType: "amount", metricUnit: "mg", indicatorLabels: [], indicatorDefaults: [] };
  const amountValue = html => html.match(/id="log-volume"[^>]*value="([^"]*)"/)?.[1];

  it("a new log of a plain amount chore starts from the latest dose", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(meds, null, "2026-10-26", [], 1, null, { volumeUnit: "ml", cachedAmount: 5, cachedMetricUnit: "mg" });
    assert.equal(amountValue(html), "5");
  });

  it("never shows a dose recorded in a different unit", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(meds, null, "2026-10-26", [], 1, null, { volumeUnit: "ml", cachedAmount: 5, cachedMetricUnit: "mL" });
    assert.equal(amountValue(html), "");
  });

  it("editing a log shows that log's amount, not the latest", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(meds, { id: 9, volumeML: 2, metricUnit: "mg", indicators: [] }, "2026-10-26", [], 1, null,
      { volumeUnit: "ml", cachedAmount: 5, cachedMetricUnit: "mg" });
    assert.equal(amountValue(html), "2");
  });
});

describe("Tab routes", () => {
  it("maps paths to tabs, with Home's alias and Stats deep links", async () => {
    const { tabForPath, routeForPath, statsSectionForPath } = await import("../router.js");
    assert.equal(tabForPath("/"), "/today");
    assert.equal(tabForPath("/today"), "/today");
    for (const path of ["/activity", "/schedule", "/settings", "/stats"]) assert.equal(tabForPath(path), path);
    assert.equal(tabForPath("/stats/chore/12"), "/stats");
    assert.equal(routeForPath("/stats/chore/12"), "/stats");
    assert.equal(statsSectionForPath("/stats/chore/12"), "chore:12");
    // Sign-in pages, email links, the marketing page and malformed links are not tabs.
    for (const path of ["/login", "/register", "/join", "/verify-email", "/home", "/stats/", "/stats/chore/0",
      "/stats/chore/012", "/stats/chore/1234567890", "/stats/chore/12/x", "/stats/chore/12%3Cb%3E", "/stats/chore/abc"]) {
      assert.equal(tabForPath(path), null, path);
      assert.equal(statsSectionForPath(path), null, path);
    }
    assert.equal(routeForPath("/login"), "/login");
    assert.equal(routeForPath(""), "/");
  });

  it("pushes a new tab, replaces in place, and leaves a URL that already shows the tab", async () => {
    const { syncTabURL } = await import("../router.js");
    const page = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost:8080/today" });
    const saved = { location: globalThis.location, history: globalThis.history };
    globalThis.location = page.window.location;
    globalThis.history = page.window.history;
    try {
      const entries = page.window.history.length;
      assert.equal(syncTabURL("/stats", { push: true }), true);
      assert.equal(page.window.location.pathname, "/stats");
      assert.equal(page.window.history.length, entries + 1);
      assert.equal(syncTabURL("/stats", { push: true }), false, "same tab adds no entry");

      // A replace keeps the entry's state, such as an open sheet's marker.
      page.window.history.replaceState({ nabuSheet: "t1" }, "", "/stats");
      assert.equal(syncTabURL("/"), true);
      assert.equal(page.window.location.pathname, "/today");
      assert.equal(page.window.history.state.nabuSheet, "t1");
      assert.equal(page.window.history.length, entries + 1);

      page.window.history.replaceState(null, "", "/");
      assert.equal(syncTabURL("/today"), false, "/ already shows Home");
      page.window.history.replaceState(null, "", "/stats/chore/12");
      assert.equal(syncTabURL("/stats"), false, "a deep link already shows Stats");
      assert.equal(syncTabURL("/login"), false, "sign-in pages are not tabs");
      assert.equal(page.window.location.pathname, "/stats/chore/12");
    } finally {
      globalThis.location = saved.location;
      globalThis.history = saved.history;
      page.window.close();
    }
  });
});

describe("Profile form", () => {
  it("escapes the name and selects the current color", async () => {
    const { renderProfileForm, PROFILE_COLORS } = await import("../profile.js");
    const html = renderProfileForm({ displayName: '"><img src=x onerror=alert(1)>', avatarColor: "#2e86ab" });
    assert.ok(!html.includes("<img"));
    assert.ok(html.includes("&quot;&gt;&lt;img"));
    const root = document.createElement("div");
    root.innerHTML = html;
    assert.equal(root.querySelector("#profile-name").value, '"><img src=x onerror=alert(1)>');
    assert.deepEqual([...root.querySelectorAll('input[name="avatarColor"]:checked')].map(i => i.value), ["#2E86AB"]);
    assert.equal(root.querySelectorAll('input[name="avatarColor"]').length, PROFILE_COLORS.length);
    assert.equal(root.querySelector("#profile-name").maxLength, 50);
  });

  it("leaves a color outside the palette unchecked, so saving a name keeps it", async () => {
    const { renderProfileForm } = await import("../profile.js");
    const root = document.createElement("div");
    root.innerHTML = renderProfileForm({ displayName: "Dana", avatarColor: "#123456" });
    assert.equal(root.querySelectorAll('input[name="avatarColor"]:checked').length, 0);
  });
});

describe("Household setup", () => {
  const packs = [
    { key: "baby", title: "Baby", icon: "👶", summary: "Feeds", nameLabel: "Baby's name" },
    { key: "plants", title: "Plants", icon: "🌱", summary: "Watering" },
    { key: "cat", title: '<b>Cats</b>', icon: '"><img src=x>', summary: "<i>s</i>", nameLabel: 'Cat"s name' },
  ];

  it("suggests a household name from the user's name", async () => {
    const { householdNameFor } = await import("../onboarding.js");
    assert.equal(householdNameFor("Dana O'Connor"), "The O'Connors' home");
    assert.equal(householdNameFor("  Sam   Jones "), "The Joneses' home");
    assert.equal(householdNameFor("Ada Fox"), "The Foxes' home");
    assert.equal(householdNameFor("Dana"), "Dana's home");
    assert.equal(householdNameFor(""), "");
    assert.equal(householdNameFor(`A ${"x".repeat(60)}`), "");
  });

  it("doesn't prefill the email-derived name and keeps a palette color", async () => {
    const { createOnboarding } = await import("../onboarding.js");
    const fresh = createOnboarding({ email: "dana.oconnor82@example.com", displayName: "dana.oconnor82", avatarColor: "#123456" });
    assert.equal(fresh.name, "");
    assert.equal(fresh.color, "#19323C");
    const named = createOnboarding({ email: "d@example.com", displayName: "Dana", avatarColor: "#7b5ea7" });
    assert.equal(named.name, "Dana");
    assert.equal(named.color, "#7B5EA7");
  });

  it("sends the picked packs in catalog order, without blank names", async () => {
    const { onboardingPicks } = await import("../onboarding.js");
    const draft = { picks: { cat: { on: true, names: [" Mongo ", "", "Roger"] }, plants: { on: true, names: ["ignored"] }, baby: { on: false, names: ["Ada"] } } };
    assert.deepEqual(onboardingPicks(draft, packs), [{ key: "plants" }, { key: "cat", names: ["Mongo", "Roger"] }]);
    assert.deepEqual(onboardingPicks({ picks: { cat: { on: true, names: ["  "] } } }, packs), [{ key: "cat" }]);
  });

  it("finds a name used twice across packs, ignoring case", async () => {
    const { duplicateName } = await import("../onboarding.js");
    assert.equal(duplicateName([{ key: "baby", names: ["Max"] }, { key: "plants" }, { key: "dog", names: ["Rex", "max"] }]), "max");
    assert.equal(duplicateName([{ key: "cat", names: ["Mongo", "Roger"] }]), "");
  });

  it("renders the picker from the draft and escapes pack metadata", async () => {
    const { renderOnboarding } = await import("../onboarding.js");
    const state = { packs, onboarding: { step: "packs", picks: { cat: { on: true, names: ["Mongo", '"><b>x'] } }, error: "", saving: false, name: "", household: "" } };
    const html = renderOnboarding(state);
    assert.ok(!html.includes("<img") && !html.includes("<b>") && !html.includes("<i>"));
    const root = document.createElement("div");
    root.innerHTML = html;
    assert.deepEqual([...root.querySelectorAll("[data-onboarding-pack]")].map(i => [i.dataset.onboardingPack, i.checked]), [["baby", false], ["plants", false], ["cat", true]]);
    assert.deepEqual([...root.querySelectorAll('[data-onboarding-pack-name="cat"]')].map(i => i.value), ["Mongo", '"><b>x']);
    assert.equal(root.querySelector("#pack-cat-name-1").getAttribute("aria-label"), 'Cat"s name 2');
    // Unpicked packs show no name fields; the picked one offers another.
    assert.equal(root.querySelectorAll('[data-onboarding-pack-name="baby"]').length, 0);
    assert.ok(root.querySelector('[data-action="onboarding-add-name"][data-pack="cat"]'));
    assert.equal(root.querySelector('#onboarding-packs-form button[type="submit"]').textContent, "Create household");
  });

  it("offers a retry of the packs once the household exists", async () => {
    const { renderOnboarding } = await import("../onboarding.js");
    const root = document.createElement("div");
    root.innerHTML = renderOnboarding({ packs, onboarding: { step: "packs", householdReady: true, picks: {}, error: "Your household is ready, but its activities weren't added.", saving: false } });
    assert.equal(root.querySelector('[data-step="household"]'), null);
    assert.equal(root.querySelector('#onboarding-packs-form button[type="submit"]').textContent, "Add activities");
    assert.equal(root.querySelector(".form-error").textContent, "Your household is ready, but its activities weren't added.");
  });

  it("links the new household's invite", async () => {
    const { renderOnboarding } = await import("../onboarding.js");
    const root = document.createElement("div");
    root.innerHTML = renderOnboarding({ household: { inviteCode: "AB&CD" }, onboarding: { step: "invite", picks: {} } });
    assert.equal(root.querySelector(".invite-link-url").textContent, "http://localhost:8080/join?code=AB%26CD");
    assert.equal(root.querySelector('[data-action="copy-invite-link"]').dataset.code, "AB&CD");
    // No Web Share in this browser, so only Copy.
    assert.equal(root.querySelector('[data-action="onboarding-share"]'), null);
    // A QR code of the same link, for someone in the room.
    const { renderQRCode } = await import("../qr.js");
    assert.equal(root.querySelector(".invite-qr svg").getAttribute("aria-label"), "QR code for the invite link");
    assert.equal(root.querySelector(".invite-qr path").getAttribute("d"),
      new dom.window.DOMParser().parseFromString(renderQRCode("http://localhost:8080/join?code=AB%26CD", ""), "image/svg+xml").querySelector("path").getAttribute("d"));
  });
});

describe("Invite QR codes", () => {
  // Decoded with Core Image and Vision when written, as were every version
  // (1-10), every mask and UTF-8 text from the same encoder.
  const GOLDEN = [
    "#######...####.#####..#######",
    "#.....#.#..###.##.##..#.....#",
    "#.###.#..#..#......##.#.###.#",
    "#.###.#...##.##...#...#.###.#",
    "#.###.#.#...##...###..#.###.#",
    "#.....#...#.##.#.#.##.#.....#",
    "#######.#.#.#.#.#.#.#.#######",
    "..........##.#...##.#........",
    "#.#.#.#..........#.##...#..#.",
    ".#...#..###.###......###.#..#",
    "##.##.###.####...#...#.##.###",
    "###.##.###.#.###.#.##......#.",
    "###..####..#.....#######.#.##",
    ".#...#...#.#.####.#.###..#..#",
    "....###.#.#...#..#....#..#.##",
    "#.#..#...#..##...##.#..#.#.#.",
    "##...##..###...###.####..#.##",
    ".###.#..#.##.#.###..#.#..##.#",
    "#.#..##.##....#..##...###..##",
    ".##.##.##..####.###..#.#.#.#.",
    "#.#.#.##..##..#..#..#####....",
    "........#.###......##...#.###",
    "#######......#...#.##.#.##.##",
    "#.....#...#....####.#...##...",
    "#.###.#.##.....#.#..#####....",
    "#.###.#.....#....#.##..##.###",
    "#.###.#.#..###.....#.#.###..#",
    "#.....#...#..#####.###.##..#.",
    "#######.#.#####.##..#.##...##",
  ];
  const picture = modules => modules.map(row => row.map(dark => (dark ? "#" : ".")).join(""));

  it("matches the standard's Reed-Solomon, format and version codes", async () => {
    const { reedSolomon, formatBits, versionBits } = await import("../qr.js");
    // "HELLO WORLD" at 1-M, the standard's worked example.
    assert.deepEqual(reedSolomon([32, 91, 11, 120, 209, 114, 220, 77, 67, 64, 236, 17, 236, 17, 236, 17], 10),
      [196, 35, 39, 119, 235, 215, 231, 226, 93, 23]);
    assert.equal(formatBits(0), 0b101010000010010);
    assert.equal(formatBits(5), 0b100000011001110);
    assert.equal(versionBits(7), 0x07c94);
    assert.equal(versionBits(10), 0x0a4d3);
  });

  it("encodes an invite link as a known-good matrix", async () => {
    const { encodeQR } = await import("../qr.js");
    const qr = encodeQR("https://nabu-app.com/join?code=7KQ2M9XWPA");
    assert.deepEqual([qr.version, qr.size, qr.mask], [3, 29, 0]);
    assert.deepEqual(picture(qr.modules), GOLDEN);
  });

  it("picks the smallest version that fits, up to 213 bytes", async () => {
    const { encodeQR, QR_MAX_BYTES } = await import("../qr.js");
    const version = n => encodeQR("x".repeat(n)).version;
    assert.deepEqual([version(14), version(15), version(42), version(43), version(122), version(123), version(212), version(213)],
      [1, 2, 3, 4, 7, 8, 10, 10]);
    // UTF-8 bytes count, not characters.
    assert.equal(encodeQR("é".repeat(7)).version, 1);
    assert.equal(encodeQR("é".repeat(8)).version, 2);
    assert.throws(() => encodeQR("x".repeat(QR_MAX_BYTES + 1)), RangeError);
  });

  it("draws the finder, timing and format patterns for every mask", async () => {
    const { encodeQR, formatBits } = await import("../qr.js");
    for (let mask = 0; mask < 8; mask++) {
      const { size, modules } = encodeQR("https://example.com/join?code=ABCDEFGHJK" + "x".repeat(mask * 20), { mask });
      const finder = ["#######", "#.....#", "#.###.#", "#.###.#", "#.###.#", "#.....#", "#######"];
      for (const [x, y] of [[0, 0], [size - 7, 0], [0, size - 7]]) {
        assert.deepEqual(picture(modules.slice(y, y + 7).map(row => row.slice(x, x + 7))), finder);
      }
      for (let i = 8; i < size - 8; i++) {
        assert.equal(modules[6][i], i % 2 === 0);
        assert.equal(modules[i][6], i % 2 === 0);
      }
      assert.equal(modules[size - 8][8], true);
      // Both copies of the format bits name level M and this mask.
      const bit = i => ((formatBits(mask) >>> i) & 1) === 1;
      const first = [...[0, 1, 2, 3, 4, 5, 7, 8].map(y => modules[y][8]), modules[8][7], ...[5, 4, 3, 2, 1, 0].map(x => modules[8][x])];
      const second = [...Array.from({ length: 8 }, (_, i) => modules[8][size - 1 - i]), ...Array.from({ length: 7 }, (_, i) => modules[size - 7 + i][8])];
      assert.deepEqual(first, Array.from({ length: 15 }, (_, i) => bit(i)));
      assert.deepEqual(second, Array.from({ length: 15 }, (_, i) => bit(i)));
    }
  });

  it("renders dark modules on white with a quiet zone and an escaped label", async () => {
    const { encodeQR, renderQRCode } = await import("../qr.js");
    const text = "https://nabu-app.com/join?code=7KQ2M9XWPA";
    const svg = new dom.window.DOMParser().parseFromString(renderQRCode(text, '"><script>x</script>'), "image/svg+xml").documentElement;
    assert.equal(svg.getAttribute("viewBox"), "0 0 37 37");
    assert.equal(svg.getAttribute("role"), "img");
    assert.equal(svg.getAttribute("aria-label"), '"><script>x</script>');
    assert.equal(svg.querySelectorAll("script").length, 0);
    assert.equal(svg.querySelector("rect").getAttribute("fill"), "#fff");
    assert.equal(svg.querySelector("path").getAttribute("fill"), "#000");
    // Each "M x y h n v1 h-n z" run covers dark modules only, offset by the
    // four-module quiet zone, and together they cover every dark module.
    const covered = new Set();
    for (const [, x, y, n] of svg.querySelector("path").getAttribute("d").matchAll(/M(\d+) (\d+)h(\d+)v1h-\3z/g)) {
      for (let i = 0; i < Number(n); i++) covered.add(`${Number(x) - 4 + i},${Number(y) - 4}`);
    }
    const dark = new Set();
    encodeQR(text).modules.forEach((row, y) => row.forEach((on, x) => { if (on) dark.add(`${x},${y}`); }));
    assert.deepEqual([...covered].sort(), [...dark].sort());
  });
});

describe("Invite page", () => {
  const page = async invite => {
    const { renderJoinView } = await import("../household.js");
    const root = document.createElement("div");
    root.innerHTML = renderJoinView(invite);
    return root;
  };

  it("says who invited you to which household, escaped", async () => {
    const root = await page({ code: "C", status: "ready", household: { name: '<img src=x onerror=alert(1)>Home', initials: "H" },
      inviter: { displayName: 'dana<script>', avatarColor: 'red;background:url(x)' } });
    assert.equal(root.querySelector(".invite-lead").textContent, "dana<script> invited you to join");
    assert.equal(root.querySelector("h1").textContent, "<img src=x onerror=alert(1)>Home");
    assert.equal(root.querySelectorAll("img, script").length, 0);
    // An invalid color falls back to the brand color.
    assert.equal(root.querySelector(".invite-avatar").getAttribute("style"), "background:#19323C");
    assert.equal(root.querySelector(".invite-avatar").textContent, "D");
    assert.ok(root.querySelector('[data-action="show-register"]') && root.querySelector('[data-action="show-login"]'));
  });

  it("shows the household's initials when no inviter is named", async () => {
    const root = await page({ code: "C", status: "ready", household: { name: "Flat 4", initials: "F4" }, inviter: null });
    assert.equal(root.querySelector(".invite-lead").textContent, "You're invited to join");
    assert.equal(root.querySelector(".invite-avatar--household").textContent, "F4");
    assert.equal((await page({ code: "C", status: "ready", household: { name: "Flat 4", initials: "" }, inviter: null })).querySelector(".invite-avatar"), null);
  });

  it("says a dead link is dead, and stays generic while loading or unreachable", async () => {
    assert.equal((await page({ code: "C", status: "invalid" })).querySelector("h1").textContent, "This invite link isn't valid any more");
    const loading = await page({ code: "C", status: "loading" });
    assert.equal(loading.querySelector("h1").textContent, "You're Invited!");
    assert.equal(loading.querySelector(".invite-card").getAttribute("aria-busy"), "true");
    assert.equal((await page({ code: "C", status: "unavailable" })).querySelector("h1").textContent, "You're Invited!");
    assert.equal((await page(null)).querySelector(".invite-card").getAttribute("aria-busy"), null);
  });

  it("shows the invite link's QR code in Settings on request", async () => {
    const { renderHouseholdView } = await import("../household.js");
    const owner = { id: 1, role: "owner" };
    const household = { name: "Home", initials: "H", inviteCode: "QRCODE1234" };
    const members = [{ userId: 1, displayName: "Dana", role: "owner" }];
    const root = document.createElement("div");
    root.innerHTML = renderHouseholdView(household, members, [], owner);
    const toggle = root.querySelector('[data-action="toggle-invite-qr"]');
    assert.deepEqual([toggle.textContent, toggle.getAttribute("aria-expanded")], ["QR code", "false"]);
    assert.equal(root.querySelector(".invite-qr"), null);
    root.innerHTML = renderHouseholdView(household, members, [], owner, { showInviteQR: true });
    assert.equal(root.querySelector('[data-action="toggle-invite-qr"]').textContent, "Hide QR code");
    assert.equal(root.querySelector(".invite-qr svg").getAttribute("aria-label"), "QR code for the invite link");
    // Members other than the owner see no invite link at all.
    root.innerHTML = renderHouseholdView({ ...household, inviteCode: "" }, members, [], { id: 2, role: "member" }, { showInviteQR: true });
    assert.equal(root.querySelector('[data-action="toggle-invite-qr"]'), null);
    assert.equal(root.querySelector(".invite-qr"), null);
  });
});

describe("Notifications offer after the first log", () => {
  // jsdom has no Web Push: stand in for a browser that supports it.
  const withPush = async ({ permission = "default", vapid = "BPUBKEY" } = {}, run) => {
    const saved = { Notification: globalThis.Notification, PushManager: globalThis.PushManager, localStorage: globalThis.localStorage, sw: navigator.serviceWorker };
    globalThis.Notification = { permission };
    globalThis.PushManager = function PushManager() {};
    globalThis.localStorage = dom.window.localStorage;
    navigator.serviceWorker = {};
    const meta = document.createElement("meta");
    meta.name = "vapid-public-key";
    meta.content = vapid;
    document.head.appendChild(meta);
    try { await run(); } finally {
      meta.remove();
      dom.window.localStorage.clear();
      globalThis.Notification = saved.Notification;
      globalThis.PushManager = saved.PushManager;
      globalThis.localStorage = saved.localStorage;
      navigator.serviceWorker = saved.sw;
    }
  };

  it("offers only after a log here, until answered, where Web Push works", async () => {
    const { shouldOfferPush, pushOfferState, setPushOfferState } = await import("../notifications.js");
    await withPush({}, async () => {
      assert.equal(shouldOfferPush(7), false, "not before a log");
      setPushOfferState(7, "due");
      assert.equal(pushOfferState(7), "due");
      assert.equal(shouldOfferPush(7), true);
      assert.equal(shouldOfferPush(8), false, "per account");
      assert.equal(shouldOfferPush(null), false);
      setPushOfferState(7, "done");
      assert.equal(shouldOfferPush(7), false, "answered");
    });
    for (const browser of [{ permission: "granted" }, { permission: "denied" }, { vapid: "" }]) {
      await withPush(browser, async () => {
        setPushOfferState(7, "due");
        assert.equal(shouldOfferPush(7), false, JSON.stringify(browser));
      });
    }
    // Without Web Push at all (no Notification), nothing is offered.
    assert.equal(shouldOfferPush(7), false);
  });

  it("renders the offer with Turn on and Not now", async () => {
    const { renderPushOffer } = await import("../notifications.js");
    const root = document.createElement("div");
    root.innerHTML = renderPushOffer();
    assert.equal(root.querySelector("h2").textContent, "Get reminders on this device?");
    assert.deepEqual([...root.querySelectorAll("button")].map(b => [b.textContent, b.dataset.action]),
      [["Turn on", "push-offer-enable"], ["Not now", "push-offer-dismiss"]]);
  });
});

describe("Wide layout", () => {
  const press = (key, init = {}, target = document.body) => ({ key, target, ctrlKey: false, metaKey: false, altKey: false,
    repeat: false, isComposing: false, defaultPrevented: false, ...init });

  it("maps 1–9, u and / to shortcuts, outside text fields and without modifiers", async () => {
    const { keyboardShortcut } = await import("../desktop.js");
    assert.deepEqual(keyboardShortcut(press("1")), { action: "log", index: 0 });
    assert.deepEqual(keyboardShortcut(press("9")), { action: "log", index: 8 });
    assert.equal(keyboardShortcut(press("0")), null);
    assert.deepEqual(keyboardShortcut(press("u")), { action: "undo" });
    assert.deepEqual(keyboardShortcut(press("U")), { action: "undo" });
    assert.deepEqual(keyboardShortcut(press("/")), { action: "search" });
    for (const flag of ["ctrlKey", "metaKey", "altKey", "repeat", "isComposing", "defaultPrevented"]) {
      assert.equal(keyboardShortcut(press("1", { [flag]: true })), null, flag);
    }
    const root = document.createElement("div");
    root.innerHTML = '<input><textarea></textarea><select></select><div contenteditable="true"><span></span></div><button></button>';
    for (const el of root.querySelectorAll("input, textarea, select, span")) {
      assert.equal(keyboardShortcut(press("u", {}, el)), null, el.tagName);
    }
    assert.deepEqual(keyboardShortcut(press("1", {}, root.querySelector("button"))), { action: "log", index: 0 });
  });

  it("lists Home first in the sidebar and in the middle of the tab bar", async () => {
    const { placeHomeTab } = await import("../desktop.js");
    const nav = document.createElement("nav");
    nav.innerHTML = '<span class="sidebar-brand"></span>'
      + ["stats", "activity", "today", "schedule", "settings"].map(n => `<a class="tab-item" data-nav="${n}"></a>`).join("")
      + '<p class="sidebar-shortcuts"></p>';
    const order = () => [...nav.querySelectorAll(".tab-item")].map(a => a.dataset.nav).join(",");
    placeHomeTab(nav, true);
    assert.equal(order(), "today,stats,activity,schedule,settings");
    placeHomeTab(nav, true);
    assert.equal(order(), "today,stats,activity,schedule,settings");
    assert.equal(nav.firstElementChild.className, "sidebar-brand");
    placeHomeTab(nav, false);
    assert.equal(order(), "stats,activity,today,schedule,settings");
  });

  it("numbers the first nine Home tiles", async () => {
    const { renderHomeView } = await import("../home.js");
    const chores = Array.from({ length: 11 }, (_, i) => ({ id: i + 1, name: `C${i + 1}`, icon: "x", color: "#000000" }));
    const state = { user: { id: 1 }, household: { id: 1 }, chores, latestLogs: {} };
    const root = document.createElement("div");
    root.innerHTML = renderHomeView(state, { keyHints: true });
    assert.deepEqual([...root.querySelectorAll(".home-chore-card")].map(t => t.getAttribute("aria-keyshortcuts")),
      ["1", "2", "3", "4", "5", "6", "7", "8", "9", null, null]);
    assert.deepEqual([...root.querySelectorAll(".home-card-key")].map(k => k.textContent),
      ["1", "2", "3", "4", "5", "6", "7", "8", "9"]);
    root.innerHTML = renderHomeView(state);
    assert.equal(root.querySelectorAll(".home-card-key").length, 0, "no numbers outside the wide layout");
    assert.equal(root.querySelectorAll("[aria-keyshortcuts]").length, 9);
  });

  it("shows the newest logs, pending ones included, under day headings, escaped", async () => {
    const { renderHomeActivity } = await import("../home.js");
    const evil = `<img src=x onerror=alert(1)>`;
    const midnight = new Date(); midnight.setHours(0, 0, 0, 0);
    const yesterday = new Date(midnight); yesterday.setDate(midnight.getDate() - 1); yesterday.setHours(12);
    const state = {
      chores: [{ id: 1, name: evil, icon: evil, color: `red;}</style>${evil}` }],
      members: [{ userId: 1, displayName: evil }],
      pendingLogs: [{ id: "pending-k1", choreId: 1, userId: 1, completedAt: new Date().toISOString(), _pending: true }],
      homeActivity: { loaded: true, logs: [
        { id: 3, choreId: 1, userId: 1, completedAt: yesterday.toISOString() },
        { id: 2, choreId: 1, userId: 1, note: evil, subject: evil, completedAt: midnight.toISOString() },
      ] },
    };
    const html = renderHomeActivity(state);
    assert.ok(!html.includes("<img"), html);
    assert.ok(!html.includes("</style>"), html);
    const root = document.createElement("div");
    root.innerHTML = html;
    assert.deepEqual([...root.querySelectorAll("h3")].map(h => h.textContent), ["Today", "Yesterday"]);
    const rows = [...root.querySelectorAll("button[data-log-id]")];
    assert.deepEqual(rows.map(r => r.dataset.logId), ["pending-k1", "2", "3"]);
    assert.ok(rows[0].disabled, "a pending log has nothing to open yet");
    assert.equal(rows[1].dataset.action, "view-log");
    assert.equal(root.querySelector('a[data-nav="activity"]').textContent, "See all");
  });

  it("shows at most 25 logs, and says when there are none or they failed", async () => {
    const { renderHomeActivity } = await import("../home.js");
    const logs = Array.from({ length: 30 }, (_, i) => ({ id: i + 1, choreId: 1, userId: 1,
      completedAt: new Date(Date.now() - i * 60000).toISOString() }));
    const root = document.createElement("div");
    root.innerHTML = renderHomeActivity({ chores: [{ id: 1, name: "Feed", icon: "🍼", color: "#000000" }], homeActivity: { loaded: true, logs } });
    assert.equal(root.querySelectorAll("button[data-log-id]").length, 25);
    root.innerHTML = renderHomeActivity({ homeActivity: { loaded: false, logs: [] } });
    assert.equal(root.querySelector(".home-activity-empty").textContent, "Loading…");
    root.innerHTML = renderHomeActivity({ homeActivity: { loaded: true, logs: [] } });
    assert.equal(root.querySelector(".home-activity-empty").textContent, "Nothing logged in the last 7 days.");
    root.innerHTML = renderHomeActivity({ homeActivity: { loaded: false, logs: [], error: "Couldn't load recent activity." } });
    assert.equal(root.querySelector('[data-action="retry-home-activity"]').textContent, "Retry");
  });
});

describe("Action registry", () => {
  it("runs the handler registered for an action, and only that one", async () => {
    const { registerActions, runAction } = await import("../actions.js");
    const calls = [];
    registerActions("click", { "registry-test-a"(event, el) { calls.push([event.type, el.id]); } });
    assert.equal(runAction("click", "registry-test-a", { type: "click" }, { id: "button" }), true);
    assert.deepEqual(calls, [["click", "button"]]);
    assert.equal(runAction("submit", "registry-test-a", { type: "submit" }, {}), false);
    assert.equal(runAction("click", "registry-test-missing", { type: "click" }, {}), false);
  });

  it("refuses a second handler for the same action", async () => {
    const { registerActions } = await import("../actions.js");
    registerActions("change", { "registry-test-b"() {} });
    assert.throws(() => registerActions("change", { "registry-test-b"() {} }), /registered twice/);
  });

  it("feature modules register their actions on import", async () => {
    const core = await import("../app-core.js");
    const { createAppState } = await import("../state.js");
    const { runAction } = await import("../actions.js");
    await import("../api-keys.js");
    await import("../exports.js");
    core.initState(createAppState());
    document.body.innerHTML = '<div id="app"></div>';
    let renders = 0;
    core.setRenderer(() => { renders++; });

    core.state.apiKeyIssued = "secret-shown-once";
    let prevented = false;
    assert.equal(runAction("click", "dismiss-api-key", { preventDefault() { prevented = true; } }, {}), true);
    assert.equal(core.state.apiKeyIssued, null);
    assert.equal(prevented, true);
    assert.equal(renders, 1);

    core.state.exportError = "old error";
    assert.equal(runAction("change", "export-range", {}, { dataset: { field: "start" }, value: "2026-01-01" }), true);
    assert.deepEqual(core.state.exportRange, { start: "2026-01-01" });
    assert.equal(core.state.exportError, null);

    assert.equal(runAction("change", "create-api-key", { target: { id: "api-key-name", value: "Nightly sync" } }, {}), true);
    assert.equal(core.state.apiKeyName, "Nightly sync");
    runAction("change", "create-api-key", { target: { id: "other-field", value: "x" } }, {});
    assert.equal(core.state.apiKeyName, "Nightly sync", "only the name field is the key name");
  });

  it("the notification panel and profile register their actions on import", async () => {
    const core = await import("../app-core.js");
    const { createAppState } = await import("../state.js");
    const { runAction } = await import("../actions.js");
    await import("../notification-panel.js");
    await import("../profile.js");
    core.initState(createAppState());
    document.body.innerHTML = `
      <div id="notif-panel-container"><p>open</p></div>
      <div id="profile-error" class="hidden"></div>
      <form id="profile-form"><input id="profile-name" value="  "></form>`;
    let prevented = 0;
    const event = { preventDefault() { prevented++; } };

    assert.equal(runAction("click", "close-notifications", event, {}), true);
    const panel = document.querySelector("#notif-panel-container");
    assert.equal(panel.hidden, true);
    assert.equal(panel.innerHTML, "");
    // Signed out, a refresh asks the server for nothing.
    assert.equal(runAction("click", "refresh-notifications", event, {}), true);
    assert.equal(prevented, 2);

    // A blank name is refused before any request.
    assert.equal(runAction("submit", "save-profile", event, document.querySelector("#profile-form")), true);
    const error = document.querySelector("#profile-error");
    assert.equal(error.textContent, "Enter your name");
    assert.equal(error.classList.contains("hidden"), false);
  });

  it("the sign-in screens register their actions on import", async () => {
    const core = await import("../app-core.js");
    const { createAppState } = await import("../state.js");
    const { registerActions, runAction } = await import("../actions.js");
    const { operatorLoginDestination } = await import("../auth.js");
    core.initState(createAppState());
    let renders = 0;
    core.setRenderer(() => { renders++; });
    document.body.innerHTML = `<div id="app"></div>
      <input id="login-password" type="password">
      <button data-action="toggle-password" data-target="login-password" aria-pressed="false" aria-label="Show password">Show</button>`;
    let prevented = 0;
    const event = { preventDefault() { prevented++; } };

    assert.equal(runAction("click", "show-register", event, { dataset: { action: "show-register" } }), true);
    assert.equal(core.state.currentRoute, "/register");
    assert.equal(runAction("click", "show-forgot-password", event, { dataset: { action: "show-forgot-password" } }), true);
    assert.equal(core.state.currentRoute, "/forgot-password");
    assert.equal(renders, 2);

    // Show/Hide changes only the field, so the other fields keep what was typed.
    const toggle = document.querySelector('[data-action="toggle-password"]');
    assert.equal(runAction("click", "toggle-password", event, toggle), true);
    assert.equal(document.querySelector("#login-password").type, "text");
    assert.equal(toggle.textContent, "Hide");
    assert.equal(toggle.getAttribute("aria-pressed"), "true");
    assert.equal(toggle.getAttribute("aria-label"), "Hide password");
    runAction("click", "toggle-password", event, toggle);
    assert.equal(document.querySelector("#login-password").type, "password");
    assert.equal(renders, 2);
    assert.equal(prevented, 4);

    for (const name of ["login", "register", "magic-link-request", "password-forgot", "password-reset"]) {
      assert.throws(() => registerActions("submit", { [name]() {} }), /registered twice/, `${name} is registered`);
    }
    for (const name of ["google-signin", "apple-signin"]) {
      assert.throws(() => registerActions("click", { [name]() {} }), /registered twice/, `${name} is registered`);
    }

    const savedWindow = globalThis.window;
    try {
      globalThis.window = { location: new URL("http://localhost:8080/login?next=/operator") };
      assert.equal(operatorLoginDestination(), "/operator");
      globalThis.window = { location: new URL("http://localhost:8080/login?next=https://evil.example") };
      assert.equal(operatorLoginDestination(), null);
      globalThis.window = { location: new URL("http://localhost:8080/register?next=/operator") };
      assert.equal(operatorLoginDestination(), null);
    } finally { globalThis.window = savedWindow; }
  });

  it("feature modules reach the identity flow app.js registers", async () => {
    const core = await import("../app-core.js");
    const calls = [];
    core.setIdentityFlow({
      finish: async (result, route, afterAdopt) => { calls.push(["finish", result, route, afterAdopt]); return true; },
      recover: async (error, route) => { calls.push(["recover", error, route]); },
      joinPendingInvite: async () => { calls.push(["join"]); },
    });
    const afterAdopt = () => {};
    assert.equal(await core.finishIdentity("result", "/settings", afterAdopt), true);
    await core.recoverIdentity("error");
    await core.joinPendingInvite();
    assert.deepEqual(calls, [["finish", "result", "/settings", afterAdopt], ["recover", "error", undefined], ["join"]]);
  });
});

// head-init.js sizes the app frame (--app-h) before first paint. It runs as a
// classic script, so each case runs its source against a stub window.
describe("head-init.js frame height", () => {
  const source = readFileSync(new URL("../head-init.js", import.meta.url), "utf8");
  function launch({ standalone, remembered = false, inset = 0, innerHeight = 812 }) {
    const stored = new Map(remembered ? [["nabu-translucent-status-bar", "1"]] : []);
    const props = {}, viewportListeners = {};
    let topInset = inset;
    const win = {
      innerHeight,
      screen: { width: 402, height: 874 },
      matchMedia: (query) => ({ matches: query.includes("standalone") && standalone }),
      visualViewport: { scale: 1, addEventListener: (type, fn) => { viewportListeners[type] = fn; } },
      addEventListener() {},
      localStorage: { getItem: (key) => stored.get(key) ?? null, setItem: (key, value) => stored.set(key, value) },
      getComputedStyle: () => ({ paddingTop: `${topInset}px` }),
    };
    const doc = {
      documentElement: { style: { setProperty: (name, value) => { props[name] = value; } }, appendChild() {}, removeChild() {} },
      createElement: () => ({ style: {} }),
    };
    new Function("window", "navigator", "document", "requestAnimationFrame", "setTimeout", source)(
      win, { standalone }, doc, () => {}, () => {});
    return {
      appH: () => props["--app-h"],
      remembered: () => stored.get("nabu-translucent-status-bar") === "1",
      // An older install's geometry settles after load: the top inset appears
      // and the viewport grows to the screen.
      settle(nextInset, nextInnerHeight) {
        topInset = nextInset;
        win.innerHeight = nextInnerHeight;
        viewportListeners.resize();
      },
    };
  }

  it("a Home Screen app under an opaque status bar is as tall as its viewport", () => {
    const app = launch({ standalone: true });
    assert.equal(app.appH(), "812px");
    assert.equal(app.remembered(), false);
  });

  it("an app added with a translucent status bar uses the screen height from first paint once it's known", () => {
    // At cold open its inset and viewport still read as an opaque app's.
    const app = launch({ standalone: true, remembered: true, inset: 0, innerHeight: 812 });
    assert.equal(app.appH(), "874px");
  });

  it("an app added with a translucent status bar is recognised by its top inset and remembered", () => {
    const app = launch({ standalone: true, inset: 62, innerHeight: 874 });
    assert.equal(app.appH(), "874px");
    assert.equal(app.remembered(), true);
  });

  it("an older install's first launch grows to the screen once its geometry settles", () => {
    const app = launch({ standalone: true, inset: 0, innerHeight: 812 });
    assert.equal(app.appH(), "812px");
    app.settle(62, 874);
    assert.equal(app.appH(), "874px");
    assert.equal(app.remembered(), true);
  });

  it("a browser tab is as tall as its viewport, whatever its insets", () => {
    const app = launch({ standalone: false, inset: 47, innerHeight: 700 });
    assert.equal(app.appH(), "700px");
    assert.equal(app.remembered(), false);
  });
});

describe("Household display", () => {
  const owner = { userId: 1, email: "owner@example.com", displayName: "Ada", avatarColor: "#2E86AB", role: "owner" };
  const member = { userId: 2, email: "sam@example.com", displayName: "", avatarColor: "#A23B72", role: "member" };
  const display = (overrides = {}) => ({
    user: { id: 1, householdId: 9, email: "owner@example.com" },
    household: { id: 9, name: "Home" },
    members: [owner, member, { userId: 3, email: "away@example.com", displayName: "Away", role: "member", suspended: true }],
    chores: [
      { id: 10, name: "Feed Cats", icon: "🐱", color: "#F59E0B", visibility: "household", subjects: [] },
      { id: 11, name: "Payroll", icon: "💼", color: "#2E86AB", visibility: "admins", subjects: [] },
      { id: 12, name: "Water Plants", icon: "🌱", color: "#386641", visibility: "household", subjects: [] },
    ],
    choreOrder: [12, 10, 11],
    hiddenHomeChoreIDs: [],
    latestLogs: {},
    display: null,
    ...overrides,
  });

  it("shows Home's tiles in order, without hidden or Admins-only ones, under the members who can log", async () => {
    const { renderDisplayView } = await import("../display.js");
    const root = JSDOM.fragment(renderDisplayView(display({ hiddenHomeChoreIDs: [10] })));
    assert.deepEqual([...root.querySelectorAll(".display-tile")].map(t => t.dataset.choreId), ["12"]);
    assert.deepEqual([...root.querySelectorAll(".display-person-name")].map(n => n.textContent), ["Ada", "sam@example.com"]);
    assert.equal(root.querySelector('[data-testid="display-prompt"]').textContent, "Tap who you are, then what you did");
    assert.ok(root.querySelector(".display-overlay").hasAttribute("hidden"));
  });

  it("names who it logs for, asks for someone first, and forgets them after two quiet minutes", async () => {
    const { renderDisplayView, PICK_IDLE_MS } = await import("../display.js");
    const prompt = (d) => JSDOM.fragment(renderDisplayView(display({ display: d }))).querySelector('[data-testid="display-prompt"]').textContent;
    const fresh = { pickedUserId: null, pickedAt: 0, needsPick: false, choosing: null, leaving: null };
    assert.equal(prompt({ ...fresh, pickedUserId: 2, pickedAt: Date.now() }), "Logging for sam@example.com");
    assert.equal(prompt({ ...fresh, needsPick: true }), "Tap who you are first");
    assert.equal(prompt({ ...fresh, pickedUserId: 2, pickedAt: Date.now() - PICK_IDLE_MS - 1000 }), "Tap who you are, then what you did");
    assert.equal(prompt({ ...fresh, pickedUserId: 3, pickedAt: Date.now() }), "Tap who you are, then what you did", "a suspended member can't be picked");
  });

  it("escapes members' and activities' text in the display and its subject chooser", async () => {
    const { renderDisplayView } = await import("../display.js");
    const hostile = '<img src=x onerror="alert(1)">';
    const html = renderDisplayView(display({
      members: [{ ...member, displayName: hostile, avatarColor: '"><script>alert(2)</script>' }],
      chores: [{ id: 10, name: hostile, icon: hostile, color: '#000;"><b>x', visibility: "household", subjects: [hostile] }],
      choreOrder: [10],
      display: { pickedUserId: null, pickedAt: 0, needsPick: false, choosing: 10, leaving: null },
    }));
    const root = JSDOM.fragment(html);
    assert.equal(root.querySelector("img"), null);
    assert.equal(root.querySelector("script"), null);
    assert.equal(root.querySelector("b"), null);
    assert.equal(root.querySelector('[data-action="display-log-subject"]').dataset.subject, hostile);
  });

  it("offers turning a device into a display to owners and admins only", async () => {
    const { renderDisplaySettings } = await import("../display.js");
    assert.match(renderDisplaySettings(display()), /Use this device as a household display/);
    const admin = display({ user: { id: 2, householdId: 9 }, members: [owner, { ...member, role: "admin" }] });
    assert.match(renderDisplaySettings(admin), /household-display/);
    assert.equal(renderDisplaySettings(display({ user: { id: 2, householdId: 9 } })), "");
    assert.equal(renderDisplaySettings(display({ household: null })), "");
  });

  it("keeps a salted hash of a 4 to 8 digit PIN, per account and household on this device", async () => {
    const { validPIN, hashPIN, displayActive, displayKey } = await import("../display.js");
    assert.ok(validPIN("1234") && validPIN("12345678"));
    assert.ok(!validPIN("123") && !validPIN("123456789") && !validPIN("12a4") && !validPIN(""));
    const hash = await hashPIN("1234", "salt");
    assert.match(hash, /^[0-9a-f]{64}$/);
    assert.equal(await hashPIN("1234", "salt"), hash);
    assert.notEqual(await hashPIN("1234", "pepper"), hash);
    assert.notEqual(await hashPIN("1235", "salt"), hash);

    const store = { [displayKey(1, 9)]: JSON.stringify({ salt: "salt", hash }) };
    globalThis.localStorage = { getItem: (k) => (k in store ? store[k] : null), setItem: () => {}, removeItem: () => {} };
    assert.equal(displayActive(display()), true);
    assert.equal(displayActive(display({ user: { id: 2, householdId: 9 } })), false, "another account on this device");
    assert.equal(displayActive(display({ household: { id: 8 } })), false, "another household");
    store[displayKey(1, 9)] = "{not json";
    assert.equal(displayActive(display()), false);
    delete globalThis.localStorage;
  });

  it("pauses after five wrong PINs, even for the right one, then lets it leave", async () => {
    const { tryLeave, MAX_PIN_TRIES, PIN_PAUSE_MS } = await import("../display.js");
    const leaving = {};
    const now = 1_000_000;
    for (let i = 1; i < MAX_PIN_TRIES; i++) {
      assert.deepEqual(tryLeave(leaving, false, now), { left: false, error: "That PIN isn't right." });
    }
    assert.deepEqual(tryLeave(leaving, false, now), { left: false, error: "Too many tries. Wait 30 seconds." });
    assert.equal(tryLeave(leaving, true, now + 1000).left, false);
    assert.match(tryLeave(leaving, true, now + 1000).error, /Wait 29 seconds/);
    assert.deepEqual(tryLeave(leaving, true, now + PIN_PAUSE_MS), { left: true, error: null });
  });
});
