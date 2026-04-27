import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 60000,
  retries: 0,
  use: {
    baseURL: "http://localhost:8080",
    headless: true,
  },
  webServer: {
    command: "make run",
    url: "http://localhost:8080/health",
    reuseExistingServer: true,
  },
});
