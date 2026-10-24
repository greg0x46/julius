import { parseValuation, type ReportValuation } from "./valuation";
import { decimal, isValidDate, requiredRecord, requiredRecordWithOptionalKeys } from "./shared";

// NetWorthBreakdown mirrors internal/httpapi's netWorthBreakdownDTO. The
// product calls it "patrimônio": what has already been realized, cash plus
// investments. No card, debt or future bill enters it (see internal/networth).
export interface NetWorthBreakdown {
  cash_balance: string;
  investment_balance: string;
  total: string;
}

export const netWorthValuationStatuses = ["complete", "stale", "unavailable"] as const;
export type NetWorthValuationStatus = (typeof netWorthValuationStatuses)[number];

/** The rate behind one currency's conversion into the BRL base. */
export interface NetWorthValuationRate {
  value: string;
  inverted: boolean;
  effective_on: string;
  source: string;
  stale: boolean;
}

/** How a day was valued: the day, the policy and each rate used. */
export interface NetWorthValuation {
  base_currency: string;
  valued_at: string;
  rate_policy: string;
  rates: Record<string, NetWorthValuationRate>;
}

export interface NetWorthPoint extends Partial<NetWorthBreakdown>, ReportValuation {
  date: string;
  // "unavailable": a day that cannot be valued (no rate, no price, or a
  // movement that cannot be reversed); it carries no figures at all, never a zero.
  valuation_status: NetWorthValuationStatus;
  valuation?: NetWorthValuation;
}

/** A point that carries its figures (every status but "unavailable"). */
export function hasNetWorthValue(point: NetWorthPoint): point is NetWorthPoint & NetWorthBreakdown {
  return point.valuation_status !== "unavailable" && point.total !== undefined;
}

export interface NetWorthSeries {
  series: NetWorthPoint[];
  latest: NetWorthPoint;
}

function parseNetWorthBreakdown(value: Record<string, unknown>): NetWorthBreakdown {
  return {
    cash_balance: decimal(value.cash_balance),
    investment_balance: decimal(value.investment_balance),
    total: decimal(value.total),
  };
}

const netWorthBreakdownKeys = ["cash_balance", "investment_balance", "total"] as const;

function parseValuationRate(value: unknown): NetWorthValuationRate {
  const rate = requiredRecord(value, ["value", "inverted", "effective_on", "source", "stale"], "Patrimônio inválido.");
  if (typeof rate.inverted !== "boolean" || typeof rate.stale !== "boolean" || typeof rate.source !== "string" || !isValidDate(rate.effective_on)) {
    throw new TypeError("Patrimônio inválido.");
  }
  const parsed = decimal(rate.value);
  if (/^-|^0(?:\.0*)?$/.test(parsed)) throw new TypeError("Patrimônio inválido.");
  return { value: parsed, inverted: rate.inverted, effective_on: rate.effective_on, source: rate.source, stale: rate.stale };
}

function parseValuationMetadata(value: unknown): NetWorthValuation {
  const valuation = requiredRecord(value, ["base_currency", "valued_at", "rate_policy", "rates"], "Patrimônio inválido.");
  if (
    typeof valuation.base_currency !== "string" ||
    !/^[A-Z]{3}$/.test(valuation.base_currency) ||
    !isValidDate(valuation.valued_at) ||
    typeof valuation.rate_policy !== "string" ||
    typeof valuation.rates !== "object" ||
    valuation.rates === null ||
    Array.isArray(valuation.rates)
  ) {
    throw new TypeError("Patrimônio inválido.");
  }
  const rates: Record<string, NetWorthValuationRate> = {};
  for (const [currency, rate] of Object.entries(valuation.rates)) {
    if (!/^[A-Z]{3}$/.test(currency)) throw new TypeError("Patrimônio inválido.");
    rates[currency] = parseValuationRate(rate);
  }
  return { base_currency: valuation.base_currency, valued_at: valuation.valued_at, rate_policy: valuation.rate_policy, rates };
}

export function parseNetWorthPoint(value: unknown): NetWorthPoint {
  const point = requiredRecordWithOptionalKeys(
    value,
    ["date", "valuation_status"],
    ["currency_code", "fx", "valuation", ...netWorthBreakdownKeys],
    "Patrimônio inválido.",
  );
  if (!isValidDate(point.date)) {
    throw new TypeError("Patrimônio inválido.");
  }
  const status = point.valuation_status;
  if (!netWorthValuationStatuses.includes(status as NetWorthValuationStatus)) {
    throw new TypeError("Patrimônio inválido.");
  }
  // The figures are all there or all absent, and only an unavailable day omits them.
  const present = netWorthBreakdownKeys.filter((key) => point[key] !== undefined).length;
  if (present !== 0 && present !== netWorthBreakdownKeys.length) throw new TypeError("Patrimônio inválido.");
  if ((present === 0) !== (status === "unavailable")) throw new TypeError("Patrimônio inválido.");
  return {
    ...parseValuation(point),
    date: point.date,
    valuation_status: status as NetWorthValuationStatus,
    ...(point.valuation === undefined || point.valuation === null ? {} : { valuation: parseValuationMetadata(point.valuation) }),
    ...(present === 0 ? {} : parseNetWorthBreakdown(point)),
  };
}

export function parseNetWorthSeries(value: unknown): NetWorthSeries {
  const response = requiredRecord(value, ["series", "latest"], "Patrimônio inválido.");
  if (!Array.isArray(response.series)) {
    throw new TypeError("Patrimônio inválido.");
  }
  const latest = parseNetWorthPoint(response.latest);
  const series = response.series.map(parseNetWorthPoint);
  if (series.some((point) => (point.currency_code ?? "BRL") !== (latest.currency_code ?? "BRL")))
    throw new TypeError("Moedas do histórico incompatíveis.");
  return { series, latest };
}
