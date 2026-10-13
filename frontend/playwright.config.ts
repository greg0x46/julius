import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  globalSetup: "./e2e/setup.ts",
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  workers: 2,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    ...devices["Desktop Chrome"],
    timezoneId: "America/Sao_Paulo",
    locale: "pt-BR",
    actionTimeout: 10_000,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
