import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { expect, it } from "vitest";

it("generated token files match tokens.json", () => {
  const script = fileURLToPath(new URL("../../scripts/gen-tokens.mjs", import.meta.url));
  const r = spawnSync(process.execPath, [script, "--check"], { encoding: "utf8" });
  expect(r.stderr).toBe("");
  expect(r.status).toBe(0);
});

it("every colour has its own dark value, except the allowlisted", () => {
  const { colors } = JSON.parse(readFileSync(new URL("./tokens.json", import.meta.url), "utf8"));
  const sameInBoth = new Set(["warning"]);
  const missing = Object.entries<{ light: string; dark?: string }>(colors)
    .filter(([name, c]) => !sameInBoth.has(name) && (c.dark === undefined || c.dark === c.light))
    .map(([name]) => name);
  expect(missing).toEqual([]);
});
