// tests/e2e/stats-dashboard.spec.js
// The Stats page loads in two requests: one for its sections and one for the
// per-chore charts, instead of one request per chart. Opening Stats again with
// nothing changed revalidates them (304) and shows the same page.

import { test, expect } from '@playwright/test';

test.use({ serviceWorkers: 'block' });

const PASSWORD = 'test123456';

async function setupWithChores(page) {
  const email = `e2e-statsdash-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', PASSWORD);
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  const headers = { 'X-CSRF-Token': csrf };
  await page.request.post('/api/household', { data: { name: `Dash ${Date.now()}` }, headers });
  await page.request.post('/api/chores/seed-defaults', { headers });
  const { chores } = await (await page.request.get('/api/chores')).json();
  // A log on every chore gives every chart and section real content.
  for (const chore of chores) {
    const res = await page.request.post('/api/logs', { headers, data: { choreId: chore.id, completedAt: new Date().toISOString() } });
    expect(res.ok()).toBeTruthy();
  }
  await page.goto('/today');
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { chores };
}

function recordAPI(page) {
  const requests = [];
  page.on('response', response => {
    const url = new URL(response.url());
    if (url.pathname.startsWith('/api/')) requests.push({ path: url.pathname, reads: url.searchParams.getAll('read'), status: response.status() });
  });
  return requests;
}

async function openStats(page) {
  await page.locator('#bottom-tabs [data-nav="stats"]').click();
  await expect(page.locator('.stats-page')).toBeVisible();
  await expect(page.locator('.stats-page')).not.toContainText('Loading charts', { timeout: 15000 });
}

test('Stats loads its sections and charts in two requests', async ({ page }) => {
  await setupWithChores(page);
  const requests = recordAPI(page);
  await openStats(page);

  const stats = requests.filter(r => r.path.startsWith('/api/stats/'));
  expect(stats.map(r => r.path)).toEqual(['/api/stats/dashboard', '/api/stats/dashboard']);
  const reads = stats.flatMap(r => r.reads);
  // Sections and charts travel apart, so the sections never wait for the charts.
  const charts = stats.find(r => r.reads.every(read => /^chores\/\d+\//.test(read)));
  const sections = stats.find(r => r !== charts);
  expect(sections.reads).toEqual(expect.arrayContaining(['overview', 'heatmap', 'busy-hours', 'chores?period=month',
    'breakdown?period=week', 'leaderboard?period=week']));
  expect(charts.reads.every(read => /^chores\/\d+\/(time-series|summary)\?/.test(read))).toBe(true);
  expect(charts.reads.length).toBeGreaterThan(2);
  expect(new Set(reads).size).toBe(reads.length);
  // Plus the Today count; nothing else goes to the network for Stats.
  expect(requests.length).toBeLessThanOrEqual(3);

  // Every section and chart has its data.
  await expect(page.locator('.chore-stat-card').first()).toBeVisible();
  await expect(page.locator('.stats-page')).not.toContainText('Some charts could not be refreshed');
  await expect(page.locator('[data-stats-section^="chore:"] .baby-member-row').first()).toBeVisible();
});

test('opening Stats again revalidates instead of downloading everything', async ({ page }) => {
  await setupWithChores(page);
  await openStats(page);
  const first = await page.locator('.stats-sections').innerText();

  await page.locator('#bottom-tabs [data-nav="today"]').click();
  await expect(page.locator('.home-grid')).toBeVisible();
  const requests = recordAPI(page);
  await openStats(page);
  const stats = requests.filter(r => r.path === '/api/stats/dashboard');
  expect(stats.map(r => r.status)).toEqual([304, 304]);
  expect(await page.locator('.stats-sections').innerText()).toBe(first);

  // A new log changes the answer: the page downloads and shows it.
  await page.locator('#bottom-tabs [data-nav="today"]').click();
  await expect(page.locator('.home-grid')).toBeVisible();
  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  const { chores } = await (await page.request.get('/api/chores')).json();
  const res = await page.request.post('/api/logs', { headers: { 'X-CSRF-Token': csrf }, data: { choreId: chores[0].id, completedAt: new Date().toISOString() } });
  expect(res.ok()).toBeTruthy();
  requests.length = 0;
  await openStats(page);
  expect(requests.filter(r => r.path === '/api/stats/dashboard').map(r => r.status)).toContain(200);
});
