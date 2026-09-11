import {test, expect} from '@playwright/test';
import {fixture, register, headers, postLog} from './review-fixtures.js';

test('private matches cannot fill search slots or hide older visible Activity', async ({page, browser}) => {
  const owner = await fixture(page, {name: 'Visible amount'});
  const hiddenResponse = await page.request.post('/api/chores', {
    headers: owner.headers,
    data: {name: 'Private task', icon: '📝', color: '#6080AA', visibility: 'admins'},
  });
  expect(hiddenResponse.ok()).toBeTruthy();
  const hidden = (await hiddenResponse.json()).chore;
  const old = new Date();
  old.setUTCDate(old.getUTCDate() - 20);
  const visibleLog = await postLog(page, owner.chore, {note: 'needle visible amount', completedAt: old.toISOString(), date: old.toISOString().slice(0, 10), volumeML: 120});
  for (let i = 0; i < 101; i++) await postLog(page, hidden, {note: 'needle private'});
  const inviteResponse = await page.request.post('/api/household/invites', {headers: owner.headers});
  expect(inviteResponse.ok()).toBeTruthy();
  const code = (await inviteResponse.json()).invite.code;
  const context = await browser.newContext();
  try {
    const member = await context.newPage();
    await register(member);
    const joined = await member.request.post('/api/household/join', {headers: await headers(member), data: {inviteCode: code}});
    expect(joined.ok()).toBeTruthy();
    await member.reload();
    await expect(member.locator('.home-grid')).toBeVisible();
    const initial = await member.request.get('/api/logs/history');
    expect(initial.ok()).toBeTruthy();
    expect(await initial.json()).toMatchObject({logs: [], hasMore: true});
    await member.locator('[data-nav="activity"]').click();
    // Empty weeks retain an explicit control; unscrolled pages do not auto-drain.
    for (let i = 0; i < 2; i++) {
      const loaded = member.waitForResponse(r => new URL(r.url()).pathname === '/api/logs/history' && new URL(r.url()).searchParams.has('before'));
      await member.locator('.load-more-btn').click();
      expect((await loaded).ok()).toBeTruthy();
    }
    await expect(member.locator('.hist-row', {hasText: 'needle visible amount'})).toBeVisible();
    const response = member.waitForResponse(r => new URL(r.url()).pathname === '/api/logs/history' && new URL(r.url()).searchParams.get('q') === 'needle');
    await member.locator('#history-search-input').fill('needle');
    const found = await response;
    expect(found.ok()).toBeTruthy();
    expect((await found.json()).logs.map(log => log.id)).toEqual([visibleLog.id]);
    await expect(member.locator('.hist-row')).toHaveCount(1);
    await expect(member.locator('.history-view')).not.toContainText('needle private');
    await member.reload();
    const summary = await member.request.get(`/api/stats/chores/${owner.chore.id}/summary?period=all`);
    expect(summary.ok()).toBeTruthy();
    expect(await summary.json()).toMatchObject({summary: {count: 1, totalML: 120}});
  } finally {
    await context.close();
  }
});
