import { describe, expect, it } from "vitest";
import { periodNavigation } from "./periodNavigation";

describe("periodNavigation", () => {
  it.each([
    ["2024-01-01", "2024-01-31", "2024-02-01", "2024-02-29"],
    ["2025-12-01", "2025-12-31", "2026-01-01", "2026-01-31"],
    ["2024-01-01", "2024-12-31", "2025-01-01", "2025-12-31"],
    ["2026-03-07", "2026-03-10", "2026-03-11", "2026-03-14"],
    ["2026-12-31", "2026-12-31", "2027-01-01", "2027-01-01"],
    ["2026-01-01", "2026-03-31", "2026-04-01", "2026-06-30"],
  ])("advances %s–%s and returns without drift", (from, to, nextFrom, nextTo) => {
    expect(periodNavigation(from, to)?.next).toEqual([nextFrom, nextTo]);
    expect(periodNavigation(nextFrom, nextTo)?.previous).toEqual([from, to]);
  });

  it("localizes period labels without changing the date range", () => {
    const period = periodNavigation("2024-01-01", "2024-01-31", "en");
    expect(period?.label).toBe("January 2024");
    expect(period?.previousLabel).toBe("Previous month");
    expect(period?.nextLabel).toBe("Next month");
    expect(period?.next).toEqual(["2024-02-01", "2024-02-29"]);
  });

  it("rejects incomplete, invalid and reversed dates and respects date limits", () => {
    expect(periodNavigation(null, "2026-01-01")).toBeNull();
    expect(periodNavigation("2026-02-30", "2026-03-01")).toBeNull();
    expect(periodNavigation("2026-03-02", "2026-03-01")).toBeNull();
    expect(periodNavigation("0001-01-01", "0001-12-31")?.previous).toBeNull();
    expect(periodNavigation("9999-01-01", "9999-12-31")?.next).toBeNull();
  });
});
