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

export interface NetWorthSnapshot extends NetWorthBreakdown, ReportValuation {
  captured_at: string;
  stored_currency?: string;
  // Mirrors internal/httpapi's netWorthSnapshotDTO.IsBackfilled: true when
  // this row was reconstructed from past transactions rather than captured
  // live, in which case investment_balance is always "0" — investment
  // history can't be reconstructed (see internal/networth's Backfill doc
  // comment).
  is_backfilled: boolean;
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

export function parseNetWorthSnapshot(value: unknown): NetWorthSnapshot {
  const snapshot = requiredRecordWithOptionalKeys(
    value,
    ["captured_at", "is_backfilled", ...netWorthBreakdownKeys],
    ["currency_code", "fx", "stored_currency"],
    "Patrimônio líquido inválido.",
  );
  if (!isValidDate(snapshot.captured_at)) {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  if (typeof snapshot.is_backfilled !== "boolean") {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  return {
    ...parseValuation(snapshot),
    ...(typeof snapshot.stored_currency === "string" ? { stored_currency: snapshot.stored_currency } : {}),
    captured_at: snapshot.captured_at,
    is_backfilled: snapshot.is_backfilled,
    ...parseNetWorthBreakdown(snapshot),
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
