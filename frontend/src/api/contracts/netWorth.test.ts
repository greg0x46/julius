import { describe, expect, it } from "vitest";

import { hasNetWorthValue, parseNetWorthPoint, parseNetWorthSeries } from "./netWorth";

const figures = { cash_balance: "2500", investment_balance: "0", total: "2500" };
const valuation = {
  base_currency: "BRL", valued_at: "2026-10-12T00:00:00Z", rate_policy: "exchange_on_valued_day_v1",
  rates: { USD: { value: "5", inverted: false, effective_on: "2026-10-12T00:00:00Z", source: "synthetic", stale: false } },
};
const complete = { date: "2026-10-12T00:00:00Z", valuation_status: "complete", valuation, ...figures };

describe("net worth point contract", () => {
  it("keeps the rates a valuation used", () => {
    const parsed = parseNetWorthPoint(complete);
    expect(hasNetWorthValue(parsed)).toBe(true);
    expect(parsed.valuation?.rates.USD).toMatchObject({ value: "5", source: "synthetic", stale: false });
  });

  it("reads an unavailable day as a gap, not as zero", () => {
    const { cash_balance: _c, investment_balance: _i, total: _t, ...bare } = complete;
    void [_c, _i, _t];
    const parsed = parseNetWorthPoint({ ...bare, valuation_status: "unavailable" });
    expect(hasNetWorthValue(parsed)).toBe(false);
    expect(parsed.total).toBeUndefined();
    expect(parseNetWorthSeries({ latest: complete, series: [parsed, complete] }).series).toHaveLength(2);
  });

  it("rejects figures that disagree with the status", () => {
    expect(() => parseNetWorthPoint({ ...complete, valuation_status: "unavailable" })).toThrow();
    const { total: _t, ...partial } = complete;
    void _t;
    expect(() => parseNetWorthPoint(partial)).toThrow();
    expect(() => parseNetWorthPoint({ ...complete, valuation_status: "guessed" })).toThrow();
    expect(() => parseNetWorthPoint({ ...complete, valuation_status: "legacy" })).toThrow();
    expect(() => parseNetWorthPoint({ ...complete, valuation: { ...valuation, rates: { USD: { ...valuation.rates.USD, value: "0" } } } })).toThrow();
  });

  it("no longer accepts liabilities, which are not wealth", () => {
    expect(() => parseNetWorthPoint({ ...complete, credit_card_balance: "10" })).toThrow();
  });
});
