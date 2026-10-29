// tests/e2e/settings-delete-account-former-member.spec.js
// Deleting an account must not erase a shared household's history: by default
// the departing member's logs stay, shown as "Former member". The deletion
// sheet offers a data download and opt-in deletion of the member's logs.

import { test, expect } from '@playwright/test';
import fs from 'node:fs';

test.use({ serviceWorkers: 'block' });

function uniqueEmail() {
  return `e2e-fm-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
}

async function register(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', uniqueEmail());
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  return getCSRF(page);
}

// The owner creates a household; a member joins and logs one chore with a note.
async function sharedHouseholdWithMemberLog(browser, note) {
  const ownerCtx = await browser.newContext();
  const owner = await ownerCtx.newPage();
  const ownerCsrf = await register(owner);
  await owner.request.post('/api/household', { data: { name: `FM ${Date.now()}` }, headers: { 'X-CSRF-Token': ownerCsrf } });
  await owner.request.post('/api/chores/seed-defaults', { headers: { 'X-CSRF-Token': ownerCsrf } });
  const { invite } = await (await owner.request.post('/api/household/invites', { headers: { 'X-CSRF-Token': ownerCsrf } })).json();

  const memberCtx = await browser.newContext();
  const member = await memberCtx.newPage();
  const memberCsrf = await register(member);
  const join = await member.request.post('/api/household/join', { data: { inviteCode: invite.code }, headers: { 'X-CSRF-Token': memberCsrf } });
  expect(join.ok()).toBe(true);
  const { chores } = await (await member.request.get('/api/chores')).json();
  const log = await member.request.post('/api/logs', {
    data: { choreId: chores[0].id, note, indicators: [], date: new Date().toISOString().slice(0, 10), completedAt: new Date().toISOString(), hour: new Date().getHours() },
    headers: { 'X-CSRF-Token': memberCsrf },
  });
  expect(log.ok()).toBe(true);
  await member.reload();
  await member.waitForSelector('.home-grid', { timeout: 15000 });
  return { owner, member, ownerCtx, memberCtx };
}

async function openDeleteSheet(page) {
  await page.click('a[data-nav="settings"]');
  await page.waitForSelector('.settings-view');
  await page.click('[data-action="open-delete-account"]');
  await expect(page.getByTestId('delete-account-confirm')).toBeVisible();
}

async function confirmDelete(page) {
  await page.fill('#delete-account-input', 'DELETE');
  await page.click('[data-action="confirm-delete-account"]');
  await page.waitForSelector('#login-form', { timeout: 10000 });
}

test.describe('Delete account: shared household history', () => {
  test('by default the member\'s logs stay as "Former member"', async ({ browser }) => {
    const note = `kept-${Date.now()}`;
    const { owner, member, ownerCtx, memberCtx } = await sharedHouseholdWithMemberLog(browser, note);

    await openDeleteSheet(member);
    await expect(member.getByTestId('delete-account-confirm')).toContainText('Former member');
    await expect(member.locator('#delete-account-logs')).not.toBeChecked();
    await expect(member.locator('#delete-account-notes')).not.toBeChecked();

    // "Download my data" returns everything about the member as JSON, their
    // logs among it; "Logs as CSV" returns just the logs.
    const [download] = await Promise.all([
      member.waitForEvent('download'),
      member.getByTestId('download-my-data').click(),
    ]);
    const data = JSON.parse(fs.readFileSync(await download.path(), 'utf8'));
    expect(data.logs.map(l => l.note)).toEqual([note]);
    const [csvDownload] = await Promise.all([
      member.waitForEvent('download'),
      member.getByTestId('download-my-logs').click(),
    ]);
    const csv = fs.readFileSync(await csvDownload.path(), 'utf8');
    expect(csv).toContain(note);

    await confirmDelete(member);

    await owner.goto('/activity');
    const row = owner.locator('.hist-row', { hasText: note });
    await expect(row).toBeVisible({ timeout: 10000 });
    await expect(row).toContainText('Former member');

    await memberCtx.close();
    await ownerCtx.close();
  });

  test('"Also delete everything I logged" removes the member\'s logs', async ({ browser }) => {
    const note = `gone-${Date.now()}`;
    const { owner, member, ownerCtx, memberCtx } = await sharedHouseholdWithMemberLog(browser, note);

    await openDeleteSheet(member);
    await member.check('#delete-account-logs');
    await confirmDelete(member);

    const { logs } = await (await owner.request.get('/api/logs/today')).json();
    expect(logs.length).toBe(0);
    await owner.goto('/activity');
    await expect(owner.getByText('No completed chores yet.')).toBeVisible({ timeout: 10000 });
    await expect(owner.locator('.hist-row', { hasText: note })).toHaveCount(0);

    await memberCtx.close();
    await ownerCtx.close();
  });

  test('"Also delete the notes I wrote" keeps the log but strips its note', async ({ browser }) => {
    const note = `stripped-${Date.now()}`;
    const { owner, member, ownerCtx, memberCtx } = await sharedHouseholdWithMemberLog(browser, note);

    await openDeleteSheet(member);
    await member.check('#delete-account-notes');
    await confirmDelete(member);

    const { logs } = await (await owner.request.get('/api/logs/today')).json();
    expect(logs.length).toBe(1);
    expect(logs[0].note).toBe('');
    expect(logs[0].userId).toBeLessThan(0);

    await memberCtx.close();
    await ownerCtx.close();
  });
});

test('deletion opt-in checkboxes sit beside their labels on a phone', async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const page = await context.newPage();
  await register(page);
  const csrf = await getCSRF(page);
  await page.request.post('/api/household', { data: { name: `FM layout ${Date.now()}` }, headers: { 'X-CSRF-Token': csrf } });
  await page.reload();
  await openDeleteSheet(page);
  for (const id of ['#delete-account-logs', '#delete-account-notes']) {
    const box = await page.locator(id).boundingBox();
    const label = await page.locator(`label:has(${id}) span`).boundingBox();
    expect(box.width).toBeLessThan(40);
    expect(label.x).toBeGreaterThan(box.x + box.width - 1);
    expect(label.x + label.width).toBeLessThanOrEqual(390);
  }
  await context.close();
});
