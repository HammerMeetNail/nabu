// tests/e2e/onboarding-packs.spec.js
// A new account sets up its household on Home: name and color, the
// household's name, then who and what it looks after. Only the picked packs
// are seeded, named for the baby and pets given. Also: going back keeps what
// was typed, joining instead, and retrying packs that failed to seed.

import { test, expect } from '@playwright/test';

async function register(page) {
  const email = `e2e-onboard-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@test.com`;
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await expect(page.getByRole('heading', { name: 'Welcome to Nabu' })).toBeVisible({ timeout: 10000 });
  return email;
}

const continueButton = page => page.getByRole('button', { name: 'Continue' });
const tileNames = page => page.locator('.home-grid .home-card-name');

test.describe('Household setup', () => {
  test('seeds only the picked packs, named for the baby and pets', async ({ page }) => {
    const email = await register(page);
    await page.fill('#onboarding-name', "Dana O'Connor");
    await page.locator('.profile-color', { hasText: 'Purple' }).click();
    await continueButton(page).click();

    await expect(page.getByText('Step 2 of 3')).toBeVisible();
    await expect(page.locator('#onboarding-household')).toHaveValue("The O'Connors' home");
    await expect(page.locator('#onboarding-initials')).toHaveValue('TOH');
    await continueButton(page).click();

    await expect(page.getByRole('heading', { name: 'Who and what do you look after?' })).toBeVisible();
    await page.locator('[data-onboarding-pack="baby"]').check();
    await page.getByLabel("Baby's name", { exact: true }).fill('Ada');
    await page.locator('[data-onboarding-pack="cat"]').check();
    await page.getByLabel("Cat's name", { exact: true }).fill('Mongo');
    await page.locator('[data-action="onboarding-add-name"][data-pack="cat"]').click();
    await expect(page.getByLabel("Cat's name 2")).toBeFocused();
    await page.getByLabel("Cat's name 2").fill('Roger');
    // Picking another pack re-renders the list but keeps the names.
    await page.locator('[data-onboarding-pack="meds"]').check();
    await expect(page.getByLabel("Cat's name 1")).toHaveValue('Mongo');
    await page.getByRole('button', { name: 'Create household' }).click();

    await expect(page.getByRole('heading', { name: 'Invite your household' })).toBeVisible({ timeout: 15000 });
    const { household } = await (await page.request.get('/api/household')).json();
    expect(household.name).toBe("The O'Connors' home");
    expect(household.initials).toBe('TOH');
    await expect(page.locator('.invite-link-url')).toHaveText(new RegExp(`/join\\?code=${household.inviteCode}$`));
    await page.getByRole('button', { name: 'Go to Home' }).click();

    const expected = ['Feed Ada', 'Change Ada', "Ada's Bath", 'Feed Mongo', 'Feed Roger', 'Clean Litter Box', "Ada's Meds", 'Cat Meds'];
    await expect(tileNames(page)).toHaveText(expected);
    await page.reload();
    await expect(tileNames(page)).toHaveText(expected);

    const { user } = await (await page.request.get('/api/me')).json();
    expect(user).toMatchObject({ email, displayName: "Dana O'Connor", avatarColor: '#7B5EA7' });
    const { chores } = await (await page.request.get('/api/chores')).json();
    // A baby's or pet's own activities are custom tasks with their template's
    // fields; the cats' shared meds keep the template and ask which cat.
    expect(chores.find(c => c.name === 'Feed Ada')).toMatchObject({ isPredefined: false, predefinedKey: '', metricType: 'amount', metricUnit: 'mL' });
    expect(chores.find(c => c.name === 'Feed Roger')).toMatchObject({ isPredefined: false, predefinedKey: '', icon: '🐱' });
    expect(chores.find(c => c.name === 'Cat Meds')).toMatchObject({ isPredefined: true, predefinedKey: 'Cat Meds', subjects: ['Mongo', 'Roger'] });

    // Logging works on a named activity.
    await page.locator('.home-chore-card', { hasText: 'Feed Mongo' }).click();
    await page.locator('[data-action="save-log"]').click();
    await expect(page.locator('.home-chore-card', { hasText: 'Feed Mongo' }).locator('.home-card-time')).not.toHaveText('never');

    // The cats' meds ask which cat.
    await page.locator('.home-chore-card', { hasText: 'Cat Meds' }).click();
    await expect(page.locator('.subject-chip')).toHaveText(['Mongo', 'Roger']);
    await page.locator('.subject-chip', { hasText: 'Roger' }).click();
    await page.locator('[data-action="save-log"]').click();
    await expect(page.locator('.home-chore-card', { hasText: 'Cat Meds' }).locator('.home-card-time')).not.toHaveText('never');
    const meds = chores.find(c => c.name === 'Cat Meds');
    const { latestLogs } = await (await page.request.get('/api/logs/latest-per-chore')).json();
    expect(latestLogs[meds.id]).toMatchObject({ subject: 'Roger' });
  });

  test('refuses a blank name or no packs, and going back keeps what was typed', async ({ page }) => {
    await register(page);
    await page.fill('#onboarding-name', '   ');
    await continueButton(page).click();
    await expect(page.locator('.onboarding .form-error')).toHaveText('Enter your name');

    await page.fill('#onboarding-name', 'Sam');
    await continueButton(page).click();
    await expect(page.locator('#onboarding-household')).toHaveValue("Sam's home");
    await page.fill('#onboarding-household', 'Flat 4');
    await page.getByRole('button', { name: 'Back' }).click();
    await expect(page.locator('#onboarding-name')).toHaveValue('Sam');
    await continueButton(page).click();
    await expect(page.locator('#onboarding-household')).toHaveValue('Flat 4');

    await page.fill('#onboarding-household', '');
    await continueButton(page).click();
    await expect(page.locator('.onboarding .form-error')).toHaveText('Name your household');
    await page.fill('#onboarding-household', 'Flat 4');
    await continueButton(page).click();
    await page.getByRole('button', { name: 'Create household' }).click();
    await expect(page.locator('.onboarding .form-error')).toHaveText('Pick at least one');
    expect((await (await page.request.get('/api/me')).json()).user.householdId).toBeNull();

    // A name used twice is refused before anything is created.
    await page.locator('[data-onboarding-pack="baby"]').check();
    await page.getByLabel("Baby's name", { exact: true }).fill('Max');
    await page.locator('[data-onboarding-pack="dog"]').check();
    await page.getByLabel("Dog's name", { exact: true }).fill('max');
    await page.getByRole('button', { name: 'Create household' }).click();
    await expect(page.locator('.onboarding .form-error')).toHaveText('"max" is used twice; give each one a different name');
    expect((await (await page.request.get('/api/me')).json()).user.householdId).toBeNull();
    await page.getByLabel("Dog's name", { exact: true }).fill('Rex');
    await page.getByRole('button', { name: 'Create household' }).click();
    await page.getByRole('button', { name: 'Go to Home' }).click();
    await expect(tileNames(page)).toHaveText(['Feed Max', 'Change Max', "Max's Bath", 'Walk Rex', 'Feed Rex']);
  });

  test('a failed seed keeps the household and retries the packs', async ({ page }) => {
    await register(page);
    await page.fill('#onboarding-name', 'Lee');
    await continueButton(page).click();
    await continueButton(page).click();
    await page.locator('[data-onboarding-pack="plants"]').check();
    let failed = false;
    await page.route('**/api/chores/packs', route => {
      if (failed) return route.continue();
      failed = true;
      return route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"failed to add activities"}' });
    });
    await page.getByRole('button', { name: 'Create household' }).click();
    await expect(page.locator('.onboarding .form-error')).toContainText("Your household is ready, but its activities weren't added.");
    await expect(page.getByRole('button', { name: 'Back' })).toHaveCount(0);
    expect((await (await page.request.get('/api/me')).json()).user.householdId).not.toBeNull();
    await expect(page.locator('[data-onboarding-pack="plants"]')).toBeChecked();
    await page.getByRole('button', { name: 'Add activities' }).click();
    await page.getByRole('button', { name: 'Go to Home' }).click();
    await expect(tileNames(page)).toHaveText(['Water Plants']);
    const { households } = await (await page.request.get('/api/households')).json();
    expect(households).toHaveLength(1);
  });

  test('joining a household instead seeds nothing', async ({ page, browser }) => {
    // Someone else's household, set up through the API.
    const owner = await browser.newContext({ baseURL: process.env.BASE_URL || 'http://localhost:8080' });
    const ownerPage = await owner.newPage();
    await register(ownerPage);
    const csrf = (await owner.cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
    const created = await ownerPage.request.post('/api/household', { data: { name: 'Shared home' }, headers: { 'X-CSRF-Token': csrf } });
    const { household } = await created.json();
    expect((await ownerPage.request.post('/api/chores/packs', { data: { packs: [{ key: 'plants' }] }, headers: { 'X-CSRF-Token': csrf } })).ok()).toBeTruthy();
    await owner.close();

    await register(page);
    await page.getByRole('button', { name: 'Joining someone? Enter an invite code' }).click();
    await page.fill('#invite-code', household.inviteCode);
    await page.getByRole('button', { name: 'Join household' }).click();
    await expect(tileNames(page)).toHaveText(['Water Plants'], { timeout: 15000 });
    await expect(page.getByRole('heading', { name: 'Invite your household' })).toHaveCount(0);
  });

  test('Settings and other tabs lead to setup on Home', async ({ page }) => {
    await register(page);
    await page.click('a[data-nav="activity"]');
    await page.getByRole('link', { name: 'Set up your household' }).click();
    await expect(page.getByRole('heading', { name: 'Welcome to Nabu' })).toBeVisible();
    await page.click('a[data-nav="settings"]');
    await page.locator('.settings-view').getByRole('link', { name: 'Set up a household' }).click();
    await expect(page.getByRole('heading', { name: 'Welcome to Nabu' })).toBeVisible();
  });
});

