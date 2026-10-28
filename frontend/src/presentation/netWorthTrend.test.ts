import { describe, expect, it } from "vitest";

import type { NetWorthPoint } from "../api/contracts";
import { netWorthTrend, sparklinePoints } from "./netWorthTrend";

function point(day: string, total: string, overrides: Partial<NetWorthPoint> = {}): NetWorthPoint {
  return {
    date: day,
    valuation_status: "complete",
    cash_balance: total,
    investment_balance: "0.00",
    total,
    ...overrides,
  };
}

describe("netWorthTrend", () => {
  it("measures the exact change from the oldest valued day inside the window", () => {
    const series = [
      point("2026-08-01", "100.00"),
      point("2026-09-25", "9007199254740993.00"),
      point("2026-10-10", "9007199254740994.50"),
      point("2026-10-22", "9007199254740995.50"),
    ];

    const trend = netWorthTrend(series, series[3]);

    expect(trend).toEqual({
      since: "2026-09-25",
      change: "2.50",
      values: ["9007199254740993.00", "9007199254740994.50", "9007199254740995.50"].map(Number),
    });
  });

  it("keeps a decline negative", () => {
    const series = [point("2026-10-01", "1000.00"), point("2026-10-22", "900.50")];

    expect(netWorthTrend(series, series[1])?.change).toBe("-99.50");
  });

  it("accepts a day valued with a stale rate and skips unavailable days", () => {
    const series = [
      point("2026-10-01", "0", { valuation_status: "unavailable", cash_balance: undefined, investment_balance: undefined, total: undefined }),
      point("2026-10-05", "60.00", { valuation_status: "stale" }),
      point("2026-10-22", "900.00"),
    ];

    expect(netWorthTrend(series, series[2])).toMatchObject({ since: "2026-10-05", change: "840.00" });
  });

  it("has no trend when the latest day itself has no figures", () => {
    const latest = point("2026-10-22", "0", { valuation_status: "unavailable", cash_balance: undefined, investment_balance: undefined, total: undefined });

    expect(netWorthTrend([point("2026-10-01", "100.00"), latest], latest)).toBeNull();
  });

  it("has no trend with a single valued day", () => {
    const series = [point("2026-10-22", "900.00")];

    expect(netWorthTrend(series, series[0])).toBeNull();
  });
});

describe("sparklinePoints", () => {
  it("scales the values into the box, highest at the top", () => {
    expect(sparklinePoints([0, 5, 10], 100, 20, 0)).toBe("0.0,20.0 50.0,10.0 100.0,0.0");
  });

  it("draws a flat series in the middle", () => {
    expect(sparklinePoints([7, 7], 100, 20, 0)).toBe("0.0,10.0 100.0,10.0");
  });

  it("draws nothing for no values", () => {
    expect(sparklinePoints([], 100, 20)).toBe("");
  });
});
