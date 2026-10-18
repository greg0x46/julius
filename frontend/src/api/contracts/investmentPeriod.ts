import {
  dateOnlyPattern,
  decimal,
  isCount,
  isNullableString,
  isUuid,
  nullableDecimal,
  requiredRecord,
} from "./shared";
import { parseValuation, type ReportValuation } from "./valuation";

// Period view of the investment book (GET /api/investment-portfolio) -----------
//
// The backend owns every figure here: start/end value, flows, yield, the
// Dietz rate, subtotals and shares. Money is a decimal string in the
// reference currency; `yield_rate` and `share` are ratio strings (0.05 = 5%).
// A figure the backend cannot state is null and its position says why in
// `unavailable_reason`; it must never be shown as zero.

export const investmentPeriodGroupings = ["none", "account", "class"] as const;
export type InvestmentPeriodGrouping = (typeof investmentPeriodGroupings)[number];

export const investmentPeriodSources = ["manual", "synced"] as const;
export type InvestmentPeriodSource = (typeof investmentPeriodSources)[number];

export const investmentPeriodReasons = [
  "sem_historico",
  "historico_incompleto",
  "saldo_indisponivel",
  "cambio_indisponivel",
] as const;
export type InvestmentPeriodReason = (typeof investmentPeriodReasons)[number];

export const investmentPeriodAssetClasses = [
  "fixed_income",
  "variable_income",
  "multimarket",
  "currency",
  "crypto",
  "other",
  "unclassified",
] as const;
export type InvestmentPeriodAssetClass = (typeof investmentPeriodAssetClasses)[number];

/** The part of the totals made of positions held (or closed) as of `to`; the two add up to the totals. */
export interface InvestmentPeriodPart {
  start_value: string;
  end_value: string;
  contributions: string;
  withdrawals: string;
  yield: string;
  capital_base: string;
  yield_rate: string | null;
}

export interface InvestmentPeriodTotals {
  held: InvestmentPeriodPart;
  closed: InvestmentPeriodPart;
  start_value: string;
  end_value: string;
  contributions: string;
  withdrawals: string;
  yield: string;
  capital_base: string;
  yield_rate: string | null;
  invested: string;
  invested_unknown_positions: number;
  costs: string;
  provisioned_taxes: string;
  positions_total: number;
  positions_unavailable: number;
  positions_partial_start: number;
  partial: boolean;
}

export interface InvestmentPeriodNative {
  start_value: string;
  end_value: string;
  contributions: string;
  withdrawals: string;
  yield: string;
  invested: string | null;
}

export interface InvestmentPeriodPosition {
  id: string;
  source: InvestmentPeriodSource;
  account_id: string;
  account_name: string;
  asset_id: string | null;
  name: string;
  ticker: string | null;
  asset_type: string;
  asset_class: InvestmentPeriodAssetClass;
  currency_code: string;
  quantity: string | null;
  closed_as_of_to: boolean;
  unavailable_reason: InvestmentPeriodReason | null;
  opening_on: string | null;
  partial_start: boolean;
  history_complete: boolean;
  start_value: string | null;
  end_value: string | null;
  contributions: string | null;
  withdrawals: string | null;
  yield: string | null;
  capital_base: string | null;
  yield_rate: string | null;
  share: string | null;
  native: InvestmentPeriodNative | null;
  invested: string | null;
  invested_unavailable_reason: InvestmentPeriodReason | null;
  costs: string | null;
  provisioned_taxes: string | null;
  reported_profit: string | null;
  reported_as_of: string | null;
}

export interface InvestmentPeriodGroup {
  key: string;
  label: string;
  account_id: string | null;
  asset_class: InvestmentPeriodAssetClass | null;
  share: string | null;
  totals: InvestmentPeriodTotals;
  positions: InvestmentPeriodPosition[];
}

export interface InvestmentPeriodRate {
  value: string;
  inverted: boolean;
  effective_on: string;
  source: string;
  stale: boolean;
}

export interface InvestmentPeriod extends Required<ReportValuation> {
  from: string | null;
  /** The effective last day: a future `to` is cut at today by the server. */
  to: string;
  to_clamped: boolean;
  group_by: InvestmentPeriodGrouping;
  source: InvestmentPeriodSource | null;
  account_id: string | null;
  native_rates: Record<string, InvestmentPeriodRate>;
  totals: InvestmentPeriodTotals;
  groups: InvestmentPeriodGroup[];
}

const invalid = () => new TypeError("Período de investimentos inválido.");

const partKeys = [
  "start_value",
  "end_value",
  "contributions",
  "withdrawals",
  "yield",
  "capital_base",
  "yield_rate",
] as const;

