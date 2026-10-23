import { expect, it } from "vitest";
import { execFileSync } from "node:child_process";
import { calendar } from "./calendar";
function inZone(zone: string, expression: string): unknown {
  return JSON.parse(
    execFileSync(
      process.execPath,
      [
        "--input-type=module",
        "-e",
        `import { calendar } from './src/presentation/calendar.ts'; console.log(JSON.stringify(${expression}));`,
      ],
      {
        cwd: process.cwd(),
        env: { ...process.env, TZ: zone },
        encoding: "utf8",
      },
    ),
  );
}
it("validates calendar days including leap years and early years", () => {
  expect(calendar("2024-02-29").isValid()).toBe(true);
  expect(calendar("2025-02-29").isValid()).toBe(false);
  expect(calendar("2026-04-31").isValid()).toBe(false);
  expect(calendar("0099-01-01").format("YYYY-MM-DD")).toBe("0099-01-01");
});
it("keeps date-only input local and reads instants in the browser zone", () => {
  expect(
    inZone(
      "America/Sao_Paulo",
      `[calendar("2026-09-15").format("DD/MM/YYYY"), calendar("2026-09-15T01:00:00Z").format("YYYY-MM-DD")]`,
    ),
  ).toEqual(["15/09/2026", "2026-09-14"]);
});
it("uses calendar arithmetic across month ends and daylight saving", () => {
  expect(
    inZone(
      "America/New_York",
      `(() => { const start=calendar("2026-03-08"); const next=start.add(1,"day"); return [next.diff(start,"day"),next.diff(start),start.format("YYYY-MM-DD")]; })()`,
    ),
  ).toEqual([1, 23 * 60 * 60 * 1000, "2026-03-08"]);
  expect(calendar("2024-02-04").endOf("month").format("YYYY-MM-DD")).toBe(
    "2024-02-29",
  );
  expect(calendar("2026-12-31").add(1, "day").format("YYYY-MM-DD")).toBe(
    "2027-01-01",
  );
  expect(calendar("0100-01-01").diff(calendar("0099-12-31"), "day")).toBe(1);
});
