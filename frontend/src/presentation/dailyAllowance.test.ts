import { describe, expect, it } from "vitest";

import { dailyAllowance } from "./dailyAllowance";

// One point per day, today first.
const days = (...balances: string[]) => balances.map((balance) => ({ balance }));
// Cash now, high enough not to cap the cases that are not about it.
const rich = "1000000.00";

describe("dailyAllowance", () => {
  it("splits a flat balance over the days, flooring to the cent", () => {
    expect(dailyAllowance(days("1000.00", "1000.00", "1000.00"), rich)).toBe("333.33");
    expect(dailyAllowance(days("855.00", "855.00", "855.00", "855.00", "855.00", "855.00", "855.00", "855.00", "855.00", "855.00"), rich)).toBe("85.50");
  });

  it("weighs a low point by how early it falls, not by the days left in the month", () => {
    // Low of 100 on the second of 31 days: only two days can draw on it, so
    // 50 a day, not 100 / 31.
    const flat = Array.from({ length: 29 }, () => "5000.00");
    expect(dailyAllowance(days("5000.00", "100.00", ...flat), rich)).toBe("50.00");
    // The same low on the last day is spread over every day.
    expect(dailyAllowance(days(...Array.from({ length: 30 }, () => "5000.00"), "310.00"), rich)).toBe("10.00");
  });

  it("is capped by an early dip even when the end of the month is rich", () => {
    expect(dailyAllowance(days("90.00", "10000.00", "10000.00"), rich)).toBe("90.00");
  });

  it("is zero when any day ends at zero or below", () => {
    expect(dailyAllowance(days("100.00", "0.00", "500.00"), rich)).toBe("0.00");
    expect(dailyAllowance(days("100.00", "-200.00", "500.00"), rich)).toBe("0.00");
    expect(dailyAllowance(days("-0.00"), rich)).toBe("0.00");
  });

  it("is the whole balance on a window of one day", () => {
    expect(dailyAllowance(days("123.45"), rich)).toBe("123.45");
  });

  it("does not count a future inflow as spendable", () => {
    // 1000 in cash; a 5000 salary lands on day 3. Nothing is due, so the
    // surplus stays out and the 1000 is what four days can draw on.
    expect(dailyAllowance(days("1000.00", "1000.00", "6000.00", "6000.00"), "1000.00")).toBe("250.00");
  });

  it("lets a future inflow cover the outflows due before and after it", () => {
    // 3000 in cash; rent of 2000 on day 2, salary of 5000 on day 3. Day 2 has
    // 1000 left; from day 3 on the cash is back to 3000, the surplus is ignored.
    expect(dailyAllowance(days("3000.00", "1000.00", "6000.00", "6000.00"), "3000.00")).toBe("500.00");
  });

  it("still counts an outflow no inflow has covered", () => {
    expect(dailyAllowance(days("3000.00", "1000.00", "1000.00"), "3000.00")).toBe("333.33");
  });

  it("is zero without points", () => {
    expect(dailyAllowance([], rich)).toBe("0.00");
  });
});
