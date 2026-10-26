import { describe, expect, it } from "vitest";

import { formatToneBRL, formatToneMoney, moneyTone } from "./money";

describe("moneyTone", () => {
  it("flow: inflow is tinted and signed, outflow keeps the text colour", () => {
    expect(moneyTone("10.00", "flow", "inflow")).toEqual({ sign: "+", color: "positive" });
    expect(moneyTone("10.00", "flow", "outflow")).toEqual({ sign: "-", color: "neutral" });
    expect(moneyTone("10.00", "flow", "unclassified")).toEqual({ sign: "", color: "neutral" });
    expect(moneyTone("-10.00", "flow")).toEqual({ sign: "-", color: "neutral" });
  });

  it("result: positive is money-in, negative keeps the text colour, zero is plain", () => {
    expect(moneyTone("5.00", "result")).toEqual({ sign: "+", color: "positive" });
    expect(moneyTone("-5.00", "result")).toEqual({ sign: "-", color: "neutral" });
    expect(moneyTone("-0.00", "result")).toEqual({ sign: "", color: "neutral" });
  });

  it("balance: only a negative balance gets colour", () => {
    expect(moneyTone("5.00", "balance")).toEqual({ sign: "", color: "neutral" });
    expect(moneyTone("-5.00", "balance")).toEqual({ sign: "-", color: "negative" });
  });
});

describe("formatToneBRL", () => {
  it.each([
    ["1234.5", "flow", "inflow", "+R$ 1.234,50"],
    ["1234.5", "flow", "outflow", "-R$ 1.234,50"],
    ["1234.5", "flow", "unclassified", "R$ 1.234,50"],
    ["-8.00", "result", undefined, "-R$ 8,00"],
    ["8.00", "result", undefined, "+R$ 8,00"],
    ["-8.00", "balance", undefined, "-R$ 8,00"],
    ["8.00", "balance", undefined, "R$ 8,00"],
    ["-8.00", "neutral", undefined, "-R$ 8,00"],
    ["-0.00", "balance", undefined, "R$ 0,00"],
  ] as const)("%s as %s/%s -> %s", (value, kind, direction, expected) => {
    expect(formatToneBRL(value, kind, direction)).toBe(expected);
  });
});

describe("currency-aware tones", () => {
  it.each(["BRL", "USD", "EUR"])("preserves signs and tones in %s", (currency) => {
    const prefix = currency === "BRL" ? "R$" : currency;
    expect(formatToneMoney("1234.50", currency, "flow", "inflow")).toBe(`+${prefix}\u00a01.234,50`);
    expect(formatToneMoney("1234.50", currency, "flow", "outflow")).toBe(`-${prefix}\u00a01.234,50`);
    expect(formatToneMoney("-8", currency, "result")).toBe(`-${prefix}\u00a08,00`);
    expect(formatToneMoney("-0.00", currency, "balance")).toBe(`${prefix}\u00a00,00`);
  });
});
