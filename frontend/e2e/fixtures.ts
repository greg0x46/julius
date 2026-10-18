import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { createInterface } from "node:readline";

export const credentials = { email: "owner@example.test", password: "synthetic-e2e-password" };

export const test = base.extend({
  baseURL: async ({ browserName }, provide, testInfo) => {
    void browserName;
    const directory = await mkdtemp(join(tmpdir(), "julius-e2e-db-"));
    const server = spawn(process.env.JULIUS_E2E_SERVER!, [
      "-db", join(directory, "test.db"), "-frontend", resolve("dist"),
    ], { stdio: ["ignore", "pipe", "pipe"] });
    let logs = "";
    server.stderr.on("data", (chunk) => { logs += chunk.toString(); });
    const exited = once(server, "exit");
    const lines = createInterface({ input: server.stdout });
    try {
      const origin = await Promise.race([
        once(lines, "line").then(([line]) => String(line)),
        exited.then(([code]) => { throw new Error(`E2E server exited (${code}): ${logs}`); }),
        once(server, "error").then(([error]) => { throw error; }),
      ]);
      await provide(origin);
    } finally {
      lines.close();
      if (server.exitCode === null && server.signalCode === null) server.kill("SIGTERM");
      await exited;
      const path = testInfo.outputPath("server.log");
      await writeFile(path, logs);
      await testInfo.attach("server log", { path, contentType: "text/plain" });
      await rm(directory, { recursive: true, force: true });
    }
  },
  page: async ({ page, baseURL }, provide, testInfo) => {
    const logs: string[] = [];
    page.on("console", (message) => logs.push(`${message.type()}: ${message.text()}`));
    page.on("pageerror", (error) => logs.push(`pageerror: ${error.message}`));
    await page.route("**/*", (route) => {
      const url = new URL(route.request().url());
      return url.origin === baseURL ? route.continue() : route.abort();
    });
    try { await provide(page); }
    finally {
      await testInfo.attach("browser log", { body: logs.join("\n"), contentType: "text/plain" });
    }
  },
});

export { expect };
