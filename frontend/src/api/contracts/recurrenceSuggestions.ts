import { parseRuleCondition, type RuleCondition } from "./automationRules";
import { parseRecurringCommitment, type RecurringCommitmentWrite } from "./recurringCommitments";
import { decimal, isRecord, requiredRecord } from "./shared";

export interface RecurrenceSuggestion {
  evidence: { id: string; date: string; description: string; amount: string }[];
  key: string;
  draft: RecurringCommitmentWrite;
  currency_code: "BRL";
  amount_kind: "fixed" | "variable";
  minimum_amount: string;
  maximum_amount: string;
  confidence: "high" | "medium";
  reasons: string[];
  transaction_ids: string[];
  card_number: string;
}
export interface SuggestionReview {
  token: string;
  conditions: RuleCondition[];
  warnings: string[];
  needs_review: boolean;
  history: {
    id: string; date: string; description: string; amount: string; currency_code: string;
    evidence: boolean; status: "matched" | "unmatched" | "ambiguous"; reasons: string[];
    failed_conditions: RuleCondition[]; rule_ids: string[]; scenario_ids: string[];
  }[];
}
export interface SuggestionApproval {
  key: string;
  draft: RecurringCommitmentWrite;
  conditions: RuleCondition[];
  token: string;
  acknowledge: boolean;
}
export interface SuggestionResult { key: string; scenario_id: string; status: number; error: string }

function strings(value: unknown): string[] {
  if (value === null) return [];
  if (!Array.isArray(value) || !value.every((v) => typeof v === "string")) throw new TypeError("Lista inválida.");
  return value;
}
function record(value: unknown) {
  if (!isRecord(value)) throw new TypeError("Sugestão inválida.");
  return value;
}
export function parseRecurrenceSuggestions(value: unknown): RecurrenceSuggestion[] {
  if (!Array.isArray(value)) throw new TypeError("Sugestões inválidas.");
  return value.map((v) => {
    const r = record(v);
    const draft = requiredRecord(r.draft, ["name", "kind", "amount", "category_id", "account_id", "cadence", "day_of_month", "month_of_year", "start_date", "end_date", "is_active"]);
    // Discovery permits an empty category; creation still uses the normal contract.
    const id = "11111111-1111-4111-8111-111111111111";
    const parsed = parseRecurringCommitment({ ...draft, category_id: draft.category_id || id, id, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" });
    if (typeof r.key !== "string" || !/^[a-f0-9]{64}$/.test(r.key) || r.currency_code !== "BRL" || !["fixed", "variable"].includes(r.amount_kind as string) || !["high", "medium"].includes(r.confidence as string) || typeof r.card_number !== "string" || typeof draft.category_id !== "string") throw new TypeError("Sugestão inválida.");
    if (!Array.isArray(r.evidence)) throw new TypeError("Evidência inválida.");
    const evidence = r.evidence.map((v) => { const e = record(v); if (typeof e.id !== "string" || typeof e.date !== "string" || typeof e.description !== "string") throw new TypeError("Evidência inválida."); return { id: e.id, date: e.date, description: e.description, amount: decimal(e.amount) }; });
    return { evidence, key: r.key, draft: { name: parsed.name, kind: parsed.kind, amount: parsed.amount, category_id: draft.category_id, account_id: parsed.account_id, cadence: parsed.cadence, day_of_month: parsed.day_of_month, month_of_year: parsed.month_of_year, start_date: parsed.start_date, end_date: parsed.end_date, is_active: parsed.is_active }, currency_code: "BRL", amount_kind: r.amount_kind as "fixed" | "variable", minimum_amount: decimal(r.minimum_amount), maximum_amount: decimal(r.maximum_amount), confidence: r.confidence as "high" | "medium", reasons: strings(r.reasons), transaction_ids: strings(r.transaction_ids), card_number: r.card_number };
  });
}
export function parseSuggestionReview(value: unknown): SuggestionReview {
  const r = record(value);
  if (typeof r.token !== "string" || !/^[a-f0-9]{64}$/.test(r.token) || typeof r.needs_review !== "boolean" || !Array.isArray(r.conditions) || !Array.isArray(r.history)) throw new TypeError("Prévia inválida.");
  return { token: r.token, conditions: r.conditions.map((v) => parseRuleCondition(v, true)), warnings: strings(r.warnings), needs_review: r.needs_review, history: r.history.map((v) => {
    const row = record(v);
    if (typeof row.id !== "string" || typeof row.date !== "string" || typeof row.description !== "string" || typeof row.currency_code !== "string" || typeof row.evidence !== "boolean" || !["matched", "unmatched", "ambiguous"].includes(row.status as string) || !Array.isArray(row.failed_conditions)) throw new TypeError("Histórico inválido.");
    return { id: row.id, date: row.date, description: row.description, currency_code: row.currency_code, amount: decimal(row.amount), evidence: row.evidence, status: row.status as "matched" | "unmatched" | "ambiguous", reasons: strings(row.reasons), failed_conditions: row.failed_conditions.map((c) => parseRuleCondition(c, true)), rule_ids: strings(row.rule_ids), scenario_ids: strings(row.scenario_ids) };
  }) };
}
export function parseSuggestionResults(value: unknown): SuggestionResult[] {
  if (!Array.isArray(value)) throw new TypeError("Resultado inválido.");
  return value.map((v) => {
    const r = record(v);
    if (typeof r.key !== "string" || typeof r.scenario_id !== "string" || typeof r.status !== "number" || typeof r.error !== "string") throw new TypeError("Resultado inválido.");
    return { key: r.key, scenario_id: r.scenario_id, status: r.status, error: r.error };
  });
}
