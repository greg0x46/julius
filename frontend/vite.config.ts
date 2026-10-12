import react from "@vitejs/plugin-react";
import { configDefaults, defineConfig } from "vitest/config";

const apiProxyTarget = process.env.JULIUS_DEV_API_URL ?? "http://localhost:8000";

// .ts tests that need the DOM; every other .ts test runs without jsdom.
const domTsTests = ["src/api/statementImports.test.ts"];

export default defineConfig({
  cacheDir: ".vite-cache",
  plugins: [react()],
  server: {
    proxy: {
      "/api": apiProxyTarget,
      "/health": apiProxyTarget,
    },
    // Only set when the dev server is reached through a hostname Vite
    // wouldn't otherwise trust.
    allowedHosts: process.env.JULIUS_DEV_ALLOWED_HOST
      ? [process.env.JULIUS_DEV_ALLOWED_HOST]
      : undefined,
  },
  // Project split and worker count are explained in TESTING.md.
  test: {
    restoreMocks: true,
    clearMocks: true,
    maxWorkers: 2,
    pool: "threads",
    testTimeout: 15_000,
    projects: [
      {
        extends: true,
        test: {
          name: "dom",
          environment: "jsdom",
          include: ["src/**/*.test.tsx", ...domTsTests],
          setupFiles: "./src/test/setup.dom.ts",
        },
      },
      {
        extends: true,
        test: {
          name: "node",
          environment: "node",
          include: ["src/**/*.test.ts"],
          exclude: [...configDefaults.exclude, ...domTsTests],
        },
      },
    ],
  },
});
