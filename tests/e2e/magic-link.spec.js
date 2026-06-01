import { test, expect } from "@playwright/test";

const BASE = "http://localhost:8080";
const MAILPIT = "http://localhost:8025";

async function waitForMagicLinkToken(request, email) {
  for (let attempt = 0; attempt < 15; attempt++) {
    await new Promise(r => setTimeout(r, 1000));
    const res = await request.get(`${MAILPIT}/api/v1/messages`);
    const data = await res.json();
    for (const msg of data.messages || []) {
      // Match by recipient AND subject to avoid cross-test contamination.
      if (!msg.Subject || !msg.Subject.includes("magic")) continue;
      const rawRes = await request.get(`${MAILPIT}/api/v1/message/${msg.ID}/raw`);
      const raw = await rawRes.text();
      if (!raw.includes(email)) continue;
      const match = raw.match(/token=([A-Za-z0-9_-]+)/);
      if (match) return match[1];
    }
  }
  return null;
}

async function waitForAppReady(page) {
  await page.waitForFunction(() => {
    const app = document.querySelector("#app");
    return app && app.children.length > 0;
  }, { timeout: 10000 });
}

test("magic link creates new user who has never registered", async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  const request = context.request;

  const email = `e2e-new-${Date.now()}@test.local`;

  await page.goto(BASE);
  await waitForAppReady(page);

  // Request magic link for unregistered email
  await page.click("[data-action='show-magic-link']");
  await page.waitForSelector("#magic-link-form", { timeout: 5000 });
  await page.fill("#magic-email", email);
  await page.click("#magic-link-form button[type='submit']");

  // Check Mailpit for the magic link email
  const token = await waitForMagicLinkToken(request, email);
  expect(token).not.toBeNull();

  // Consume magic link via SPA route. Wait for the API call to complete
  // by listening for the network request, then verify authenticated state.
  const consumePromise = page.waitForResponse(
    r => r.url().includes("/api/auth/magic-link/consume") && r.status() === 200,
    { timeout: 15000 }
  );
  await page.goto(`${BASE}/magic-login?token=${encodeURIComponent(token)}`);
  await consumePromise;

  // Session cookie should now be set. Reload to ensure authenticated view.
  await page.reload();
  await page.waitForSelector("#top-bar:not([hidden])", { timeout: 10000 });
  await expect(page.locator("#bottom-tabs")).not.toBeHidden({ timeout: 3000 });

  await context.close();
});

test("existing user can request and use magic link", async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  const request = context.request;

  const email = `e2e-existing-${Date.now()}@test.local`;
  const password = "password123";

  // Register via API — get CSRF token first from a page visit.
  const csrfPage = await context.newPage();
  await csrfPage.goto(BASE);
  await waitForAppReady(csrfPage);
  const csrfToken = (await context.cookies()).find(c => c.name === "nabu_csrf")?.value || "";
  await csrfPage.close();

  const regRes = await request.post(`${BASE}/api/auth/register`, {
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken },
    data: { email, password },
  });
  expect(regRes.ok()).toBeTruthy();

  // Clear cookies so we get unauthenticated state.
  await context.clearCookies();

  // Request a magic link via the UI.
  await page.goto(BASE);
  await waitForAppReady(page);

  await page.click("[data-action='show-magic-link']");
  await page.waitForSelector("#magic-link-form", { timeout: 5000 });
  await page.fill("#magic-email", email);
  await page.click("#magic-link-form button[type='submit']");

  // Get token from Mailpit
  const token = await waitForMagicLinkToken(request, email);
  expect(token).not.toBeNull();

  // Navigate to magic-login and wait for the consume API call to complete.
  const consumePromise = page.waitForResponse(
    r => r.url().includes("/api/auth/magic-link/consume") && r.status() === 200,
    { timeout: 15000 }
  );
  await page.goto(`${BASE}/magic-login?token=${encodeURIComponent(token)}`);
  await consumePromise;

  // Session cookie should be set. Reload for clean authenticated view.
  await page.reload();
  await page.waitForSelector("#top-bar:not([hidden])", { timeout: 10000 });
  await expect(page.locator("#bottom-tabs")).not.toBeHidden({ timeout: 5000 });

  await context.close();
});
