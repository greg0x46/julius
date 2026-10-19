import { describe, expect, it } from "vitest";
import type { NetWorthSnapshot } from "../../api/contracts";
import { investmentPeriodFixture, periodSynced } from "../../test/investmentWorkspaceFixtures";
import { changePercent } from "../../presentation/decimal";
import { reliablePortfolio, scalePoints, wealthPoints } from "./evolution";

const snapshot: NetWorthSnapshot = {
  captured_at: "2026-01-01", is_backfilled: false, valuation_version: 1, valuation_status: "complete",
  net_worth: "1000000", cash_balance: "10", investment_balance: "999990", total_assets: "1000000",
  total_liabilities: "0", credit_card_balance: "0", payables_debt: "0",
};

describe("evolution scales", () => {
  const points = [
    { date: "2026-01-01", values: { wealth: "1000000", cash: "10" } },
    { date: "2026-01-02", values: { wealth: "1100000", cash: "-5" } },
  ];
  it("keeps vastly different magnitudes on the common absolute scale, including negatives", () => {
    expect(scalePoints(points, ["wealth", "cash"], "absolute").data).toEqual([
      { date: "2026-01-01", wealth: 1000000, cash: 10 }, { date: "2026-01-02", wealth: 1100000, cash: -5 },
    ]);
  });
  it("normalizes from the same date and retains declines through zero", () => {
    expect(scalePoints(points, ["wealth", "cash"], "percent")).toMatchObject({ invalid: [], data: [
      { wealth: 0, cash: 0 }, { wealth: 10, cash: -150 },
    ] });
  });
  it.each([null, "0", "-1"])("rejects missing, zero and negative baseline %s", (baseline) => {
    expect(scalePoints([{ date: "2026-01-01", values: { cash: baseline } }, points[1]], ["cash"], "percent").invalid).toEqual(["cash"]);
  });
  it("does not use different dates for non-overlapping coverage", () => {
    expect(scalePoints([{ date: "2026-01-01", values: { a: "1", b: null } }, { date: "2026-01-02", values: { a: null, b: "2" } }], ["a", "b"], "percent").invalid).toEqual(["b"]);
  });
  it("computes changes before converting exact decimal strings to chart coordinates", () => {
    expect(changePercent("9007199254740993.02", "9007199254740993.01")).toBe("0.0000");
    expect(changePercent("0.000000000000000002", "0.000000000000000001")).toBe("100.0000");
  });
});

describe("historical integrity", () => {
  it("does not turn backfilled investment zero into a wealth jump, nor bridge missing days", () => {
    const points = wealthPoints([
      { ...snapshot, is_backfilled: true, valuation_status: "reconstructed", net_worth: "10", investment_balance: "0" },
      { ...snapshot, captured_at: "2026-01-03" },
      { ...snapshot, captured_at: "2026-01-04", valuation_status: "legacy" },
      { ...snapshot, captured_at: "2026-01-05", valuation_status: "unavailable" },
    ], "2026-01-01", "2026-01-05");
    expect(scalePoints(points, ["netWorth"], "absolute").data.map((point) => point.netWorth)).toEqual([null, null, 1000000, null, null]);
    expect(scalePoints(points, ["netWorth"], "percent").invalid).toEqual(["netWorth"]);
    expect(points.every((point) => point.values.investments == null)).toBe(true);
  });
  it("uses portfolio valuation rather than contributions; incomplete coverage stays unavailable", () => {
    const period = investmentPeriodFixture({ from: "2026-01-01", to: "2026-01-31" });
    period.totals.partial = false;
    period.totals.positions_partial_start = 0;
    period.groups = [{ ...period.groups[0], positions: [{ ...periodSynced, history_complete: true, partial_start: false }] }];
    expect(reliablePortfolio(period)).toBe(true);
    const history = { baseline: { ...period, to: "2026-01-01" }, periods: [period] };
    const points = wealthPoints([snapshot], "2026-01-01", "2026-01-31", history);
    expect(points.at(-1)!.values.investments).toBe(period.totals.end_value);
    expect(points[1].values.investments).toBeUndefined();
    period.totals.partial = true;
    expect(wealthPoints([snapshot], "2026-01-01", "2026-01-31", history).at(-1)!.values.investments).toBeNull();
  });
  it("preserves source-specific stale currency qualifications", () => {
    const points = wealthPoints([{ ...snapshot, valuation_status: "stale" }], "2026-01-01", "2026-01-01");
    expect(points[0].notes).toContain("Patrimônio e saldo: câmbio desatualizado");
    expect(points[0].values.netWorth).toBe("1000000");
  });
});
