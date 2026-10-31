import { describe, expect, it } from "vitest";

import { formatAxisMoney } from "./chartAxis";

describe("formatAxisMoney", () => {
  it("rounds to thousands and millions without a currency prefix", () => {
    expect(formatAxisMoney(0)).toBe("0");
    expect(formatAxisMoney(850)).toBe("850");
    expect(formatAxisMoney(18_638)).toBe("19 mil");
    expect(formatAxisMoney(1_500_000)).toBe("1,5 mi");
  });

  it("keeps a decimal below 10 mil so neighbouring ticks stay distinct", () => {
    expect(formatAxisMoney(1_500)).toBe("1,5 mil");
    expect(formatAxisMoney(2_000)).toBe("2 mil");
    expect(formatAxisMoney(-2_500)).toBe("-2,5 mil");
    expect(formatAxisMoney(1_000)).toBe("1 mil");
  });

  it("switches to millions where thousands would round to 1000 mil", () => {
    expect(formatAxisMoney(999_499)).toBe("999 mil");
    expect(formatAxisMoney(999_500)).toBe("1 mi");
    expect(formatAxisMoney(1_000_000)).toBe("1 mi");
    expect(formatAxisMoney(-999_600)).toBe("-1 mi");
  });

  it("never prints -0", () => {
    expect(formatAxisMoney(-0.4)).toBe("0");
    expect(formatAxisMoney(-0)).toBe("0");
  });

  it("keeps the sign of a negative balance", () => {
    expect(formatAxisMoney(-500_000)).toBe("-500 mil");
    expect(formatAxisMoney(-1_200_000)).toBe("-1,2 mi");
  });
});

describe("formatAxisMoney in English", () => {
  it("uses k/M suffixes and a decimal point", () => {
    expect(formatAxisMoney(2_000, "en")).toBe("2k");
    expect(formatAxisMoney(1_500, "en")).toBe("1.5k");
    expect(formatAxisMoney(-1_500_000, "en")).toBe("-1.5M");
  });
});
