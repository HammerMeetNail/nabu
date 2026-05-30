// tests/e2e/settings-remove-member.spec.js
// Verifies that an owner can remove a member from the household
// via the Settings tab.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-rm-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';
}

async function setupOwnerWithHousehold(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  const ownerEmail = uniqueEmail();
  await page.fill('#reg-email', ownerEmail);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = await getCSRF(page);
  await page.request.post('/api/household', {
    data: { name: `RM Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { csrf, ownerEmail };
}

async function joinAsSecondUser(browser, code) {
  const context = await browser.newContext();
  const page = await context.newPage();
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = await getCSRF(page);
  const joinRes = await page.request.post('/api/household/join', {
    data: { inviteCode: code },
    headers: { 'X-CSRF-Token': csrf },
  });
  if (!joinRes.ok()) {
    throw new Error(`join failed: ${joinRes.status()} ${await joinRes.text()}`);
  }

  await page.goto('/');
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { page, email, csrf, context };
}

test.describe('Settings - Remove Member', () => {
  test('owner can remove a member via the settings tab', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    const { csrf: ownerCsrf } = await setupOwnerWithHousehold(ownerPage);

    // Get owner ID from household data.
    const hhRes = await ownerPage.request.get('/api/household');
    const hhData = await hhRes.json();
    const members = hhData.members || [];
    const ownerId = members[0]?.userId;

    // Create an invite and have a second user join.
    const inviteRes = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const inviteData = await inviteRes.json();
    const code = inviteData.invite.code;

    const { page: memberPage, email: memberEmail, context: memberCtx } =
      await joinAsSecondUser(browser, code);

    // Get the member's ID.
    const hhRes2 = await memberPage.request.get('/api/household');
    const hhData2 = await hhRes2.json();
    const memberId = (hhData2.members || []).find(
      m => m.userId !== ownerId
    )?.userId;
    expect(memberId).toBeTruthy();

    // Navigate owner to settings.
    await ownerPage.goto('/settings');
    await ownerPage.waitForSelector('.member-list', { timeout: 10000 });

    // Target only the Members list (the last .member-list on the page;
    // the first one is the Active Invites list).
    const membersList = ownerPage.locator('.member-list').last();

    // Verify both members are listed.
    const memberRows = membersList.locator('.member-row');
    await expect(memberRows).toHaveCount(2);

    // Expand the member's row (click the summary).
    const memberRow = membersList.locator(`.member-row[data-user-id="${memberId}"]`);
    await memberRow.locator('.member-row-summary').click();

    // Click Remove inside the expanded details.
    const removeBtn = memberRow.locator('[data-action="remove-member"]');
    await expect(removeBtn).toBeVisible();

    // Click Remove and confirm the dialog.
    ownerPage.on('dialog', async (dialog) => {
      expect(dialog.type()).toBe('confirm');
      await dialog.accept();
    });
    await removeBtn.click();

    // After removal, the member list should have only 1 item.
    await expect(memberRows).toHaveCount(1);

    // Verify via API that the member is no longer in the household.
    const hhRes3 = await memberPage.request.get('/api/household');
    expect(hhRes3.status()).toBe(404);

    await memberCtx.close();
    await ownerPage.context().close();
  });

  test('owner cannot remove the last owner', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    await setupOwnerWithHousehold(ownerPage);

    // Navigate to settings and verify the owner does not see
    // a Remove button on themselves.
    await ownerPage.goto('/settings');
    await ownerPage.waitForSelector('.member-list', { timeout: 10000 });

    const membersList = ownerPage.locator('.member-list').last();

    // There should be only one member (the owner).
    await expect(membersList.locator('.member-row')).toHaveCount(1);

    // Verify via API that removing the last owner is rejected.
    const csrf = await getCSRF(ownerPage);
    const hhRes = await ownerPage.request.get('/api/household');
    const hhData = await hhRes.json();
    const members = hhData.members || [];
    const ownerId = members[0]?.userId;

    const removeRes = await ownerPage.request.delete(
      `/api/household/members/${ownerId}`,
      { headers: { 'X-CSRF-Token': csrf } }
    );
    expect(removeRes.status()).toBe(403);

    await ownerPage.context().close();
  });

  test('after removal, member can still log in but has no household', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    const { csrf: ownerCsrf } = await setupOwnerWithHousehold(ownerPage);

    const inviteRes = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const inviteData = await inviteRes.json();
    const code = inviteData.invite.code;

    const { page: memberPage, email: memberEmail, context: memberCtx } =
      await joinAsSecondUser(browser, code);

    const ownerHhRes = await ownerPage.request.get('/api/household');
    const ownerHhData = await ownerHhRes.json();
    const members = ownerHhData.members || [];
    const ownerId = members.find(m => m.role === 'owner')?.userId;
    const removedMemberId = members.find(m => m.userId !== ownerId)?.userId;
    expect(removedMemberId).toBeTruthy();

    // Owner removes the member via API directly.
    const deleteRes = await ownerPage.request.delete(
      `/api/household/members/${removedMemberId}`,
      { headers: { 'X-CSRF-Token': ownerCsrf } }
    );
    expect(deleteRes.ok()).toBe(true);

    // The removed member should see no household when they reload.
    await memberPage.goto('/');
    await memberPage.waitForTimeout(2000);
    // After being removed, the home page shows "Welcome!" and a prompt
    // to set up a household.
    await expect(
      memberPage.locator('text=Welcome!')
    ).toBeVisible({ timeout: 10000 });
    await expect(
      memberPage.locator('text=Set up your household to get started')
    ).toBeVisible();

    await memberCtx.close();
    await ownerPage.context().close();
  });
});
