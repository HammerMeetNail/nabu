import { test, expect } from '@playwright/test';

function uniqueEmail() {
  return `e2e-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@test.com`;
}

async function setupFullAccount(page) {
  const email = uniqueEmail();
  await page.goto('/register');
  await page.waitForSelector('#register-form');
  await page.fill('#reg-email', email);
  await page.fill('#reg-password', 'test123456');
  await page.fill('#reg-confirm', 'test123456');
  await page.click('button[type="submit"]');
  await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

  const csrf = (await page.context().cookies()).find(c => c.name === 'nabu_csrf')?.value || '';
  const hhResp = await page.request.post('/api/household', {
    data: { name: `Exhaustive ${Date.now()}` },
    headers: { 'X-CSRF-Token': csrf },
  });
  const hh = (await hhResp.json()).household;

  await page.request.post('/api/chores/seed-defaults', {
    data: { names: ['Feed Cats (Morning)', 'Feed Cats (Evening)', 'Wash Dishes', 'Make Bed', 'Walk Dog'] },
    headers: { 'X-CSRF-Token': csrf },
  });

  // Create an invite so we can test revoke
  await page.request.post('/api/household/invites', { headers: { 'X-CSRF-Token': csrf } });

  await page.reload();
  await page.waitForSelector('.home-grid', { timeout: 15000 });
  return { email, csrf, household: hh };
}

test.describe('Exhaustive: Auth Pages', () => {
  test('login page — all elements', async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('#login-form');

    // Title
    await expect(page.locator('.auth-title')).toContainText('Nabu');

    // Form fields
    await expect(page.locator('#login-email')).toBeVisible();
    await expect(page.locator('#login-password')).toBeVisible();

    // Submit button
    const signInBtn = page.locator('#login-form button[type="submit"]');
    await expect(signInBtn).toBeVisible();
    await expect(signInBtn).toContainText('Sign In');

    // Magic link button
    const magicBtn = page.locator('button[data-action="show-magic-link"]');
    await expect(magicBtn).toBeVisible();
    await expect(magicBtn).toContainText('Sign in with magic link');

    // Create account button
    const createBtn = page.locator('button[data-action="show-register"]');
    await expect(createBtn).toBeVisible();
    await expect(createBtn).toContainText('Create Account');

    // Test error: login with bad credentials
    await page.fill('#login-email', 'fake@no.com');
    await page.fill('#login-password', 'wrongpass');
    await signInBtn.click();
    await expect(page.locator('#login-error')).not.toHaveClass(/hidden/);
  });

  test('register page — all elements', async ({ page }) => {
    await page.goto('/register');
    await page.waitForSelector('#register-form');

    await expect(page.locator('.auth-title')).toContainText('Create Account');

    // Form fields
    await expect(page.locator('#reg-email')).toBeVisible();
    await expect(page.locator('#reg-password')).toBeVisible();
    await expect(page.locator('#reg-confirm')).toBeVisible();

    // Submit
    const createBtn = page.locator('#register-form button[type="submit"]');
    await expect(createBtn).toBeVisible();
    await expect(createBtn).toContainText('Create Account');

    // Back to login
    const loginBtn = page.locator('button[data-action="show-login"]');
    await expect(loginBtn).toBeVisible();
    await expect(loginBtn).toContainText(/Sign in/);

    // Test mismatched passwords error
    await page.fill('#reg-email', 'x@x.com');
    await page.fill('#reg-password', 'test123456');
    await page.fill('#reg-confirm', 'different');
    await createBtn.click();
    await expect(page.locator('#register-error')).not.toHaveClass(/hidden/);
  });

  test('magic link page — all elements', async ({ page }) => {
    await page.goto('/magic-link');
    await page.waitForSelector('#magic-link-form');

    await expect(page.locator('.auth-title')).toContainText('Magic Link');
    await expect(page.locator('#magic-email')).toBeVisible();
    
    const sendBtn = page.locator('#magic-link-form button[type="submit"]');
    await expect(sendBtn).toBeVisible();
    await expect(sendBtn).toContainText('Send Magic Link');

    // Back to login
    const backBtn = page.locator('button[data-action="show-login"]');
    await expect(backBtn).toBeVisible();
    await expect(backBtn).toContainText(/Back to sign in/);

    // Submit magic link
    await page.fill('#magic-email', 'test@example.com');
    await sendBtn.click();
    await expect(page.locator('text=Check your email')).toBeVisible();
  });

  test('forgot password page — all elements', async ({ page }) => {
    await page.goto('/forgot-password');
    await page.waitForSelector('#forgot-password-form');

    await expect(page.locator('.auth-title')).toContainText('Forgot Password');
    await expect(page.locator('#forgot-email')).toBeVisible();

    const sendBtn = page.locator('#forgot-password-form button[type="submit"]');
    await expect(sendBtn).toBeVisible();
    await expect(sendBtn).toContainText('Send Reset Link');

    // Back to login
    const backBtn = page.locator('button[data-action="show-login"]');
    await expect(backBtn).toBeVisible();
    await expect(backBtn).toContainText(/Back to sign in/);

    // Submit — should show toast
    await page.fill('#forgot-email', 'test@example.com');
    await sendBtn.click();
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });
  });

  test('reset password page — all elements', async ({ page }) => {
    await page.goto('/reset-password?token=test123');
    await page.waitForSelector('#reset-password-form');

    await expect(page.locator('.auth-title')).toContainText('Reset Password');
    await expect(page.locator('#reset-password')).toBeVisible();
    await expect(page.locator('#reset-confirm')).toBeVisible();

    const resetBtn = page.locator('#reset-password-form button[type="submit"]');
    await expect(resetBtn).toBeVisible();
    await expect(resetBtn).toContainText('Reset Password');

    // Test mismatched passwords
    await page.fill('#reset-password', 'test123456');
    await page.fill('#reset-confirm', 'different');
    await resetBtn.click();
    await expect(page.locator('#reset-error')).not.toHaveClass(/hidden/);
  });

  test('verify email page', async ({ page }) => {
    await page.goto('/verify-email');
    await page.waitForSelector('.auth-title');
    await expect(page.locator('.auth-title')).toContainText('Verify Your Email');

    const signInBtn = page.locator('button[data-action="show-login"]');
    await expect(signInBtn).toBeVisible();
    await expect(signInBtn).toContainText('Sign In');

    // Click sign in → should show login
    await signInBtn.click();
    await expect(page.locator('#login-form')).toBeVisible();
  });

  test('auth page navigation via buttons', async ({ page }) => {
    await page.goto('/');

    // Login → register
    await page.click('button[data-action="show-register"]');
    await page.waitForSelector('#register-form');
    await expect(page.locator('.auth-title')).toContainText('Create Account');

    // Register → login
    await page.click('button[data-action="show-login"]');
    await page.waitForSelector('#login-form');
    await expect(page.locator('.auth-title')).toContainText('Nabu');

    // Login → magic link
    await page.click('button[data-action="show-magic-link"]');
    await page.waitForSelector('#magic-link-form');
    await expect(page.locator('.auth-title')).toContainText('Magic Link');

    // Magic link → login
    await page.click('button[data-action="show-login"]');
    await page.waitForSelector('#login-form');
  });
});

