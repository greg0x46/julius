import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

export default function setup() {
  const directory = mkdtempSync(join(tmpdir(), "julius-e2e-build-"));
  try {
    execFileSync("npm", ["run", "build"], { stdio: "inherit" });
    const binary = join(directory, "server");
    execFileSync("go", ["build", "-o", binary, "./internal/e2etestserver"], {
      cwd: resolve(".."), stdio: "inherit",
    });
    process.env.JULIUS_E2E_SERVER = binary;
    return () => rmSync(directory, { recursive: true, force: true });
  } catch (error) {
    rmSync(directory, { recursive: true, force: true });
    throw error;
  }
}
