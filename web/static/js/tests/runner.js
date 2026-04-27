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
  });

  it("resets authed state", async () => {
    const { createAppState, resetAuthedState } = await import("../state.js");
    const state = createAppState();
    state.user = { email: "test@example.com" };
    resetAuthedState(state);
    assert.equal(state.user, null);
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