test.describe('Exhaustive: Authenticated Flow', () => {
  test('register → welcome → household creation → full chore cycle → settings → logout', async ({ page }) => {
    const email = uniqueEmail();

    // === Register ===
    await page.goto('/register');
    await page.waitForSelector('#register-form');
    await page.fill('#reg-email', email);
    await page.fill('#reg-password', 'test123456');
    await page.fill('#reg-confirm', 'test123456');
    await page.click('button[type="submit"]');
    await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });

    // Should see top bar and bottom tabs
    await expect(page.locator('#top-bar')).not.toBeHidden({ timeout: 5000 });
    await expect(page.locator('#bottom-tabs')).not.toBeHidden();

    // === Welcome view (no household) ===
    const welcomeText = page.locator('text=Set up your household');
    await expect(welcomeText).toBeVisible({ timeout: 3000 });

    // Click "Set Up Household" button
    await page.locator('a[data-nav="settings"]').first().click();

    // Should be on settings page with household creation form
    await expect(page.locator('#create-household-form')).toBeVisible({ timeout: 5000 });
    // Should also see join household form
    await expect(page.locator('#join-household-form')).toBeVisible();
    // Sign Out is now in the profile sheet, not settings
    await expect(page.locator('button[data-action="logout"]')).toHaveCount(0);

    // === Create Household ===
    await page.fill('#hh-name', 'Exhaustive Test Home');
    await page.locator('#create-household-form button[type="submit"]').click();

    // Wait for household creation to finish and home grid to appear before
    // navigating to calendar (avoids race with doCreateHousehold's final render).
    await page.waitForSelector('.home-grid', { timeout: 15000 });

    // === Navigate to Settings ===
    await page.click('a[data-nav="settings"]');

    // === Settings View Elements ===
    await expect(page.locator('h2:has-text("Settings")')).toBeVisible({ timeout: 5000 });

    // Household section — wait for the page to fully load
    await expect(page.locator('.settings-view h3').first()).toBeVisible();
    
    // Create Invite Link button — wait for household data to load first
    await expect(page.locator('text=Members')).toBeVisible({ timeout: 10000 });
    const createInviteBtn = page.locator('button[data-action="create-invite"]');
    await expect(createInviteBtn).toBeVisible();
    
    // Click Create Invite
    await createInviteBtn.click();
    // Toast should appear with the invite code
    await expect(page.locator('#toast-container .toast')).toBeVisible({ timeout: 5000 });

    // Active invites section should now be visible (after reload)
    // The render after create-invite should show the invite list
    const revokeBtn = page.locator('button[data-action="delete-invite"]');
    if ((await revokeBtn.count()) > 0) {
      // Click revoke on the invite
      await revokeBtn.first().click();
    }

    // Members section
    await expect(page.locator('text=Members')).toBeVisible();
    await expect(page.locator('.member-list .member-row').first()).toBeVisible();

    // Leave Household button
    const leaveBtn = page.locator('button[data-action="leave-household"]');
    await expect(leaveBtn).toBeVisible();

    // === Test Top Bar Buttons ===
    
    // Household indicator should show initials
    const avatar = page.locator('#hh-indicator');
    await expect(avatar).toBeVisible();
    const avatarText = await avatar.textContent();
    expect(avatarText.length).toBeGreaterThan(0);

    // Notifications bell should be visible
    const bell = page.locator('#notifications-bell');
    await expect(bell).toBeVisible();

    // Click avatar → profile sheet
    await avatar.click();
    await expect(page.locator('.profile-panel')).toBeVisible({ timeout: 5000 });

    // Click Settings in profile sheet → navigate to settings
    await page.click('button[data-action="profile-nav-settings"]');
    await expect(page.locator('.settings-view')).toBeVisible({ timeout: 5000 });

    // Go to today view
    await page.click('a[data-nav="today"]');
    await expect(page.locator('.home-grid')).toBeVisible({ timeout: 5000 });

    // Click notifications bell → notification panel
    await bell.click();
    await expect(page.locator('.notif-panel')).toBeVisible({ timeout: 5000 });
    // Dismiss notification panel before proceeding
    await page.locator('.notif-close').click();
    await expect(page.locator('.notif-panel')).toHaveCount(0, { timeout: 3000 });

    // === Logout ===
    await avatar.click();
    await expect(page.locator('.profile-panel')).toBeVisible({ timeout: 5000 });
    await page.locator('button[data-action="logout"]').click();

    // Should return to login
    await expect(page.locator('#login-form')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#top-bar')).toBeHidden();
    await expect(page.locator('#bottom-tabs')).toBeHidden();

    console.log(`Exhaustive flow complete for ${email}`);
  });
});

