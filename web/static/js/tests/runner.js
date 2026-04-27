import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body></body></html>", {
  url: "http://localhost:8080",
});

globalThis.document = dom.window.document;
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

describe("App", () => {
  it("creates state", async () => {
    const { createAppState } = await import("../state.js");
    const state = createAppState();
    assert.equal(state.user, null);
    assert.equal(state.networkOnline, true);
    assert.deepEqual(state.chores, []);
  });

  it("resets authed state", async () => {
    const { createAppState, resetAuthedState } = await import("../state.js");
    const state = createAppState();
    state.user = { email: "test@example.com" };
    state.household = { id: 1 };
    resetAuthedState(state);
    assert.equal(state.user, null);
    assert.equal(state.household, null);
  });

  it("morphInnerHTML updates root element", async () => {
    const { morphInnerHTML } = await import("../morph.js");
    const root = dom.window.document.createElement("div");
    root.innerHTML = "<p>Hello</p>";
    morphInnerHTML(root, "<p>World</p>");
    assert.equal(root.textContent, "World");
  });

  it("apiFetch includes CSRF token", async () => {
    const { getCSRFToken } = await import("../api.js");
    const token = getCSRFToken();
    assert.equal(token, "");
  });
});
