import { describe, expect, it } from "vitest";

import type { InvestmentPosition } from "../../api/contracts";
import { formatBRL } from "../../presentation/money";
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
  total_cost: "32000.00",
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
    expect(quoted.basis).toBe("32000.00");
    expect(quoted.value).toBe("12180.21");
    expect(quoted.percent).toBeCloseTo(38.0631, 3);
  });

  it("uses exact backend cost even when the displayed average would reconstruct another value", () => {
    const estimate = positionYield({
      ...position, quantity: "3", average_cost: "33.33", total_cost: "100.00", current_value: "110.00",
    }, new Map());
    expect(estimate).toMatchObject({ known: true, basis: "100.00", gross: "10.00", value: "10.00" });
  });

  it.each([undefined, null])("does not reconstruct an unavailable backend cost (%s)", (total_cost) => {
    const estimate = positionYield({ ...position, total_cost }, new Map());
    expect(estimate).toMatchObject({ known: false, short: "Sem custo de aquisição" });
  });

  it("keeps the proportional acquisition cost exact after a partial sale and rounds only for display", () => {
    const estimate = positionYield({
      ...position, quantity: "2", average_cost: "33.3333333333333334",
      total_cost: "66.6666666666666667", current_value: "80.00",
    }, new Map());
    expect(estimate).toMatchObject({ known: true, basis: "66.6666666666666667", gross: "13.3333333333333333", value: "13.3333333333333333" });
    expect(estimate.known && formatBRL(estimate.value)).toBe("R$\u00a013,33");
  });

  it("deducts the position's costs from a market-quoted gain", () => {
    const estimate = positionYield(position, new Map(), "180.21");
    expect(estimate.known && estimate.value).toBe("12000.00");
  });

  it("subtracts fractional acquisition cost before rounding gain for display", () => {
    const estimate = positionYield({ ...position, average_cost: "0.331", total_cost: "0.993", quantity: "3", current_value: "1.00" }, new Map(), "0.003");
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
