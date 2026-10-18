import { ApiError } from "../api/problems";
import type { CurrencyTotals, ReportingSummary } from "../api/contracts";

export function reportingLabel(report: ReportingSummary): string {
  return report.status === "incomplete" ? "Total incompleto" : "Total indisponível";
}

export function reportingTotals(report: ReportingSummary | undefined, totals: CurrencyTotals[]): CurrencyTotals | null {
  if (report) return report.totals;
  if (totals.some((t) => t.currency_code !== "BRL")) return null;
  return totals[0] ?? { currency_code: "BRL", inflow: "0", outflow: "0", balance: "0" };
}

export function reportingError(error: unknown): ApiError | null {
  return error instanceof ApiError && error.problem?.type === "/problems/reporting-fx-unavailable" ? error : null;
}

/** The server's own explanation (e.g. a missing exchange rate) for a failed report, if it sent one. */
export function problemMessage(error: unknown): string | undefined {
  return error instanceof ApiError ? error.message : undefined;
}
