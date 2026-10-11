import { describe, expect, it } from "vitest";
import { hasNetWorthValue, parseInvestmentSummary, parseNetWorthSnapshot, parseValuation } from "./contracts";

const valuation = {
  reference_currency: "BRL",
  valued_at: "2020-01-02T12:00:00Z",
  rate_policy: "latest_observed_at_or_before_v1",
  rates: { USD: { value: "5", observed_at: "2020-01-02T00:00:00Z", source: "test" } },
};
const amounts = {
  cash_balance: "2500", investment_balance: "100", credit_card_balance: "50", payables_debt: "30",
  total_assets: "2600", total_liabilities: "80", net_worth: "2520",
};
const snapshot = { captured_at: "2020-01-02", is_backfilled: false, valuation, valuation_version: 1, valuation_status: "complete", ...amounts };

describe("reference currency valuation contracts", () => {
  it("preserves exact rate context and accepts valued snapshots", () => {
    const parsed = parseNetWorthSnapshot(snapshot);
    expect(parsed.valuation).toEqual(valuation);
    expect(hasNetWorthValue(parsed)).toBe(true);
    expect(parsed.net_worth).toBe("2520");
    expect(hasNetWorthValue(parseNetWorthSnapshot({ ...snapshot, is_backfilled: true, valuation_status: "reconstructed_partial" }))).toBe(true);
  });

  it("keeps unavailable history without fabricating zero amounts", () => {
    const parsed = parseNetWorthSnapshot({ captured_at: "2020-01-02", is_backfilled: true, valuation, valuation_version: 1, valuation_status: "unavailable" });
    expect(parsed.net_worth).toBeUndefined();
    expect(hasNetWorthValue(parsed)).toBe(false);
    expect(() => parseNetWorthSnapshot({ ...snapshot, valuation_status: "unavailable" })).toThrow();
  });

  it("retains legacy amounts without qualifying them as BRL", () => {
    for (const context of [{}, { valuation: null, valuation_version: 0, valuation_status: "legacy_unknown" }]) {
      const parsed = parseNetWorthSnapshot({ captured_at: "2020-01-02", is_backfilled: false, ...amounts, ...context });
      expect(parsed.net_worth).toBe("2520");
      expect(parsed.valuation_status).toBe("legacy_unknown");
      expect(hasNetWorthValue(parsed)).toBe(false);
    }
    expect(() => parseNetWorthSnapshot({ ...snapshot, valuation: null })).toThrow();
  });

  it("rejects future and invalid FX rates", () => {
    for (const rate of [
      { value: "0", observed_at: "2020-01-02T00:00:00Z", source: "test" },
      { value: "5", observed_at: "2020-01-03T00:00:00Z", source: "test" },
    ]) expect(() => parseValuation({ ...valuation, rates: { USD: rate } })).toThrow();
  });

  it("reads converted investment totals with their rate context", () => {
    const parsed = parseInvestmentSummary({ currency_code: "BRL", total_value: "10000", manual_value: "0", synced_value: "10000", cash_balance: "0", unrealized_gain: "0", portfolios: [], accounts: [], valuation });
    expect(parsed.synced_value).toBe("10000");
    expect(parsed.valuation).toEqual(valuation);
  });
});
