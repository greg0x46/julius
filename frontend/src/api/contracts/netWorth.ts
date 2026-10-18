import { parseValuation, type ReportValuation } from "./valuation";
import { decimal, isValidDate, requiredRecord, requiredRecordWithOptionalKeys } from "./shared";

// NetWorthBreakdown mirrors internal/httpapi's netWorthBreakdownDTO: the
// four underlying balances plus the three totals derived from them
// (assets/liabilities/net worth), all pre-computed server-side so the
// frontend never has to re-derive money math from parts. Receivables
// (money owed to the user) never enter this model — see internal/networth's
// Breakdown doc comment for why.
export interface NetWorthBreakdown {
  cash_balance: string;
  investment_balance: string;
  credit_card_balance: string;
  payables_debt: string;
  total_assets: string;
  total_liabilities: string;
  net_worth: string;
}

export const netWorthValuationStatuses = ["legacy", "complete", "stale", "reconstructed", "unavailable"] as const;
export type NetWorthValuationStatus = (typeof netWorthValuationStatuses)[number];

/** The rate behind one currency's conversion into the BRL base. */
export interface NetWorthValuationRate {
  value: string;
  inverted: boolean;
  effective_on: string;
  source: string;
  stale: boolean;
}

/** How a stored snapshot was valued: the day, the policy and each rate used. */
export interface NetWorthValuation {
  base_currency: string;
  valued_at: string;
  rate_policy: string;
  rates: Record<string, NetWorthValuationRate>;
}

export interface NetWorthSnapshot extends Partial<NetWorthBreakdown>, ReportValuation {
  captured_at: string;
  stored_currency?: string;
  // 0 / "legacy": written before foreign balances were valued, so it covers
  // BRL accounts and holdings only. "unavailable": a past day that cannot be
  // valued; it carries no figures at all (never a zero).
  valuation_version: number;
  valuation_status: NetWorthValuationStatus;
  valuation?: NetWorthValuation;
  // Mirrors internal/httpapi's netWorthSnapshotDTO.IsBackfilled: true when
  // this row was reconstructed from past transactions rather than captured
  // live, in which case investment_balance is always "0" — investment
  // history can't be reconstructed (see internal/networth's Backfill doc
  // comment).
  is_backfilled: boolean;
}

/** A snapshot that carries its figures (every status but "unavailable"). */
export function hasNetWorthValue(snapshot: NetWorthSnapshot): snapshot is NetWorthSnapshot & NetWorthBreakdown {
  return snapshot.valuation_status !== "unavailable" && snapshot.net_worth !== undefined;
}

export interface NetWorthSeries {
  series: NetWorthSnapshot[];
  latest: NetWorthSnapshot;
}

function parseNetWorthBreakdown(value: Record<string, unknown>): NetWorthBreakdown {
  return {
    cash_balance: decimal(value.cash_balance),
    investment_balance: decimal(value.investment_balance),
    credit_card_balance: decimal(value.credit_card_balance),
    payables_debt: decimal(value.payables_debt),
    total_assets: decimal(value.total_assets),
    total_liabilities: decimal(value.total_liabilities),
    net_worth: decimal(value.net_worth),
  };
}

const netWorthBreakdownKeys = [
  "cash_balance",
  "investment_balance",
  "credit_card_balance",
  "payables_debt",
  "total_assets",
  "total_liabilities",
  "net_worth",
] as const;

function parseValuationRate(value: unknown): NetWorthValuationRate {
  const rate = requiredRecord(value, ["value", "inverted", "effective_on", "source", "stale"], "Patrimônio líquido inválido.");
  if (typeof rate.inverted !== "boolean" || typeof rate.stale !== "boolean" || typeof rate.source !== "string" || !isValidDate(rate.effective_on)) {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  const parsed = decimal(rate.value);
  if (/^-|^0(?:\.0*)?$/.test(parsed)) throw new TypeError("Patrimônio líquido inválido.");
  return { value: parsed, inverted: rate.inverted, effective_on: rate.effective_on, source: rate.source, stale: rate.stale };
}

function parseSnapshotValuation(value: unknown): NetWorthValuation {
  const valuation = requiredRecord(value, ["base_currency", "valued_at", "rate_policy", "rates"], "Patrimônio líquido inválido.");
  if (
    typeof valuation.base_currency !== "string" ||
    !/^[A-Z]{3}$/.test(valuation.base_currency) ||
    !isValidDate(valuation.valued_at) ||
    typeof valuation.rate_policy !== "string" ||
    typeof valuation.rates !== "object" ||
    valuation.rates === null ||
    Array.isArray(valuation.rates)
  ) {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  const rates: Record<string, NetWorthValuationRate> = {};
  for (const [currency, rate] of Object.entries(valuation.rates)) {
    if (!/^[A-Z]{3}$/.test(currency)) throw new TypeError("Patrimônio líquido inválido.");
    rates[currency] = parseValuationRate(rate);
  }
  return { base_currency: valuation.base_currency, valued_at: valuation.valued_at, rate_policy: valuation.rate_policy, rates };
}

export function parseNetWorthSnapshot(value: unknown): NetWorthSnapshot {
  const snapshot = requiredRecordWithOptionalKeys(
    value,
    ["captured_at", "is_backfilled", "valuation_version", "valuation_status"],
    ["currency_code", "fx", "stored_currency", "valuation", ...netWorthBreakdownKeys],
    "Patrimônio líquido inválido.",
  );
  if (!isValidDate(snapshot.captured_at)) {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  if (typeof snapshot.is_backfilled !== "boolean") {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  const status = snapshot.valuation_status;
  if (
    typeof snapshot.valuation_version !== "number" ||
    !Number.isInteger(snapshot.valuation_version) ||
    snapshot.valuation_version < 0 ||
    !netWorthValuationStatuses.includes(status as NetWorthValuationStatus)
  ) {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  // The figures are all there or all absent, and only an unavailable day omits them.
  const present = netWorthBreakdownKeys.filter((key) => snapshot[key] !== undefined).length;
  if (present !== 0 && present !== netWorthBreakdownKeys.length) throw new TypeError("Patrimônio líquido inválido.");
  if ((present === 0) !== (status === "unavailable")) throw new TypeError("Patrimônio líquido inválido.");
  return {
    ...parseValuation(snapshot),
    ...(typeof snapshot.stored_currency === "string" ? { stored_currency: snapshot.stored_currency } : {}),
    captured_at: snapshot.captured_at,
    is_backfilled: snapshot.is_backfilled,
    valuation_version: snapshot.valuation_version,
    valuation_status: status as NetWorthValuationStatus,
    ...(snapshot.valuation === undefined || snapshot.valuation === null ? {} : { valuation: parseSnapshotValuation(snapshot.valuation) }),
    ...(present === 0 ? {} : parseNetWorthBreakdown(snapshot)),
  };
}

export function parseNetWorthSeries(value: unknown): NetWorthSeries {
  const response = requiredRecord(value, ["series", "latest"], "Patrimônio líquido inválido.");
  if (!Array.isArray(response.series)) {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  const latest = parseNetWorthSnapshot(response.latest);
  const series = response.series.map(parseNetWorthSnapshot);
  if (series.some((snapshot) => (snapshot.currency_code ?? "BRL") !== (latest.currency_code ?? "BRL")))
    throw new TypeError("Moedas do histórico incompatíveis.");
  return { series, latest };
}