const totalsKeys = [
  "held",
  "closed",
  "start_value",
  "end_value",
  "contributions",
  "withdrawals",
  "yield",
  "capital_base",
  "yield_rate",
  "invested",
  "invested_unknown_positions",
  "costs",
  "provisioned_taxes",
  "positions_total",
  "positions_unavailable",
  "positions_partial_start",
  "partial",
] as const;

const nativeKeys = ["start_value", "end_value", "contributions", "withdrawals", "yield", "invested"] as const;

const positionKeys = [
  "id",
  "source",
  "account_id",
  "account_name",
  "asset_id",
  "name",
  "ticker",
  "asset_type",
  "asset_class",
  "currency_code",
  "quantity",
  "closed_as_of_to",
  "unavailable_reason",
  "opening_on",
  "partial_start",
  "history_complete",
  "start_value",
  "end_value",
  "contributions",
  "withdrawals",
  "yield",
  "capital_base",
  "yield_rate",
  "share",
  "native",
  "invested",
  "invested_unavailable_reason",
  "costs",
  "provisioned_taxes",
  "reported_profit",
  "reported_as_of",
] as const;

const groupKeys = ["key", "label", "account_id", "asset_class", "share", "totals", "positions"] as const;

const rateKeys = ["value", "inverted", "effective_on", "source", "stale"] as const;

const topKeys = [
  "currency_code",
  "fx",
  "from",
  "to",
  "to_clamped",
  "group_by",
  "source",
  "account_id",
  "native_rates",
  "totals",
  "groups",
] as const;

function oneOf<T extends string>(values: readonly T[], value: unknown): T {
  if (!values.some((candidate) => candidate === value)) throw invalid();
  return value as T;
}

function nullableOneOf<T extends string>(values: readonly T[], value: unknown): T | null {
  return value === null ? null : oneOf(values, value);
}

function day(value: unknown): string {
  if (typeof value !== "string" || !dateOnlyPattern.test(value)) throw invalid();
  return value;
}

function nullableDay(value: unknown): string | null {
  return value === null ? null : day(value);
}

function count(value: unknown): number {
  if (!isCount(value)) throw invalid();
  return value;
}

function boolean(value: unknown): boolean {
  if (typeof value !== "boolean") throw invalid();
  return value;
}

function text(value: unknown): string {
  if (typeof value !== "string" || value.trim() === "") throw invalid();
  return value;
}

function nullableText(value: unknown): string | null {
  if (!isNullableString(value)) throw invalid();
  return value;
}

/** Account ids are a uuid or `integrated:<uuid>` (see investmentWorkspace.ts). */
function accountId(value: unknown): string {
  if (typeof value !== "string") throw invalid();
  const bare = value.startsWith("integrated:") ? value.slice("integrated:".length) : value;
  if (!isUuid(bare)) throw invalid();
  return value;
}

function nullableAccountId(value: unknown): string | null {
  return value === null ? null : accountId(value);
}

function uuid(value: unknown): string {
  if (typeof value !== "string" || !isUuid(value)) throw invalid();
  return value;
}

function parsePart(value: unknown): InvestmentPeriodPart {
  const item = requiredRecord(value, partKeys, "Totais do período inválidos.");
  return {
    start_value: decimal(item.start_value),
    end_value: decimal(item.end_value),
    contributions: decimal(item.contributions),
    withdrawals: decimal(item.withdrawals),
    yield: decimal(item.yield),
    capital_base: decimal(item.capital_base),
    yield_rate: nullableDecimal(item.yield_rate),
  };
}

function parseTotals(value: unknown): InvestmentPeriodTotals {
  const item = requiredRecord(value, totalsKeys, "Totais do período inválidos.");
  return {
    held: parsePart(item.held),
    closed: parsePart(item.closed),
    start_value: decimal(item.start_value),
    end_value: decimal(item.end_value),
    contributions: decimal(item.contributions),
    withdrawals: decimal(item.withdrawals),
    yield: decimal(item.yield),
    capital_base: decimal(item.capital_base),
    yield_rate: nullableDecimal(item.yield_rate),
    invested: decimal(item.invested),
    invested_unknown_positions: count(item.invested_unknown_positions),
    costs: decimal(item.costs),
    provisioned_taxes: decimal(item.provisioned_taxes),
    positions_total: count(item.positions_total),
    positions_unavailable: count(item.positions_unavailable),
    positions_partial_start: count(item.positions_partial_start),
    partial: boolean(item.partial),
  };
}

function parseNative(value: unknown): InvestmentPeriodNative | null {
  if (value === null) return null;
  const item = requiredRecord(value, nativeKeys, "Valores nativos inválidos.");
  return {
    start_value: decimal(item.start_value),
    end_value: decimal(item.end_value),
    contributions: decimal(item.contributions),
    withdrawals: decimal(item.withdrawals),
    yield: decimal(item.yield),
    invested: nullableDecimal(item.invested),
  };
}

