import { describe, expect, it } from "vitest";
import { parseRecurrenceSuggestions, parseSuggestionReview, parseSuggestionResults } from "./recurrenceSuggestions";

const draft = { name: "Internet", kind: "expense", amount: "100.1234", category_id: "", account_id: null, cadence: "monthly", day_of_month: 5, month_of_year: null, start_date: "2026-12-05", end_date: null, is_active: true };
const suggestion = { key: "a".repeat(64), draft, currency_code: "BRL", amount_kind: "fixed", minimum_amount: "100.1234", maximum_amount: "100.1234", confidence: "high", reasons: ["category_required"], transaction_ids: ["synthetic"], card_number: "", evidence: [{ id: "synthetic", date: "2026-10-05", description: "Internet", amount: "-100.1234" }] };
describe("recurrence suggestion contracts", () => {
  it("preserves exact draft money and a missing category for owner review", () => {
    expect(parseRecurrenceSuggestions([suggestion])[0].draft).toEqual(draft);
    expect(parseRecurrenceSuggestions([])).toEqual([]);
  });
  it("rejects invalid schedule, currency, confidence and numeric money", () => {
    for (const changed of [{ ...suggestion, currency_code: "USD" }, { ...suggestion, confidence: "certain" }, { ...suggestion, minimum_amount: 100 }, { ...suggestion, draft: { ...draft, day_of_month: 32 } }]) expect(() => parseRecurrenceSuggestions([changed])).toThrow();
  });
  it("accepts nullable warning lists and validates preview conditions", () => {
    const preview = { token: "b".repeat(64), conditions: [{ field: "amount", operator: "within_percent", value: "100.1234:0" }], warnings: null, needs_review: false, history: [] };
    expect(parseSuggestionReview(preview).warnings).toEqual([]);
    expect(() => parseSuggestionReview({ ...preview, conditions: [{ field: "amount", operator: "contains", value: "100" }] })).toThrow();
  });
  it("retains per-item failure results", () => {
    const result = [{ key: suggestion.key, scenario_id: "", status: 409, error: "Revise novamente." }];
    expect(parseSuggestionResults(result)).toEqual(result);
    expect(() => parseSuggestionResults([{ ...result[0], status: "409" }])).toThrow();
  });
});
