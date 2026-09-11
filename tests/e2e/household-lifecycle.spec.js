import { test, expect } from '@playwright/test';

function uniqueEmail() { return `e2e-lifecycle-${Date.now()}-${Math.random().toString(36).slice(2)}@test.com`; }
async function register(page) {
  await page.goto('/register');
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf').value;
  const response = await page.request.post('/api/auth/register', {
    data: { email: uniqueEmail(), password: 'test123456' }, headers: { 'X-CSRF-Token': csrf },
  });
  expect(response.status()).toBe(201);
  const user = (await response.json()).user;
  return { user, headers: { 'X-CSRF-Token': csrf } };
}
async function createHome(page, headers, name) {
  const result = await page.request.post('/api/household', { data: { name }, headers });
  expect(result.status()).toBe(201);
  return (await result.json()).household;
}
async function createChore(page, headers, name, extra = {}) {
  const result = await page.request.post('/api/chores', { data: { name, icon: '🧹', color: '#19323C', ...extra }, headers });
  expect(result.status()).toBe(201);
  return (await result.json()).chore;
}

test('removal while active elsewhere revokes learned links; a fresh one-use link still joins', async ({ page, browser }) => {
  const owner = await register(page);
  const home = await createHome(page, owner.headers, 'Shared');
  await createChore(page, owner.headers, 'Sweep');
  const context = await browser.newContext();
  try {
    const memberPage = await context.newPage();
    const member = await register(memberPage);
    expect((await memberPage.request.post('/api/household/join', { data: { inviteCode: home.inviteCode }, headers: member.headers })).ok()).toBe(true);
    const shared = await (await memberPage.request.get('/api/household')).json();
    expect(shared.household.inviteCode).toBe('');
    await createHome(memberPage, member.headers, 'Other active home');
    const old = (await (await page.request.post('/api/household/invites', { headers: owner.headers })).json()).invite;
    await page.goto('/settings');
    const row = page.locator(`.member-row[data-user-id="${member.user.id}"]`);
    await row.locator('summary').click();
    page.once('dialog', dialog => dialog.accept());
    await row.locator('[data-action="remove-member"]').click();
    await expect(row).toHaveCount(0);
    for (const code of [home.inviteCode, old.code]) {
      expect((await memberPage.request.post('/api/household/join', { data: { inviteCode: code }, headers: member.headers })).status()).toBe(400);
    }
    expect((await memberPage.request.post(`/api/households/${home.id}/activate`, { headers: member.headers })).status()).toBe(403);
    await page.locator('[data-action="create-invite"]').click();
    await expect(page.locator('.invite-item')).toContainText('Expires');
    const fresh = (await (await page.request.get('/api/household/invites')).json()).invites[0];
    expect(fresh.maxUses).toBe(1);
    expect(fresh.expiresAt).toBeTruthy();
    expect((await memberPage.request.post('/api/household/join', { data: { inviteCode: fresh.code }, headers: member.headers })).ok()).toBe(true);
    await memberPage.goto('/settings');
    await expect(memberPage.locator('.member-list')).toBeVisible();
    await expect(memberPage.locator('[data-action="create-invite"]')).toHaveCount(0);
  } finally { await context.close(); }
});

test('deleting two sole homes preserves another member and shared private content', async ({ page, browser }) => {
  const deleting = await register(page);
  await createHome(page, deleting.headers, 'First sole home');
  await createChore(page, deleting.headers, 'First sole chore');
  await createHome(page, deleting.headers, 'Second sole home');
  await createChore(page, deleting.headers, 'Second sole chore');
  const context = await browser.newContext();
  try {
    const otherPage = await context.newPage();
    const other = await register(otherPage);
    const shared = await createHome(otherPage, other.headers, 'Surviving home');
    expect((await page.request.post('/api/household/join', { data: { inviteCode: shared.inviteCode }, headers: deleting.headers })).ok()).toBe(true);
    expect((await otherPage.request.patch(`/api/household/members/${deleting.user.id}`, { data: { role: 'admin' }, headers: other.headers })).ok()).toBe(true);
    const retained = await createChore(page, deleting.headers, 'Retained private chore', { visibility: 'admins' });
    await page.goto('/settings');
    await page.locator('[data-action="open-delete-account"]').click();
    await page.locator('#delete-account-input').fill('DELETE');
    await page.locator('[data-action="confirm-delete-account"]').click();
    await expect(page.locator('#login-form')).toBeVisible();
    const remaining = (await (await otherPage.request.get('/api/chores')).json()).chores;
    expect(remaining.find(c => c.id === retained.id)).toMatchObject({ createdBy: null, visibility: 'admins' });
    const household = await (await otherPage.request.get('/api/household')).json();
    expect(household.members.map(m => m.userId)).toEqual([other.user.id]);
    await otherPage.goto('/');
    await expect(otherPage.locator('.home-grid')).toContainText('Retained private chore');
  } finally { await context.close(); }
});

test('idempotent replay reauthorizes the stored log and rejects changed requests', async ({ page, browser }) => {
  const owner = await register(page);
  const home = await createHome(page, owner.headers, 'Replay home');
  const first = await createChore(page, owner.headers, 'Shared then private');
  const second = await createChore(page, owner.headers, 'Always shared');
  const context = await browser.newContext();
  try {
    const memberPage = await context.newPage();
    const member = await register(memberPage);
    expect((await memberPage.request.post('/api/household/join', { data: { inviteCode: home.inviteCode }, headers: member.headers })).ok()).toBe(true);
    const payload = { choreId: first.id, note: 'private replay sentinel', idempotencyKey: crypto.randomUUID() };
    const post = body => memberPage.request.post('/api/logs', { data: body, headers: member.headers });
    const one = await post(payload);
    expect(one.status()).toBe(201);
    const original = (await one.json()).log;
    expect((await (await post(payload)).json()).log.id).toBe(original.id);
    expect((await page.request.patch(`/api/chores/${first.id}`, { data: { ...first, visibility: 'admins' }, headers: owner.headers })).ok()).toBe(true);
    for (const body of [payload, { ...payload, choreId: second.id }]) {
      const denied = await post(body);
      expect(denied.ok()).toBe(false);
      expect(await denied.text()).not.toContain('private replay sentinel');
    }
    await memberPage.goto('/activity');
    await expect(memberPage.locator('body')).not.toContainText('private replay sentinel');
    const logs = (await (await page.request.get('/api/logs/history')).json()).logs;
    expect(logs.filter(l => l.id === original.id)).toHaveLength(1);
  } finally { await context.close(); }
});
