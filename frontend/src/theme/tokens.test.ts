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
  const sameInBoth = new Set(["warning", "appBar"]);
  const missing = Object.entries<{ light: string; dark?: string }>(colors)
    .filter(([name, c]) => !sameInBoth.has(name) && (c.dark === undefined || c.dark === c.light))
    .map(([name]) => name);
  expect(missing).toEqual([]);
});

it("native controls follow the selected theme, including overrides of the OS theme", () => {
  const css = readFileSync(new URL("../styles/tokens.css", import.meta.url), "utf8");
  expect(css).toMatch(/:root \{\s*color-scheme: light;/);
  expect(css).toMatch(
    /@media \(prefers-color-scheme: dark\) \{\s*:root:not\(\[data-theme="light"\]\) \{\s*color-scheme: dark;/,
  );
  expect(css).toMatch(/:root\[data-theme="dark"\] \{\s*color-scheme: dark;/);
});

it("the demo notice has readable text in both themes", () => {
  const css = readFileSync(new URL("../styles/global.css", import.meta.url), "utf8");
  const notice = css.match(/\.demo-notice\s*\{([^}]+)\}/)?.[1] ?? "";
  const { colors } = JSON.parse(readFileSync(new URL("./tokens.json", import.meta.url), "utf8"));
  const color = notice.match(/(?:^|;)\s*color:\s*var\((--[a-z-]+)\)/)?.[1];
  const background = notice.match(/(?:^|;)\s*background:\s*var\((--[a-z-]+)\)/)?.[1];
  const values = Object.values<{ var: string; light: string; dark: string }>(colors);
  const foregroundToken = values.find((token) => token.var === color);
  const backgroundToken = values.find((token) => token.var === background);
  expect(foregroundToken).toBeDefined();
  expect(backgroundToken).toBeDefined();
  const luminance = (hex: string) => {
    const channels = hex.slice(1).match(/../g)!.map((channel) => {
      const value = parseInt(channel, 16) / 255;
      return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
    });
    return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
  };
  for (const theme of ["light", "dark"] as const) {
    const text = luminance(foregroundToken![theme]);
    const surface = luminance(backgroundToken![theme]);
    expect((Math.max(text, surface) + 0.05) / (Math.min(text, surface) + 0.05)).toBeGreaterThanOrEqual(4.5);
  }
});
