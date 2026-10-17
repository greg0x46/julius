import { isCount, parseCurrencyTotals, requiredRecord, type CurrencyTotals } from "./shared";

export interface ReportingSummary {
  currency_code: string;
  status: "complete" | "stale" | "incomplete" | "unavailable";
  eligible: number;
  converted: number;
  missing: number;
  stale: number;
  totals: CurrencyTotals | null;
}

export function parseReportingSummary(value: unknown): ReportingSummary {
  const item = requiredRecord(value, ["currency_code", "status", "eligible", "converted", "missing", "stale", "totals"]);
  if (typeof item.currency_code !== "string" || !/^[A-Z]{3}$/.test(item.currency_code) || !["complete", "stale", "incomplete", "unavailable"].includes(item.status as string) ||
    ![item.eligible, item.converted, item.missing, item.stale].every(isCount)) {
    throw new TypeError("Conversão do relatório inválida.");
  }
  const eligible = item.eligible as number, converted = item.converted as number, missing = item.missing as number, stale = item.stale as number;
  const totals = item.totals === null ? null : parseCurrencyTotals(item.totals);
  if (eligible !== converted + missing || stale > converted ||
    ((missing > 0) !== (totals === null)) ||
    item.status !== (missing > 0 ? converted > 0 ? "incomplete" : "unavailable" : stale > 0 ? "stale" : "complete") ||
    (totals && totals.currency_code !== item.currency_code)) throw new TypeError("Conversão do relatório inválida.");
  return { currency_code: item.currency_code, status: item.status as ReportingSummary["status"], eligible, converted, missing, stale, totals };
}
