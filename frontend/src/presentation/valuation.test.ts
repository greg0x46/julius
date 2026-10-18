import { describe, expect, it } from "vitest";
import { valueInReferenceCurrency } from "./valuation";
import { formatReportMoney } from "./money";
import type { FXRate } from "../api/contracts/valuation";

const fx: FXRate = {
  base: "BRL",
  quote: "USD",
  value: "0.2",
  inverted: false,
  as_of: "2026-10-12T00:00:00Z",
  effective_on: "2026-10-12T00:00:00Z",
  fetched_at: "2026-10-12T01:00:00Z",
  source: "synthetic",
  stale: false,
};

describe("reference valuation", () => {
  it("converts exact decimal aggregates in either stored rate orientation", () => {
    expect(valueInReferenceCurrency("1000.50", fx)).toBe("200.100");
    expect(
      valueInReferenceCurrency("-1000.50", {
        ...fx,
        value: "5",
        inverted: true,
      }),
    ).toBe("-200.100000000000");
    expect(valueInReferenceCurrency("999999999999999999.99", fx)).toBe("199999999999999999.998");
    expect(valueInReferenceCurrency("1", { ...fx, value: "3", inverted: true })).toBe("0.333333333333");
    expect(valueInReferenceCurrency("1", { ...fx, value: "0.000000000000123" })).toBe("0.000000000000123");
  });
  it("keeps BRL regression and formats the converted currency", () => {
    expect(valueInReferenceCurrency("1000.50")).toBe("1000.50");
    expect(formatReportMoney("200.100000000000", "USD")).toBe("USD\u00a0200,10");
    expect(formatReportMoney("1000.50", "BRL")).toBe("R$\u00a01.000,50");
  });
});
