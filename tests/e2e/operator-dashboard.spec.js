import { test, expect } from "@playwright/test";

// Run with OPERATOR_E2E=1 against a fresh PostgreSQL stack configured with
// OPERATOR_OWNER_USER_ID=1. The standard in-memory CI stack leaves the feature off.
test.skip(process.env.OPERATOR_E2E !== "1", "operator E2E needs the enabled PostgreSQL stack");
// The fixture reserves database user 1, so a retry needs a fresh database.
test.describe.configure({ mode: "serial", retries: 0 });

const email = `operator-e2e-${Date.now()}@nabu.local`;
const password = "operator-local-password";
const mailpit = process.env.MAILPIT_URL || "http://localhost:18026";

function deferred() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
}

async function finishHeldResponses(page, areas, release, completed) {
  const before = await page.evaluate(areas => {
    if (!window.__operatorSettledCounts) {
      window.__operatorSettledCounts = {};
      window.addEventListener("operator-request-settled", event => {
        const counts = window.__operatorSettledCounts;
        counts[event.detail] = (counts[event.detail] || 0) + 1;
      });
    }
    return Object.fromEntries(areas.map(area => [area, window.__operatorSettledCounts[area] || 0]));
  }, areas);
  release();
  await Promise.all(completed.map(signal => signal.promise));
  await expect.poll(() => page.evaluate(before => Object.entries(before).every(([area, count]) =>
    (window.__operatorSettledCounts?.[area] || 0) > count), before)).toBe(true);
}

async function signInOwner(page) {
  await page.goto("/login");
  await page.locator("#login-email").fill(email);
  await page.locator("#login-password").fill(password);
  await page.locator("#login-form button[type=submit]").click();
  await expect(page.locator("#hh-indicator")).toBeVisible();
}

async function logOut(page) {
  await page.locator("#hh-indicator").click();
  await page.locator('[data-action="logout"]').click();
}

async function csrf(page) {
  return (await page.context().cookies()).find(cookie => cookie.name === "nabu_csrf")?.value || "";
}

async function register(page, address) {
  await page.goto("/register");
  await page.locator("#reg-email").fill(address);
  await page.locator("#reg-password").fill(password);
  await page.locator("#reg-confirm").fill(password);
  await page.locator("#register-form button[type=submit]").click();
  await expect(page.locator("#hh-indicator")).toBeVisible();
}

async function verificationToken(request, address) {
  let token;
  await expect.poll(async () => {
    const res = await request.get(`${mailpit}/api/v1/messages`);
    for (const msg of (await res.json()).messages || []) {
      if (!msg.Subject?.includes("Verify your")) continue;
      const raw = await (await request.get(`${mailpit}/api/v1/message/${msg.ID}/raw`)).text();
      if (raw.includes(address)) token = raw.match(/token=([A-Za-z0-9_-]+)/)?.[1];
      if (token) return true;
    }
    return false;
  }, { timeout: 20000 }).toBe(true);
  return token;
}

