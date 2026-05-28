import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

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
});

describe("Auth Views", () => {
  it("renders login view", async () => {
    const { renderLoginView } = await import("../auth.js");
    const html = renderLoginView();
    assert.ok(html.includes("Sign In"));
    assert.ok(html.includes("Choresy"));
    assert.ok(html.includes("Create Account"));
  });

  it("renders register view", async () => {
    const { renderRegisterView } = await import("../auth.js");
    const html = renderRegisterView();
    assert.ok(html.includes("Create Account"));
    assert.ok(html.includes("Confirm Password"));
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
    const html = renderVerifyEmailView(true);
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
      createdAt: "2026-04-01T00:00:00Z",
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

