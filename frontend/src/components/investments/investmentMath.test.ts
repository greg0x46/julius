import { describe, expect, it } from "vitest";

import type { InvestmentPosition } from "../../api/contracts";
import { positionYield } from "./investmentMath";

const position: InvestmentPosition = {
  id: "p1",
  source: "manual",
  account_id: "a1",
  asset_id: "asset-1",
  portfolio_id: null,
  name: "Bitcoin",
  ticker: "BTC",
  asset_type: "Criptomoeda",
  quantity: "3.2",
  average_cost: "10000.00",
  current_value: "44180.21",
  current_unit_price: "13806.32",
  valued_on: "2026-09-30",
  currency_code: "BRL",
  closed: false,
  linked_investment_id: null,
  notes: null,
  valuation_basis: "market_quote",
};

describe("positionYield", () => {
  it("states the gain of a position valued at a market quote like one valued by hand", () => {
    const quoted = positionYield(position, new Map());
    const typed = positionYield({ ...position, valuation_basis: "manual_valuation" }, new Map());

    expect(quoted.known).toBe(true);
    expect(quoted).toEqual(typed);
    if (!quoted.known) return;
    // 44.180,21 over 3,2 × 10.000,00.
    expect(quoted.basis).toBe("32000.000");
    expect(quoted.value).toBe("12180.210");
    expect(quoted.percent).toBeCloseTo(38.0631, 3);
  });

  it("deducts the position's costs from a market-quoted gain", () => {
    const estimate = positionYield(position, new Map(), "180.21");
    expect(estimate.known && estimate.value).toBe("12000.000");
  });

  it("subtracts fractional acquisition cost before rounding gain for display", () => {
    const estimate = positionYield({ ...position, average_cost: "0.331", quantity: "3", current_value: "1.00" }, new Map(), "0.003");
    expect(estimate.known && estimate.basis).toBe("0.993");
    expect(estimate.known && estimate.value).toBe("0.004");
  });

  it("does not invent a gain for a position still valued at cost", () => {
    const estimate = positionYield({ ...position, valuation_basis: "cost_basis" }, new Map());
    expect(estimate.known).toBe(false);
    if (estimate.known) return;
    expect(estimate.short).toBe("Sem cotação registrada");
  });
});