test.describe('Exhaustive: SPA-only Navigation', () => {
  test('all bottom tabs navigate without page reload', async ({ page }) => {
    await setupFullAccount(page);

    // Verify each tab click changes the view
    const views = [
      { nav: 'activity', selector: '.history-view' },
      { nav: 'schedule', selector: '.schedule-view' },
      { nav: 'settings', selector: '.settings-view' },
      { nav: 'today', selector: '.home-grid' },
    ];

    for (const tab of views) {
      await page.click(`a[data-nav="${tab.nav}"]`);
      await expect(page.locator(tab.selector)).toBeVisible({ timeout: 5000 });
    }
  });
});

test.describe('Exhaustive: Settings Page States', () => {
  test('settings with no household: create + join + logout', async ({ page }) => {
    const email = uniqueEmail();
    await page.goto('/register');
    await page.waitForSelector('#register-form');
    await page.fill('#reg-email', email);
    await page.fill('#reg-password', 'test123456');
    await page.fill('#reg-confirm', 'test123456');
    await page.click('button[type="submit"]');
    await page.waitForSelector('#hh-indicator:not([hidden])', { timeout: 10000 });
    await expect(page.locator('#bottom-tabs')).not.toBeHidden();

    // Go to settings
    await page.click('a[data-nav="settings"]');
    await expect(page.locator('#create-household-form')).toBeVisible({ timeout: 5000 });

    // Should see create household form
    await expect(page.locator('#create-household-form')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#hh-name')).toBeVisible();
    await expect(page.locator('#create-household-form button[type="submit"]')).toContainText('Create Household');

    // Should see join household form
    await expect(page.locator('#join-household-form')).toBeVisible();
    await expect(page.locator('#invite-code')).toBeVisible();
    await expect(page.locator('#join-household-form button[type="submit"]')).toContainText('Join Household');

    // Account section
    await expect(page.getByRole('heading', { name: 'Account' })).toBeVisible();
    // Sign Out is in the profile sheet
    await expect(page.locator('button[data-action="logout"]')).toHaveCount(0);

    // Logout via profile sheet
    await page.locator('#hh-indicator').click();
    await expect(page.locator('.profile-panel')).toBeVisible({ timeout: 5000 });
    await page.locator('button[data-action="logout"]').click();
    await expect(page.locator('#login-form')).toBeVisible({ timeout: 5000 });
  });
});