test("operator dashboard and scoped key lifecycle", async ({ browser }) => {
  const ownerContext = await browser.newContext();
  const owner = await ownerContext.newPage();
  const outsiderContext = await browser.newContext();
  const outsider = await outsiderContext.newPage();
  const keyContext = await browser.newContext();
  try {
    expect((await owner.goto("/operator")).status()).toBe(200);
    await expect(owner).toHaveURL(/\/login\?next=%2Foperator$/);
    await expect(owner.locator("#login-form")).toBeVisible();
    await register(owner, email);
    const profile = (await (await owner.request.get("/api/me")).json()).user;
    expect(profile.id, "start with a fresh stack so this configured owner is user 1").toBe(1);
    const token = await verificationToken(owner.request, email);
    await owner.goto(`/verify-email?token=${token}`);
    await expect(owner.getByRole("heading", { name: "Email Verified!" })).toBeVisible();

    const ownerCsrf = await csrf(owner);
    // Verification of a registration claim clears its old password. Set one
    // through the authenticated flow so this fixture can be used after E2E.
    const passwordSetup = await owner.request.post("/api/auth/password", {
      headers: { "X-CSRF-Token": ownerCsrf },
      data: { current_password: "", new_password: password },
    });
    expect(passwordSetup.status()).toBe(200);
    const reusableLogin = await owner.request.post("/api/auth/login", {
      headers: { "X-CSRF-Token": ownerCsrf }, data: { email, password },
    });
    expect(reusableLogin.status()).toBe(200);
    const household = await owner.request.post("/api/household", {
      headers: { "X-CSRF-Token": ownerCsrf }, data: { name: "Operator test household" },
    });
    expect(household.status()).toBe(201);
    const seeded = await owner.request.post("/api/chores/seed-defaults", { headers: { "X-CSRF-Token": ownerCsrf } });
    expect(seeded.ok()).toBe(true);
    const chores = (await (await owner.request.get("/api/chores")).json()).chores;
    const logged = await owner.request.post("/api/logs", {
      headers: { "X-CSRF-Token": ownerCsrf }, data: { choreId: chores[0].id, completedAt: new Date().toISOString() },
    });
    expect(logged.status()).toBe(201);

    await register(outsider, `operator-outsider-${Date.now()}@test.local`);
    const outsiderCsrf = await csrf(outsider);
    const outsideHousehold = await outsider.request.post("/api/household", {
      headers: { "X-CSRF-Token": outsiderCsrf }, data: { name: "Other household" },
    });
    expect(outsideHousehold.status()).toBe(201);
    expect((await outsider.goto("/operator")).status()).toBe(403);
    expect((await outsider.request.get("/api/operator/v1/users")).status()).toBe(403);

    await owner.goto("/operator");
    await expect(owner.getByRole("heading", { name: "App dashboard" })).toBeVisible();
    await expect(owner.locator("#users-body tr")).toHaveCount(2);
    await expect(owner.locator("#users-body")).toContainText(email);
    await expect(owner.locator("#households-body")).toContainText("Operator test household");
    await expect(owner.locator("#overview")).toContainText("Registered users");
    await owner.emulateMedia({ colorScheme: "dark" });
    await expect(owner.locator("html")).toHaveCSS("color-scheme", "dark");
    await expect(owner.locator("section").first()).toHaveCSS("background-color", "rgb(24, 28, 39)");
    await owner.emulateMedia({ colorScheme: "light" });
    await expect(owner.locator("html")).toHaveCSS("color-scheme", "light");
    await expect(owner.locator("section").first()).toHaveCSS("background-color", "rgb(255, 255, 255)");
    const loginContext = await browser.newContext();
    try {
      const loginPage = await loginContext.newPage();
      await loginPage.goto("/operator");
      await expect(loginPage).toHaveURL(/\/login\?next=%2Foperator$/);
      await loginPage.locator("#login-email").fill(email);
      await loginPage.locator("#login-password").fill(password);
      await loginPage.locator("#login-form button[type=submit]").click();
      await expect(loginPage).toHaveURL(/\/operator$/);
      await expect(loginPage.getByRole("heading", { name: "App dashboard" })).toBeVisible();
    } finally {
      await loginContext.close();
    }
    if (process.env.OPERATOR_SCREENSHOT) {
      await owner.screenshot({ path: process.env.OPERATOR_SCREENSHOT, fullPage: true });
    }

    // State-changing key management is protected by the existing CSRF layer.
    expect((await owner.request.post("/api/operator/v1/keys", {
      data: { name: "blocked", scope: "full", days: 7 }, headers: { "X-CSRF-Token": "invalid" },
    })).status()).toBe(403);

    await owner.locator('#key-form input[name="name"]').fill("Dashboard full access");
    await owner.locator('#key-form select[name="scope"]').selectOption("full");
    await owner.locator("#key-form button[type=submit]").click();
    await expect(owner.locator("#issued-key")).toBeVisible();
    const fullToken = await owner.locator("#token").inputValue();
    expect(fullToken).toMatch(/^nabu_op_/);
    await owner.locator("#dismiss-key").click();
    await owner.reload();
    await expect(owner.locator("#keys-body")).toContainText("Dashboard full access");
    expect(await owner.locator("#token").inputValue()).toBe("");

    const keyUsers = await keyContext.request.get("/api/operator/v1/users", { headers: { Authorization: `Bearer ${fullToken}` } });
    expect(keyUsers.status()).toBe(200);
    expect((await keyUsers.json()).users.map(user => user.email)).toContain(email);
    const fullKeyID = (await (await owner.request.get("/api/operator/v1/keys")).json()).keys.find(key => key.name === "Dashboard full access").id;
    const revoked = await owner.request.delete(`/api/operator/v1/keys/${fullKeyID}`, { headers: { "X-CSRF-Token": await csrf(owner) } });
    expect(revoked.status()).toBe(204);
    expect((await keyContext.request.get("/api/operator/v1/users", { headers: { Authorization: `Bearer ${fullToken}` } })).status()).toBe(401);

    const summaryKey = await owner.request.post("/api/operator/v1/keys", {
      headers: { "X-CSRF-Token": await csrf(owner) }, data: { name: "Summary only", scope: "summary", days: 7 },
    });
    expect(summaryKey.status()).toBe(201);
    const summaryToken = (await summaryKey.json()).token;
    expect((await keyContext.request.get("/api/operator/v1/summary", { headers: { Authorization: `Bearer ${summaryToken}` } })).status()).toBe(200);
    expect((await keyContext.request.get("/api/operator/v1/users", { headers: { Authorization: `Bearer ${summaryToken}` } })).status()).toBe(401);
  } finally {
    await ownerContext.close();
    await outsiderContext.close();
    await keyContext.close();
  }
});

