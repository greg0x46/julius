import { describe, expect, it } from "vitest";

import vectors from "../../../testdata/money_precision.json";
import { roundDecimal } from "./decimal";

import { clampBRLAtZero, divideBRLFloor, minBRL, formatBRL, formatMoney, formatToneBRL, moneyTone, subtractBRL, sumBRL } from "./money";

describe("divideBRLFloor", () => {
  it("drops the leftover fraction of a cent", () => {
    expect(divideBRLFloor("10.00", 3)).toBe("3.33");
    expect(divideBRLFloor("0.05", 2)).toBe("0.02");
    expect(divideBRLFloor("1234567.89", 1)).toBe("1234567.89");
  });

  it("splits zero, signed or not, into zero", () => {
    expect(divideBRLFloor("0.00", 7)).toBe("0.00");
    expect(divideBRLFloor("-0.00", 7)).toBe("0.00");
  });

  it("rejects a negative amount instead of rounding it toward zero", () => {
    expect(() => divideBRLFloor("-10.00", 3)).toThrow(RangeError);
    expect(() => divideBRLFloor("-0.01", 2)).toThrow(RangeError);
  });

  it("rejects a non-positive or fractional divisor", () => {
    expect(() => divideBRLFloor("10.00", 0)).toThrow(RangeError);
    expect(() => divideBRLFloor("10.00", -2)).toThrow(RangeError);
    expect(() => divideBRLFloor("10.00", 1.5)).toThrow(RangeError);
  });
});

describe("clampBRLAtZero", () => {
  it("keeps a positive amount and zeroes a negative one", () => {
    expect(clampBRLAtZero("12.34")).toBe("12.34");
    expect(clampBRLAtZero("-12.34")).toBe("0.00");
    expect(clampBRLAtZero("-0.01")).toBe("0.00");
  });

  it("reads negative zero as zero", () => {
    expect(clampBRLAtZero("-0.00")).toBe("0.00");
    expect(clampBRLAtZero("-0")).toBe("0.00");
    expect(clampBRLAtZero("0.00")).toBe("0.00");
  });
});

describe("minBRL", () => {
  it("picks the smallest amount, negatives included", () => {
    expect(minBRL(["3.00", "1.50", "2.25"])).toBe("1.50");
    expect(minBRL(["3.00", "-1.50", "0.00"])).toBe("-1.50");
    expect(minBRL(["7.00"])).toBe("7.00");
  });

  it("needs at least one amount", () => {
    expect(() => minBRL([])).toThrow(RangeError);
  });
});

describe("shared monetary precision policy", () => {
  it.each(vectors.rounding)("rounds $value to $rounded", ({ value, rounded }) => {
    expect(roundDecimal(value, 2)).toBe(rounded);
    const [whole, fraction] = rounded.replace(/^-/, "").split(".");
    expect(formatBRL(value)).toBe(`${rounded.startsWith("-") ? "-" : ""}R$\u00a0${whole.replace(/\B(?=(\d{3})+(?!\d))/g, ".")},${fraction}`);
  });

  it.each(vectors.sums)("sums $values exactly", ({ values, total }) => {
    expect(sumBRL(values)).toBe(total);
  });

  it.each(vectors.shares)("floors $value / $divisor", ({ value, divisor, share }) => {
    expect(divideBRLFloor(value, divisor)).toBe(share);
  });

  it("preserves precision in subtraction, minimum and clamping", () => {
    expect(subtractBRL("9007199254740993.005", "9007199254740993.004")).toBe("0.001");
    expect(minBRL(["0.005", "0.004"])).toBe("0.004");
    expect(clampBRLAtZero("0.004")).toBe("0.004");
    expect(() => divideBRLFloor("-0.001", 2)).toThrow(RangeError);
  });

  it.each(["BRL", "USD", "EUR", "JPY", "KWD"])("normalizes signed zero in %s", (code) => {
    expect(formatMoney("-0.00", code)).not.toMatch(/^-/);
  });

  it("keeps foreign currency precision without imposing minor-unit rules", () => {
    expect(formatMoney("-9007199254740993.005", "USD")).toBe("-USD\u00a09.007.199.254.740.993,005");
    expect(formatMoney("1.005", "KWD")).toBe("KWD\u00a01,005");
    expect(formatMoney("100", "JPY")).toBe("JPY\u00a0100,00");
  });

  it("does not sign or tint a BRL value that displays as zero", () => {
    expect(formatToneBRL("-0.004", "balance")).toBe("R$\u00a00,00");
    expect(moneyTone("0.004", "flow", "inflow")).toEqual({ sign: "", color: "neutral" });
  });
});
