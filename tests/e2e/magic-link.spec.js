import { test, expect } from "@playwright/test";

const BASE = "http://localhost:8080";
const MAILPIT = "http://localhost:8025";

async function mailpitAvailable() {
  try {
    const res = await fetch(`${MAILPIT}/api/v1/messages`);
    return res.ok;
  } catch {
    return false;
  }
}

test.beforeAll(async () => {
  if (!(await mailpitAvailable())) {
    test.skip(true, "Mailpit not available; skipping magic link tests");
  }
});

async function waitForMagicLinkToken(request, subject) {
  for (let attempt = 0; attempt < 15; attempt++) {
    await new Promise(r => setTimeout(r, 1000));
    const res = await request.get(`${MAILPIT}/api/v1/messages`);
    const data = await res.json();
    for (const msg of data.messages || []) {
      if (msg.Subject && msg.Subject.includes(subject)) {
        const rawRes = await request.get(`${MAILPIT}/api/v1/message/${msg.ID}/raw`);
        const raw = await rawRes.text();
        const match = raw.match(/token=([A-Za-z0-9_-]+)/);
        if (match) return match[1];
      }
    }
  }
  return null;
}

async function waitForAppReady(page) {
  // Wait for the SPA to render something into #app
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
  const token = await waitForMagicLinkToken(request, "magic");
  expect(token).not.toBeNull();

  // Consume magic link - should auto-create account and log in
  await page.goto(`${BASE}/magic-login?token=${token}`);
  await page.waitForTimeout(2000);

  // Verify logged in — top bar should be visible
  await expect(page.locator("#top-bar")).not.toBeHidden({ timeout: 5000 });
  await expect(page.locator("#bottom-tabs")).not.toBeHidden({ timeout: 3000 });

  await context.close();
});

test("existing user can request and use magic link", async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  const request = context.request;

  const email = `e2e-existing-${Date.now()}@test.local`;
  const password = "password123";

  // Register via API (get CSRF first)
  const csrfPage = await context.newPage();
  await csrfPage.goto(BASE);
  await waitForAppReady(csrfPage);
  const csrfCookie = (await context.cookies()).find(c => c.name === "choresy_csrf");
  const csrfToken = csrfCookie?.value || "";
  await csrfPage.close();

  const regRes = await request.post(`${BASE}/api/auth/register`, {
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken },
    data: { email, password },
  });
  expect(regRes.ok()).toBeTruthy();

  // Clear cookies so we get the login form (API register set a session cookie)
  await context.clearCookies();

  // Now go to the app and request a magic link
  await page.goto(BASE);
  await waitForAppReady(page);

  await page.click("[data-action='show-magic-link']");
  await page.waitForSelector("#magic-link-form", { timeout: 5000 });
  await page.fill("#magic-email", email);
  await page.click("#magic-link-form button[type='submit']");

  // Check Mailpit for the magic link email
  const token = await waitForMagicLinkToken(request, "magic");
  expect(token).not.toBeNull();

  // Consume magic link
  await page.goto(`${BASE}/magic-login?token=${token}`);
  await page.waitForTimeout(2000);

  // Verify logged in
  await expect(page.locator("#top-bar")).not.toBeHidden({ timeout: 5000 });
  await expect(page.locator("#bottom-tabs")).not.toBeHidden({ timeout: 3000 });

  await context.close();
});
