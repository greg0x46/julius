import { readdirSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const dir = new URL("./", import.meta.url);

// Allowed: keywords, 0, scale vars, and the 16px mobile input rule (stops iOS zoom).
const allowed = (prop: string, value: string): boolean => {
  if (value === "inherit" || value === "0") return true;
  if (prop === "font-size" && value === "16px") return true;
  if (/^clamp\(.*var\(--fs-[a-z]+\)\)$/.test(value)) return true;
  const rest = value.replace(/var\(--[a-z-]+\)/g, "").replace(/[\s0]/g, "");
  return rest === "" && value.includes("var(");
};

describe("type and radius scales", () => {
  const files = readdirSync(dir).filter((f) => f.endsWith(".css") && f !== "tokens.css");

  it.each(files)("%s uses tokens for font-size and border-radius", (file) => {
    const css = readFileSync(new URL(file, dir), "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    const bad: string[] = [];
    for (const m of css.matchAll(/(?:^|[;{\s])(font-size|border-radius)\s*:\s*([^;}]+)/g)) {
      const value = m[2].trim();
      if (!allowed(m[1], value)) bad.push(`${m[1]}: ${value}`);
    }
    expect(bad).toEqual([]);
  });
});
