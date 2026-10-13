import { describe, expect, it } from "vitest";

import type { Investment, InvestmentTransaction } from "../api/contracts";
import { formatDecimal, movementTypeLabel, netContributed, quotaCount, yieldUnavailable } from "./investmentLabels";

function movement(overrides: Partial<InvestmentTransaction>): InvestmentTransaction {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    external_id: "ext",
    movement_type: "BUY",
    direction: "inflow",
    quantity: null,
    value: null,
    amount: "100.00",
    occurred_at: null,
    trade_date: null,
    ...overrides,
  };
}

function investment(overrides: Partial<Investment>): Investment {
  return {
    yield_value: null,
    yield_source: null,
    yield_unavailable_reason: null,
    ...overrides,
  } as Investment;
}

describe("netContributed", () => {
  // Regression: a dividend/JCP payout arrives as movement_type INTEREST but
  // leaves the investment. Classifying by movement type counted it as an
  // aporte, which inflated net contributed and turned gains into losses.
  it("subtracts a dividend payout instead of adding it", () => {
    expect(
      netContributed([
        movement({ movement_type: "BUY", direction: "inflow", amount: "800.00" }),
        movement({ movement_type: "INTEREST", direction: "outflow", amount: "20.00" }),
      ]),
    ).toBe("780.00");
  });

  it("nets aplicações against resgates", () => {
    expect(
      netContributed([
        movement({ direction: "inflow", amount: "1200.00" }),
        movement({ movement_type: "SELL", direction: "outflow", amount: "300.00" }),
      ]),
    ).toBe("900.00");
  });

  it("keeps fractional cents and large principal exact", () => {
    expect(netContributed([
      movement({ direction: "inflow", amount: "9007199254740993.005" }),
      movement({ direction: "outflow", amount: "9007199254740993.001" }),
    ])).toBe("0.004");
  });

  it("returns null when no aplicação was captured", () => {
    expect(netContributed([movement({ movement_type: "SELL", direction: "outflow" })])).toBeNull();
  });

  it("returns null when a movement has no established direction", () => {
    expect(
      netContributed([movement({ direction: "inflow" }), movement({ direction: null })]),
    ).toBeNull();
  });

  // Skipping an amountless movement and totalling the rest hides the hole it
  // leaves: the figure comes out confident and short.
  it("returns null when a movement has no amount", () => {
    expect(
      netContributed([movement({ amount: "1200.00" }), movement({ amount: null })]),
    ).toBeNull();
  });
});

describe("yieldUnavailable", () => {
  it("distinguishes a partial history from no history at all", () => {
    expect(yieldUnavailable(investment({ yield_unavailable_reason: "sem_historico" })).label).toBe(
      "Sem histórico",
    );
    expect(
      yieldUnavailable(investment({ yield_unavailable_reason: "historico_incompleto" })).label,
    ).toBe("Histórico incompleto");
  });

  // A complete history with no balance behind it is neither of the above, and
  // saying "Não disponível" next to a full movement list reads as a bug.
  it("names a missing balance rather than falling back to Não disponível", () => {
    const { label, hint } = yieldUnavailable(
      investment({ yield_unavailable_reason: "saldo_indisponivel" }),
    );
    expect(label).toBe("Sem saldo atual");
    expect(hint).not.toBe("");
  });
});

describe("pt-BR figures and provider labels", () => {
  it("writes decimals the pt-BR way, without trailing zeros", () => {
    expect(formatDecimal("10.65")).toBe("10,65");
    expect(formatDecimal("100.00")).toBe("100");
    expect(formatDecimal("0.0215")).toBe("0,0215");
    expect(formatDecimal("1234567.5")).toBe("1.234.567,5");
    expect(formatDecimal("-3.2")).toBe("-3,2");
  });

  it("makes the unit agree with the number of cotas", () => {
    expect(quotaCount("1")).toBe("1 cota");
    expect(quotaCount("0.2")).toBe("0,2 cotas");
    expect(quotaCount("20")).toBe("20 cotas");
  });

  it("never shows a raw provider enum", () => {
    expect(movementTypeLabel("OTHER_MOVEMENT")).toBe("Outra movimentação");
    expect(movementTypeLabel("SOMETHING_NEW")).toBe("Something new");
    expect(movementTypeLabel("Aplicação programada")).toBe("Aplicação programada");
  });
});
