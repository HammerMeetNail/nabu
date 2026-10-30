// tests/e2e/household-display.spec.js
// The household display: an owner turns a device into a shared display with a
// PIN only that device keeps. It shows Home's tiles under the household's
// members; picking someone and tapping a tile logs at once for them, with
// Undo. Admins-only activities stay off it, and only the PIN leaves it.

import { test, expect } from '@playwright/test';

function uniqueEmail(tag) {
  return `e2e-display-${tag}-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
}

async function register(page, email) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  return getCSRF(page);
}

async function post(page, url, csrf, data) {
  const res = await page.request.post(url, { data, headers: { 'X-CSRF-Token': csrf } });
  if (!res.ok()) throw new Error(`${url} failed: ${res.status()} ${await res.text()}`);
  return res.json();
}

async function setUp(browser) {
  const ownerContext = await browser.newContext();
  const owner = await ownerContext.newPage();
  const ownerCsrf = await register(owner, uniqueEmail('owner'));
  await post(owner, '/api/household', ownerCsrf, { name: `Display ${Date.now()}` });
  await post(owner, '/api/chores/seed-defaults', ownerCsrf);
  const meds = await post(owner, '/api/chores', ownerCsrf, { name: 'Flea Drops', icon: '💊', color: '#A23B72', subjects: ['Mongo', 'Ziggy'] });
  await post(owner, '/api/chores', ownerCsrf, { name: 'Pay the sitter', icon: '💼', color: '#2E86AB', visibility: 'admins' });
  const invite = await post(owner, '/api/household/invites', ownerCsrf);

  const memberContext = await browser.newContext();
  const memberPage = await memberContext.newPage();
  const memberEmail = uniqueEmail('member');
  const memberCsrf = await register(memberPage, memberEmail);
  await post(memberPage, '/api/household/join', memberCsrf, { inviteCode: invite.invite.code });
  const me = (await (await memberPage.request.get('/api/me')).json()).user;
  return { owner, memberPage, memberName: me.displayName || memberEmail, memberId: me.id, medsId: meds.chore.id };
}

async function latestFor(page, choreId) {
  const data = await (await page.request.get('/api/logs/latest-per-chore')).json();
  return data.latestLogs?.[choreId] || null;
}

test('an owner turns on the display, logs for a member with one tap, and only the PIN leaves it', async ({ browser }) => {
  const { owner, memberPage, memberName, memberId, medsId } = await setUp(browser);

  await owner.goto('/settings');
  const card = owner.locator('[data-key="household-display"]');
  await card.getByRole('button', { name: 'Use this device as a household display' }).click();
  await owner.fill('#display-pin', '1234');
  await owner.fill('#display-pin-again', '1243');
  await card.getByRole('button', { name: 'Turn on' }).click();
  await expect(owner.getByTestId('display-setup-error')).toHaveText("The PINs don't match.");
  await owner.fill('#display-pin', '1234');
  await owner.fill('#display-pin-again', '1234');
  await card.getByRole('button', { name: 'Turn on' }).click();

  // No tabs or top bar: just the household's members and Home's tiles,
  // without the Admins-only one.
  await expect(owner.locator('.display-view')).toBeVisible();
  await expect(owner.locator('#bottom-tabs')).toBeHidden();
  await expect(owner.locator('#top-bar')).toBeHidden();
  await expect(owner.locator('.display-person')).toHaveCount(2);
  await expect(owner.locator('.display-tile', { hasText: 'Flea Drops' })).toBeVisible();
  await expect(owner.locator('.display-tile', { hasText: 'Pay the sitter' })).toHaveCount(0);

  // Before anyone is picked, a tile logs nothing.
  const tile = owner.locator('.display-tile').first();
  const choreId = Number(await tile.getAttribute('data-chore-id'));
  await tile.click();
  await expect(owner.getByTestId('display-prompt')).toHaveText('Tap who you are first');
  expect(await latestFor(owner, choreId)).toBeNull();

  // Pick the member, then the tile: logged at once for them, now.
  await owner.locator('.display-person', { hasText: memberName }).click();
  await expect(owner.getByTestId('display-prompt')).toHaveText(`Logging for ${memberName}`);
  const before = Date.now();
  await tile.click();
  const toast = owner.locator('#toast-container [data-undo-toast]').last();
  await expect(toast).toContainText(`for ${memberName}`);
  await expect.poll(async () => (await latestFor(owner, choreId))?.userId).toBe(memberId);
  const logged = await latestFor(owner, choreId);
  expect(Math.abs(new Date(logged.completedAt).getTime() - before)).toBeLessThan(60_000);
  expect(logged.slotHour).toBe(new Date(logged.completedAt).getHours());
  await expect(tile.locator('.home-card-time')).toHaveText('just now');

  // Undo takes it back.
  await toast.getByRole('button', { name: 'Undo' }).click();
  await expect.poll(async () => latestFor(owner, choreId)).toBeNull();

  // An activity for several cats asks which one first.
  await owner.locator('.display-tile', { hasText: 'Flea Drops' }).click();
  await expect(owner.getByRole('dialog', { name: /which one/ })).toBeVisible();
  await owner.getByRole('button', { name: 'Ziggy' }).click();
  await expect(owner.locator('#toast-container [data-undo-toast]').last()).toContainText(`Flea Drops (Ziggy) for ${memberName}`);
  await expect.poll(async () => (await latestFor(owner, medsId))?.subject).toBe('Ziggy');
  expect((await latestFor(owner, medsId)).userId).toBe(memberId);

  // The display survives a reload.
  await owner.reload();
  await expect(owner.locator('.display-view')).toBeVisible();
  await expect(owner.locator('#bottom-tabs')).toBeHidden();

  // Only the PIN leaves it.
  await owner.getByRole('button', { name: 'Leave the household display' }).click();
  await owner.fill('#display-leave-pin', '0000');
  await owner.getByRole('button', { name: 'Leave', exact: true }).click();
  await expect(owner.getByTestId('display-leave-error')).toHaveText("That PIN isn't right.");
  await expect(owner.locator('.display-view')).toBeVisible();
  await owner.fill('#display-leave-pin', '1234');
  await owner.getByRole('button', { name: 'Leave', exact: true }).click();
  await expect(owner.locator('.display-view')).toHaveCount(0);
  await expect(owner.locator('#bottom-tabs')).toBeVisible();
  await owner.reload();
  await expect(owner.locator('#bottom-tabs')).toBeVisible();

  // A regular member can't turn a device into a display.
  await memberPage.goto('/settings');
  await expect(memberPage.locator('.settings-view')).toBeVisible();
  await expect(memberPage.locator('[data-key="household-display"]')).toHaveCount(0);
});

test('five wrong PINs pause leaving, and neither closing the dialog nor reloading ends the pause', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  const csrf = await register(page, uniqueEmail('pause'));
  await post(page, '/api/household', csrf, { name: `Display ${Date.now()}` });
  await post(page, '/api/chores/seed-defaults', csrf);
  await page.goto('/settings');
  await page.getByRole('button', { name: 'Use this device as a household display' }).click();
  await page.fill('#display-pin', '2468');
  await page.fill('#display-pin-again', '2468');
  await page.getByRole('button', { name: 'Turn on' }).click();
  await expect(page.locator('.display-view')).toBeVisible();

  const leave = page.getByRole('button', { name: 'Leave', exact: true });
  const error = page.getByTestId('display-leave-error');
  await page.getByRole('button', { name: 'Leave the household display' }).click();
  for (let i = 1; i < 5; i++) {
    await page.fill('#display-leave-pin', '1111');
    await leave.click();
    await expect(error).toHaveText("That PIN isn't right.");
  }
  await page.fill('#display-leave-pin', '1111');
  await leave.click();
  await expect(error).toHaveText('Too many tries. Wait 30 seconds.');

  // Even the right PIN waits, after closing the dialog and after a reload.
  await page.getByRole('button', { name: 'Cancel' }).click();
  await page.getByRole('button', { name: 'Leave the household display' }).click();
  await page.fill('#display-leave-pin', '2468');
  await leave.click();
  await expect(error).toContainText('Too many tries. Wait');
  await page.reload();
  await page.getByRole('button', { name: 'Leave the household display' }).click();
  await page.fill('#display-leave-pin', '2468');
  await leave.click();
  await expect(error).toContainText('Too many tries. Wait');
  await expect(page.locator('.display-view')).toBeVisible();
});
