import { describe, expect, it } from "vitest";
import { parseSpendingByCategory } from "./contracts/spendingByCategory";

const legacy = { month: "2026-03", currency_code: "BRL", total: "50", items: [] };
const totals = { currency_code: "BRL", inflow: "0", outflow: "50", balance: "-50" };
const reporting = { currency_code: "BRL", status: "stale", eligible: 1, converted: 1, missing: 0, stale: 1, totals };

describe("spending report coverage contract", () => {
  it("retains stale coverage and original currency totals", () => {
    const body = { ...legacy, reporting, totals: [{ currency_code: "USD", inflow: "0", outflow: "10", balance: "-10" }] };
    expect(parseSpendingByCategory(body)).toEqual(body);
  });

  it("accepts existing BRL responses", () => {
    expect(parseSpendingByCategory(legacy)).toEqual(legacy);
  });

  it("rejects unavailable coverage paired with a numeric success total", () => {
    expect(() => parseSpendingByCategory({ ...legacy, reporting: { ...reporting, status: "unavailable", converted: 0, missing: 1, stale: 0, totals: null } })).toThrow();
  });

  it("validates raw totals and coverage", () => {
    expect(() => parseSpendingByCategory({ ...legacy, totals: [{ ...totals, outflow: "invalid" }] })).toThrow();
    expect(() => parseSpendingByCategory({ ...legacy, reporting: { ...reporting, status: "complete" } })).toThrow();
  });
});
