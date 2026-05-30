// tests/e2e/household-roles.spec.js
// Verifies role-based permissions: owner/admin/member restrictions,
// invite section visibility, role management, and invite revoke fix.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-role-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function getCSRF(page) {
  return (await page.context().cookies()).find(c => c.name === 'choresy_csrf')?.value || '';
}

async function setupOwnerWithHousehold(page) {
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  const email = uniqueEmail();
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = await getCSRF(page);
  await page.request.post('/api/household', {
    data: { name: `Roles Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { csrf, email };
}

async function joinAsUser(browser, code) {
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

test.describe('Household Roles', () => {
  test('revoked invite disappears from UI without reload', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    const { csrf: ownerCsrf } = await setupOwnerWithHousehold(ownerPage);

    const inviteRes = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const inviteData = await inviteRes.json();
    expect(inviteData.invite).toBeTruthy();

    // Reload so the page fetches fresh household data.
    await ownerPage.reload();
    await ownerPage.waitForSelector('.home-grid', { timeout: 15000 });

    await ownerPage.click('[data-nav="settings"]');
    await ownerPage.waitForSelector('.invite-link-url', { timeout: 10000 });
    await expect(ownerPage.locator('[data-action="delete-invite"]')).toBeVisible();

    const revokeBtn = ownerPage.locator('[data-action="delete-invite"]').first();
    await revokeBtn.click();

    await expect(ownerPage.locator('[data-action="delete-invite"]')).not.toBeVisible({ timeout: 5000 });

    await ownerPage.context().close();
  });

  test('owner sees invite section and member controls', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    await setupOwnerWithHousehold(ownerPage);

    await ownerPage.click('[data-nav="settings"]');
    await ownerPage.waitForSelector('.member-list', { timeout: 10000 });

    await expect(ownerPage.locator('.invite-link-url')).toBeVisible();
    await expect(ownerPage.locator('[data-action="create-invite"]')).toBeVisible();
    await expect(ownerPage.locator('[data-action="leave-household"]')).toBeVisible();

    await ownerPage.context().close();
  });

  test('admin cannot see invite section', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    const { csrf: ownerCsrf } = await setupOwnerWithHousehold(ownerPage);

    const inviteRes = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const code = (await inviteRes.json()).invite.code;

    const { page: adminPage, context: adminCtx } =
      await joinAsUser(browser, code);

    // Get admin's user ID from owner's perspective.
    const hhRes = await ownerPage.request.get('/api/household');
    const hhData = await hhRes.json();
    const ownerId = hhData.members.find(m => m.role === 'owner')?.userId;
    const adminUserId = hhData.members.find(m => m.userId !== ownerId)?.userId;
    expect(adminUserId).toBeTruthy();

    // Owner promotes the member to admin via API.
    const promoteRes = await ownerPage.request.patch(
      `/api/household/members/${adminUserId}`,
      { data: { role: 'admin' }, headers: { 'X-CSRF-Token': ownerCsrf } }
    );
    expect(promoteRes.ok()).toBe(true);

    // Reload admin page to pick up the new role.
    await adminPage.reload();
    await adminPage.waitForSelector('.home-grid', { timeout: 15000 });

    // Admin navigates to settings, should NOT see invite section.
    await adminPage.click('[data-nav="settings"]');
    await adminPage.waitForSelector('.member-list', { timeout: 10000 });

    await expect(adminPage.locator('.invite-link-url')).not.toBeVisible();
    await expect(adminPage.locator('[data-action="create-invite"]')).not.toBeVisible();

    await adminCtx.close();
    await ownerPage.context().close();
  });

  test('owner can change a member\'s role via dropdown', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    const { csrf: ownerCsrf } = await setupOwnerWithHousehold(ownerPage);

    const inviteRes = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const code = (await inviteRes.json()).invite.code;

    const { page: memberPage, context: memberCtx } =
      await joinAsUser(browser, code);

    // Get member's user ID.
    const hhRes = await memberPage.request.get('/api/household');
    const hhData = await hhRes.json();
    const ownerId = hhData.members.find(m => m.role === 'owner')?.userId;
    const memberId = hhData.members.find(m => m.userId !== ownerId)?.userId;
    expect(memberId).toBeTruthy();

    // Reload owner page so it picks up the new member.
    await ownerPage.reload();
    await ownerPage.waitForSelector('.home-grid', { timeout: 15000 });

    // Navigate to settings, expand the member row, and change role.
    await ownerPage.click('[data-nav="settings"]');
    await ownerPage.waitForSelector('.member-list', { timeout: 10000 });

    // Expand the member's row.
    const memberRow = ownerPage.locator(`.member-row[data-user-id="${memberId}"]`);
    await memberRow.locator('.member-row-summary').click();

    const roleSelect = memberRow.locator('[data-action="update-member-role"]');
    await expect(roleSelect).toBeVisible();
    await expect(roleSelect).toHaveValue('member');

    // Select admin - this triggers the change event which calls the API.
    await roleSelect.selectOption('admin');

    // Wait for API response and page re-render.
    await ownerPage.waitForTimeout(2000);

    // After re-render, expand again and verify the select value.
    await ownerPage.locator(`.member-row[data-user-id="${memberId}"] .member-row-summary`).click();
    await expect(ownerPage.locator(`.member-row[data-user-id="${memberId}"] [data-action="update-member-role"]`)).toHaveValue('admin', { timeout: 5000 });
    const verifyRes = await ownerPage.request.get('/api/household');
    const verifyData = await verifyRes.json();
    const updatedMember = verifyData.members.find(m => m.userId === memberId);
    expect(updatedMember.role).toBe('admin');

    await memberCtx.close();
    await ownerPage.context().close();
  });

  test('owner can transfer ownership via Make Owner button', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    const { csrf: ownerCsrf } = await setupOwnerWithHousehold(ownerPage);

    const inviteRes = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const code = (await inviteRes.json()).invite.code;

    const { page: memberPage, context: memberCtx } =
      await joinAsUser(browser, code);

    const hhRes = await memberPage.request.get('/api/household');
    const hhData = await hhRes.json();
    const ownerId = hhData.members.find(m => m.role === 'owner')?.userId;
    const memberId = hhData.members.find(m => m.userId !== ownerId)?.userId;
    expect(memberId).toBeTruthy();

    // Reload owner page so it picks up the new member.
    await ownerPage.reload();
    await ownerPage.waitForSelector('.home-grid', { timeout: 15000 });

    // Owner navigates to settings, expands the member row, and selects Owner role.
    await ownerPage.click('[data-nav="settings"]');
    await ownerPage.waitForSelector('.member-list', { timeout: 10000 });

    // Expand the member's row.
    const memberRow = ownerPage.locator(`.member-row[data-user-id="${memberId}"]`);
    await memberRow.locator('.member-row-summary').click();

    // Select "owner" from the role dropdown, confirm the dialog.
    const roleSelect = memberRow.locator('[data-action="update-member-role"]');
    await expect(roleSelect).toBeVisible();

    ownerPage.on('dialog', async (dialog) => {
      expect(dialog.type()).toBe('confirm');
      await dialog.accept();
    });
    await roleSelect.selectOption('owner');

    // Wait for re-render.
    await ownerPage.waitForTimeout(1000);

    // Verify the ownership transferred via API.
    const verifyRes = await ownerPage.request.get('/api/household');
    const verifyData = await verifyRes.json();
    const oldOwner = verifyData.members.find(m => m.userId === ownerId);
    const newOwner = verifyData.members.find(m => m.userId === memberId);
    expect(oldOwner.role).toBe('admin');
    expect(newOwner.role).toBe('owner');

    await memberCtx.close();
    await ownerPage.context().close();
  });

  test('member cannot see remove buttons or invite section', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    const { csrf: ownerCsrf } = await setupOwnerWithHousehold(ownerPage);

    const inviteRes = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const code = (await inviteRes.json()).invite.code;

    const { page: memberPage, context: memberCtx } =
      await joinAsUser(browser, code);

    // Member navigates to settings.
    await memberPage.click('[data-nav="settings"]');
    await memberPage.waitForSelector('.member-list', { timeout: 10000 });

    // Member should NOT see remove buttons or role-select dropdowns.
    await expect(memberPage.locator('[data-action="remove-member"]')).toHaveCount(0);
    await expect(memberPage.locator('[data-action="update-member-role"]')).toHaveCount(0);
    // Member should NOT see the invite link section.
    await expect(memberPage.locator('.invite-link-url')).not.toBeVisible();
    // Member should NOT see "New tracked link" button.
    await expect(memberPage.locator('[data-action="create-invite"]')).not.toBeVisible();
    // Member should NOT see role-select dropdowns.
    await expect(memberPage.locator('[data-action="update-member-role"]')).not.toBeVisible();
    // Member SHOULD see role badges (owner + member).
    await expect(memberPage.locator('.role-badge')).toHaveCount(2);
    // Member should see "Leave Household" button.
    await expect(memberPage.locator('[data-action="leave-household"]')).toBeVisible();

    await memberCtx.close();
    await ownerPage.context().close();
  });

  test('admin cannot remove members via API', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    const { csrf: ownerCsrf } = await setupOwnerWithHousehold(ownerPage);

    const inviteRes1 = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const code1 = (await inviteRes1.json()).invite.code;

    const inviteRes2 = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const code2 = (await inviteRes2.json()).invite.code;

    const { page: adminPage, csrf: adminCsrf, context: adminCtx } =
      await joinAsUser(browser, code1);
    const { page: memberPage, context: memberCtx } =
      await joinAsUser(browser, code2);

    // Get all member IDs.
    const hhRes = await ownerPage.request.get('/api/household');
    const hhData = await hhRes.json();
    const ownerId = hhData.members.find(m => m.role === 'owner')?.userId;
    const members = hhData.members.filter(m => m.role === 'member');
    expect(members.length).toBe(2);

    // Promote first member to admin.
    const promoteRes = await ownerPage.request.patch(
      `/api/household/members/${members[0].userId}`,
      { data: { role: 'admin' }, headers: { 'X-CSRF-Token': ownerCsrf } }
    );
    expect(promoteRes.ok()).toBe(true);

    // Admin tries to remove the other member via API.
    const removeRes = await adminPage.request.delete(
      `/api/household/members/${members[1].userId}`,
      { headers: { 'X-CSRF-Token': adminCsrf } }
    );
    expect(removeRes.status()).toBe(403);

    await memberCtx.close();
    await adminCtx.close();
    await ownerPage.context().close();
  });

  test('admin cannot create invites via API', async ({ browser }) => {
    const ownerCtx = await browser.newContext();
    const ownerPage = await ownerCtx.newPage();

    const { csrf: ownerCsrf } = await setupOwnerWithHousehold(ownerPage);

    const inviteRes = await ownerPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': ownerCsrf },
    });
    const code = (await inviteRes.json()).invite.code;

    const { page: adminPage, csrf: adminCsrf, context: adminCtx } =
      await joinAsUser(browser, code);

    // Get admin's user ID and promote to admin.
    const hhRes = await ownerPage.request.get('/api/household');
    const hhData = await hhRes.json();
    const ownerId = hhData.members.find(m => m.role === 'owner')?.userId;
    const adminUserId = hhData.members.find(m => m.userId !== ownerId)?.userId;
    await ownerPage.request.patch(
      `/api/household/members/${adminUserId}`,
      { data: { role: 'admin' }, headers: { 'X-CSRF-Token': ownerCsrf } }
    );

    // Admin tries to create an invite.
    const adminInviteRes = await adminPage.request.post('/api/household/invites', {
      headers: { 'X-CSRF-Token': adminCsrf },
    });
    expect(adminInviteRes.status()).toBe(403);

    await adminCtx.close();
    await ownerPage.context().close();
  });
});
