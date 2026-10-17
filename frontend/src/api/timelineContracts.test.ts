import { describe, expect, it } from "vitest";

import { parseTimelineResponse } from "./contracts";

const validResponse = {
  base: {
    points: [
      { date: "2026-08-01", balance: "700.00", inflow: "0.00", outflow: "100.00", lowest_tier: "realizado" },
      { date: "2026-08-15", balance: "1000.00", inflow: "2000.00", outflow: "0.00", lowest_tier: "realizado" },
    ],
    entries: [
      {
        date: "2026-08-01",
        description: "Mercado",
        amount: "-100.00",
        category_id: "000433b6-3094-5a9c-87df-465b70574a4b",
        category_name: "Supermercado",
        tier: "realizado",
        source: "real",
        source_ref_id: "11111111-1111-4111-8111-111111111111",
        scenario_id: null,
      },
    ],
    starting_balance: "1000.00",
    lowest_balance: { date: "2026-08-15", balance: "1000.00", inflow: "0.00", outflow: "0.00", lowest_tier: "realizado" },
    first_negative: null,
  },
  period_totals: { income: "2000.00", expense: "100.00", result: "1900.00" },
  simulation: null,
  scenario_impacts: [],
};

describe("timeline contracts", () => {
  it("accepts a well-formed response", () => {
    // This fixture predates the investment reading, like a response from an
    // older server: the absent reportable_amount is filled from amount
    // instead of rejecting the payload.
    expect(parseTimelineResponse(validResponse)).toEqual({
      ...validResponse,
      base: {
        ...validResponse.base,
        entries: validResponse.base.entries.map((entry) => ({
          ...entry,
          reportable_amount: entry.amount,
        })),
      },
    });
  });

  it("accepts a null first_negative and scenario_id", () => {
    expect(() => parseTimelineResponse(validResponse)).not.toThrow();
  });

  it.each([
    { ...validResponse, base: { ...validResponse.base, starting_balance: "not-a-number" } },
    { ...validResponse, base: { ...validResponse.base, points: "not-an-array" } },
    {
      ...validResponse,
      base: {
        ...validResponse.base,
        entries: [{ ...validResponse.base.entries[0], tier: "unknown" }],
      },
    },
    { ...validResponse, scenario_impacts: "not-an-array" },
    { ...validResponse, period_totals: { income: "1.00", expense: "muito", result: "0" } },
  ])("rejects malformed responses", (payload) => {
    expect(() => parseTimelineResponse(payload)).toThrow();
  });
});

it("preserves balance conversion coverage separately from period reporting", () => {
  const balanceReporting = { currency_code: "BRL", status: "stale", eligible: 1, converted: 1, missing: 0, stale: 1, totals: { currency_code: "BRL", inflow: "500", outflow: "0", balance: "500" } };
  const parsed = parseTimelineResponse({ ...validResponse, base: { ...validResponse.base, balance_reporting: balanceReporting } });
  expect(parsed.base.balance_reporting).toEqual(balanceReporting);
  expect(parsed.period_totals.reporting).toBeUndefined();
});

it("sends the reporting timezone to the timeline API", async () => {
  const { getTimeline } = await import("./timeline");
  const { vi } = await import("vitest");
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(validResponse), { headers: { "Content-Type": "application/json" } }));
  vi.stubGlobal("fetch", fetchMock);
  try {
    await getTimeline({ referenceDate: "2026-08-15", from: "2026-08-01", to: "2026-08-31", timezone: "America/Sao_Paulo" });
    const query = new URL(fetchMock.mock.calls[0][0], "http://localhost").searchParams;
    expect(query.get("timezone")).toBe("America/Sao_Paulo");
  } finally {
    vi.unstubAllGlobals();
  }
});
