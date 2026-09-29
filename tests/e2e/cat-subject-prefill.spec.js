// tests/e2e/cat-subject-prefill.spec.js
// Regression: opening a new log sheet for a chore with subjects (e.g. Cat
// Meds with cat names) must preselect the previous log's subject, mirroring
// how the latest log's indicator selection is echoed. Previously the sheet
// only preselected the subject of the log being edited, so the previously
// selected cat was not autoselected on the next entry.

import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-catsubject-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
}

async function setupWithSubjectChore(page, choreName = 'Cat Meds', subjects = ['Milo', 'Nala']) {
  const email = uniqueEmail();
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  const headers = { 'X-CSRF-Token': csrf };

  const hh = await page.request.post('/api/household', { data: { name: `Cat Subject ${Date.now()}` }, headers });
  expect(hh.ok()).toBeTruthy();
  await page.request.post('/api/chores/seed-defaults', { headers });

  const { chores } = await (await page.request.get('/api/chores')).json();
  const chore = chores.find(c => c.name === choreName);
  expect(chore).toBeDefined();

  // Give the meds chore its subject (cat) tags.
  const patch = await page.request.patch(`/api/chores/${chore.id}`, { headers, data: { subjects } });
  expect(patch.ok()).toBeTruthy();

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { email, headers, chore };
}

const subjectChip = (name) => `.subject-chip[data-subject="${name}"]`;

test('new log sheet preselects the previous log\'s subject', async ({ page }) => {
  const { headers, chore } = await setupWithSubjectChore(page);

  // Seed a log with the "Milo" subject so the next sheet should echo it.
  const seed = await page.request.post('/api/logs', {
    headers,
    data: { choreId: chore.id, subject: 'Milo', volumeML: 20, hour: 12, completedAt: new Date().toISOString() },
  });
  expect(seed.ok()).toBeTruthy();

  // Reload so state.latestLogs picks up the seed log (drives the echo).
  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  // Open a fresh log sheet from the home tab.
  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
  await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

  // The previously selected cat must be preselected.
  await expect(page.locator(subjectChip('Milo'))).toHaveClass(/subject-chip--on/);
  await expect(page.locator(subjectChip('Nala'))).not.toHaveClass(/subject-chip--on/);

  // Saving without touching the subject must keep the preselection.
  await page.locator('[data-action="save-log"]').click();
  await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });
  await expect(page.locator('.bottom-sheet')).toHaveCount(0);

  const { logs } = await (await page.request.get('/api/logs/history')).json();
  const created = logs.filter(l => l.choreId === chore.id).sort((a, b) => b.id - a.id)[0];
  expect(created.subject).toBe('Milo');
});

test('new log sheet preselects nothing when the previous log has no subject', async ({ page }) => {
  const { headers, chore } = await setupWithSubjectChore(page);

  // Seed a log without a subject.
  const seed = await page.request.post('/api/logs', {
    headers,
    data: { choreId: chore.id, volumeML: 20, hour: 12, completedAt: new Date().toISOString() },
  });
  expect(seed.ok()).toBeTruthy();

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });

  await page.locator(`[data-home-chore-id="${chore.id}"]`).click();
  await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

  // Subject chips render (the chore has subjects) but none is preselected.
  await expect(page.locator(subjectChip('Milo'))).toBeVisible();
  await expect(page.locator('.subject-chip.subject-chip--on')).toHaveCount(0);
});

test('editing a saved log keeps its own subject preselected', async ({ page }) => {
  const { headers, chore } = await setupWithSubjectChore(page);

  // Seed two logs so the latest ("Nala") and an older one ("Milo") exist.
  const first = await page.request.post('/api/logs', {
    headers,
    data: { choreId: chore.id, subject: 'Milo', volumeML: 20, hour: 12, completedAt: new Date().toISOString() },
  });
  expect(first.ok()).toBeTruthy();
  const second = await page.request.post('/api/logs', {
    headers,
    data: { choreId: chore.id, subject: 'Nala', volumeML: 10, hour: 13, completedAt: new Date().toISOString() },
  });
  expect(second.ok()).toBeTruthy();
  const log = (await second.json()).log;

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  await page.locator('[data-nav="activity"]').click();
  await page.locator(`[data-action="view-log"][data-log-id="${log.id}"]`).click();
  await expect(page.locator('.bottom-sheet')).toBeVisible({ timeout: 3000 });

  // The edited log's own subject wins over the latest-log echo.
  await expect(page.locator(subjectChip('Nala'))).toHaveClass(/subject-chip--on/);
  await expect(page.locator(subjectChip('Milo'))).not.toHaveClass(/subject-chip--on/);
});
