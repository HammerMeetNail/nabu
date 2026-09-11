import {test,expect} from '@playwright/test';
import {fixture,register,headers,postLog} from './review-fixtures.js';

test('read pages retain older notifications, failed loads and deletes are retryable, and receipts persist',async({page,browser})=>{
  const owner=await fixture(page);
  const invite=await page.request.post('/api/household/invites',{headers:owner.headers});
  const code=(await invite.json()).invite.code;
  const context=await browser.newContext();const member=await context.newPage();
  try {
    await register(member);
    const memberHeaders=await headers(member);
    expect((await member.request.post('/api/household/join',{headers:memberHeaders,data:{inviteCode:code}})).ok()).toBeTruthy();
    await member.goto('/');await expect(member.locator('.home-grid')).toBeVisible();
    for(let i=0;i<55;i++) await postLog(page,owner.chore,{note:`Synthetic notification history ${i}`});
    await expect.poll(async()=>((await (await member.request.get('/api/notifications')).json()).unreadCount),{timeout:15000}).toBe(55);
    const first=await (await member.request.get('/api/notifications')).json();
    expect(first.notifications).toHaveLength(50);expect(first.nextCursor).toBeTruthy();
    for(const row of first.notifications) expect((await member.request.post(`/api/notifications/${row.id}/read`,{headers:memberHeaders})).ok()).toBeTruthy();
    await member.reload();await expect(member.locator('.home-grid')).toBeVisible();
    await member.locator('#notifications-bell').click();
    await expect(member.locator('.notif-item')).toHaveCount(50);
    await expect(member.locator('.notif-read')).toHaveCount(50);
    const older=member.locator('[data-action="more-notifications"]');
    await expect(older).toBeVisible();
    await member.route('**/api/notifications?cursor=**',route=>route.fulfill({status:500,contentType:'application/json',body:'{"error":"Synthetic history failure"}'}),{times:1});
    await older.click();
    await expect(member.getByTestId('notification-error')).toHaveText('Synthetic history failure');
    await expect(member.locator('.notif-item')).toHaveCount(50);
    await older.click();
    await expect(member.locator('.notif-item')).toHaveCount(55);
    await expect(older).toHaveCount(0);
    // resumeSession starts notification loading before its household refresh.
    // Observing that later request establishes whether the old eager read ran.
    const resumeNotifications=[];
    const countNotification=request=>{if(new URL(request.url()).pathname==='/api/notifications') resumeNotifications.push(request);};
    member.on('request',countNotification);
    const householdRefresh=member.waitForRequest(request=>new URL(request.url()).pathname==='/api/household');
    await member.evaluate(()=>window.dispatchEvent(new Event('online')));
    await householdRefresh;
    expect(resumeNotifications).toHaveLength(0);
    member.off('request',countNotification);
    await expect(member.locator('.notif-item')).toHaveCount(55);
    const lastId=await member.locator('.notif-item').last().getAttribute('data-notif-id');
    const last=member.locator(`.notif-item[data-notif-id="${lastId}"]`);
    await member.route(`**/api/notifications/${lastId}`,route=>route.fulfill({status:500,contentType:'application/json',body:'{"error":"Synthetic delete failure"}'}),{times:1});
    await last.locator('[data-action="dismiss-notification"]').click();
    await expect(member.getByTestId('notification-error')).toHaveText('Synthetic delete failure');
    await expect(last).toBeVisible();
    await last.locator('[data-action="dismiss-notification"]').click();
    await expect(last).toHaveCount(0);await expect(member.locator('.notif-item')).toHaveCount(54);
    await member.reload();await expect(member.locator('.home-grid')).toBeVisible();
    await member.locator('#notifications-bell').click();await expect(member.locator('.notif-item')).toHaveCount(50);
    await member.locator('[data-action="more-notifications"]').click();
    await expect(member.locator('.notif-item')).toHaveCount(54);
    await expect(member.locator('.notif-read')).toHaveCount(50);
    await expect(last).toHaveCount(0);
  } finally {await context.close();}
});

test('notification refresh failures remain visible and can be retried and dismissed',async({page})=>{
  await fixture(page);
  await page.route('**/api/notifications',route=>route.fulfill({status:500,contentType:'application/json',body:'{"error":"Notifications unavailable"}'}));
  await page.locator('#notifications-bell').click();
  await expect(page.getByTestId('notification-error')).toHaveText('Notifications unavailable');
  await expect(page.getByRole('button',{name:'Refresh notifications',exact:true})).toBeEnabled();
  await page.unroute('**/api/notifications');
  await page.getByRole('button',{name:'Refresh notifications',exact:true}).click();
  await expect(page.getByTestId('notification-error')).toHaveCount(0);
  await page.locator('.notif-close').click();
  await expect(page.locator('#notif-panel-container')).toBeHidden();
});