test.describe('Manage → Add activities', () => {
  test('adds a pack named for a new pet, keeps what the household has, and cancels cleanly', async ({ page }) => {
    // A household from before packs: the full default set.
    await register(page);
    const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
    const headers = { 'X-CSRF-Token': csrf };
    expect((await page.request.post('/api/household', { data: { name: 'Older home' }, headers })).ok()).toBeTruthy();
    expect((await page.request.post('/api/chores/seed-defaults', { headers })).ok()).toBeTruthy();
    await page.reload();
    await expect(tileNames(page)).toHaveCount(16);

    await page.locator('[data-action="switch-home-view"][data-view="manage"]').click();
    await page.getByRole('button', { name: 'Add activities' }).click();
    await expect(page.locator('.packs-sheet')).toBeVisible();
    await page.locator('.packs-sheet [data-onboarding-pack="dog"]').check();
    await page.locator('.packs-sheet').getByLabel("Dog's name", { exact: true }).fill('Rex');
    await page.locator('.packs-sheet').getByRole('button', { name: 'Add', exact: true }).click();
    await expect(page.locator('#toast-container .toast')).toContainText('Added 2 activities');
    await expect(page.locator('.packs-sheet')).toHaveCount(0);
    await expect(page.locator('.chore-row-name')).toContainText(['Walk Rex', 'Feed Rex']);

    // A pack the household has already adds nothing.
    await page.getByRole('button', { name: 'Add activities' }).click();
    await page.locator('.packs-sheet [data-onboarding-pack="plants"]').check();
    await page.locator('.packs-sheet').getByRole('button', { name: 'Add', exact: true }).click();
    await expect(page.locator('#toast-container .toast').last()).toContainText('You have these activities already');

    // Cancel adds nothing.
    await page.getByRole('button', { name: 'Add activities' }).click();
    await page.locator('.packs-sheet [data-onboarding-pack="personal"]').check();
    await page.locator('.packs-sheet').getByRole('button', { name: 'Cancel' }).click();
    await expect(page.locator('.packs-sheet')).toHaveCount(0);

    const { chores } = await (await page.request.get('/api/chores')).json();
    expect(chores).toHaveLength(18);
    expect(chores.find(c => c.name === 'Walk Rex')).toMatchObject({ isPredefined: false, predefinedKey: '' });
    await page.locator('[data-action="switch-home-view"][data-view="log"]').click();
    await expect(page.locator('.home-chore-card', { hasText: 'Feed Rex' })).toBeVisible();
  });
});