test("operator tabs clear reports and one-time keys across logout, account switch, and late replies", async ({ browser }) => {
  test.setTimeout(120000);
  const context = await browser.newContext();
  try {
    const app = await context.newPage();
    await signInOwner(app);
    const dashboard = await context.newPage();
    await dashboard.goto("/operator");
    await expect(dashboard.locator("#users-body")).toContainText(email);
    await dashboard.locator('#key-form input[name="name"]').fill("Clear on logout");
    await dashboard.locator("#key-form button[type=submit]").click();
    await expect(dashboard.locator("#issued-key")).toBeVisible();
    expect(await dashboard.locator("#token").inputValue()).toMatch(/^nabu_op_/);

    const entered = deferred(), release = deferred(), completed = deferred();
    await dashboard.route("**/api/operator/v1/users?*", async route => {
      try {
        const response = await route.fetch();
        entered.resolve();
        await release.promise;
        await route.fulfill({ response });
      } finally { completed.resolve(); }
    });
    try {
      await dashboard.locator("#refresh").click();
      await entered.promise;
      await logOut(app);
      await expect(app.locator("#login-form")).toBeVisible();
      await expect(dashboard.locator("main")).toBeHidden();
      await expect(dashboard.locator("#issued-key")).toBeHidden();
      expect(await dashboard.locator("#token").inputValue()).toBe("");
      await register(app, `operator-switch-${Date.now()}@test.local`);
      await finishHeldResponses(dashboard, ["users"], () => release.resolve(), [completed]);
      await expect(dashboard.locator("#users-body tr")).toHaveCount(0);
      await expect(dashboard.locator("#overview")).toBeEmpty();
      expect(await dashboard.locator("#token").inputValue()).toBe("");
    } finally { release.resolve(); }
  } finally { await context.close(); }
});

