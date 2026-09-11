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
  });

  it("renders register view", async () => {
    const { renderRegisterView } = await import("../auth.js");
    const html = renderRegisterView();
    assert.ok(html.includes("Create Account"));
    assert.ok(html.includes("Confirm Password"));
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
    const breastSel = html.match(/data-indicator="🤱 breast"[\s\S]*?<\/select>/)?.[0] || "";
    assert.ok(breastSel.includes('<option value="95" selected>95 mL</option>'), "breast volume preselected");
  });

  it("does not prefill a volume for a type absent from the previous selection", async () => {
    const { renderLogSheet } = await import("../schedule.js");
    const html = renderLogSheet(feedChore, null, "2026-07-02", [], 1, null, {
      volumeUnit: "ml",
      cachedIndicators: ["🍼 formula"],
      // Polluted cache: a stale breast volume the user never selected.
      cachedIndicatorVolumes: { "🍼 formula": 150, "🤱 breast": 150 },
    });
    const breastSel = html.match(/data-indicator="🤱 breast"[\s\S]*?<\/select>/)?.[0] || "";
    assert.ok(!breastSel.includes('value="150" selected'), "breast stale volume must not be preselected");
    assert.match(breastSel, /<option value="" selected>--<\/option>/);
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
    const formulaSel = html.match(/data-indicator="🍼 formula"[\s\S]*?<\/select>/)?.[0] || "";
    assert.ok(formulaSel.includes('<option value="" selected>--</option>'), "no volume when nothing cached");
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
    assert.ok(html.includes('Amount (g)'));assert.ok(html.includes('37 g'));assert.ok(!html.includes('37 mL'));
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
    state.stats.sectionHidden=['activity','busy-hours','chores','top-chores','leaderboard','baby'];
    let release, ready;
    const gate=new Promise(resolve=>{release=resolve;});
    const progressed=new Promise(resolve=>{ready=resolve;});
    globalThis.fetch=async path=>{
      if(path.includes('breakdown?period=week')) await gate;
      return new Response(JSON.stringify(path.includes('/overview') ? {overview:{recap:{totalChores:42}}} : {breakdown:[]}),{headers:{'Content-Type':'application/json'}});
    };
    let updates=0;
    const pending=loadStatsPage(state,()=>{
      updates++;
      if(state.stats.overview) ready();
    });
    try {
      await progressed;
      assert.equal(state.stats.loading.categories,true);
      assert.equal(state.stats.overview.recap.totalChores,42);
      state.stats.categoriesPeriod='month';
      await loadStatsPage(state);
      const before=updates;
      release();await pending;
      assert.equal(updates,before);
      assert.equal(state.stats.loading.categories,false);
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
    for(const action of ['refresh','delete','reset']) {
      const state=createAppState();
      let release;const gate=new Promise(resolve=>{release=resolve;});let reads=0,entered=false;
      globalThis.fetch=async(path,options)=>{
        if(options.method==='DELETE') return new Response('{"status":"ok"}',{headers:{'Content-Type':'application/json'}});
        if(path.includes('cursor=')){entered=true;await gate;return new Response(JSON.stringify({notifications:[{id:60},{id:1}],nextCursor:'obsolete',unreadCount:3}),{headers:{'Content-Type':'application/json'}});}
        return new Response(JSON.stringify({notifications:[{id:++reads===1?60:70,isRead:false}],nextCursor:reads===1?'first':'current',unreadCount:1}),{headers:{'Content-Type':'application/json'}});
      };
      await loadNotificationPage(state);const old=loadNotificationPage(state,{append:true});assert.equal(entered,true);
      try {
        if(action==='delete') await mutateNotification(state,'delete',60);
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
    assert.ok(html.includes('Other amount…'));
  });
});