function parsePosition(value: unknown): InvestmentPeriodPosition {
  const item = requiredRecord(value, positionKeys, "Posição do período inválida.");
  if (typeof item.currency_code !== "string" || !/^[A-Z]{3}$/.test(item.currency_code)) throw invalid();
  if (typeof item.asset_type !== "string") throw invalid();
  const unavailable = nullableOneOf(investmentPeriodReasons, item.unavailable_reason);
  const position: InvestmentPeriodPosition = {
    id: uuid(item.id),
    source: oneOf(investmentPeriodSources, item.source),
    account_id: accountId(item.account_id),
    account_name: text(item.account_name),
    asset_id: item.asset_id === null ? null : uuid(item.asset_id),
    name: text(item.name),
    ticker: nullableText(item.ticker),
    asset_type: item.asset_type,
    asset_class: oneOf(investmentPeriodAssetClasses, item.asset_class),
    currency_code: item.currency_code,
    quantity: nullableDecimal(item.quantity),
    closed_as_of_to: boolean(item.closed_as_of_to),
    unavailable_reason: unavailable,
    opening_on: nullableDay(item.opening_on),
    partial_start: boolean(item.partial_start),
    history_complete: boolean(item.history_complete),
    start_value: nullableDecimal(item.start_value),
    end_value: nullableDecimal(item.end_value),
    contributions: nullableDecimal(item.contributions),
    withdrawals: nullableDecimal(item.withdrawals),
    yield: nullableDecimal(item.yield),
    capital_base: nullableDecimal(item.capital_base),
    yield_rate: nullableDecimal(item.yield_rate),
    share: nullableDecimal(item.share),
    native: parseNative(item.native),
    invested: nullableDecimal(item.invested),
    invested_unavailable_reason: nullableOneOf(investmentPeriodReasons, item.invested_unavailable_reason),
    costs: nullableDecimal(item.costs),
    provisioned_taxes: nullableDecimal(item.provisioned_taxes),
    reported_profit: nullableDecimal(item.reported_profit),
    reported_as_of: nullableDay(item.reported_as_of),
  };
  // An unavailable position carries no period figures; an available one carries all of them.
  const figures = [
    position.opening_on,
    position.start_value,
    position.end_value,
    position.contributions,
    position.withdrawals,
    position.yield,
    position.capital_base,
    position.native,
  ];
  if (unavailable === null ? figures.some((figure) => figure === null) : figures.some((figure) => figure !== null)) {
    throw invalid();
  }
  return position;
}

function parseGroup(value: unknown): InvestmentPeriodGroup {
  const item = requiredRecord(value, groupKeys, "Grupo do período inválido.");
  if (!Array.isArray(item.positions)) throw invalid();
  return {
    key: text(item.key),
    label: text(item.label),
    account_id: nullableAccountId(item.account_id),
    asset_class: nullableOneOf(investmentPeriodAssetClasses, item.asset_class),
    share: nullableDecimal(item.share),
    totals: parseTotals(item.totals),
    positions: item.positions.map(parsePosition),
  };
}

function parseRate(value: unknown): InvestmentPeriodRate {
  const item = requiredRecord(value, rateKeys, "Cotação inválida.");
  const rate = decimal(item.value);
  if (/^-|^0(?:\.0*)?$/.test(rate)) throw invalid();
  return {
    value: rate,
    inverted: boolean(item.inverted),
    effective_on: day(item.effective_on),
    source: text(item.source),
    stale: boolean(item.stale),
  };
}

export function parseInvestmentPeriod(value: unknown): InvestmentPeriod {
  const item = requiredRecord(value, topKeys, "Período de investimentos inválido.");
  const valuation = parseValuation(item);
  if (valuation.currency_code === undefined || valuation.fx === undefined) throw invalid();
  if (typeof item.native_rates !== "object" || item.native_rates === null || Array.isArray(item.native_rates)) {
    throw invalid();
  }
  if (!Array.isArray(item.groups)) throw invalid();
  const nativeRates: Record<string, InvestmentPeriodRate> = {};
  for (const [code, rate] of Object.entries(item.native_rates)) {
    if (!/^[A-Z]{3}$/.test(code)) throw invalid();
    nativeRates[code] = parseRate(rate);
  }
  return {
    currency_code: valuation.currency_code,
    fx: valuation.fx,
    from: nullableDay(item.from),
    to: day(item.to),
    to_clamped: boolean(item.to_clamped),
    group_by: oneOf(investmentPeriodGroupings, item.group_by),
    source: nullableOneOf(investmentPeriodSources, item.source),
    account_id: nullableAccountId(item.account_id),
    native_rates: nativeRates,
    totals: parseTotals(item.totals),
    groups: item.groups.map(parseGroup),
  };
}
