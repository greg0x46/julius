import { decimal, isValidDate, requiredRecord, requiredRecordWithOptionalKeys } from "./shared";
import { parseValuation, type Valuation } from "./valuation";

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

export interface NetWorthSnapshot extends Partial<NetWorthBreakdown> {
  valuation?: Valuation | null;
  valuation_version?: number;
  valuation_status?: "complete" | "reconstructed_partial" | "unavailable" | "legacy_unknown";
  captured_at: string;
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
    ["captured_at", "is_backfilled"],
    [...netWorthBreakdownKeys, "valuation", "valuation_version", "valuation_status"],
    "Patrimônio líquido inválido.",
  );
  const status = snapshot.valuation_status ?? "legacy_unknown";
  if (!["complete", "reconstructed_partial", "unavailable", "legacy_unknown"].includes(status as string)) {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  const version = snapshot.valuation_version ?? 0;
  if (version !== 0 && version !== 1) throw new TypeError("Versão de avaliação inválida.");
  const valuation = snapshot.valuation == null ? null : parseValuation(snapshot.valuation);
  if ((status === "legacy_unknown" && (version !== 0 || valuation !== null)) ||
      (status !== "legacy_unknown" && (version !== 1 || valuation === null))) {
    throw new TypeError("Contexto de avaliação inválido.");
  }
  if (status === "unavailable" && netWorthBreakdownKeys.some((key) => key in snapshot)) {
    throw new TypeError("Avaliação indisponível contém valores.");
  }
  if (!isValidDate(snapshot.captured_at)) {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  if (typeof snapshot.is_backfilled !== "boolean") {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  return {
    captured_at: snapshot.captured_at,
    is_backfilled: snapshot.is_backfilled,
    valuation,
    valuation_version: version,
    valuation_status: status as NetWorthSnapshot["valuation_status"],
    ...(status === "unavailable" ? {} : parseNetWorthBreakdown(snapshot)),
  };
}

export function parseNetWorthSeries(value: unknown): NetWorthSeries {
  const response = requiredRecord(value, ["series", "latest"], "Patrimônio líquido inválido.");
  if (!Array.isArray(response.series)) {
    throw new TypeError("Patrimônio líquido inválido.");
  }
  return {
    series: response.series.map(parseNetWorthSnapshot),
    latest: parseNetWorthSnapshot(response.latest),
  };
}

export function hasNetWorthValue(snapshot: NetWorthSnapshot): snapshot is NetWorthSnapshot & NetWorthBreakdown {
  return (snapshot.valuation_status === "complete" || snapshot.valuation_status === "reconstructed_partial") &&
    snapshot.valuation?.reference_currency === "BRL" && netWorthBreakdownKeys.every((key) => typeof snapshot[key] === "string");
}
