// tests/e2e/reminder-snooze.spec.js
// Phase 2.6 — the snooze endpoint is CSRF-exempt (SW-invoked) yet
// ownership-checked, and reschedules a one-off follow-up.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-snooze-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setup(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  await page.request.post('/api/household', {
    data: { name: `Snooze ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  const res = await page.request.post('/api/chores', {
    data: { name: 'Medication', icon: '💊', color: '#A78BFA' },
    headers: { 'X-CSRF-Token': csrf },
  });
  const chore = (await res.json()).chore;
  return { csrf, chore };
}

test.describe('Reminder snooze (Phase 2.6)', () => {
  test('snooze works without a CSRF header and creates a follow-up', async ({ page }) => {
    const { chore } = await setup(page);

    // The service worker cannot present a CSRF token, so this route is exempt;
    // the session cookie (shared by the request context) still authenticates.
    const snooze = await page.request.post('/api/reminders/snooze', {
      data: { choreId: chore.id, minutes: 30 },
    });
    expect(snooze.status()).toBe(200);

    const scheds = await (await page.request.get('/api/schedules')).json();
    const followUp = (scheds.schedules || []).find(
      s => s.choreId === chore.id && s.isFollowUp && s.frequencyType === 'once',
    );
    expect(followUp).toBeTruthy();
  });

  test('snooze rejects a chore outside the household', async ({ page }) => {
    await setup(page);
    const snooze = await page.request.post('/api/reminders/snooze', {
      data: { choreId: 999999, minutes: 30 },
    });
    expect(snooze.status()).toBe(403);
  });
});
