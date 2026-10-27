// tests/e2e/duration-timer.spec.js
// Phase 5.2 — duration timer: start on the log sheet, persistent top-bar chip
// that survives reload, stop & log with a duration.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-timer-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithDurationChore(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', {
    data: { name: `Timer ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  const res = await page.request.post('/api/chores', {
    data: { name: 'Nap', icon: '😴', color: '#60A5FA', metricType: 'duration' },
    headers: { 'X-CSRF-Token': csrf },
  });
  const chore = (await res.json()).chore;
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { csrf, chore };
}

test.describe('Duration timer (Phase 5.2)', () => {
  test('start shows a chip that survives reload; stop logs a duration', async ({ page }) => {
    const { csrf, chore } = await setupWithDurationChore(page);

    await page.click(`.home-chore-card[data-home-chore-id="${chore.id}"]`);
    await page.waitForSelector('[data-action="start-timer"]');
    await page.click('[data-action="start-timer"]');

    await expect(page.locator('#timer-chip')).toBeVisible();

    // Survives a full reload (localStorage-backed).
    await page.reload();
    await expect(page.locator('#timer-chip')).toBeVisible();

    // Stop & log. The chip is a fixed overlay on document.body with a delegated
    // click handler; dispatch a click event on it (Playwright's coordinate click
    // on a body-level fixed element under the morphing #app is flaky, but the
    // element is genuinely top-most per elementFromPoint, so a real tap fires it).
    await page.dispatchEvent('#timer-chip', 'click');
    await expect(page.locator('#timer-chip')).toHaveCount(0);
    // The log POST completes after the chip clears; wait for the success toast.
    await expect(page.locator('#toast-container .toast')).toContainText('Logged');

    // A log now exists for the chore.
    const latest = await (await page.request.get('/api/logs/latest-per-chore')).json();
    expect(latest.latestLogs[String(chore.id)]).toBeTruthy();
  });
});
