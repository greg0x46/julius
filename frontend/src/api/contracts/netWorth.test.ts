import { describe, expect, it } from "vitest";

import { hasNetWorthValue, parseNetWorthSeries, parseNetWorthSnapshot } from "./netWorth";

const figures = {
  cash_balance: "2500", investment_balance: "0", credit_card_balance: "0", payables_debt: "0",
  total_assets: "2500", total_liabilities: "0", net_worth: "2500",
};
const valuation = {
  base_currency: "BRL", valued_at: "2026-10-12T00:00:00Z", rate_policy: "exchange_on_valued_day_v1",
  rates: { USD: { value: "5", inverted: false, effective_on: "2026-10-12T00:00:00Z", source: "synthetic", stale: false } },
};
const complete = {
  captured_at: "2026-10-12T00:00:00Z", is_backfilled: false, valuation_version: 1, valuation_status: "complete",
  valuation, ...figures,
};

describe("net worth snapshot contract", () => {
  it("keeps the rates a valuation used", () => {
    const parsed = parseNetWorthSnapshot(complete);
    expect(hasNetWorthValue(parsed)).toBe(true);
    expect(parsed.valuation?.rates.USD).toMatchObject({ value: "5", source: "synthetic", stale: false });
  });

  it("reads a legacy row without inventing a valuation", () => {
    const parsed = parseNetWorthSnapshot({ ...complete, valuation_version: 0, valuation_status: "legacy", valuation: undefined });
    expect(parsed.valuation).toBeUndefined();
    expect(parsed.valuation_status).toBe("legacy");
  });

  it("reads an unavailable day as a gap, not as zero", () => {
    const { cash_balance: _c, investment_balance: _i, credit_card_balance: _cc, payables_debt: _p, total_assets: _a,
      total_liabilities: _l, net_worth: _n, ...bare } = complete;
    void [_c, _i, _cc, _p, _a, _l, _n];
    const parsed = parseNetWorthSnapshot({ ...bare, valuation_status: "unavailable" });
    expect(hasNetWorthValue(parsed)).toBe(false);
    expect(parsed.net_worth).toBeUndefined();
    expect(parseNetWorthSeries({ latest: complete, series: [parsed, complete] }).series).toHaveLength(2);
  });

  it("rejects figures that disagree with the status", () => {
    expect(() => parseNetWorthSnapshot({ ...complete, valuation_status: "unavailable" })).toThrow();
    const { net_worth: _n, ...partial } = complete;
    void _n;
    expect(() => parseNetWorthSnapshot(partial)).toThrow();
    expect(() => parseNetWorthSnapshot({ ...complete, valuation_status: "guessed" })).toThrow();
    expect(() => parseNetWorthSnapshot({ ...complete, valuation: { ...valuation, rates: { USD: { ...valuation.rates.USD, value: "0" } } } })).toThrow();
  });
});
