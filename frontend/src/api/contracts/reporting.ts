import { isCount, parseCurrencyTotals, requiredRecordWithOptionalKeys, dateOnlyPattern, type CurrencyTotals } from "./shared";

export interface ReportingIssue {
  currency: string;
  reason: "unsupported" | "historical_unavailable" | "provider_failure";
  date?: string;
  account?: string;
}

export interface ReportingSummary {
  issues?: ReportingIssue[];
  currency_code: string;
  status: "complete" | "stale" | "incomplete" | "unavailable";
  eligible: number;
  converted: number;
  missing: number;
  stale: number;
  totals: CurrencyTotals | null;
}

export function parseReportingSummary(value: unknown): ReportingSummary {
  const item = requiredRecordWithOptionalKeys(value, ["currency_code", "status", "eligible", "converted", "missing", "stale", "totals"], ["issues"]);
  if (item.currency_code !== "BRL" || !["complete", "stale", "incomplete", "unavailable"].includes(item.status as string) ||
    ![item.eligible, item.converted, item.missing, item.stale].every(isCount)) {
    throw new TypeError("Conversão do relatório inválida.");
  }
  const eligible = item.eligible as number, converted = item.converted as number, missing = item.missing as number, stale = item.stale as number;
  const totals = item.totals === null ? null : parseCurrencyTotals(item.totals);
  if (eligible !== converted + missing || stale > converted ||
    ((missing > 0) !== (totals === null)) ||
    item.status !== (missing > 0 ? converted > 0 ? "incomplete" : "unavailable" : stale > 0 ? "stale" : "complete") ||
    (totals && totals.currency_code !== item.currency_code)) throw new TypeError("Conversão do relatório inválida.");
  let issues: ReportingIssue[] | undefined;
  if (item.issues !== undefined) {
    if (!Array.isArray(item.issues)) throw new TypeError("Conversão do relatório inválida.");
    issues = item.issues.map((value) => {
      const issue = requiredRecordWithOptionalKeys(value, ["currency", "reason"], ["date", "account"]);
      if (typeof issue.currency !== "string" || !/^[A-Z]{3}$/.test(issue.currency) ||
        !["unsupported", "historical_unavailable", "provider_failure"].includes(issue.reason as string) ||
        (issue.date !== undefined && (typeof issue.date !== "string" || !dateOnlyPattern.test(issue.date))) ||
        (issue.account !== undefined && typeof issue.account !== "string")) throw new TypeError("Conversão do relatório inválida.");
      return issue as unknown as ReportingIssue;
    });
  }
  return { ...(issues ? { issues } : {}), currency_code: item.currency_code, status: item.status as ReportingSummary["status"], eligible, converted, missing, stale, totals };
}
