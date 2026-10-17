import { describe, expect, it } from "vitest";

import { dailyAllowance } from "./dailyAllowance";

// One point per day, today first.
const days = (...balances: string[]) => balances.map((balance) => ({ balance }));

describe("dailyAllowance", () => {
  it("splits a flat balance over the days, flooring to the cent", () => {
    expect(dailyAllowance(days("1000.00", "1000.00", "1000.00"))).toBe("333.33");
    expect(dailyAllowance(days("855.00", "855.00", "855.00", "855.00", "855.00", "855.00", "855.00", "855.00", "855.00", "855.00"))).toBe("85.50");
  });

  it("weighs a low point by how early it falls, not by the days left in the month", () => {
    // Low of 100 on the second of 31 days: only two days can draw on it, so
    // 50 a day, not 100 / 31.
    const flat = Array.from({ length: 29 }, () => "5000.00");
    expect(dailyAllowance(days("5000.00", "100.00", ...flat))).toBe("50.00");
    // The same low on the last day is spread over every day.
    expect(dailyAllowance(days(...Array.from({ length: 30 }, () => "5000.00"), "310.00"))).toBe("10.00");
  });

  it("is capped by an early dip even when the end of the month is rich", () => {
    expect(dailyAllowance(days("90.00", "10000.00", "10000.00"))).toBe("90.00");
  });

  it("is zero when any day ends at zero or below", () => {
    expect(dailyAllowance(days("100.00", "0.00", "500.00"))).toBe("0.00");
    expect(dailyAllowance(days("100.00", "-200.00", "500.00"))).toBe("0.00");
    expect(dailyAllowance(days("-0.00"))).toBe("0.00");
  });

  it("is the whole balance on a window of one day", () => {
    expect(dailyAllowance(days("123.45"))).toBe("123.45");
  });

  it("is zero without points", () => {
    expect(dailyAllowance([])).toBe("0.00");
  });
});
