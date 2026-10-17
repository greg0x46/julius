import fixture from "../../../internal/httpapi/testdata/financial-contracts.json";
import { describe, expect, expectTypeOf, it } from "vitest";

import {
  parseAccountList,
  parseAccountBillList,
  parseAccountCardList,
  parseInvestmentList,
  parseInvestmentAccountList,
  parseInvestmentPositionList,
  parseInvestmentOperationList,
  parseInvestmentSummary,
  parseNetWorthSeries,
  parseTimelineResponse,
  parseTransactionQueryResult,
} from "./contracts";
import { decimal } from "./contracts/shared";

// Go's TestFinancialContracts pins this file to the actual response DTOs.
const payloads: Record<string, unknown> = fixture;

const contracts = [
  ["accounts", parseAccountList],
  ["cards", parseAccountCardList],
  ["bills", parseAccountBillList],
  ["transactions", parseTransactionQueryResult],
  ["investments", parseInvestmentList],
  ["investment_accounts", parseInvestmentAccountList],
  ["positions", parseInvestmentPositionList],
  ["operations", parseInvestmentOperationList],
  ["investment_summary", parseInvestmentSummary],
  ["timeline", parseTimelineResponse],
  ["timeline_simulation", parseTimelineResponse],
  ["net_worth", parseNetWorthSeries],
] as const;

function unwrap(value: unknown): unknown {
  return value !== null && typeof value === "object" && Object.keys(value).length === 1 && "items" in value
    ? value.items
    : value;
}

describe("shared Go/TypeScript financial contracts", () => {
  it.each(contracts)("parses %s without losing wire fields or precision", (name, parse) => {
    expect(parse(payloads[name])).toEqual(unwrap(payloads[name]));
  });

  it("round-trips exact money through JSON without number conversion", () => {
    const values = payloads.money as string[];
    expect(values).toContain("9007199254740993.123456789012345678");
    expect(values).toContain("0.000000000000000001");
    for (const value of values) {
      const parsed = decimal(JSON.parse(JSON.stringify(value)));
      expect(parsed).toBe(value);
      expect(decimal(JSON.parse(JSON.stringify(parsed)))).toBe(value);
    }
  });

  it.each([1.23, null, "1e3", "NaN", "Infinity", "1,23", " 1.23 "])(
    "rejects non-contract money %s",
    (value) => expect(() => decimal(value)).toThrow(),
  );

  it("preserves backend pending/posted eligibility and unknown nullable money", () => {
    const result = parseTransactionQueryResult(payloads.transactions);
    const [pending, posted, processing, unknown, ignored] = result.items;
    expect(pending.provider_status).toBe("PENDING");
    expect(posted.provider_status).toBe("POSTED");
    expect(pending.totals_eligibility).toEqual({ included: true, reason: null });
    expect(posted.totals_eligibility).toEqual({ included: true, reason: null });
    expect(processing.totals_eligibility).toEqual({ included: false, reason: "ineligible_status" });
    expect(unknown).toMatchObject({
      occurred_at: null, provider_status: null, amount: null,
      currency_code: null, effective_money: null, reportable_amount: null,
      internal_category: null, card: null,
    });
    expect(ignored.inclusion).toMatchObject({ state: "ignored", origin: "rule" });
    expect(ignored.totals_eligibility).toEqual({ included: false, reason: "ignored" });
    expectTypeOf(pending.amount).toEqualTypeOf<string | null>();
    expectTypeOf(pending.totals_eligibility.included).toEqualTypeOf<boolean>();
    expectTypeOf(pending.classification).toEqualTypeOf<"inflow" | "outflow" | "unclassified">();
  });

  it("rejects incompatible transaction representations", () => {
    const result = parseTransactionQueryResult(payloads.transactions);
    for (const patch of [
      { amount: 123.45 },
      { occurred_at: "not-a-date" },
      { classification: "DEBIT" },
      { totals_eligibility: { included: true, reason: "ineligible_status" } },
      { inclusion: { state: "POSTED", changed_at: null, origin: "manual", rule_name: null } },
    ]) {
      expect(() => parseTransactionQueryResult({ ...result, items: [{ ...result.items[0], ...patch }] })).toThrow();
    }
    const { amount: _amount, ...missingNullableField } = result.items[0];
    void _amount;
    expect(() => parseTransactionQueryResult({ ...result, items: [missingNullableField] })).toThrow();
  });

  it("keeps calendar days distinct from instants and preserves simulation nullability", () => {
    const timeline = parseTimelineResponse(payloads.timeline);
    expect(timeline.base.points[0].date).toBe("2026-07-30");
    expect(timeline.simulation).toBeNull();
    expect(timeline.base.first_negative).toBeNull();
    expect(parseTimelineResponse(payloads.timeline_simulation).simulation?.first_negative).toBe("2026-07-30");
    const netWorth = parseNetWorthSeries(payloads.net_worth);
    expect(netWorth.latest.captured_at).toBe("2026-07-30T12:34:56.123456789Z");
    expectTypeOf(netWorth.latest.net_worth).toEqualTypeOf<string | undefined>();
    // An unvaluable day omits every figure instead of reporting zero.
    expect(netWorth.series[0]).toMatchObject({ valuation_status: "unavailable" });
    expect(netWorth.series[0].net_worth).toBeUndefined();
    expect(netWorth.latest.valuation_status).toBe("complete");
    const positions = parseInvestmentPositionList(payloads.positions);
    expect(positions[0].valued_on).toBe("2026-07-30");
    expect(positions.at(-1)?.average_cost).toBeNull();
    expectTypeOf(positions[0].average_cost).toEqualTypeOf<string | null>();
    expect(positions[0].total_cost).toBe("9007199254740993.123456789012345678");
    expect(positions.at(-1)?.total_cost).toBeNull();
    expect(() => parseInvestmentPositionList({ items: [{ ...positions[0], total_cost: 100 }] })).toThrow();
  });
});