test("unfinished logout blocks direct operator navigation, reload, and key creation", async ({ browser }) => {
  test.setTimeout(120000);
  const context = await browser.newContext();
  try {
    const app = await context.newPage();
    await signInOwner(app);
    // Keep the advisory mirror stale while the durable record becomes pending.
    await app.evaluate(() => {
      const set = Storage.prototype.setItem;
      Storage.prototype.setItem = function(key, value) {
        if (key === "nabu-browser-identity") throw new Error("mirror unavailable");
        return set.call(this, key, value);
      };
    });
    await app.route("**/api/auth/logout", route => route.abort("connectionreset"));
    await logOut(app);
    await expect(app.getByRole("heading", { name: "Sign-out is unfinished" })).toBeVisible();
    const dashboard = await context.newPage();
    let operatorRequests = 0;
    dashboard.on("request", request => {
      if (new URL(request.url()).pathname.startsWith("/api/operator/")) operatorRequests++;
    });
    expect((await dashboard.goto("/operator")).status()).toBe(200);
    await expect(dashboard.locator("#message")).toContainText("Sign-out is unfinished");
    await expect(dashboard.locator("main")).toBeHidden();
    await dashboard.locator("#key-form").evaluate(form => {
      form.elements.name.value = "Must stay blocked";
      form.requestSubmit();
    });
    await dashboard.reload();
    await expect(dashboard.locator("#message")).toContainText("Sign-out is unfinished");
    await expect(dashboard.locator("main")).toBeHidden();
    expect(await dashboard.locator("#token").inputValue()).toBe("");
    expect(operatorRequests).toBe(0);
    const unreadable = await context.newPage();
    await unreadable.addInitScript(() => {
      const open = IDBFactory.prototype.open;
      IDBFactory.prototype.open = function(name, ...args) {
        if (name === "nabu-device") throw new Error("authority unavailable");
        return open.call(this, name, ...args);
      };
    });
    let unreadableRequests = 0;
    unreadable.on("request", request => {
      if (new URL(request.url()).pathname.startsWith("/api/operator/")) unreadableRequests++;
    });
    await unreadable.goto("/operator");
    await expect(unreadable.locator("#message")).toContainText("could not be confirmed");
    await expect(unreadable.locator("main")).toBeHidden();
    expect(unreadableRequests).toBe(0);
    await app.unroute("**/api/auth/logout");
    await app.locator('[data-action="retry-logout"]').click();
    await expect(app.locator("#login-form")).toBeVisible();
    expect((await (await app.request.get("/api/me")).json()).user).toBeNull();
  } finally { await context.close(); }
});

test("operator tab clears when another tab cannot read sign-out authority", async ({ browser }) => {
  test.setTimeout(120000);
  const context = await browser.newContext();
  try {
    const app = await context.newPage();
    await signInOwner(app);
    const dashboard = await context.newPage();
    await dashboard.goto("/operator");
    await expect(dashboard.locator("#users-body")).toContainText(email);
    await dashboard.locator('#key-form input[name="name"]').fill("Clear on unreadable sign-out");
    await dashboard.locator("#key-form button[type=submit]").click();
    await expect(dashboard.locator("#issued-key")).toBeVisible();
    await app.evaluate(() => {
      const open = IDBFactory.prototype.open;
      IDBFactory.prototype.open = function(name, ...args) {
        if (name === "nabu-device") throw new Error("authority unavailable");
        return open.call(this, name, ...args);
      };
    });
    await logOut(app);
    await expect(app.getByRole("heading", { name: "Sign-out is unfinished" })).toBeVisible();
    await expect(dashboard.locator("main")).toBeHidden();
    await expect(dashboard.locator("#message")).toBeVisible();
    expect(await dashboard.locator("#token").inputValue()).toBe("");
    expect((await (await app.request.get("/api/me")).json()).user).not.toBeNull();
  } finally { await context.close(); }
});

