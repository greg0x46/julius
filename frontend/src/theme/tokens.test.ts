import { spawnSync } from "node:child_process";
import { expect, it } from "vitest";

it("generated token files match tokens.json", () => {
  const r = spawnSync(process.execPath, ["scripts/gen-tokens.mjs", "--check"], {
    encoding: "utf8",
  });
  expect(r.stderr).toBe("");
  expect(r.status).toBe(0);
});
