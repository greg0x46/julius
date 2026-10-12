import { describe, expect, it } from "vitest";

import { clampBRLAtZero, divideBRLFloor, minBRL } from "./money";

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
