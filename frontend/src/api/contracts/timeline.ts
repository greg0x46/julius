import { parseValuation, type ReportValuation } from "./valuation";
import { dateOnlyPattern, decimal, isUuid, requiredRecord, requiredRecordWithOptionalKeys } from "./shared";

export const certaintyTiers = ["realizado", "confirmado", "projetado", "hipotetico"] as const;

export type CertaintyTier = (typeof certaintyTiers)[number];

export const timelineSourceKinds = [
  "real",
  "investment",
  "recorrente",
  "plano_pagamento",
  "cenario",
] as const;

export type TimelineSourceKind = (typeof timelineSourceKinds)[number];

export interface TimelineEntry {
  date: string;
  description: string;
  amount: string;
  /**
   * The parcel of `amount` that counts as income/expense in the reports. It
   * equals `amount` for an ordinary entry; for a transfer to or from an
   * investment it is what is left after the transferred parcel is taken out.
   */
  reportable_amount: string;
  category_id: string | null;
  category_name: string;
  tier: CertaintyTier;
  source: TimelineSourceKind;
  source_ref_id: string;
  scenario_id: string | null;
}

export interface TimelineDayPoint {
  date: string;
  balance: string;
  inflow: string;
  outflow: string;
  lowest_tier: CertaintyTier;
}

export interface TimelineSeries {
  points: TimelineDayPoint[];
  entries: TimelineEntry[];
  starting_balance: string;
  lowest_balance: TimelineDayPoint;
  first_negative: string | null;
}

export interface ScenarioImpact {
  scenario_id: string;
  scenario_name: string;
  delta: string;
}

/** Income/expense/result over the whole window — see PeriodTotals in internal/timeline/aggregate.go. */
export interface PeriodTotals {
  income: string;
  expense: string;
  result: string;
}

export interface TimelineResponse extends ReportValuation {
  base: TimelineSeries;
  period_totals: PeriodTotals;
  simulation: TimelineSeries | null;
  scenario_impacts: ScenarioImpact[];
}

export interface TimelineParams {
  /** Balance anchor — always today. Never the month being browsed. */
  referenceDate: string;
  from: string;
  to: string;
  accountIds?: string[];
  categoryIds?: string[];
  cardNumbers?: string[];
  scenarioIds?: string[];
}

function isNullableCategoryId(value: unknown): value is string | null {
  return value === null || (typeof value === "string" && isUuid(value));
}

function parseTimelineEntry(value: unknown): TimelineEntry {
  const entry = requiredRecordWithOptionalKeys(
    value,
    [
      "date",
      "description",
      "amount",
      "category_id",
      "category_name",
      "tier",
      "source",
      "source_ref_id",
      "scenario_id",
    ],
    ["reportable_amount"],
    "Entrada da linha do tempo inválida.",
  );
  if (
    !dateOnlyPattern.test(entry.date as string) ||
    typeof entry.description !== "string" ||
    !isNullableCategoryId(entry.category_id) ||
    typeof entry.category_name !== "string" ||
    !certaintyTiers.includes(entry.tier as CertaintyTier) ||
    !timelineSourceKinds.includes(entry.source as TimelineSourceKind) ||
    typeof entry.source_ref_id !== "string" ||
    entry.source_ref_id === "" ||
    !isNullableCategoryId(entry.scenario_id)
  ) {
    throw new TypeError("Entrada da linha do tempo inválida.");
  }
  const amount = decimal(entry.amount);
  return {
    date: entry.date as string,
    description: entry.description,
    amount,
    // Servers and cached responses that predate the investment allocation
    // report nothing transferred, so the whole entry is reportable.
    reportable_amount:
      entry.reportable_amount === undefined ? amount : decimal(entry.reportable_amount),
    category_id: entry.category_id as string | null,
    category_name: entry.category_name,
    tier: entry.tier as CertaintyTier,
    source: entry.source as TimelineSourceKind,
    source_ref_id: entry.source_ref_id,
    scenario_id: entry.scenario_id as string | null,
  };
}