test("late key issuance and authorization failures cannot leave operator data visible", async ({ browser }) => {
  test.setTimeout(120000);
  const context = await browser.newContext();
  try {
    const app = await context.newPage();
    await signInOwner(app);
    const dashboard = await context.newPage();
    await dashboard.goto("/operator");
    await expect(dashboard.locator("#users-body")).toContainText(email);
    const entered = deferred(), release = deferred(), completed = deferred();
    await dashboard.route("**/api/operator/v1/keys", async route => {
      if (route.request().method() !== "POST") return route.continue();
      entered.resolve();
      try {
        await release.promise;
        await route.fulfill({ status: 201, json: { token: "nabu_op_synthetic-test-token" } });
      } finally { completed.resolve(); }
    });
    try {
      await dashboard.locator('#key-form input[name="name"]').fill("Never show late token");
      await dashboard.locator("#key-form button[type=submit]").click();
      await entered.promise;
      await logOut(app);
      await expect(app.locator("#login-form")).toBeVisible();
      await expect(dashboard.locator("main")).toBeHidden();
      await finishHeldResponses(dashboard, ["issue-key"], () => release.resolve(), [completed]);
      expect(await dashboard.locator("#token").inputValue()).toBe("");
      await expect(dashboard.locator("#issued-key")).toBeHidden();
    } finally { release.resolve(); }
  } finally { await context.close(); }

  for (const status of [401, 403]) {
    const denied = await browser.newContext();
    try {
      const app = await denied.newPage();
      await signInOwner(app);
      const dashboard = await denied.newPage();
      await dashboard.goto("/operator");
      await expect(dashboard.locator("#users-body")).toContainText(email);
      await dashboard.route("**/api/operator/v1/summary", route =>
        route.fulfill({ status, json: { error: "authorization changed" } }));
      await dashboard.locator("#refresh").click();
      await expect(dashboard.locator("main")).toBeHidden();
      await expect(dashboard.locator("#message")).toBeVisible();
      await expect(dashboard.locator("#users-body tr")).toHaveCount(0);
      expect(await dashboard.locator("#token").inputValue()).toBe("");
    } finally { await denied.close(); }
  }
});

