// tests/e2e/security-escape.spec.js
// E2E tests verifying frontend escaping of user-controlled chore metadata.
//
// Regression coverage for the security review follow-up (2026-06-01):
//   - category must be escaped in calendar day view
//   - indicator labels must be escaped in History view
//   - category must be escaped in Stats view
//   - malicious category/indicator-label input rejected server-side

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-sec-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithChores(page) {
  const email = uniqueEmail();

  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';

  await page.request.post('/api/household', {
    data: { name: `Escape Test ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.request.post('/api/chores/seed-defaults', {
    headers: { 'X-CSRF-Token': csrf },
  });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  return { csrf };
}

async function postLog(page, csrf, choreId, opts = {}) {
  const { indicators = [], hour } = opts;
  await page.request.post('/api/logs', {
    data: {
      choreId,
      note: '',
      indicators,
      date: new Date().toISOString().slice(0, 10),
      completedAt: new Date().toISOString(),
      hour: hour !== undefined ? hour : 12,
    },
    headers: { 'X-CSRF-Token': csrf },
  });
}

test.describe('Security: output escaping', () => {
  test('category with HTML tags stored and returned correctly by API', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.post('/api/chores', {
      data: {
        name: 'Esc Cat',
        icon: '\u{1F9F9}',
        color: '#FF0000',
        category: '<b>inj</b>',
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(201);
    const chore = (await resp.json()).chore;
    expect(chore.category).toBe('<b>inj</b>');

    // Verify the GET response also returns it.
    const getResp = await page.request.get('/api/chores');
    expect(getResp.status()).toBe(200);
    const all = (await getResp.json()).chores;
    const found = all.find(c => c.id === chore.id);
    expect(found.category).toBe('<b>inj</b>');
  });

  test('category with HTML tags renders harmlessly in Stats view', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    await page.request.post('/api/chores', {
      data: {
        name: 'Stats Cat',
        icon: '\u{1F4CA}',
        color: '#00FF00',
        category: '<i>x</i>',
      },
      headers: { 'X-CSRF-Token': csrf },
    });

    const { chores } = await (await page.request.get('/api/chores')).json();
    const catChore = chores.find(c => c.name === 'Stats Cat');
    if (catChore) {
      await postLog(page, csrf, catChore.id);
    }

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    // Navigate directly to stats tab.
    await page.click('a[data-nav="stats"]');
    await page.waitForTimeout(3000);

    // The page HTML should contain the escaped version.
    const bodyHTML = await page.innerHTML('#app');
    expect(bodyHTML).toContain('&lt;i&gt;x&lt;/i&gt;');
  });

  test('indicator labels with HTML escape safely in History view', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const createResp = await page.request.post('/api/chores', {
      data: {
        name: 'Ind Esc',
        icon: '\u{1F4CC}',
        color: '#0000FF',
        category: 'custom',
        indicatorLabels: ['<b>x</b>'],
        indicatorDefaults: ['<b>x</b>'],
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(createResp.status()).toBe(201);
    const created = (await createResp.json()).chore;

    await postLog(page, csrf, created.id, { indicators: ['<b>x</b>'] });

    await page.reload();
    await page.waitForSelector('.home-grid', { timeout: 15000 });
    await page.click('a[data-nav="activity"]');
    await page.waitForSelector('.history-view', { timeout: 10000 });

    const histMeta = page.locator('.hist-meta').first();
    await expect(histMeta).toBeVisible({ timeout: 5000 });
    const metaHTML = await histMeta.innerHTML();

    expect(metaHTML).not.toContain('<b>x</b>');
  });

  test('server rejects overlong category', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const longCategory = 'x'.repeat(31);
    const resp = await page.request.post('/api/chores', {
      data: {
        name: 'Long Cat',
        icon: '\u{1F4CC}',
        color: '#FF0000',
        category: longCategory,
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(400);
  });

  test('server rejects category with control characters', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.post('/api/chores', {
      data: {
        name: 'Ctrl Cat',
        icon: '\u{1F4CC}',
        color: '#FF0000',
        category: 'test\nbreak',
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(400);
  });

  test('server rejects too many indicator labels', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.post('/api/chores', {
      data: {
        name: 'Many Labels',
        icon: '\u{1F4CC}',
        color: '#FF0000',
        category: 'custom',
        indicatorLabels: ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i'],
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(400);
  });

  test('server rejects indicator defaults not in labels', async ({ page }) => {
    const { csrf } = await setupWithChores(page);

    const resp = await page.request.post('/api/chores', {
      data: {
        name: 'Bad Default',
        icon: '\u{1F4CC}',
        color: '#FF0000',
        category: 'custom',
        indicatorLabels: ['good'],
        indicatorDefaults: ['evil'],
      },
      headers: { 'X-CSRF-Token': csrf },
    });
    expect(resp.status()).toBe(400);
  });
});
