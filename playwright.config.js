import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 60000,
  expect: { timeout: 10000 },
  use: {
    baseURL: 'http://localhost:8080',
    headless: true,
    viewport: { width: 390, height: 844 },
  },
  projects: [
    {
      name: 'chromium',
      use: {
        browserName: 'chromium',
        ...(process.env.CHROMIUM_PATH
          ? { executablePath: process.env.CHROMIUM_PATH }
          : {}),
      },
    },
  ],
});
