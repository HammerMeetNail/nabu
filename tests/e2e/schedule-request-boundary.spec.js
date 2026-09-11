import {test, expect} from '@playwright/test';
import {fixture} from './review-fixtures.js';

test('schedule mutations reject path injection and valid updates/deletes persist', async ({page}) => {
  const {chore, headers} = await fixture(page);
  const response = await page.request.post('/api/schedules', {headers, data: {
    choreId: chore.id, timePeriod: 'anytime', specificTime: '08:00',
    frequencyType: 'daily', isActive: true,
  }});
  expect(response.ok()).toBeTruthy();
  const {schedule} = await response.json();
  await page.reload();
  await expect(page.locator('.home-grid')).toBeVisible();

  // Exercise the actual served mutation boundary; the legacy calendar drop UI
  // that accepts external drag data is no longer routed from the Activity tab.
  const mutations = [];
  page.on('request', request => {
    if (['PATCH', 'DELETE'].includes(request.method())) mutations.push(request.url());
  });
  const errors = await page.evaluate(async () => {
    const suffix = new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
    const {updateSchedule, deleteSchedule} = await import(`/static/js/schedule.js${suffix}`);
    const errors = [];
    for (const id of ['../notification-preferences', '%2e%2e/chores', '1?other=1', '1#fragment', 0, -1, 1.5, null, true]) {
      for (const mutate of [() => updateSchedule(id, {specificTime:'09:00'}), () => deleteSchedule(id)]) {
        try { await mutate(); errors.push('accepted'); }
        catch (error) { errors.push(error.message); }
      }
    }
    return errors;
  });
  expect(errors).toEqual(Array(18).fill('Invalid schedule ID'));
  expect(mutations).toEqual([]);

  await page.evaluate(async id => {
    const suffix = new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
    const {updateSchedule} = await import(`/static/js/schedule.js${suffix}`);
    await updateSchedule(id, {specificTime:'09:00'});
  }, schedule.id);
  await page.reload();
  await page.locator('[data-nav="schedule"]').click();
  await expect(page.locator('.sch-time').first()).toContainText('9:00 AM');
  const updated = await page.request.get('/api/schedules');
  expect((await updated.json()).schedules.find(s => s.id === schedule.id).specificTime).toBe('09:00');

  await page.evaluate(async id => {
    const suffix = new URL(document.querySelector('script[src*="/static/js/app.js"]').src).search;
    const {deleteSchedule} = await import(`/static/js/schedule.js${suffix}`);
    await deleteSchedule(String(id));
  }, schedule.id);
  await page.reload();
  await page.locator('[data-nav="schedule"]').click();
  await expect(page.locator('.sch-row')).toHaveCount(0);
  const deleted = await page.request.get('/api/schedules');
  expect((await deleted.json()).schedules).toEqual([]);
});
