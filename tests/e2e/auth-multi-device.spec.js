import { test, expect } from "@playwright/test";

function uniqueEmail() {
  return `e2e-md-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

test("login on second device does not invalidate first device session", async ({
  browser,
}) => {
  const email = uniqueEmail();
  const password = "test123456";

  const ctxA = await browser.newContext();
  const pageA = await ctxA.newPage();

  await pageA.goto("/register");
  await pageA.waitForSelector("#register-form");
  await pageA.fill("#reg-email", email);
  await pageA.fill("#reg-password", password);
  await pageA.fill("#reg-confirm", password);
  await pageA.click("button[type=\"submit\"]");
  await pageA.waitForSelector("#hh-indicator:not([hidden])", {
    timeout: 10000,
  });

  const cookiesA = await ctxA.cookies();
  const sessionCookieA = cookiesA.find((c) => c.name === "nabu_session");

  const ctxB = await browser.newContext();
  const pageB = await ctxB.newPage();

  await pageB.goto("/login");
  await pageB.waitForSelector("#login-form");
  await pageB.fill("#login-email", email);
  await pageB.fill("#login-password", password);
  await pageB.click("#login-form button[type=\"submit\"]");
  await pageB.waitForSelector("#hh-indicator:not([hidden])", {
    timeout: 10000,
  });

  await pageA.reload();
  await pageA.waitForSelector("#hh-indicator:not([hidden])", {
    timeout: 10000,
  });

  const meRes = await pageA.request.get("/api/me", {
    headers: { Cookie: `nabu_session=${sessionCookieA.value}` },
  });
  const meData = await meRes.json();
  expect(meData.user).not.toBeNull();
  expect(meData.user.email).toBe(email);

  await ctxA.close();
  await ctxB.close();
});

test("password change on one device invalidates sessions on all devices", async ({
  browser,
}) => {
  const email = uniqueEmail();
  const password = "test123456";

  const ctxA = await browser.newContext();
  const pageA = await ctxA.newPage();

  await pageA.goto("/register");
  await pageA.waitForSelector("#register-form");
  await pageA.fill("#reg-email", email);
  await pageA.fill("#reg-password", password);
  await pageA.fill("#reg-confirm", password);
  await pageA.click("button[type=\"submit\"]");
  await pageA.waitForSelector("#hh-indicator:not([hidden])", {
    timeout: 10000,
  });

  const cookiesA = await ctxA.cookies();
  const sessionCookieA = cookiesA.find((c) => c.name === "nabu_session");

  const ctxB = await browser.newContext();
  const pageB = await ctxB.newPage();

  await pageB.goto("/login");
  await pageB.waitForSelector("#login-form");
  await pageB.fill("#login-email", email);
  await pageB.fill("#login-password", password);
  await pageB.click("#login-form button[type=\"submit\"]");
  await pageB.waitForSelector("#hh-indicator:not([hidden])", {
    timeout: 10000,
  });

  await pageB.click("a[data-nav=\"settings\"]");
  await pageB.waitForSelector(".settings-view", { timeout: 5000 });
  await pageB.fill("#current-password", password);
  await pageB.fill("#new-password", "newpassword789");
  await pageB.fill("#confirm-password", "newpassword789");
  await pageB.click("#change-password-form button[type=\"submit\"]");

  const toast = pageB.locator("#toast-container .toast-success");
  await expect(toast.first()).toBeVisible({ timeout: 5000 });

  const meResA = await pageA.request.get("/api/me", {
    headers: { Cookie: `nabu_session=${sessionCookieA.value}` },
  });
  const meDataA = await meResA.json();
  expect(meDataA.user).toBeNull();

  await ctxA.close();
  await ctxB.close();
});