function parseTimelineDayPoint(value: unknown): TimelineDayPoint {
  const point = requiredRecord(
    value,
    ["date", "balance", "inflow", "outflow", "lowest_tier"],
    "Ponto da linha do tempo inválido.",
  );
  if (
    !dateOnlyPattern.test(point.date as string) ||
    !certaintyTiers.includes(point.lowest_tier as CertaintyTier)
  ) {
    throw new TypeError("Ponto da linha do tempo inválido.");
  }
  return {
    date: point.date as string,
    balance: decimal(point.balance),
    inflow: decimal(point.inflow),
    outflow: decimal(point.outflow),
    lowest_tier: point.lowest_tier as CertaintyTier,
  };
}

function parseTimelineSeries(value: unknown): TimelineSeries {
  const series = requiredRecord(
    value,
    ["points", "entries", "starting_balance", "lowest_balance", "first_negative"],
    "Série da linha do tempo inválida.",
  );
  if (
    !Array.isArray(series.points) ||
    !Array.isArray(series.entries) ||
    !(series.first_negative === null || dateOnlyPattern.test(series.first_negative as string))
  ) {
    throw new TypeError("Série da linha do tempo inválida.");
  }
  return {
    points: series.points.map(parseTimelineDayPoint),
    entries: series.entries.map(parseTimelineEntry),
    starting_balance: decimal(series.starting_balance),
    lowest_balance: parseTimelineDayPoint(series.lowest_balance),
    first_negative: series.first_negative as string | null,
  };
}

function parsePeriodTotals(value: unknown): PeriodTotals {
  const totals = requiredRecord(value, ["income", "expense", "result"], "Totais do período inválidos.");
  return {
    income: decimal(totals.income),
    expense: decimal(totals.expense),
    result: decimal(totals.result),
  };
}

function parseScenarioImpact(value: unknown): ScenarioImpact {
  const impact = requiredRecord(
    value,
    ["scenario_id", "scenario_name", "delta"],
    "Impacto de cenário inválido.",
  );
  if (
    typeof impact.scenario_id !== "string" ||
    !isUuid(impact.scenario_id) ||
    typeof impact.scenario_name !== "string"
  ) {
    throw new TypeError("Impacto de cenário inválido.");
  }
  return {
    scenario_id: impact.scenario_id,
    scenario_name: impact.scenario_name,
    delta: decimal(impact.delta),
  };
}

export function parseTimelineResponse(value: unknown): TimelineResponse {
  const response = requiredRecordWithOptionalKeys(
    value,
    ["base", "period_totals", "simulation", "scenario_impacts"],
    ["currency_code", "fx"],
    "Resposta da linha do tempo inválida.",
  );
  if (!Array.isArray(response.scenario_impacts)) {
    throw new TypeError("Resposta da linha do tempo inválida.");
  }
  return {
    ...parseValuation(response),
    base: parseTimelineSeries(response.base),
    period_totals: parsePeriodTotals(response.period_totals),
    simulation: response.simulation === null ? null : parseTimelineSeries(response.simulation),
    scenario_impacts: response.scenario_impacts.map(parseScenarioImpact),
  };
}

/**
 * The widest window the balance curve can cover, from internal/timeline's
 * LoadDataRange: `from` is the oldest transaction on record, `to` the last
 * planned installment (or this month's end when there is none).
 */
export interface TimelineDataRange {
  from: string;
  to: string;
}

export function parseTimelineDataRange(value: unknown): TimelineDataRange {
  const range = requiredRecord(value, ["from", "to"], "Intervalo da linha do tempo inválido.");
  if (!dateOnlyPattern.test(range.from as string) || !dateOnlyPattern.test(range.to as string)) {
    throw new TypeError("Intervalo da linha do tempo inválido.");
  }
  return { from: range.from as string, to: range.to as string };
}