test("overlapping report and key replies cannot replace newer dashboard state", async ({ browser }) => {
  test.setTimeout(180000);
  const context = await browser.newContext();
  try {
    const app = await context.newPage();
    await signInOwner(app);
    const dashboard = await context.newPage();
    const userEntered = deferred(), userRelease = deferred(), userCompleted = deferred();
    const householdEntered = deferred(), householdRelease = deferred(), householdCompleted = deferred();
    let userInitial = 0, householdInitial = 0, userPages = 0, householdPages = 0;
    const userRow = id => ({ email: `person-${id}@test.local`, registeredAt: null, emailVerified: true,
      authored7Days: 0, authored30Days: 0, householdLogs30Days: 0, lastAuthoredAt: null, lastSessionSeenAt: null });
    const householdRow = id => ({ name: `Household ${id}`, createdAt: null, members: 1, logs30Days: 0, lastLogAt: null });
    await dashboard.route("**/api/operator/v1/users?*", async route => {
      if (new URL(route.request().url()).searchParams.has("after")) {
        userPages++;
        userEntered.resolve();
        try {
          await userRelease.promise;
          await route.fulfill({ json: { users: Array.from({ length: 10 }, (_, i) => userRow(51 + i)), nextCursor: "" } });
        } finally { userCompleted.resolve(); }
      } else {
        userInitial++;
        await route.fulfill({ json: userInitial === 1 ?
          { users: Array.from({ length: 50 }, (_, i) => userRow(i + 1)), nextCursor: "50" } :
          { users: [userRow("fresh")], nextCursor: "" } });
      }
    });
    await dashboard.route("**/api/operator/v1/households?*", async route => {
      if (new URL(route.request().url()).searchParams.has("after")) {
        householdPages++;
        householdEntered.resolve();
        try {
          await householdRelease.promise;
          await route.fulfill({ json: { households: Array.from({ length: 10 }, (_, i) => householdRow(51 + i)), nextCursor: "" } });
        } finally { householdCompleted.resolve(); }
      } else {
        householdInitial++;
        await route.fulfill({ json: householdInitial === 1 ?
          { households: Array.from({ length: 50 }, (_, i) => householdRow(i + 1)), nextCursor: "50" } :
          { households: [householdRow("fresh")], nextCursor: "" } });
      }
    });
    await dashboard.goto("/operator");
    await expect(dashboard.locator("#more-users")).toBeVisible();
    await expect(dashboard.locator("#more-households")).toBeVisible();
    try {
      await dashboard.locator("#more-users").evaluate(button => { button.click(); button.click(); });
      await dashboard.locator("#more-households").evaluate(button => { button.click(); button.click(); });
      await Promise.all([userEntered.promise, householdEntered.promise]);
      // Both clicks have no DOM signal until their responses are released.
      await dashboard.waitForTimeout(100);
      expect(userPages).toBe(1);
      expect(householdPages).toBe(1);
      await dashboard.locator("#refresh").click();
      await expect(dashboard.locator("#users-body tr")).toHaveCount(1);
      await expect(dashboard.locator("#households-body tr")).toHaveCount(1);
      await expect(dashboard.locator("#users-body")).toContainText("person-fresh@test.local");
      await expect(dashboard.locator("#households-body")).toContainText("Household fresh");
      await expect(dashboard.locator("#message")).toHaveText("");
      await finishHeldResponses(dashboard, ["users", "households"], () => {
        userRelease.resolve();
        householdRelease.resolve();
      }, [userCompleted, householdCompleted]);
      await expect(dashboard.locator("#users-body tr")).toHaveCount(1);
      await expect(dashboard.locator("#households-body tr")).toHaveCount(1);
      await expect(dashboard.locator("#more-users")).toBeHidden();
      await expect(dashboard.locator("#more-households")).toBeHidden();
    } finally { userRelease.resolve(); householdRelease.resolve(); }

    const summary = count => ({ asOf: new Date().toISOString(), registeredUsers: count,
      verifiedUsers: count, households: 1, authors7Days: 1, authors30Days: 1,
      activeHouseholds30Days: 1, logs30Days: 1, logsWithActor30Days: 1 });
    const overviewEntered = deferred(), overviewRelease = deferred(), overviewCompleted = deferred();
    let overviewCalls = 0;
    await dashboard.route("**/api/operator/v1/summary", async route => {
      const call = ++overviewCalls;
      if (call === 1) {
        overviewEntered.resolve();
        await overviewRelease.promise;
      }
      try { await route.fulfill({ json: summary(call === 1 ? 111 : 222) }); }
      finally { if (call === 1) overviewCompleted.resolve(); }
    });
    try {
      await dashboard.locator("#refresh").click();
      await overviewEntered.promise;
      await dashboard.locator("#refresh").click();
      await expect(dashboard.locator("#overview strong").first()).toHaveText("222");
      await expect(dashboard.locator("#message")).toHaveText("");
      await finishHeldResponses(dashboard, ["all"], () => overviewRelease.resolve(), [overviewCompleted]);
      await expect(dashboard.locator("#overview strong").first()).toHaveText("222");
    } finally { overviewRelease.resolve(); }
    await dashboard.unroute("**/api/operator/v1/summary");

    const oldErrorEntered = deferred(), oldErrorRelease = deferred(), oldErrorCompleted = deferred();
    let errorCalls = 0;
    await dashboard.route("**/api/operator/v1/summary", async route => {
      errorCalls++;
      if (errorCalls === 1) {
        oldErrorEntered.resolve();
        await oldErrorRelease.promise;
        try { await route.fulfill({ status: 500, json: { error: "obsolete summary failure" } }); }
        finally { oldErrorCompleted.resolve(); }
      } else await route.fulfill({ json: summary(333) });
    });
    try {
      await dashboard.locator("#refresh").click();
      await oldErrorEntered.promise;
      await dashboard.locator("#refresh").click();
      await expect(dashboard.locator("#overview strong").first()).toHaveText("333");
      await expect(dashboard.locator("#message")).toHaveText("");
      await finishHeldResponses(dashboard, ["all"], () => oldErrorRelease.resolve(), [oldErrorCompleted]);
      await expect(dashboard.locator("#message")).toHaveText("");
    } finally { oldErrorRelease.resolve(); }
    await dashboard.unroute("**/api/operator/v1/summary");

    const activityEntered = deferred(), activityRelease = deferred(), activityCompleted = deferred();
    let activityHeld = false;
    await dashboard.route("**/api/operator/v1/activity?*", async route => {
      const days = Number(new URL(route.request().url()).searchParams.get("days"));
      if (days === 30 && !activityHeld) {
        activityHeld = true;
        activityEntered.resolve();
        await activityRelease.promise;
      }
      try { await route.fulfill({ json: { days: Array.from({ length: days }, (_, i) =>
        ({ date: `2026-09-${String(i % 30 + 1).padStart(2, "0")}`, registrations: 1, authors: 1 })) } }); }
      finally { if (days === 30) activityCompleted.resolve(); }
    });
    try {
      await dashboard.locator("#activity-days").evaluate(select => select.dispatchEvent(new Event("change", { bubbles: true })));
      await activityEntered.promise;
      await dashboard.locator("#activity-days").selectOption("90");
      await expect(dashboard.locator("#activity-chart")).toHaveAttribute("aria-label", /90-day chart/);
      await expect(dashboard.locator("#activity-chart .chart-day")).toHaveCount(90);
      await finishHeldResponses(dashboard, ["activity"], () => activityRelease.resolve(), [activityCompleted]);
      await expect(dashboard.locator("#activity-chart")).toHaveAttribute("aria-label", /90-day chart/);
      await expect(dashboard.locator("#activity-chart .chart-day")).toHaveCount(90);
    } finally { activityRelease.resolve(); }
    await dashboard.unroute("**/api/operator/v1/activity?*");

    const activityErrorEntered = deferred(), activityErrorRelease = deferred(), activityErrorCompleted = deferred();
    await dashboard.route("**/api/operator/v1/activity?*", async route => {
      const days = Number(new URL(route.request().url()).searchParams.get("days"));
      if (days === 30) {
        activityErrorEntered.resolve();
        try {
          await activityErrorRelease.promise;
          await route.fulfill({ status: 500, json: { error: "obsolete activity failure" } });
        } finally { activityErrorCompleted.resolve(); }
      } else await route.fulfill({ json: { days: Array.from({ length: days }, (_, i) =>
        ({ date: `2026-09-${String(i % 30 + 1).padStart(2, "0")}`, registrations: 1, authors: 1 })) } });
    });
    try {
      await dashboard.locator("#activity-days").selectOption("30");
      await activityErrorEntered.promise;
      const settled90 = await dashboard.evaluate(() => window.__operatorSettledCounts.activity || 0);
      await dashboard.locator("#activity-days").selectOption("90");
      await expect.poll(() => dashboard.evaluate(() => window.__operatorSettledCounts.activity || 0)).toBeGreaterThan(settled90);
      await expect(dashboard.locator("#activity-chart .chart-day")).toHaveCount(90);
      await finishHeldResponses(dashboard, ["activity"], () => activityErrorRelease.resolve(), [activityErrorCompleted]);
      await expect(dashboard.locator("#message")).toHaveText("");
      await expect(dashboard.locator("#activity-chart")).toHaveAttribute("aria-label", /90-day chart/);
    } finally { activityErrorRelease.resolve(); }
    await dashboard.unroute("**/api/operator/v1/activity?*");

    await dashboard.locator('#key-form input[name="name"]').fill("Stale key status");
    await dashboard.locator("#key-form button[type=submit]").click();
    const keyRow = dashboard.locator("#keys-body tr").filter({ hasText: "Stale key status" });
    await expect(keyRow).toContainText("Active");
    const keysEntered = deferred(), keysRelease = deferred(), keysCompleted = deferred();
    let heldKeys = false;
    await dashboard.route("**/api/operator/v1/keys", async route => {
      if (route.request().method() === "GET" && !heldKeys) {
        heldKeys = true;
        const response = await route.fetch();
        keysEntered.resolve();
        try {
          await keysRelease.promise;
          await route.fulfill({ response });
        } finally { keysCompleted.resolve(); }
      } else await route.continue();
    });
    dashboard.on("dialog", dialog => dialog.accept());
    try {
      await dashboard.locator("#refresh").click();
      await keysEntered.promise;
      await keyRow.getByRole("button", { name: "Revoke" }).click();
      await expect(keyRow).toContainText("Revoked");
      await finishHeldResponses(dashboard, ["keys"], () => keysRelease.resolve(), [keysCompleted]);
      await expect(keyRow).toContainText("Revoked");
      await expect(keyRow.getByRole("button", { name: "Revoke" })).toHaveCount(0);
    } finally { keysRelease.resolve(); }
  } finally { await context.close(); }
});
