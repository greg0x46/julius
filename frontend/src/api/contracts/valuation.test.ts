import { expect, it } from "vitest";
import { parseNetWorthSeries } from "./netWorth";
import { parseValuation } from "./valuation";

const snapshot = {
  date: "2026-10-12T00:00:00Z", valuation_status: "complete",
  cash_balance: "100", investment_balance: "0", total: "100",
};

it("reads wealth points as BRL and rejects a mixed valuation history", () => {
  expect(parseNetWorthSeries({ latest: snapshot, series: [snapshot] }).series).toHaveLength(1);
  expect(() => parseNetWorthSeries({ latest: { ...snapshot, currency_code: "USD" }, series: [snapshot] })).toThrow(/Moedas/);
});

it("rejects mismatched or unavailable FX rather than displaying an identity conversion", () => {
  const fx = { base: "BRL", quote: "USD", value: "0.2", inverted: false, source: "synthetic", stale: false,
    as_of: "2026-10-12T00:00:00Z", effective_on: "2026-10-12T00:00:00Z", fetched_at: "2026-10-12T01:00:00Z" };
  expect(parseValuation({ currency_code: "USD", fx }).fx?.source).toBe("synthetic");
  expect(() => parseValuation({ currency_code: "EUR", fx })).toThrow();
  expect(() => parseValuation({ currency_code: "USD", fx: { ...fx, value: "0" } })).toThrow();
});
