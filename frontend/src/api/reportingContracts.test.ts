import { describe, expect, it } from "vitest";
import { parseReportingSummary } from "./contracts/reporting";
import { parseCategoryBreakdown } from "./contracts/spendingByCategory";
import { parseProblem } from "./contracts/problems";

const missing = { currency_code: "BRL", status: "unavailable", eligible: 1, converted: 0, missing: 1, stale: 0, totals: null };
describe("reporting coverage contracts", () => {
  it("accepts a category report with no total and preserves raw totals", () => {
    const body = { month: "2026-03", classification: "outflow", currency_code: "BRL", total: null, items: [], reporting: missing, totals: [{ currency_code: "USD", inflow: "0", outflow: "10", balance: "-10" }] };
    expect(parseCategoryBreakdown(body)).toEqual(body);
  });
  it("rejects missing coverage masquerading as complete", () => {
    expect(() => parseReportingSummary({ ...missing, status: "complete" })).toThrow();
    expect(() => parseReportingSummary({ ...missing, converted: 1 })).toThrow();
    expect(() => parseReportingSummary({ ...missing, totals: { currency_code: "BRL", inflow: "0", outflow: "0", balance: "0" } })).toThrow();
  });
  it("retains conversion coverage on a timeline unavailable response", () => {
    const body = { type: "/problems/reporting-fx-unavailable", title: "Total indisponível", status: 503, reporting: missing };
    expect(parseProblem(body)).toEqual(body);
  });
});
